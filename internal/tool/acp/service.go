package acp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
)

type Service struct {
	managers  map[string]*acpruntime.Manager
	defaultID string
}

func NewMulti(defaultID string, managers map[string]*acpruntime.Manager) *Service {
	copied := make(map[string]*acpruntime.Manager, len(managers))
	for id, manager := range managers {
		if manager != nil {
			copied[id] = manager
		}
	}
	return &Service{managers: copied, defaultID: strings.TrimSpace(defaultID)}
}

func (s *Service) Close() error {
	if s == nil || len(s.managers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.managers))
	for id := range s.managers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var closeErrors []error
	for _, id := range ids {
		if err := s.managers[id].Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close ACP profile %s: %w", id, err))
		}
	}
	return errors.Join(closeErrors...)
}

func (s *Service) managerFor(profileID string) (*acpruntime.Manager, string, error) {
	if s == nil || len(s.managers) == 0 {
		return nil, "", validationError("ACP_NOT_CONFIGURED", "ACP runtime is not configured", nil)
	}
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		profileID = s.defaultID
	}
	manager := s.managers[profileID]
	if manager == nil {
		return nil, "", validationError("ACP_PROFILE_NOT_FOUND", "ACP profile was not found", map[string]any{"profile_id": profileID})
	}
	return manager, profileID, nil
}

// IsLiveManagedSession validates a known local ACP session against the exact
// configured profile without opening/loading a persisted session. This is
// advisory (not a native child ownership or dispatch capability).
func (s *Service) IsLiveManagedSession(profileID, localID string) bool {
	if s == nil || profileID == "" || localID == "" {
		return false
	}
	manager, _, err := s.managerFor(profileID)
	return err == nil && manager.IsLiveManagedSession(localID)
}

// NextCodexAppServerBirths requires the exact Codex profile and a private
// authenticated Adapter challenge; never activate an absent ACP process.
// A kernel-correlated app-server does not prove per-thread MCP ownership.
func (s *Service) NextCodexAppServerBirths(ctx context.Context, key []byte) (acpruntime.NextCodexAppServerBirths, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || len(key) != 32 {
		return acpruntime.NextCodexAppServerBirths{}, errors.New("Next Codex MCP app-server evidence unavailable")
	}
	manager, profileID, err := s.managerFor("codex")
	if err != nil || profileID != "codex" {
		return acpruntime.NextCodexAppServerBirths{}, errors.New("Next Codex MCP app-server evidence unavailable")
	}
	return manager.NextCodexAppServerBirths(ctx, key)
}
