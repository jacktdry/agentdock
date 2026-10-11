//go:build !darwin

package mcpowneraudit

import "context"

const available = false

func readTable(context.Context) ([]row, error) { return nil, errUnavailable }
