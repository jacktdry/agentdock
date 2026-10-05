package permission

import (
	"strings"
	"testing"
)

func testFacts() PermissionFacts {
	return PermissionFacts{
		EffectsKnown: true,
		Filesystem:   FileNone,
		ReadOnly:     true,
		Tool:         "read_file",
		Binding: PermissionBinding{
			Principal: AuthorizationPrincipal{
				Kind:          "oauth",
				ID:            "client-secret-identity",
				Authenticated: true,
				Stable:        true,
			},
			RuntimeEpoch:     "epoch-1",
			Source:           "mcp",
			WorkspaceRoot:    "/secret/workspace",
			WorkspaceID:      "workspace-1",
			TrustedWorkspace: true,
			ACPSessionID:     "acp-secret",
			Provider:         "provider-secret",
		},
	}
}

func mustDecision(t *testing.T, policy Policy, facts PermissionFacts) Decision {
	t.Helper()
	got, err := Evaluate(policy, facts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return got
}

func TestDefaultPolicyBootstrap(t *testing.T) {
	p := DefaultPolicy()
	if p.SchemaVersion != SchemaVersion || p.Revision != 1 || p.GlobalMode != Full {
		t.Fatalf("default policy header = %+v", p)
	}
	if p.Settings.Profile != (Profile{Filesystem: FileWrite, Network: Allow, SandboxBoundary: BoundaryNone}) {
		t.Fatalf("default profile = %+v", p.Settings.Profile)
	}
	if p.Settings.Approval.Mode != OnRequest || p.Settings.Approval.Granular != nil || p.Settings.Reviewer != Defer {
		t.Fatalf("default settings = %+v", p.Settings)
	}
	if len(p.Scopes) != 0 || len(p.Rules) != 0 {
		t.Fatalf("default policy contains scopes/rules: %+v", p)
	}
	if err := ValidatePolicy(p); err != nil {
		t.Fatalf("ValidatePolicy(default): %v", err)
	}
}

func TestValidatePolicyRejectsInvalidSettings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Policy)
	}{
		{"schema", func(p *Policy) { p.SchemaVersion = 99 }},
		{"revision", func(p *Policy) { p.Revision = 0 }},
		{"mode", func(p *Policy) { p.GlobalMode = "auto" }},
		{"filesystem", func(p *Policy) { p.Settings.Profile.Filesystem = "root" }},
		{"network", func(p *Policy) { p.Settings.Profile.Network = "maybe" }},
		{"boundary", func(p *Policy) { p.Settings.Profile.SandboxBoundary = "host" }},
		{"reviewer", func(p *Policy) { p.Settings.Reviewer = "auto" }},
		{"granular-missing", func(p *Policy) { p.Settings.Approval = ApprovalPolicy{Mode: Granular} }},
		{"granular-on-request", func(p *Policy) {
			g := allApprovalCategories()
			p.Settings.Approval = ApprovalPolicy{Mode: OnRequest, Granular: &g}
		}},
		{"bad-rule-effect", func(p *Policy) {
			p.Rules = []Rule{{ID: "r", Tool: "x", Effect: "sometimes"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPolicy()
			tc.mutate(&p)
			if err := ValidatePolicy(p); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRestrictedProfileUnknownEffectsFailClosed(t *testing.T) {
	p := DefaultPolicy()
	p.Settings.Profile.Filesystem = FileRead
	f := testFacts()
	f.EffectsKnown = false
	f.ReadOnly = false
	d := mustDecision(t, p, f)
	if d.Effect != Deny || d.RuleID != "profile-unknown-effects" {
		t.Fatalf("decision = %+v", d)
	}
}

func TestProfileCeilings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Policy, *PermissionFacts)
		rule   string
	}{
		{"filesystem-deny", func(p *Policy, f *PermissionFacts) {
			p.Settings.Profile.Filesystem = Deny
			f.Filesystem = FileRead
		}, "profile-filesystem"},
		{"filesystem-read", func(p *Policy, f *PermissionFacts) {
			p.Settings.Profile.Filesystem = FileRead
			f.Filesystem = FileWrite
			f.ReadOnly = false
		}, "profile-filesystem"},
		{"network", func(p *Policy, f *PermissionFacts) {
			p.Settings.Profile.Network = Deny
			f.Network = true
		}, "profile-network"},
		{"workspace", func(p *Policy, f *PermissionFacts) {
			p.Settings.Profile.SandboxBoundary = BoundaryWorkspace
			f.WorkspaceBound = false
		}, "profile-boundary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPolicy()
			f := testFacts()
			tc.mutate(&p, &f)
			d := mustDecision(t, p, f)
			if d.Effect != Deny || d.RuleID != tc.rule {
				t.Fatalf("decision = %+v", d)
			}
		})
	}
}

func TestRulePrecedenceDenyAskAllow(t *testing.T) {
	p := DefaultPolicy()
	p.GlobalMode = Full
	p.Rules = []Rule{
		{ID: "allow", Tool: "exec_command", Effect: Allow},
		{ID: "ask", Tool: "exec_command", Effect: Ask},
		{ID: "deny", Tool: "exec_command", Effect: Deny},
	}
	f := testFacts()
	f.Tool = "exec_command"
	f.ReadOnly = false
	f.Commands = true
	f.Filesystem = FileWrite
	d := mustDecision(t, p, f)
	if d.Effect != Deny || d.RuleID != "deny" {
		t.Fatalf("decision = %+v", d)
	}

	p.Rules = p.Rules[:2]
	d = mustDecision(t, p, f)
	if d.Effect != Ask || d.RuleID != "ask" {
		t.Fatalf("ask precedence decision = %+v", d)
	}
}

func TestWorkspaceCannotWidenGlobalRule(t *testing.T) {
	for _, globalEffect := range []string{Deny, Ask} {
		t.Run(globalEffect, func(t *testing.T) {
			p := DefaultPolicy()
			p.Scopes = []WorkspaceScope{{ID: "workspace-1"}}
			p.Rules = []Rule{
				{ID: "global", Tool: "exec_command", Effect: globalEffect},
				{ID: "workspace-allow", Tool: "exec_command", WorkspaceID: "workspace-1", Effect: Allow},
			}
			f := testFacts()
			f.Tool = "exec_command"
			f.ReadOnly = false
			f.Commands = true
			f.Filesystem = FileWrite
			d := mustDecision(t, p, f)
			if d.Effect != globalEffect || d.RuleID != "global" {
				t.Fatalf("decision = %+v", d)
			}
		})
	}
}

func TestWorkspaceSettingsOnlyNarrow(t *testing.T) {
	p := DefaultPolicy()
	scopeSettings := Settings{
		Profile:  Profile{Filesystem: FileRead, Network: Deny, SandboxBoundary: BoundaryWorkspace},
		Approval: ApprovalPolicy{Mode: Never},
		Reviewer: Defer,
	}
	p.Scopes = []WorkspaceScope{{ID: "workspace-1", Mode: ReadOnly, Settings: &scopeSettings}}
	f := testFacts()
	d := mustDecision(t, p, f)
	if d.Effective.Mode != ReadOnly ||
		d.Effective.Settings.Profile.Filesystem != FileRead ||
		d.Effective.Settings.Profile.Network != Deny ||
		d.Effective.Settings.Profile.SandboxBoundary != BoundaryWorkspace ||
		d.Effective.Settings.Approval.Mode != Never {
		t.Fatalf("effective = %+v", d.Effective)
	}

	f.Binding.TrustedWorkspace = false
	d = mustDecision(t, p, f)
	if d.Effective.WorkspaceID != "" || d.Effective.Mode != Full {
		t.Fatalf("untrusted workspace influenced effective policy: %+v", d.Effective)
	}
}

func TestModeFallbacks(t *testing.T) {
	t.Run("readonly-safe", func(t *testing.T) {
		p := DefaultPolicy()
		p.GlobalMode = ReadOnly
		d := mustDecision(t, p, testFacts())
		if d.Effect != Allow {
			t.Fatalf("decision = %+v", d)
		}
	})
	t.Run("readonly-side-effect", func(t *testing.T) {
		p := DefaultPolicy()
		p.GlobalMode = ReadOnly
		f := testFacts()
		f.ReadOnly = false
		f.Filesystem = FileWrite
		d := mustDecision(t, p, f)
		if d.Effect != Deny {
			t.Fatalf("decision = %+v", d)
		}
	})
	t.Run("rules-known-side-effect", func(t *testing.T) {
		p := DefaultPolicy()
		p.GlobalMode = Rules
		f := testFacts()
		f.ReadOnly = false
		f.Filesystem = FileWrite
		d := mustDecision(t, p, f)
		if d.Effect != Ask {
			t.Fatalf("decision = %+v", d)
		}
	})
	t.Run("full-known-side-effect", func(t *testing.T) {
		p := DefaultPolicy()
		f := testFacts()
		f.ReadOnly = false
		f.Filesystem = FileWrite
		d := mustDecision(t, p, f)
		if d.Effect != Allow {
			t.Fatalf("decision = %+v", d)
		}
	})
}

func TestRulesUnknownRequiresStableEligiblePrincipal(t *testing.T) {
	p := DefaultPolicy()
	p.GlobalMode = Rules
	f := testFacts()
	f.EffectsKnown = false
	f.ReadOnly = false
	f.OneShotEligible = true
	d := mustDecision(t, p, f)
	if d.Effect != Ask || d.RuleID != "rules-opaque-review" {
		t.Fatalf("stable principal decision = %+v", d)
	}

	for _, mutate := range []func(*PermissionFacts){
		func(f *PermissionFacts) { f.Binding.Principal.Authenticated = false },
		func(f *PermissionFacts) { f.Binding.Principal.Stable = false },
		func(f *PermissionFacts) { f.Binding.Principal.ID = "" },
		func(f *PermissionFacts) { f.OneShotEligible = false },
	} {
		f2 := f
		f2.Binding = f.Binding
		f2.Binding.Principal = f.Binding.Principal
		mutate(&f2)
		got := mustDecision(t, p, f2)
		if got.Effect != Deny {
			t.Fatalf("unstable/anonymous principal decision = %+v", got)
		}
	}
}

func TestFullUnknownIsTruthfulCompatibilityAllow(t *testing.T) {
	p := DefaultPolicy()
	f := testFacts()
	f.EffectsKnown = false
	f.ReadOnly = false
	d := mustDecision(t, p, f)
	if d.Effect != Allow || d.RuleID != "full-unclassified" || !strings.Contains(d.Reason, "unclassified") {
		t.Fatalf("decision = %+v", d)
	}
}

func TestGranularRequiresEveryApplicableCategory(t *testing.T) {
	p := DefaultPolicy()
	p.GlobalMode = Rules
	g := ApprovalCategories{FileWrites: true, Commands: true, Network: false, Other: true}
	p.Settings.Approval = ApprovalPolicy{Mode: Granular, Granular: &g}
	f := testFacts()
	f.ReadOnly = false
	f.Filesystem = FileWrite
	f.Commands = true
	f.Network = true
	d := mustDecision(t, p, f)
	if d.Effect != Deny || d.RuleID != "approval-policy-granular" {
		t.Fatalf("decision = %+v", d)
	}

	g.Network = true
	p.Settings.Approval.Granular = &g
	d = mustDecision(t, p, f)
	if d.Effect != Ask {
		t.Fatalf("all categories allowed decision = %+v", d)
	}

	f.Filesystem = FileNone
	f.Commands = false
	f.Network = false
	f.Other = false
	g.Other = false
	p.Settings.Approval.Granular = &g
	d = mustDecision(t, p, f)
	if d.Effect != Deny {
		t.Fatalf("unmatched should use other category: %+v", d)
	}
}

func TestApprovalNeverConvertsAskToDeny(t *testing.T) {
	p := DefaultPolicy()
	p.GlobalMode = Rules
	p.Settings.Approval = ApprovalPolicy{Mode: Never}
	f := testFacts()
	f.ReadOnly = false
	f.Filesystem = FileWrite
	d := mustDecision(t, p, f)
	if d.Effect != Deny || d.RuleID != "approval-policy-never" {
		t.Fatalf("decision = %+v", d)
	}
}

func TestAuditSafeBindingDoesNotExposeRawIdentity(t *testing.T) {
	f := testFacts()
	audit := AuditBindingFor(f.Binding)
	encoded := strings.Join([]string{
		audit.PrincipalKind,
		audit.PrincipalHash,
		audit.Source,
		audit.WorkspaceHash,
		audit.SessionHash,
		audit.ProviderHash,
	}, "|")
	for _, secret := range []string{
		f.Binding.Principal.ID,
		f.Binding.WorkspaceRoot,
		f.Binding.WorkspaceID,
		f.Binding.ACPSessionID,
		f.Binding.Provider,
	} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("audit binding leaked %q: %s", secret, encoded)
		}
	}
	if audit.PrincipalHash == "" || audit.WorkspaceHash == "" || !strings.HasPrefix(audit.PrincipalHash, "sha256:") {
		t.Fatalf("audit hashes missing: %+v", audit)
	}
}

