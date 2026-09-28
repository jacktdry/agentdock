package desktopruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedTailscalePIDValidatesRecordedProcess(t *testing.T) {
	const binary = "/opt/tailscale/tailscale.exe"
	for _, test := range []struct {
		name, record, actual string
		inspectErr           error
		wantPID              uint32
		wantFile             bool
	}{
		{"owned", "42", binary, nil, 42, true},
		{"missing process", "42", "", os.ErrNotExist, 0, false},
		{"different executable", "42", "/other/tailscale.exe", nil, 0, false},
		{"invalid PID", "invalid", "", nil, 0, false},
		{"zero PID", "0", "", nil, 0, false},
		{"access denied", "42", "", os.ErrPermission, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tailscale-funnel.pid")
			if err := os.WriteFile(path, []byte(test.record), 0o600); err != nil {
				t.Fatal(err)
			}
			inspected := false
			pid, err := ownedTailscalePID(path, binary, func(got uint32) (string, error) {
				inspected = true
				if got != 42 {
					t.Fatalf("inspected unexpected PID %d", got)
				}
				return test.actual, test.inspectErr
			})
			if pid != test.wantPID {
				t.Fatalf("PID=%d, want %d", pid, test.wantPID)
			}
			if test.inspectErr == os.ErrPermission && !errors.Is(err, os.ErrPermission) {
				t.Fatalf("error=%v", err)
			}
			if test.inspectErr != os.ErrPermission && err != nil {
				t.Fatal(err)
			}
			if (test.record == "invalid" || test.record == "0") && inspected {
				t.Fatal("inspected invalid PID")
			}
			_, statErr := os.Stat(path)
			if (statErr == nil) != test.wantFile {
				t.Fatalf("record presence=%v, want %v", statErr == nil, test.wantFile)
			}
		})
	}
}

func TestStopOwnedTailscalePIDTargetsOnlyValidatedRecord(t *testing.T) {
	const binary = "/opt/tailscale/tailscale.exe"
	path := filepath.Join(t.TempDir(), "tailscale-funnel.pid")
	if err := os.WriteFile(path, []byte("42"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stopped uint32
	err := stopOwnedTailscalePID(path, binary,
		func(pid uint32) (string, error) { return binary, nil },
		func(pid uint32, expected string) error {
			stopped = pid
			if expected != binary {
				t.Fatalf("expected path=%q", expected)
			}
			return nil
		})
	if err != nil || stopped != 42 {
		t.Fatalf("stopped PID=%d, error=%v", stopped, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record remained: %v", err)
	}
	if err := os.WriteFile(path, []byte("43"), 0o600); err != nil {
		t.Fatal(err)
	}
	stopped = 0
	if err := stopOwnedTailscalePID(path, binary,
		func(pid uint32) (string, error) { return "/other/tailscale.exe", nil },
		func(pid uint32, expected string) error { stopped = pid; return nil }); err != nil || stopped != 0 {
		t.Fatalf("mismatched process terminated: PID=%d, error=%v", stopped, err)
	}
}

func TestStopOwnedTailscalePIDKeepsReplacedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailscale-funnel.pid")
	if err := os.WriteFile(path, []byte("42"), 0o600); err != nil {
		t.Fatal(err)
	}
	const binary = "/opt/tailscale/tailscale.exe"
	if err := stopOwnedTailscalePID(path, binary,
		func(pid uint32) (string, error) { return binary, nil },
		func(pid uint32, expected string) error { return os.WriteFile(path, []byte("43"), 0o600) }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "43" {
		t.Fatalf("replacement record=%q, error=%v", data, err)
	}
}

func TestStopOwnedTailscalePIDToleratesMissingAndGoneProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailscale-funnel.pid")
	const binary = "/opt/tailscale/tailscale.exe"
	called := false
	inspect := func(pid uint32) (string, error) { return binary, nil }
	terminate := func(pid uint32, expected string) error { called = true; return os.ErrNotExist }
	if err := stopOwnedTailscalePID(path, binary, inspect, terminate); err != nil || called {
		t.Fatalf("missing record: called=%v, error=%v", called, err)
	}
	if err := os.WriteFile(path, []byte("42"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stopOwnedTailscalePID(path, binary, inspect, terminate); err != nil || !called {
		t.Fatalf("gone process: called=%v, error=%v", called, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale record remained: %v", err)
	}
}
