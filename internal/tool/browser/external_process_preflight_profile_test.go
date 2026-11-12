package browser

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func profileFixtureMetadata(ctx context.Context, dir string) (edgeProfileMetadata, error) {
	return readEdgeProfileMetadata(ctx, dir, 501, func(path string) (edgeProfileComponent, error) {
		return edgeProfileComponent{device: 1, inode: uint64(len(path)), uid: 501, mode: fs.ModeDir | 0700}, nil
	})
}

func TestEdgeProfileMetadataBoundedReadOnly(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // POSIX ancestor metadata does not qualify Windows profile identity.
	}
	for _, scenario := range []string{"success", "symlink", "uid", "group mode", "other mode", "file", "zero inode", "zero device", "replacement", "owner change", "device change", "cancel", "error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			m, err := readEdgeProfileMetadata(ctx, "/owned/profile", 501, func(path string) (edgeProfileComponent, error) {
				calls++
				// A metadata-only callback must never see children/content paths.
				if path != "/" && path != "/owned" && path != "/owned/profile" {
					t.Fatal("traversed outside bounded ancestor chain")
				}
				c := edgeProfileComponent{device: 1, inode: uint64(len(path)), uid: 501, mode: fs.ModeDir | 0700}
				if path == "/owned" {
					switch scenario {
					case "symlink":
						c.mode = fs.ModeSymlink | 0700
					case "uid":
						c.uid = 502
					case "group mode":
						c.mode |= 0020
					case "other mode":
						c.mode |= 0002
					case "file":
						c.mode = 0600
					case "zero inode":
						c.inode = 0
					case "zero device":
						c.device = 0
					case "replacement":
						if calls > 3 {
							c.inode++
						}
					case "owner change":
						if calls > 3 {
							c.uid = 0
						}
					case "device change":
						if calls > 3 {
							c.device++
						}
					case "cancel":
						cancel()
					case "error":
						return c, errors.New("/owned/profile secret content")
					}
				}
				return c, nil
			})
			if scenario == "success" {
				if err != nil || !m.valid() || calls != 6 {
					t.Fatal("stable metadata rejected")
				}
			} else if err != errEdgeProcessUnqualified || m != (edgeProfileMetadata{}) || strings.Contains(err.Error(), "/owned") {
				t.Fatal("unsafe metadata exposed evidence or path")
			}
		})
	}
	for _, dir := range []string{"/", "relative", "/owned/../profile", "/" + strings.Repeat("a/", 33) + "profile", "/" + strings.Repeat("a", 4096)} {
		_, err := readEdgeProfileMetadata(context.Background(), dir, 501, func(string) (edgeProfileComponent, error) {
			t.Fatal("invalid path caused filesystem access")
			return edgeProfileComponent{}, nil
		})
		if err != errEdgeProcessUnqualified {
			t.Fatal("unbounded path accepted")
		}
	}
}

func TestEdgeProcessPreflightProfileFence(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // POSIX ancestor metadata does not qualify Windows profile identity.
	}
	for _, scenario := range []string{"success", "inode", "device", "owner", "mode", "cancel", "error", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			f := edgeFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.onProfile = func(c context.Context, dir string, i int) (edgeProfileMetadata, error) {
				m, err := profileFixtureMetadata(c, dir)
				if i == 0 && scenario == "invalid" {
					return edgeProfileMetadata{}, nil
				}
				if i == 1 {
					if f.liveSignatureCalls != 1 || f.commandCalls != 2 {
						t.Fatal("metadata does not bracket process/signing evidence")
					}
					switch scenario {
					case "inode":
						m.components[2].inode++
					case "device":
						m.components[2].device++
					case "owner":
						m.owner++
						m.components[2].uid++
					case "mode":
						m.components[2].mode |= 0020
					case "cancel":
						cancel()
					case "error":
						err = errors.New("/tmp/edge-fixture sensitive metadata")
					}
				}
				return m, err
			}
			e, err := readEdgeProcessPreflight(ctx, "ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
			if scenario == "success" {
				if err != nil || e.pid != 1245 {
					t.Fatal("stable profile fixture rejected")
				}
			} else if err != errEdgeProcessUnqualified || e != (edgeProcessPreflight{}) {
				t.Fatal("profile replacement/cancellation leaked evidence")
			}
			if scenario == "invalid" && f.listenerCalls != 0 {
				t.Fatal("unsafe initial metadata advanced process inspection")
			}
		})
	}
}