func TestDecisionSourcesAreBounded(t *testing.T) {
	p := DefaultPolicy()
	f := testFacts()
	f.Constraints = make([]DecisionSource, MaxSources+20)
	for i := range f.Constraints {
		f.Constraints[i] = DecisionSource{
			Kind:   strings.Repeat("k", 300),
			ID:     strings.Repeat("i", 500),
			Effect: "constrain",
			Reason: strings.Repeat("r", MaxReasonBytes+100),
		}
	}
	d := mustDecision(t, p, f)
	if len(d.Sources) != MaxSources {
		t.Fatalf("sources len=%d want=%d", len(d.Sources), MaxSources)
	}
	for _, source := range d.Sources {
		if len(source.Reason) > MaxReasonBytes || len(source.Kind) > 128 || len(source.ID) > 256 {
			t.Fatalf("unbounded source: %+v", source)
		}
	}
}

func TestHardConstraintDenyWinsBeforePolicy(t *testing.T) {
	p := DefaultPolicy()
	f := testFacts()
	f.Constraints = []DecisionSource{
		{Kind: "runtime_constraint", ID: "broker-owner", Effect: Deny, Reason: "broker owner mismatch"},
	}
	d := mustDecision(t, p, f)
	if d.Effect != Deny || d.RuleID != "broker-owner" {
		t.Fatalf("decision = %+v", d)
	}
}

func TestWorkspaceBoundaryRequiresTrustedBinding(t *testing.T) {
	p := DefaultPolicy()
	p.Settings.Profile.SandboxBoundary = BoundaryWorkspace
	f := testFacts()
	f.WorkspaceBound = true

	for _, mutate := range []func(*PermissionFacts){
		func(f *PermissionFacts) { f.Binding.TrustedWorkspace = false },
		func(f *PermissionFacts) { f.Binding.WorkspaceID = "" },
	} {
		gotFacts := f
		gotFacts.Binding = f.Binding
		mutate(&gotFacts)
		d := mustDecision(t, p, gotFacts)
		if d.Effect != Deny || d.RuleID != "profile-boundary" {
			t.Fatalf("decision = %+v", d)
		}
	}
}
