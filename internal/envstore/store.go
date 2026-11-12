package envstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
	"github.com/uvwt/agentdock/internal/fs/filelock"
)

type ScopeKind string

const (
	ScopeSkill       ScopeKind = "skill"
	ScopePluginSkill ScopeKind = "plugin_skill"
	ScopeMCP         ScopeKind = "mcp"

	maxEnvironmentFileBytes = 1 << 20
)

type Scope struct {
	Kind   ScopeKind
	Name   string
	Plugin string
}

type Entry struct {
	Key        string `json:"key"`
	Configured bool   `json:"configured"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func New(agentDockHome string) (*Store, error) {
	if agentDockHome == "" {
		return nil, errors.New("agentdock home is required")
	}
	store := &Store{root: filepath.Join(agentDockHome, "env")}
	if err := store.ensureDirectories(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) acquireStoreLock() (func(), error) {
	s.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	releaseFileLock, err := filelock.Acquire(ctx, filepath.Join(s.root, ".store.lock"))
	cancel()
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("lock environment store: %w", err)
	}
	return func() {
		releaseFileLock()
		s.mu.Unlock()
	}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Path(scope Scope) (string, error) {
	if err := validateScope(scope); err != nil {
		return "", err
	}
	if scope.Kind == ScopePluginSkill {
		return filepath.Join(s.root, "skill", "plugin", scope.Plugin, scope.Name+".env"), nil
	}
	return filepath.Join(s.root, string(scope.Kind), scope.Name+".env"), nil
}

func (s *Store) RemovePluginSkillScopes(plugin string) error {
	probe := Scope{Kind: ScopePluginSkill, Plugin: strings.TrimSpace(plugin), Name: "probe"}
	if err := validateScope(probe); err != nil {
		return err
	}
	release, err := s.acquireStoreLock()
	if err != nil {
		return err
	}
	defer release()
	root := filepath.Join(s.root, "skill", "plugin", probe.Plugin)
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Plugin Skill environment root is not a regular directory: %s", root)
	}
	return os.RemoveAll(root)
}

// Set remains an unconditional write, but advances the shared scope revision.
func (s *Store) Set(scope Scope, key, value string) error {
	_, _, err := s.mutate(scope, key, value, "set", nil)
	return err
}

func (s *Store) Unset(scope Scope, key string) (bool, error) {
	removed, _, err := s.mutate(scope, key, "", "unset", nil)
	return removed, err
}

func (s *Store) List(scope Scope) ([]Entry, error) {
	snapshot, err := s.Snapshot(scope)
	return snapshot.Entries, err
}

func (s *Store) Load(scope Scope) (map[string]string, error) {
	if err := validateScope(scope); err != nil {
		return nil, err
	}

	release, err := s.acquireStoreLock()
	if err != nil {
		return nil, err
	}
	defer release()
	return s.loadLocked(scope)
}

func (s *Store) loadLocked(scope Scope) (map[string]string, error) {
	values, _, err := s.loadSnapshotLocked(scope)
	return values, err
}

func (s *Store) loadSnapshotLocked(scope Scope) (map[string]string, Snapshot, error) {
	if err := s.ensureDirectories(); err != nil {
		return nil, Snapshot{}, err
	}
	path, err := s.Path(scope)
	if err != nil {
		return nil, Snapshot{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, protectedSnapshot(metadataRevision("absent", path, 0), nil), nil
	}
	if err != nil {
		return nil, Snapshot{}, fmt.Errorf("read %s environment: %w", scope.Kind, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxEnvironmentFileBytes+1))
	if err != nil {
		return nil, Snapshot{}, fmt.Errorf("read %s environment: %w", scope.Kind, err)
	}
	if len(data) > maxEnvironmentFileBytes {
		return nil, Snapshot{}, fmt.Errorf("%s environment exceeds %d bytes", scope.Kind, maxEnvironmentFileBytes)
	}
	if err := secureFile(path); err != nil {
		return nil, Snapshot{}, err
	}
	values, revision, err := parseDocument(data)
	if err != nil {
		return nil, Snapshot{}, fmt.Errorf("parse %s environment %q: %w", scope.Kind, scope.Name, err)
	}
	if revision == "" {
		info, err := file.Stat()
		if err != nil {
			return nil, Snapshot{}, fmt.Errorf("stat %s environment: %w", scope.Kind, err)
		}
		revision = metadataRevision("legacy", path, info.ModTime().UnixNano())
	}
	return values, protectedSnapshot(revision, values), nil
}

func (s *Store) writeLocked(scope Scope, values map[string]string, revision string) error {
	path, err := s.Path(scope)
	if err != nil {
		return err
	}
	for key := range values {
		if err := ValidateKey(key); err != nil {
			return err
		}
	}
	data := append([]byte("# "+revisionMarker+": "+revision+"\n"), marshal(values)...)
	if len(data) > maxEnvironmentFileBytes {
		return fmt.Errorf("%s environment exceeds %d bytes", scope.Kind, maxEnvironmentFileBytes)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create %s environment directory: %w", scope.Kind, err)
	}
	if err := secureDirectory(parent); err != nil {
		return err
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s environment: %w", scope.Kind, err)
	}
	return secureFile(path)
}
