package desktopruntime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

var ErrStaleTunnelGeneration = errors.New("stale_tunnel_generation")
var ErrTunnelRecoveryRequired = errors.New("recovery_required")
var ErrNamedManualRouteRequired = errors.New("named_manual_route_required")

const tunnelGenerationFile = "tunnel-generation.txt"
const tunnelRecoveryFile = "tunnel-recovery-required.json"

// TunnelGeneration reads the opaque launch/config identity, never credentials.
// Empty identifies legacy configuration until its first serialized mutation.
func TunnelGeneration(root string) (string, error) {
	path := filepath.Join(root, tunnelGenerationFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64 {
		return "", errors.New("invalid_tunnel_generation")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if len(value) != 32 {
		return "", errors.New("invalid_tunnel_generation")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("invalid_tunnel_generation")
	}
	return value, nil
}

// AdvanceTunnelGenerationLocked requires the caller's Desktop mutation lock.
// Port/config workers must also invalidate active Quick URL/OAuth state before
// restarting a changed target. Generation is never rolled back to an old value.
func AdvanceTunnelGenerationLocked(root string) (string, error) {
	if err := validateTunnelStateRoot(root); err != nil {
		return "", err
	}
	if _, err := TunnelGeneration(root); err != nil {
		return "", err
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	generation := hex.EncodeToString(bytes[:])
	return generation, atomicfile.Write(filepath.Join(root, tunnelGenerationFile), []byte(generation), 0o600)
}

func checkTunnelGeneration(root, expected string) error {
	current, err := TunnelGeneration(root)
	if err != nil {
		return err
	}
	if current != expected {
		return ErrStaleTunnelGeneration
	}
	if tunnelRecoveryPending(root) {
		return ErrTunnelRecoveryRequired
	}
	return nil
}

func tunnelRecoveryPending(root string) bool {
	_, err := os.Lstat(filepath.Join(root, tunnelRecoveryFile))
	return !errors.Is(err, os.ErrNotExist)
}

func tunnelBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// Nil observations mean unknown; false is an actual negative observation.
type TunnelObservation struct {
	Registered       *bool  `json:"registered"`
	Running          *bool  `json:"running"`
	Connected        *bool  `json:"connected"`
	Autostart        string `json:"autostart"`
	TokenState       string `json:"token_state"`
	Generation       string `json:"tunnel_generation"`
	PublicEndpoint   string `json:"public_endpoint"`
	RemoteRoute      string `json:"remote_route,omitempty"`
	RecoveryRequired bool   `json:"recovery_required"`
}

func completeTunnelStatus(status TunnelStatus, root, tokenState string) TunnelStatus {
	observation := &TunnelObservation{Running: &status.Running, Autostart: "unknown", TokenState: tokenState, PublicEndpoint: "not_configured"}
	var generationErr error
	observation.Generation, generationErr = TunnelGeneration(root)
	if !status.Running || status.Mode == "none" {
		disconnected := false
		observation.Connected = &disconnected
	}
	if status.PublicURL != "" {
		observation.PublicEndpoint = "unchecked"
	}
	if status.Mode == "named" {
		observation.RemoteRoute = "manual_route_required"
		status.Ready = false
	}
	observation.RecoveryRequired = tunnelRecoveryPending(root) || generationErr != nil
	if observation.RecoveryRequired {
		status.Ready = false
		status.PublicURL = ""
		observation.PublicEndpoint = "stale"
	}
	status.Observation = observation
	return status
}
