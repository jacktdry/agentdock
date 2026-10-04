//go:build !darwin

package computer

import "context"

func platformFrontmostApp(context.Context) (*AppIdentity, error) { return nil, nil }
