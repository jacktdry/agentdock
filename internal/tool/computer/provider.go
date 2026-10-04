package computer

import "context"

type Provider interface {
	ID() ProviderID
	Observe(context.Context, ObservationRequest) (map[string]any, error)
	Act(context.Context, ActionRequest) (map[string]any, error)
	FrontmostApp(context.Context) (*AppIdentity, error)
}
