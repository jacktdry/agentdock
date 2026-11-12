package app

import (
	"testing"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
)

func TestDefaultACPSessionLifecycleStableAndNext(t *testing.T) {
	for _, tc := range []struct {
		name    string
		variant string
		stdio   bool
		want    acpruntime.SessionLifecyclePolicy
	}{
		{name: "stable desktop", variant: "stable", want: acpruntime.LifecyclePersistent},
		{name: "unset desktop", variant: "", want: acpruntime.LifecyclePersistent},
		{name: "next desktop", variant: "next", want: acpruntime.LifecycleIdleManaged},
		{name: "next stdio", variant: "next", stdio: true, want: acpruntime.LifecyclePersistent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", tc.variant)
			got, err := acpruntime.NormalizeSessionLifecycleOptions(defaultACPSessionLifecycle(config.Config{Stdio: tc.stdio}))
			if err != nil {
				t.Fatal(err)
			}
			if got.Policy != tc.want {
				t.Fatalf("policy=%q want=%q", got.Policy, tc.want)
			}
			if tc.want == acpruntime.LifecycleIdleManaged && got.IdleCloseAfter != acpruntime.DefaultIdleCloseAfter {
				t.Fatalf("idle timeout=%s want=%s", got.IdleCloseAfter, acpruntime.DefaultIdleCloseAfter)
			}
		})
	}
}
