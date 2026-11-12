package oauthclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
	"github.com/uvwt/agentdock/internal/fs/filelock"
	"github.com/uvwt/agentdock/internal/fs/securepath"
	"golang.org/x/oauth2"
)

const storeSchemaVersion = 1

var storageKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type ClientRecord struct {
	Issuer                  string    `json:"issuer"`
	RedirectURL             string    `json:"redirect_url"`
	ClientID                string    `json:"client_id"`
	ClientSecret            string    `json:"client_secret,omitempty"`
	TokenEndpointAuthMethod string    `json:"token_endpoint_auth_method,omitempty"`
	ClientIDIssuedAt        time.Time `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   time.Time `json:"client_secret_expires_at,omitempty"`
	LastUsedAt              time.Time `json:"last_used_at,omitempty"`
}

type Grant struct {
	SchemaVersion   int       `json:"schema_version"`
	Endpoint        string    `json:"endpoint"`
	Resource        string    `json:"resource"`
	Issuer          string    `json:"issuer"`
	RedirectURL     string    `json:"redirect_url"`
	RegistrationKey string    `json:"registration_key"`
	TokenURL        string    `json:"token_url"`
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token,omitempty"`
	TokenType       string    `json:"token_type,omitempty"`
	Expiry          time.Time `json:"expiry,omitempty"`
	GrantedScopes   []string  `json:"granted_scopes,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type clientFile struct {
	SchemaVersion int                     `json:"schema_version"`
	Clients       map[string]ClientRecord `json:"clients"`
}

type store struct {
	root        string
	clientsPath string
}

func newStore(agentDockHome string) (*store, error) {
	home := filepath.Clean(strings.TrimSpace(agentDockHome))
	if home == "." || !filepath.IsAbs(home) {
		return nil, errors.New("AgentDockHome must be an absolute path")
	}
	root := filepath.Join(home, "data", "mcp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create MCP OAuth data directory: %w", err)
	}
	if err := securepath.EnsurePrivate(root); err != nil {
		return nil, fmt.Errorf("secure MCP OAuth data directory: %w", err)
	}
	return &store{root: root, clientsPath: filepath.Join(root, "clients.json")}, nil
}

func (s *store) grantPath(storageKey string) (string, error) {
	storageKey = strings.TrimSpace(storageKey)
	if !storageKeyPattern.MatchString(storageKey) || storageKey == "clients" {
		return "", fmt.Errorf("invalid MCP OAuth storage key %q", storageKey)
	}
	// Prefix every grant filename so Windows device names such as CON/NUL remain valid.
	// The stable storage key itself is still preserved inside the runtime identity.
	return filepath.Join(s.root, "grant-"+storageKey+".json"), nil
}

// grantEnvelope is versioned separately from the unchanged v1 client registry.
// Epoch is random authorization identity, never derived from credential material.
type grantEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	Epoch         string `json:"epoch"`
	Grant         *Grant `json:"grant,omitempty"`
}

func (s *store) readEnvelope(storageKey string) (grantEnvelope, error) {
	path, err := s.grantPath(storageKey)
	if err != nil {
		return grantEnvelope{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return grantEnvelope{}, nil
	}
	if err != nil {
		return grantEnvelope{}, err
	}
	var envelope grantEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return grantEnvelope{}, errors.New("decode MCP OAuth authorization")
	}
	switch envelope.SchemaVersion {
	case 1:
		var grant Grant
		if err := json.Unmarshal(data, &grant); err != nil {
			return grantEnvelope{}, errors.New("decode MCP OAuth grant")
		}
		envelope.Grant = &grant
	case 2:
		if envelope.Epoch == "" {
			return grantEnvelope{}, errors.New("missing MCP OAuth authorization epoch")
		}
	default:
		return grantEnvelope{}, errors.New("unsupported MCP OAuth authorization schema")
	}
	return envelope, nil
}

func (s *store) loadGrant(storageKey string) (Grant, error) {
	envelope, err := s.readEnvelope(storageKey)
	if err != nil {
		return Grant{}, err
	}
	if envelope.Grant == nil {
		return Grant{}, os.ErrNotExist
	}
	return *envelope.Grant, nil
}

// mutateAuthorization serializes read/compare/write with the same lock used by
// every reservation, grant writer and tombstone writer. No callback performs I/O
// outside this store (in particular, no network requests).
func (s *store) mutateAuthorization(storageKey string, change func(*grantEnvelope) error) error {
	path, err := s.grantPath(storageKey)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := filelock.Acquire(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("lock MCP OAuth grant: %w", err)
	}
	defer release()
	envelope, err := s.readEnvelope(storageKey)
	if err != nil {
		return err
	}
	if err := change(&envelope); err != nil {
		return err
	}
	if envelope.Epoch == "" {
		envelope.Epoch, err = randomState()
		if err != nil {
			return err
		}
	}
	envelope.SchemaVersion = 2
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return errors.New("encode MCP OAuth authorization")
	}
	if err := atomicfile.Write(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("persist MCP OAuth authorization: %w", err)
	}
	return securepath.EnsurePrivate(path)
}

func (s *store) reserve(storageKey string) (string, error) {
	epoch, err := randomState()
	if err != nil {
		return "", err
	}
	err = s.mutateAuthorization(storageKey, func(e *grantEnvelope) error { e.Epoch = epoch; return nil })
	return epoch, err
}

func (s *store) checkEpoch(storageKey, epoch string) error {
	e, err := s.readEnvelope(storageKey)
	if err != nil {
		return err
	}
	if epoch == "" || e.Epoch != epoch {
		return &AuthRequiredError{}
	}
	return nil
}

func (s *store) saveGrantChecked(storageKey, epoch string, grant Grant) error {
	return s.mutateAuthorization(storageKey, func(e *grantEnvelope) error {
		if epoch == "" || e.Epoch != epoch {
			return &AuthRequiredError{}
		}
		grant.SchemaVersion = 1
		grant.UpdatedAt = time.Now().UTC()
		e.Grant = &grant
		return nil
	})
}

// loadGrantWithEpoch migrates legacy grants before any refresh network request.
func (s *store) loadGrantWithEpoch(storageKey string) (Grant, string, error) {
	var grant Grant
	var epoch string
	err := s.mutateAuthorization(storageKey, func(e *grantEnvelope) error {
		if e.Grant == nil {
			return os.ErrNotExist
		}
		if e.Epoch == "" {
			var err error
			e.Epoch, err = randomState()
			if err != nil {
				return err
			}
		}
		grant, epoch = *e.Grant, e.Epoch
		return nil
	})
	return grant, epoch, err
}

func (s *store) clearChecked(storageKey string, expected *string) error {
	return s.mutateAuthorization(storageKey, func(e *grantEnvelope) error {
		if expected != nil && (*expected == "" || e.Epoch != *expected) {
			return &AuthRequiredError{}
		}
		epoch, err := randomState()
		if err != nil {
			return err
		}
		*e = grantEnvelope{Epoch: epoch}
		return nil
	})
}

func (s *store) removeGrant(storageKey string) error { return s.clearChecked(storageKey, nil) }

func registrationKey(issuer, redirectURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(issuer) + "\x00" + strings.TrimSpace(redirectURL)))
	return hex.EncodeToString(sum[:])
}

func (s *store) loadClient(key string) (ClientRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := filelock.Acquire(ctx, s.clientsPath+".lock")
	if err != nil {
		return ClientRecord{}, fmt.Errorf("lock MCP OAuth clients: %w", err)
	}
	defer release()
	clients, err := s.loadClientsUnlocked()
	if err != nil {
		return ClientRecord{}, err
	}
	client, ok := clients.Clients[key]
	if !ok {
		return ClientRecord{}, os.ErrNotExist
	}
	return client, nil
}

func (s *store) saveClient(key string, client ClientRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := filelock.Acquire(ctx, s.clientsPath+".lock")
	if err != nil {
		return fmt.Errorf("lock MCP OAuth clients: %w", err)
	}
	defer release()
	clients, err := s.loadClientsUnlocked()
	if err != nil {
		return err
	}
	client.LastUsedAt = time.Now().UTC()
	clients.Clients[key] = client
	data, err := json.MarshalIndent(clients, "", "  ")
	if err != nil {
		return fmt.Errorf("encode MCP OAuth clients: %w", err)
	}
	data = append(data, '\n')
	if err := atomicfile.Write(s.clientsPath, data, 0o600); err != nil {
		return fmt.Errorf("persist MCP OAuth clients: %w", err)
	}
	return securepath.EnsurePrivate(s.clientsPath)
}

func (s *store) touchClient(key string) {
	client, err := s.loadClient(key)
	if err != nil {
		return
	}
	_ = s.saveClient(key, client)
}

func (s *store) loadClientsUnlocked() (clientFile, error) {
	data, err := os.ReadFile(s.clientsPath)
	if errors.Is(err, os.ErrNotExist) {
		return clientFile{SchemaVersion: storeSchemaVersion, Clients: make(map[string]ClientRecord)}, nil
	}
	if err != nil {
		return clientFile{}, fmt.Errorf("read MCP OAuth clients: %w", err)
	}
	var clients clientFile
	if err := json.Unmarshal(data, &clients); err != nil {
		return clientFile{}, fmt.Errorf("decode MCP OAuth clients: %w", err)
	}
	if clients.SchemaVersion != storeSchemaVersion {
		return clientFile{}, fmt.Errorf("unsupported MCP OAuth clients schema version %d", clients.SchemaVersion)
	}
	if clients.Clients == nil {
		clients.Clients = make(map[string]ClientRecord)
	}
	return clients, nil
}

func tokenFromGrant(grant Grant) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  grant.AccessToken,
		RefreshToken: grant.RefreshToken,
		TokenType:    grant.TokenType,
		Expiry:       grant.Expiry,
	}
}

func grantTokenChanged(grant Grant, token *oauth2.Token) bool {
	if token == nil {
		return false
	}
	return grant.AccessToken != token.AccessToken ||
		grant.RefreshToken != token.RefreshToken ||
		grant.TokenType != token.TokenType ||
		!grant.Expiry.Equal(token.Expiry)
}
