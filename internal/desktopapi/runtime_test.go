package desktopapi

import "testing"

func TestResolveRuntimeRootPrefersExplicitValue(t *testing.T) {
	t.Setenv(runtimeRootEnv, "/from-env")
	got, err := resolveRuntimeRoot("/explicit")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/explicit" {
		t.Fatalf("resolveRuntimeRoot() = %q", got)
	}
}

func TestRuntimeActionRejectsUnknownActionBeforeTouchingRuntime(t *testing.T) {
	service := NewRuntimeService("/definitely-not-a-real-runtime-root")
	result := service.Action(t.Context(), RuntimeAction("destroy"))
	if result.Completed || result.Error == nil {
		t.Fatalf("Action() = %#v", result)
	}
	if result.Error.Code != "runtime_action_invalid" ||
		result.Error.Category != ErrorCategoryValidation ||
		result.Error.Retryable {
		t.Fatalf("Action() error = %#v", result.Error)
	}
}
