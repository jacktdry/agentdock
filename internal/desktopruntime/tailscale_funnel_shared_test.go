package desktopruntime

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTailscaleStatusAndArgsSharedAcrossPlatforms(t *testing.T) {
	status := []byte(`{"BackendState":"Running","Self":{"DNSName":"win.example.ts.net.","Online":true,"Capabilities":["funnel","https"]}}`)
	if origin, err := parseTailscaleFunnelStatus(status); err != nil || origin != "https://win.example.ts.net" {
		t.Fatalf("origin = %q, %v", origin, err)
	}
	if got, want := tailscaleFunnelArgs("http://127.0.0.1:8765"), []string{"funnel", "--yes", "http://127.0.0.1:8765"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestTailscaleStatusReadyRequiresOwnedProcessAndCurrentOrigin(t *testing.T) {
	const origin = "https://win.example.ts.net"
	for _, test := range []struct {
		name                string
		running             bool
		configured, current string
		err                 error
		want                bool
	}{
		{"ready", true, origin, origin, nil, true},
		{"stopped", false, origin, origin, nil, false},
		{"disconnected", true, origin, origin, errors.New("offline"), false},
		{"dns changed", true, origin, "https://other.example.ts.net", nil, false},
		{"no origin", true, "", origin, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := tailscaleStatusReady(test.running, test.configured, test.current, test.err); got != test.want {
				t.Fatalf("ready=%v, want %v", got, test.want)
			}
		})
	}
}

func TestTailscaleWindowsCandidateOrderAndDiscovery(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "configured.exe")
	pathBinary := filepath.Join(root, "path.exe")
	for _, path := range []string{configured, pathBinary} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidates := tailscaleWindowsCandidates(configured, pathBinary, root, "", "")
	if got, want := candidates[:2], []string{configured, pathBinary}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate precedence = %q, want %q", got, want)
	}
	if got, err := firstRegularFile(candidates); err != nil || got != configured {
		t.Fatalf("configured discovery = %q, %v", got, err)
	}
	if got, err := firstRegularFile(candidates[1:]); err != nil || got != pathBinary {
		t.Fatalf("PATH discovery = %q, %v", got, err)
	}
	if _, err := firstRegularFile([]string{root, filepath.Join(root, "missing.exe")}); err == nil {
		t.Fatal("expected discovery failure for directories and missing files")
	}
}
