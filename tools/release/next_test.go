package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/buildinfo"
	"github.com/uvwt/agentdock/internal/updateidentity"
)

func validNextInput() nextInput {
	return nextInput{"0.9.1-rc.2", "next-v0.9.1-rc.2", strings.Repeat("a", 40), "2026-10-14T00:00:00Z", "2"}
}

func TestNextContract(t *testing.T) {
	m, err := nextContract(validNextInput())
	if err != nil {
		t.Fatal(err)
	}
	if m.MacShortVersion != "0.9.1" || m.MacBuildVersion != "2.0.0" || m.WindowsVersion != "0.9.1.2" || m.CoreVersion != "0.9.1-rc.2" || m.CoreCommit != m.Commit {
		t.Fatalf("incorrect platform mapping: %+v", m)
	}
	if m.ReleaseReady || m.UpdateFeed != "disabled/unpublished" || m.UpdateURL != "" || m.Channel != "next-rc" {
		t.Fatal("preflight must never grant update/release authority")
	}
	blockers := strings.Join(m.Blockers, ",")
	for _, stale := range []string{"core-next-version-not-wired", "selfupdate-upstream-api-not-next-channelized"} {
		if strings.Contains(blockers, stale) {
			t.Fatalf("stale gate: %s", stale)
		}
	}
	for _, gate := range []string{"macos-final-arm64-artifacts-not-verified", "hosted-macos-ci-not-verified", "p11-signing-updater-native-uat"} {
		if !strings.Contains(blockers, gate) {
			t.Fatalf("missing remaining gate: %s", gate)
		}
	}
	if len(m.Artifacts) != 2 {
		t.Fatalf("P10 macOS-only release must not promise Windows Setup artifacts: %v", m.Artifacts)
	}
	if strings.Contains(blockers, "windows-next-installer-not-isolated") || strings.Contains(blockers, "installer-metadata-and-architecture-not-wired") {
		t.Fatalf("Windows non-shipping feasibility cannot block macOS P10: %s", blockers)
	}
	for _, name := range m.Artifacts {
		if strings.Contains(name, "universal") {
			t.Fatalf("P10 Next is arm64-only: %s", name)
		}
		if !strings.HasPrefix(name, "AgentDock-Next") {
			t.Fatalf("Stable artifact: %s", name)
		}
		for _, stable := range ReleaseCatalog() {
			if stable.Name == name {
				t.Fatal("Next overlaps Stable catalog")
			}
		}
	}
	id, err := updateidentity.Resolve("next")
	if err != nil || m.Artifacts[0] != id.Artifact {
		t.Fatalf("manifest/update identity mismatch: %v, %v", m.Artifacts, err)
	}
	if m.Artifacts[0] != "AgentDock-Next-macos-arm64.zip" || m.Artifacts[1] != "AgentDock-Next-macos-arm64.dmg" {
		t.Fatalf("incorrect P10 macOS assets: %v", m.Artifacts)
	}
	if strings.Contains(buildinfo.Version, "rc") {
		t.Fatal("Stable buildinfo was changed to Next")
	}
}

func TestNextContractRejectsInsufficientOrAmbiguousMetadata(t *testing.T) {
	cases := map[string]func(*nextInput){
		"missing-version":  func(i *nextInput) { i.Version = "" },
		"stable-version":   func(i *nextInput) { i.Version = "0.9.1" },
		"leading-zero":     func(i *nextInput) { i.Version = "0.09.1-rc.2" },
		"rc-zero":          func(i *nextInput) { i.Version = "0.9.1-rc.0" },
		"windows-overflow": func(i *nextInput) { i.Version = "65536.9.1-rc.2" },
		"rc-overflow":      func(i *nextInput) { i.Version = "0.9.1-rc.65536" },
		"build-metadata":   func(i *nextInput) { i.Version += "+dev" },
		"stable-tag":       func(i *nextInput) { i.Tag = "v0.9.1" },
		"tag-mismatch":     func(i *nextInput) { i.Tag = "next-v0.9.1-rc.3" },
		"short-commit":     func(i *nextInput) { i.Commit = "78d89360" },
		"zero-commit":      func(i *nextInput) { i.Commit = strings.Repeat("0", 40) },
		"missing-date":     func(i *nextInput) { i.BuildDate = "" },
		"non-utc-date":     func(i *nextInput) { i.BuildDate = "2026-10-14T08:00:00+08:00" },
		"missing-build":    func(i *nextInput) { i.BundleBuild = "" },
		"zero-build":       func(i *nextInput) { i.BundleBuild = "0" },
		"bundle-overflow":  func(i *nextInput) { i.BundleBuild = "10000" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validNextInput()
			mutate(&in)
			if _, err := nextContract(in); err == nil {
				t.Fatal("invalid candidate accepted")
			}
		})
	}
}

func TestNextIdentityRejectsStableAndMissingFields(t *testing.T) {
	source, err := os.ReadFile("../../packaging/macos/app-identity.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyNextIdentity(source); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"AgentDock Next", "AgentDock"}, {"dev.dropabit.agentdock.next", "dev.dropabit.agentdock"}, {"AgentDock-Next", "AgentDock"}, {".agentdock-next", ".agentdock"}, {"DEFAULT_PORT=8767", "DEFAULT_PORT=8765"}, {"next)", "missing)"}} {
		if err := verifyNextIdentity(bytes.ReplaceAll(source, []byte(pair[0]), []byte(pair[1]))); err == nil {
			t.Fatalf("accepted identity drift: %v", pair)
		}
	}
}

func TestNextPreflightReadOnlyHEADAndManifestVerification(t *testing.T) {
	read := func(args ...string) string {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	head := read("rev-parse", "HEAD")
	stamp, err := strconv.ParseInt(read("show", "-s", "--format=%ct", "HEAD"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"next-preflight", "--version", "0.9.1-rc.2", "--tag", "next-v0.9.1-rc.2", "--commit", head, "--build-date", time.Unix(stamp, 0).UTC().Format(time.RFC3339), "--bundle-build", "2"}
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "--verify", path), discard{}); err != nil {
		t.Fatal(err)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &values); err != nil {
		t.Fatal(err)
	}
	for field := range values {
		t.Run(field, func(t *testing.T) {
			copyValues := map[string]json.RawMessage{}
			for key, value := range values {
				copyValues[key] = value
			}
			copyValues[field] = json.RawMessage(`null`)
			altered, err := json.MarshalIndent(copyValues, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(altered, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if err := run(append(args, "--verify", path), discard{}); err == nil {
				t.Fatal("altered manifest accepted")
			}
		})
	}
	wrong := append([]string(nil), args...)
	wrong[6] = strings.Repeat("b", 40)
	if err := run(wrong, discard{}); err == nil {
		t.Fatal("non-HEAD commit accepted")
	}
	wrong = append([]string(nil), args...)
	wrong[8] = "2000-01-01T00:00:00Z"
	if err := run(wrong, discard{}); err == nil {
		t.Fatal("wrong source timestamp accepted")
	}
}
