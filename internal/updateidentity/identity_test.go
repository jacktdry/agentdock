package updateidentity

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestIdentityCannotDowngradeNextMetadata(t *testing.T) {
	next, _ := Resolve("next")
	stable, _ := Resolve("")
	if err := next.ValidateMetadata("next", next.BundleID, next.Name, next.Name); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"", "stable", "unknown"} {
		if err := next.ValidateMetadata(marker, next.BundleID, next.Name, next.Name); err == nil {
			t.Fatalf("accepted marker %q", marker)
		}
	}
	if err := stable.ValidateMetadata("", stable.BundleID, "", ""); err != nil {
		t.Fatal("legacy stable rejected", err)
	}
	if err := stable.ValidateMetadata("", next.BundleID, next.Name, next.Name); err == nil {
		t.Fatal("Next downgraded to stable")
	}
	if _, err := Resolve("NEXT"); err == nil {
		t.Fatal("unknown identity accepted")
	}
	if next.Artifact != "AgentDock-Next-macos-arm64.zip" || next.Label("core") != "dev.dropabit.agentdock.next.core" {
		t.Fatal("Next contract drift")
	}
	for _, path := range []string{"/Applications/AgentDock.app", "/tmp/AgentDock Next.app", "AgentDock Next.app"} {
		if err := next.ValidateDestination(path, "/fixture"); err == nil {
			t.Fatalf("unsafe destination %s", path)
		}
	}
}

// Exercise only the canonical shell identity assignments, never bundle creation.
func TestArtifactMatchesCanonicalShellIdentity(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("canonical macOS shell identity requires zsh on macOS")
	}
	for _, variant := range []string{"next", "stable"} {
		t.Run(variant, func(t *testing.T) {
			id, err := Resolve(variant)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/zsh", "-c", `source ../../packaging/macos/app-identity.sh; print -r -- "$ZIP_NAME" "$DMG_NAME"`)
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "AGENTDOCK_MACOS_APP_VARIANT=") && !strings.HasPrefix(value, "AGENTDOCK_MACOS_ARCHES=") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "AGENTDOCK_MACOS_APP_VARIANT="+variant, "AGENTDOCK_MACOS_ARCHES=arm64")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("shell identity: %v: %s", err, out)
			}
			expected := id.Artifact + " " + strings.TrimSuffix(id.Artifact, ".zip") + ".dmg"
			if strings.TrimSpace(string(out)) != expected {
				t.Fatalf("shell/update identity mismatch: %s != %s", out, expected)
			}
		})
	}
}

func TestP10ArtifactArchitectureContract(t *testing.T) {
	next, _ := Resolve("next")
	stable, _ := Resolve("stable")
	legacy, _ := Resolve("")
	if next.Artifact != "AgentDock-Next-macos-arm64.zip" || stable.Artifact != "AgentDock-macos-universal.zip" || legacy != stable {
		t.Fatal("Next arm64 or historical Stable universal contract drift")
	}
}
