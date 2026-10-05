package media

import (
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/controlplane"
	"github.com/uvwt/agentdock/internal/workspace"
)

type CommandEnv func(extra map[string]string) ([]string, error)

type Service struct {
	cfg        config.Config
	ws         *workspace.Workspace
	commandEnv CommandEnv
	protected  *controlplane.PathSet
}

func New(cfg config.Config, ws *workspace.Workspace, commandEnv CommandEnv) *Service {
	return &Service{cfg: cfg, ws: ws, commandEnv: commandEnv}
}

func (s *Service) SetProtectedPathSet(paths *controlplane.PathSet) {
	if s != nil {
		s.protected = paths
	}
}

func (s *Service) guardProtectedLocalPath(absPath, displayPath string, includeAncestors bool) error {
	if s == nil || s.protected == nil {
		return nil
	}
	blocked := s.protected.Contains(absPath)
	if includeAncestors {
		blocked = s.protected.Intersects(absPath)
	}
	if !blocked {
		return nil
	}
	return toolErrorDetails(
		"PROTECTED_CONTROL_PATH",
		"AgentDock control-plane paths are not accessible through local media/file surfaces",
		"permission",
		map[string]any{"path": displayPath},
	)
}
