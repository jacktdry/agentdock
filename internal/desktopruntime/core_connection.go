package desktopruntime

import (
	"errors"
	"strings"
)

var ErrCoreConnectionUnavailable = errors.New("core connection is unavailable")

type CoreConnection struct {
	endpoint  string
	authToken string
}

func (c CoreConnection) Endpoint() string {
	return c.endpoint
}

func (c CoreConnection) AuthToken() string {
	return c.authToken
}

func ReadCoreConnection(runtimeRoot string) (CoreConnection, error) {
	connection, err := platformReadCoreConnection(strings.TrimSpace(runtimeRoot))
	if err != nil {
		return CoreConnection{}, err
	}
	if strings.TrimSpace(connection.endpoint) == "" {
		return CoreConnection{}, ErrCoreConnectionUnavailable
	}
	return connection, nil
}
