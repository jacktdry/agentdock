package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/fs/filelock"
)

// DesktopRegistry is private Core data. Transport projections must never marshal it.
type DesktopRegistry struct {
	Revision   string
	States     []State
	Recoveries []DesktopRecovery
}
type DesktopRecovery struct {
	Name       string
	Generation string
	State      string
}

// AcquireManagement serializes Core model and Desktop lifecycle operations across
// service instances. Per-Plugin writer leases continue to protect package readers.
func (s *Store) AcquireManagement(ctx context.Context) (func(), error) {
	return filelock.Acquire(ctx, filepath.Join(s.lockRoot, "desktop-management.lock"))
}

func DesktopGeneration(state State) string {
	if len(state.Generation) == 64 {
		if _, err := hex.DecodeString(state.Generation); err == nil {
			return state.Generation
		}
	}
	return desktopDigest(struct {
		Name, Digest string
		InstalledAt  any
	}{state.Name, state.PackageDigest, state.InstalledAt})
}
func desktopDigest(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// DesktopRegistrySnapshot reads durable state and recovery metadata only. It does
// not verify/activate packages, recover transactions, or reconcile MCP clients.
func (s *Store) DesktopRegistrySnapshot() (DesktopRegistry, error) {
	states, err := s.List()
	if err != nil {
		return DesktopRegistry{}, err
	}
	transactions, err := s.ListActivationTransactions()
	if err != nil {
		return DesktopRegistry{}, err
	}
	records := []removalRecord{}
	entries, err := os.ReadDir(filepath.Join(s.stateRoot, "removals"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return DesktopRegistry{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.LoadRemovalRecord(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return DesktopRegistry{}, err
		}
		records = append(records, record)
	}
	result := DesktopRegistry{States: states, Recoveries: []DesktopRecovery{}}
	for _, transaction := range transactions {
		result.Recoveries = append(result.Recoveries, DesktopRecovery{transaction.Name, DesktopGeneration(transaction.Candidate), "activation_pending"})
	}
	for _, record := range records {
		if record.Phase != removalPhasePurging {
			continue
		}
		generation := desktopDigest(record)
		if record.RemovedState != nil {
			generation = DesktopGeneration(*record.RemovedState)
		}
		result.Recoveries = append(result.Recoveries, DesktopRecovery{record.Name, generation, "cleanup_required"})
	}
	// Include durable write timestamps so an enable/disable ABA cannot revive an
	// older editor revision even when the semantic state returns to its old value.
	writes := []int64{}
	for _, state := range states {
		path, err := s.StatePath(state.Name)
		if err != nil {
			return DesktopRegistry{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return DesktopRegistry{}, err
		}
		writes = append(writes, info.ModTime().UnixNano())
	}
	for _, record := range records {
		info, err := os.Stat(filepath.Join(s.stateRoot, "removals", record.Name+".json"))
		if err != nil {
			return DesktopRegistry{}, err
		}
		writes = append(writes, info.ModTime().UnixNano())
	}
	result.Revision = desktopDigest(struct {
		States       []State
		Transactions []ActivationTransaction
		Removals     []removalRecord
		Writes       []int64
	}{states, transactions, records, writes})
	return result, nil
}
