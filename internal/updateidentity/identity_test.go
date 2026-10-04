package updateidentity

import "testing"

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
	if next.Artifact != "AgentDock-Next-macos-universal.zip" || next.Label("core") != "dev.dropabit.agentdock.next.core" {
		t.Fatal("Next contract drift")
	}
	for _, path := range []string{"/Applications/AgentDock.app", "/tmp/AgentDock Next.app", "AgentDock Next.app"} {
		if err := next.ValidateDestination(path, "/fixture"); err == nil {
			t.Fatalf("unsafe destination %s", path)
		}
	}
}
