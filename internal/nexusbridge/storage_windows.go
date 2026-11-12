//go:build windows

package nexusbridge

import (
	"os"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

func preparePairHome(agentDockHome string) error {
	return os.MkdirAll(agentDockHome, 0o700)
}

func readIdentityData(agentDockHome string) ([]byte, error) {
	return os.ReadFile(identityPath(agentDockHome))
}

func writeIdentityData(agentDockHome string, data []byte) error {
	return atomicfile.Write(identityPath(agentDockHome), data, 0o600)
}
