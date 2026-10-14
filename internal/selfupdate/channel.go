package selfupdate

import (
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/uvwt/agentdock/internal/updateidentity"
)

var errNextChannelDisabled = errors.New("AgentDock Next update channel is disabled/unpublished: no approved Next feed exists; use P10 release preflight and wait for an authorized Next release; stable releases cannot update Next")

// The legacy empty/stable marker retains its feed unless another source proves
// this is Next. Conflicting or unknown markers never select the stable feed.
func validateUpdateChannel(opts options) error {
	id, err := updateidentity.Resolve(os.Getenv("AGENTDOCK_DESKTOP_VARIANT"))
	if err != nil {
		return err
	}
	next := id.Variant == "next" || nextCandidateVersion(opts.CurrentVersion) || nextCandidateVersion(opts.DesktopCurrentVersion) || nextIdentityPath(opts.ExecutablePath) || nextIdentityPath(opts.DesktopTargetPath)
	if !next {
		return nil
	}
	// Internal fixtures are explicit and cannot become an external feed override.
	// No exported API, command flag or environment variable can set this option.
	if opts.nextChannelFixture {
		u, err := url.Parse(opts.ReleaseAPI)
		if err == nil && u.Scheme == "http" && u.User == nil && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
			return nil
		}
	}
	return errNextChannelDisabled
}

func nextCandidateVersion(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	return strings.HasPrefix(version, "next-") || strings.Contains(version, "-rc")
}

func nextIdentityPath(path string) bool {
	// Split both separators so Windows identity fixtures work on every host.
	for _, component := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' || r == '\\' }) {
		switch component {
		case "agentdock next", "agentdock next.app", "agentdock-next", "agentdocknext", ".agentdock-next":
			return true
		}
	}
	return false
}
