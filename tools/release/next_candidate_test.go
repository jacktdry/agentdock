package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func candidateFixture(t *testing.T) (string, nextManifest, []byte) {
	t.Helper()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"packaging/macos/app-identity.sh", "packaging/macos/build-app.sh", "internal/buildinfo/buildinfo.go"} {
		data, err := os.ReadFile(filepath.Join("../..", name))
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := nextContract(validNextInput())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return root, m, append(data, '\n')
}

func TestNextCandidateGate(t *testing.T) {
	root, m, data := candidateFixture(t)
	if _, err := verifyNextCandidate(data, root, m.Commit, m.BuildDate, ""); err != nil {
		t.Fatal(err)
	}
	for name, values := range map[string][3]string{
		"tracked-dirty": {m.Commit, m.BuildDate, " M source.go"},
		"untracked":     {m.Commit, m.BuildDate, "?? source.go"},
		"wrong-head":    {strings.Repeat("b", 40), m.BuildDate, ""},
		"wrong-date":    {m.Commit, "2000-01-01T00:00:00Z", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyNextCandidate(data, root, values[0], values[1], values[2]); err == nil {
				t.Fatal("candidate accepted")
			}
		})
	}
	for _, bad := range [][]byte{bytes.ReplaceAll(data, []byte(m.MacBundleID), []byte("com.uvwt.agentdock")), bytes.ReplaceAll(data, []byte(m.CoreVersion), []byte("0.9.1")), append([]byte(" "), data...), bytes.Replace(data, []byte("{\n"), []byte("{\n  \"unknown\":true,\n"), 1), bytes.Replace(data, []byte("{\n"), []byte("{\n  \"version\":\"0.9.1-rc.2\",\n"), 1)} {
		if _, err := verifyNextCandidate(bad, root, m.Commit, m.BuildDate, ""); err == nil {
			t.Fatal("noncanonical manifest accepted")
		}
	}
}

