package command

import (
	"context"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/tool/command/session"
	"github.com/uvwt/agentdock/internal/workspace"
)

type ConfigProvider func() config.Config

type SkillLease struct {
	Name         string
	Root         string
	EnvScope     *envstore.Scope
	SkillDataDir string
	RuntimeEnv   map[string]string
	Release      func()
}

type SkillResolver func(ctx context.Context, skillRef string) (SkillLease, error)
type CommandContext func() (context.Context, error)

type SessionLifecycle struct {
	SessionID string
	Done      <-chan struct{}
	Metadata  func() session.Snapshot
}

type SessionLifecycleHook func(context.Context, SessionLifecycle)

type Service struct {
	config         ConfigProvider
	ws             *workspace.Workspace
	envs           *envstore.Store
	sessions       *session.Store
	resolveSkill   SkillResolver
	commandContext CommandContext
	sessionHook    SessionLifecycleHook
}

func New(configProvider ConfigProvider, ws *workspace.Workspace, envs *envstore.Store, resolveSkill SkillResolver, commandContext CommandContext) *Service {
	return &Service{
		config: configProvider, ws: ws, envs: envs, sessions: session.NewStore(),
		resolveSkill: resolveSkill, commandContext: commandContext,
	}
}

func (s *Service) CommandEnv(skillName string, extra map[string]string) ([]string, error) {
	return s.commandEnv(skillName, extra)
}

func (s *Service) InternalCommandEnv(extra map[string]string) ([]string, error) {
	return s.internalCommandEnv(extra)
}

func (s *Service) SetSessionLifecycleHook(hook SessionLifecycleHook) {
	if s != nil {
		s.sessionHook = hook
	}
}

func (s *Service) notifySessionLifecycle(ctx context.Context, current *session.Session) {
	if s == nil || s.sessionHook == nil || current == nil {
		return
	}
	s.sessionHook(ctx, SessionLifecycle{
		SessionID: current.ID,
		Done:      current.Done,
		Metadata: func() session.Snapshot {
			return current.Metadata("exited")
		},
	})
}

// MaxOutputBytes is the public exec/session output contract limit used by schema generation.
const MaxOutputBytes = maxCommandOutputBytes
