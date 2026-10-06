package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
	"github.com/uvwt/agentdock/internal/fs/filelock"
)

const (
	registryVersion      = 2
	maxRegistryFileBytes = 1 << 20
)

type registryFile struct {
	Version     int               `json:"version"`
	Revision    string            `json:"revision,omitempty"`
	Generations map[string]string `json:"generations,omitempty"`
	Servers     []ServerConfig    `json:"servers"`
}

type store struct {
	path     string
	lockPath string
}

func newStore(agentDockHome string) *store {
	root := filepath.Join(agentDockHome, "mcp")
	return &store{path: filepath.Join(root, "servers.json"), lockPath: filepath.Join(root, ".store.lock")}
}

func (s *store) load() (map[string]ServerConfig, error) {
	snapshot, err := s.snapshot()
	return snapshot.Servers, err
}

func (s *store) snapshot() (RegistrySnapshot, error) {
	release, err := s.acquire()
	if err != nil {
		return RegistrySnapshot{}, err
	}
	defer release()
	return s.loadUnlocked()
}

func (s *store) loadUnlocked() (RegistrySnapshot, error) {
	registryHandle, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return RegistrySnapshot{Revision: opaqueMetadata("absent"), Servers: map[string]ServerConfig{}}, nil
	}
	if err != nil {
		return RegistrySnapshot{}, fmt.Errorf("read dynamic MCP registry: %w", err)
	}
	defer registryHandle.Close()
	info, err := registryHandle.Stat()
	if err != nil {
		return RegistrySnapshot{}, fmt.Errorf("stat dynamic MCP registry: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(registryHandle, maxRegistryFileBytes+1))
	if err != nil {
		return RegistrySnapshot{}, fmt.Errorf("read dynamic MCP registry: %w", err)
	}
	if len(data) > maxRegistryFileBytes {
		return RegistrySnapshot{}, fmt.Errorf("dynamic MCP registry exceeds %d bytes", maxRegistryFileBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file registryFile
	if err := decoder.Decode(&file); err != nil {
		return RegistrySnapshot{}, fmt.Errorf("decode dynamic MCP registry: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return RegistrySnapshot{}, errors.New("decode dynamic MCP registry: trailing JSON value")
		}
		return RegistrySnapshot{}, fmt.Errorf("decode dynamic MCP registry trailing data: %w", err)
	}
	if file.Version != 1 && file.Version != registryVersion {
		return RegistrySnapshot{}, fmt.Errorf("unsupported dynamic MCP registry version %d", file.Version)
	}
	// Legacy files have no tokens. Bootstrap from non-secret file metadata;
	// preserve these generations on upgrade. Never hash file contents or size.
	legacy := opaqueMetadata("legacy", info.ModTime().UTC().Format(time.RFC3339Nano))
	if file.Version == 1 {
		if file.Revision != "" || len(file.Generations) != 0 {
			return RegistrySnapshot{}, errors.New("unexpected version-1 registry metadata")
		}
		file.Revision = legacy
	} else if !validToken(file.Revision) {
		return RegistrySnapshot{}, errors.New("invalid registry revision")
	}
	servers := make(map[string]ServerConfig, len(file.Servers))
	for _, raw := range file.Servers {
		cfg := normalizeServerConfig(raw)
		if err := validateServerConfig(cfg); err != nil {
			return RegistrySnapshot{}, fmt.Errorf("validate dynamic MCP server %q: %w", cfg.Name, err)
		}
		if _, exists := servers[cfg.Name]; exists {
			return RegistrySnapshot{}, fmt.Errorf("duplicate dynamic MCP server %q", cfg.Name)
		}
		if file.Version == 1 {
			cfg.Generation = opaqueMetadata(legacy, cfg.Name)
		} else {
			cfg.Generation = file.Generations[cfg.Name]
			if !validToken(cfg.Generation) {
				return RegistrySnapshot{}, fmt.Errorf("invalid generation for MCP server %q", cfg.Name)
			}
		}
		servers[cfg.Name] = cfg
	}
	if file.Version == registryVersion && len(file.Generations) != len(servers) {
		return RegistrySnapshot{}, errors.New("registry generation inventory mismatch")
	}
	return RegistrySnapshot{Revision: file.Revision, Servers: servers}, nil
}

// opaqueMetadata hashes only explicitly non-secret identity/token metadata.
func opaqueMetadata(parts ...string) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newRegistryToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func validToken(token string) bool {
	data, err := hex.DecodeString(token)
	return err == nil && len(data) == 32
}

func (s *store) saveUnlocked(snapshot RegistrySnapshot) error {
	names := make([]string, 0, len(snapshot.Servers))
	for name := range snapshot.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	file := registryFile{Version: registryVersion, Revision: snapshot.Revision, Generations: make(map[string]string), Servers: make([]ServerConfig, 0, len(names))}
	for _, name := range names {
		cfg := snapshot.Servers[name]
		file.Servers = append(file.Servers, cfg)
		file.Generations[name] = cfg.Generation
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dynamic MCP registry: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxRegistryFileBytes {
		return fmt.Errorf("dynamic MCP registry exceeds %d bytes", maxRegistryFileBytes)
	}
	if err := atomicfile.Write(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write dynamic MCP registry: %w", err)
	}
	return nil
}

func (s *store) update(mutator func(RegistrySnapshot) error) (RegistrySnapshot, error) {
	release, err := s.acquire()
	if err != nil {
		return RegistrySnapshot{}, err
	}
	defer release()
	snapshot, err := s.loadUnlocked()
	if err != nil {
		return RegistrySnapshot{}, newError("MCP_REGISTRY_READ_FAILED", "read dynamic MCP registry", true, nil, err)
	}
	before := make(map[string]ServerConfig, len(snapshot.Servers))
	for name, cfg := range snapshot.Servers {
		before[name] = normalizeServerConfig(cfg)
	}
	// The mutator checks revision, incarnation, and ownership while this lock
	// still protects the authoritative read, before any persistence occurs.
	if err := mutator(snapshot); err != nil {
		return RegistrySnapshot{}, err
	}
	for name, cfg := range snapshot.Servers {
		previous, exists := before[name]
		cfg.Generation = previous.Generation
		if !exists || !reflect.DeepEqual(previous, cfg) {
			cfg.Generation, err = newRegistryToken()
			if err != nil {
				return RegistrySnapshot{}, err
			}
		}
		snapshot.Servers[name] = cfg
	}
	snapshot.Revision, err = newRegistryToken()
	if err != nil {
		return RegistrySnapshot{}, err
	}
	if err := s.saveUnlocked(snapshot); err != nil {
		return RegistrySnapshot{}, err
	}
	return snapshot, nil
}

func (s *store) acquire() (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	release, err := filelock.Acquire(ctx, s.lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock dynamic MCP registry: %w", err)
	}
	return release, nil
}

func validateServerConfig(cfg ServerConfig) error {
	if !serverNamePattern.MatchString(cfg.Name) {
		return fmt.Errorf("name must match %s", serverNamePattern.String())
	}
	if cfg.Description == "" {
		return errors.New("description is required")
	}
	if cfg.TimeoutMS < 1 || cfg.TimeoutMS > maxTimeoutMS {
		return fmt.Errorf("timeout_ms must be between 1 and %d", maxTimeoutMS)
	}
	if cfg.ProtocolVersion != "" && !supportedMCPProtocolVersion(cfg.ProtocolVersion) {
		return fmt.Errorf("unsupported protocol_version %q", cfg.ProtocolVersion)
	}
	for header, envName := range cfg.HeaderEnv {
		if strings.TrimSpace(header) == "" || strings.TrimSpace(envName) == "" {
			return errors.New("header_env keys and values must be non-empty")
		}
		if !headerNamePattern.MatchString(header) {
			return fmt.Errorf("invalid HTTP header name %q", header)
		}
		if isReservedMCPHeader(header) {
			return fmt.Errorf("header_env may not override reserved HTTP header %q", header)
		}
		if !envNamePattern.MatchString(envName) {
			return fmt.Errorf("invalid host environment variable name %q", envName)
		}
	}
	for childName, hostName := range cfg.EnvFromEnv {
		if strings.TrimSpace(childName) == "" || strings.TrimSpace(hostName) == "" {
			return errors.New("env_from_env keys and values must be non-empty")
		}
		if !envNamePattern.MatchString(childName) || !envNamePattern.MatchString(hostName) {
			return fmt.Errorf("invalid environment variable mapping %q -> %q", childName, hostName)
		}
	}
	for _, envName := range cfg.RequiredEnv {
		if !envNamePattern.MatchString(envName) {
			return fmt.Errorf("invalid required environment variable name %q", envName)
		}
	}
	for key := range cfg.StaticEnv {
		if !envNamePattern.MatchString(key) {
			return fmt.Errorf("invalid static environment variable name %q", key)
		}
	}
	for header, value := range cfg.StaticHeaders {
		if !headerNamePattern.MatchString(header) || strings.TrimSpace(header) == "" {
			return fmt.Errorf("invalid HTTP header name %q", header)
		}
		if isReservedMCPHeader(header) {
			return fmt.Errorf("static headers may not override reserved HTTP header %q", header)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("static HTTP header %q contains a newline", header)
		}
	}

	switch cfg.Transport {
	case TransportStreamableHTTP:
		if cfg.URL == "" {
			return errors.New("url is required for streamable_http")
		}
		parsed, err := url.Parse(cfg.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("url must be an absolute HTTP(S) URL: %q", cfg.URL)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("url must use http or https: %q", cfg.URL)
		}
		if parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("url must not contain user info or a fragment: %q", cfg.URL)
		}
		if cfg.Command != "" || len(cfg.Args) > 0 || cfg.Cwd != "" || len(cfg.EnvFromEnv) > 0 {
			return errors.New("stdio-only fields are not allowed for streamable_http")
		}
	case TransportStdio:
		if cfg.Command == "" {
			return errors.New("command is required for stdio")
		}
		if cfg.URL != "" || len(cfg.HeaderEnv) > 0 || len(cfg.StaticHeaders) > 0 {
			return errors.New("HTTP-only fields are not allowed for stdio")
		}
		if cfg.Cwd != "" && !filepath.IsAbs(cfg.Cwd) {
			return errors.New("cwd must be an absolute path")
		}
	default:
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
	return nil
}

func isReservedMCPHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "accept", "connection", "content-length", "content-type", "host", "mcp-protocol-version", "mcp-session-id", "transfer-encoding", "user-agent":
		return true
	default:
		return false
	}
}