// Real Go compilation with the production overlay generator, but a tiny offline
// fixture main. This is not a clean-HEAD Core or release artifact acceptance.
func TestNextOverlayBinary(t *testing.T) {
	root, m, _ := candidateFixture(t)
	out := filepath.Join(root, "dist/fixture")
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNextOverlay(root, out, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/uvwt/agentdock\n\ngo 1.25\n"), 0600); err != nil {
		t.Fatal(err)
	}
	main := `package main
import("encoding/json"; "os"; "github.com/uvwt/agentdock/internal/buildinfo")
func main(){ if len(os.Args)!=3 || os.Args[1]!="version" || os.Args[2]!="--json" { os.Exit(2) }; json.NewEncoder(os.Stdout).Encode(buildinfo.Current()) }
`
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(main), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(out, "fixture")
	flags := "-X github.com/uvwt/agentdock/internal/buildinfo.Commit=" + m.Commit + " -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate=" + m.BuildDate
	cmd := exec.Command("go", "build", "-overlay", filepath.Join(out, "overlay.json"), "-ldflags", flags, "-o", binary, ".")
	cmd.Dir = root
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	data, err := exec.Command(binary, "version", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Version, Commit string
		BuildDate       string `json:"build_date"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != m.Version || info.Commit != m.Commit[:12] || info.BuildDate != m.BuildDate {
		t.Fatalf("incorrect binary metadata: %s", data)
	}
	metadata, err := exec.Command("go", "version", "-m", binary).Output()
	if err != nil || !bytes.Contains(metadata, []byte(m.Commit)) || !bytes.Contains(metadata, []byte(m.BuildDate)) {
		t.Fatalf("full provenance missing: %s (%v)", metadata, err)
	}
	stable, _ := os.ReadFile(filepath.Join(root, "internal/buildinfo/buildinfo.go"))
	if !bytes.Contains(stable, []byte(`const Version = "0.9.1"`)) {
		t.Fatal("overlay changed Stable source")
	}
	if err := writeNextOverlay(root, out, m); err == nil {
		t.Fatal("overlay overwritten")
	}
}

// Compile the actual bundled arbiter with the same overlay generator and linker
// provenance as Core. Inspect build metadata only; never execute the arbiter.
func TestNextArbiterOverlayBuild(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	m, err := nextContract(validNextInput())
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := writeNextOverlay(root, out, m); err != nil {
		t.Fatal(err)
	}
	overlaySource, err := os.ReadFile(filepath.Join(out, "next-buildinfo.go"))
	if err != nil || !bytes.Contains(overlaySource, []byte(`const Version = "0.9.1-rc.2"`)) {
		t.Fatalf("arbiter RC overlay missing: %v", err)
	}
	binary := filepath.Join(out, "arbiter-fixture")
	flags := "-X github.com/uvwt/agentdock/internal/buildinfo.Commit=" + m.Commit + " -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate=" + m.BuildDate
	cmd := exec.Command("go", "build", "-mod=readonly", "-buildvcs=false", "-overlay", filepath.Join(out, "overlay.json"), "-ldflags", flags, "-o", binary, "./cmd/agentdock-arbiter")
	cmd.Dir = root
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	metadata, err := exec.Command("go", "version", "-m", binary).Output()
	if err != nil || !bytes.Contains(metadata, []byte(m.Commit)) || !bytes.Contains(metadata, []byte(m.BuildDate)) {
		t.Fatalf("arbiter provenance missing: %s (%v)", metadata, err)
	}
}

func TestNextCandidatePlistFixture(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS plist fixture requires plutil/zsh")
	}
	root, m, data := candidateFixture(t)
	// Fixed read-only Git responses model a clean checkout without committing or
	// modifying the real checkout. The actual candidate command still validates it.
	bin := filepath.Join(root, "fixture-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(bin, "release")
	if out, err := exec.Command("go", "build", "-o", release, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	git := "#!/bin/sh\ncase \"$*\" in\n'rev-parse --show-toplevel') echo '" + root + "';;\n'rev-parse HEAD') echo '" + m.Commit + "';;\n'show -s --format=%ct HEAD') echo 1791936000;;\n'status --porcelain --untracked-files=all') printf '%s' \"${FIXTURE_DIRTY:-}\";;\n*) exit 2;; esac\n"
	for name, script := range map[string]string{"git": git, "go": "#!/bin/sh\n[ \"$1\" = run ] && [ \"$2\" = ./tools/release ] || exit 2\nshift 2\nexec '" + release + "' \"$@\"\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "AGENTDOCK_MACOS_APP_VARIANT=next", "AGENTDOCK_NEXT_CANDIDATE_MANIFEST="+manifest)
	output := filepath.Join(root, "dist/plist")
	cmd := exec.Command("zsh", filepath.Join(root, "packaging/macos/build-app.sh"), "--metadata-only", output)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	plist := filepath.Join(output, "AgentDock Next.app/Contents/Info.plist")
	for key, want := range map[string]string{"CFBundleShortVersionString": m.MacShortVersion, "CFBundleVersion": m.MacBuildVersion, "CFBundleIdentifier": m.MacBundleID} {
		value, err := exec.Command("plutil", "-extract", key, "raw", "-o", "-", plist).Output()
		if err != nil || strings.TrimSpace(string(value)) != want {
			t.Fatalf("%s: %q %v", key, value, err)
		}
	}
	cmd = exec.Command("zsh", filepath.Join(root, "packaging/macos/build-app.sh"), "--metadata-only", filepath.Join(root, "refused"))
	cmd.Env = append(env, "FIXTURE_DIRTY= M source.go")
	if out, err := cmd.CombinedOutput(); err == nil || !bytes.Contains(out, []byte("clean checkout")) {
		t.Fatalf("dirty accepted: %s %v", out, err)
	}
}
