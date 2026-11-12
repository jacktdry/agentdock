package permission

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidPolicy = errors.New("invalid permission policy")
)

func ValidatePolicy(policy Policy) error {
	if policy.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version=%d", ErrInvalidPolicy, policy.SchemaVersion)
	}
	if policy.Revision == 0 {
		return fmt.Errorf("%w: revision must be positive", ErrInvalidPolicy)
	}
	if !validMode(policy.GlobalMode) {
		return fmt.Errorf("%w: global_mode=%q", ErrInvalidPolicy, policy.GlobalMode)
	}
	if err := validateSettings(policy.Settings); err != nil {
		return err
	}

	seenScopes := make(map[string]struct{}, len(policy.Scopes))
	for i, scope := range policy.Scopes {
		scope.ID = strings.TrimSpace(scope.ID)
		if scope.ID == "" {
			return fmt.Errorf("%w: scopes[%d] has empty workspace id", ErrInvalidPolicy, i)
		}
		if _, exists := seenScopes[scope.ID]; exists {
			return fmt.Errorf("%w: duplicate workspace scope %q", ErrInvalidPolicy, scope.ID)
		}
		seenScopes[scope.ID] = struct{}{}
		if scope.Mode != "" && !validMode(scope.Mode) {
			return fmt.Errorf("%w: scopes[%d] mode=%q", ErrInvalidPolicy, i, scope.Mode)
		}
		if scope.Settings != nil {
			if err := validateSettings(*scope.Settings); err != nil {
				return fmt.Errorf("%w: scopes[%d]: %v", ErrInvalidPolicy, i, err)
			}
		}
	}

	seenRules := make(map[string]struct{}, len(policy.Rules))
	for i, rule := range policy.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("%w: rules[%d] has empty id", ErrInvalidPolicy, i)
		}
		if _, exists := seenRules[rule.ID]; exists {
			return fmt.Errorf("%w: duplicate rule id %q", ErrInvalidPolicy, rule.ID)
		}
		seenRules[rule.ID] = struct{}{}
		if strings.TrimSpace(rule.Tool) == "" {
			return fmt.Errorf("%w: rules[%d] has empty tool", ErrInvalidPolicy, i)
		}
		if !validEffect(rule.Effect) {
			return fmt.Errorf("%w: rules[%d] effect=%q", ErrInvalidPolicy, i, rule.Effect)
		}
		if rule.WorkspaceID != "" {
			if _, exists := seenScopes[rule.WorkspaceID]; !exists {
				return fmt.Errorf("%w: rules[%d] references unknown workspace %q", ErrInvalidPolicy, i, rule.WorkspaceID)
			}
		}
	}
	return nil
}

func validateSettings(settings Settings) error {
	switch settings.Profile.Filesystem {
	case Deny, FileRead, FileWrite:
	default:
		return fmt.Errorf("%w: filesystem=%q", ErrInvalidPolicy, settings.Profile.Filesystem)
	}
	switch settings.Profile.Network {
	case Deny, Allow:
	default:
		return fmt.Errorf("%w: network=%q", ErrInvalidPolicy, settings.Profile.Network)
	}
	switch settings.Profile.SandboxBoundary {
	case BoundaryNone, BoundaryWorkspace:
	default:
		return fmt.Errorf("%w: sandbox_boundary=%q", ErrInvalidPolicy, settings.Profile.SandboxBoundary)
	}
	switch settings.Approval.Mode {
	case OnRequest, Never:
		if settings.Approval.Granular != nil {
			return fmt.Errorf("%w: granular categories require granular approval mode", ErrInvalidPolicy)
		}
	case Granular:
		if settings.Approval.Granular == nil {
			return fmt.Errorf("%w: granular approval mode requires categories", ErrInvalidPolicy)
		}
	default:
		return fmt.Errorf("%w: approval_mode=%q", ErrInvalidPolicy, settings.Approval.Mode)
	}
	if settings.Reviewer != Defer {
		return fmt.Errorf("%w: reviewer=%q", ErrInvalidPolicy, settings.Reviewer)
	}
	return nil
}

func validMode(mode string) bool {
	return mode == ReadOnly || mode == Rules || mode == Full
}

func validEffect(effect string) bool {
	return effect == Deny || effect == Ask || effect == Allow
}

func Evaluate(policy Policy, facts PermissionFacts) (Decision, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Decision{}, err
	}
	effective := effectivePolicy(policy, facts.Binding)
	decision := Decision{
		Effect:         Deny,
		RuleID:         "default-deny",
		Reason:         "permission evaluation did not produce an admission result",
		PolicyRevision: policy.Revision,
		Binding:        AuditBindingFor(facts.Binding),
		Effective:      effective,
		Sources:        make([]DecisionSource, 0, 8),
	}

	if source, constrained := hardConstraintDecision(facts.Constraints); constrained {
		decision.Effect = Deny
		decision.RuleID = source.ID
		decision.Reason = source.Reason
		decision.Sources = appendSource(decision.Sources, source)
		return decision, nil
	}

	if constrained, source := profileDecision(effective.Settings.Profile, facts); constrained {
		decision.Effect = Deny
		decision.RuleID = source.ID
		decision.Reason = source.Reason
		decision.Sources = appendSource(decision.Sources, source)
		return decision, nil
	}

	if source, matched := strongestRule(policy, facts); matched {
		decision.Effect = source.Effect
		decision.RuleID = source.ID
		decision.Reason = source.Reason
		decision.Sources = appendSource(decision.Sources, source)
	} else {
		source := modeDecision(effective.Mode, effective.Settings.Profile, facts)
		decision.Effect = source.Effect
		decision.RuleID = source.ID
		decision.Reason = source.Reason
		decision.Sources = appendSource(decision.Sources, source)
	}

	if decision.Effect == Ask {
		source, changed := approvalDecision(effective.Settings.Approval, facts)
		if changed {
			decision.Effect = source.Effect
			decision.RuleID = source.ID
			decision.Reason = source.Reason
			decision.Sources = appendSource(decision.Sources, source)
		}
	}

	for _, source := range facts.Constraints {
		decision.Sources = appendSource(decision.Sources, source)
	}
	decision.Reason = boundedUTF8(strings.TrimSpace(decision.Reason), MaxReasonBytes)
	return decision, nil
}

func effectivePolicy(policy Policy, binding PermissionBinding) Effective {
	mode := policy.GlobalMode
	settings := policy.Settings
	workspaceID := ""

	if binding.TrustedWorkspace && strings.TrimSpace(binding.WorkspaceID) != "" {
		for _, scope := range policy.Scopes {
			if scope.ID != binding.WorkspaceID {
				continue
			}
			workspaceID = scope.ID
			if scope.Mode != "" {
				mode = narrowerMode(mode, scope.Mode)
			}
			if scope.Settings != nil {
				settings = intersectSettings(settings, *scope.Settings)
			}
			break
		}
	}

	return Effective{Mode: mode, Settings: settings, WorkspaceID: workspaceID}
}

func narrowerMode(global, workspace string) string {
	if modeRank(workspace) < modeRank(global) {
		return workspace
	}
	return global
}

func modeRank(mode string) int {
	switch mode {
	case ReadOnly:
		return 0
	case Rules:
		return 1
	case Full:
		return 2
	default:
		return -1
	}
}

func intersectSettings(global, workspace Settings) Settings {
	return Settings{
		Profile: Profile{
			Filesystem:      narrowerFilesystem(global.Profile.Filesystem, workspace.Profile.Filesystem),
			Network:         narrowerNetwork(global.Profile.Network, workspace.Profile.Network),
			SandboxBoundary: narrowerBoundary(global.Profile.SandboxBoundary, workspace.Profile.SandboxBoundary),
		},
		Approval: intersectApproval(global.Approval, workspace.Approval),
		Reviewer: Defer,
	}
}

func narrowerFilesystem(a, b string) string {
	if filesystemRank(b) < filesystemRank(a) {
		return b
	}
	return a
}

func filesystemRank(value string) int {
	switch value {
	case Deny:
		return 0
	case FileRead:
		return 1
	case FileWrite:
		return 2
	default:
		return -1
	}
}

func narrowerNetwork(a, b string) string {
	if a == Deny || b == Deny {
		return Deny
	}
	return Allow
}

func narrowerBoundary(a, b string) string {
	if a == BoundaryWorkspace || b == BoundaryWorkspace {
		return BoundaryWorkspace
	}
	return BoundaryNone
}

func intersectApproval(a, b ApprovalPolicy) ApprovalPolicy {
	if a.Mode == Never || b.Mode == Never {
		return ApprovalPolicy{Mode: Never}
	}
	if a.Mode == OnRequest && b.Mode == OnRequest {
		return ApprovalPolicy{Mode: OnRequest}
	}
	var ac, bc ApprovalCategories
	if a.Mode == Granular && a.Granular != nil {
		ac = *a.Granular
	} else {
		ac = allApprovalCategories()
	}
	if b.Mode == Granular && b.Granular != nil {
		bc = *b.Granular
	} else {
		bc = allApprovalCategories()
	}
	combined := ApprovalCategories{
		FileWrites: ac.FileWrites && bc.FileWrites,
		Commands:   ac.Commands && bc.Commands,
		Network:    ac.Network && bc.Network,
		MCP:        ac.MCP && bc.MCP,
		Management: ac.Management && bc.Management,
		Other:      ac.Other && bc.Other,
	}
	return ApprovalPolicy{Mode: Granular, Granular: &combined}
}

func allApprovalCategories() ApprovalCategories {
	return ApprovalCategories{
		FileWrites: true,
		Commands:   true,
		Network:    true,
		MCP:        true,
		Management: true,
		Other:      true,
	}
}

func hardConstraintDecision(constraints []DecisionSource) (DecisionSource, bool) {
	for _, source := range constraints {
		if source.Effect != Deny {
			continue
		}
		if strings.TrimSpace(source.ID) == "" {
			source.ID = "runtime-constraint"
		}
		if strings.TrimSpace(source.Kind) == "" {
			source.Kind = "runtime_constraint"
		}
		if strings.TrimSpace(source.Reason) == "" {
			source.Reason = "Core hard constraint denied this operation"
		}
		return source, true
	}
	return DecisionSource{}, false
}

func profileDecision(profile Profile, facts PermissionFacts) (bool, DecisionSource) {
	restricted := profileRestricted(profile)
	unknown := !facts.EffectsKnown || facts.OpaqueProviderExecution
	if restricted && unknown {
		return true, DecisionSource{
			Kind:   "profile",
			ID:     "profile-unknown-effects",
			Effect: Deny,
			Reason: "restricted Permission Profile cannot prove this operation's effects; approval cannot widen the profile",
		}
	}
	if profile.Filesystem == Deny && facts.Filesystem != "" && facts.Filesystem != FileNone {
		return true, DecisionSource{Kind: "profile", ID: "profile-filesystem", Effect: Deny, Reason: "Permission Profile denies filesystem access"}
	}
	if profile.Filesystem == FileRead && facts.Filesystem == FileWrite {
		return true, DecisionSource{Kind: "profile", ID: "profile-filesystem", Effect: Deny, Reason: "Permission Profile allows filesystem reads but denies writes"}
	}
	if profile.Network == Deny && facts.Network {
		return true, DecisionSource{Kind: "profile", ID: "profile-network", Effect: Deny, Reason: "Permission Profile denies network access"}
	}
	if profile.SandboxBoundary == BoundaryWorkspace &&
		(!facts.WorkspaceBound || !facts.Binding.TrustedWorkspace || strings.TrimSpace(facts.Binding.WorkspaceID) == "") {
		return true, DecisionSource{Kind: "profile", ID: "profile-boundary", Effect: Deny, Reason: "Permission Profile requires all relevant targets to be proven inside the trusted workspace"}
	}
	return false, DecisionSource{}
}

func profileRestricted(profile Profile) bool {
	return profile.Filesystem != FileWrite || profile.Network != Allow || profile.SandboxBoundary != BoundaryNone
}

func strongestRule(policy Policy, facts PermissionFacts) (DecisionSource, bool) {
	bestRank := -1
	var best DecisionSource
	for _, rule := range policy.Rules {
		if rule.Tool != facts.Tool {
			continue
		}
		if rule.Action != "" && rule.Action != facts.Action {
			continue
		}
		if rule.WorkspaceID != "" {
			if !facts.Binding.TrustedWorkspace || rule.WorkspaceID != facts.Binding.WorkspaceID {
				continue
			}
		}
		rank := effectRank(rule.Effect)
		if rank <= bestRank {
			continue
		}
		bestRank = rank
		best = DecisionSource{
			Kind:   "rule",
			ID:     rule.ID,
			Effect: rule.Effect,
			Reason: boundedUTF8(strings.TrimSpace(rule.Reason), MaxReasonBytes),
		}
		if best.Reason == "" {
			best.Reason = "matched explicit permission rule"
		}
	}
	return best, bestRank >= 0
}

func effectRank(effect string) int {
	switch effect {
	case Allow:
		return 0
	case Ask:
		return 1
	case Deny:
		return 2
	default:
		return -1
	}
}

func modeDecision(mode string, profile Profile, facts PermissionFacts) DecisionSource {
	unknown := !facts.EffectsKnown || facts.OpaqueProviderExecution
	provenReadOnly := facts.EffectsKnown && !facts.OpaqueProviderExecution && facts.ReadOnly

	switch mode {
	case ReadOnly:
		if provenReadOnly {
			return DecisionSource{Kind: "default", ID: "readonly-safe", Effect: Allow, Reason: "Core proved this operation is read-only"}
		}
		return DecisionSource{Kind: "default", ID: "readonly-deny", Effect: Deny, Reason: "readonly mode denies operations that are not Core-proven read-only"}
	case Rules:
		if provenReadOnly {
			return DecisionSource{Kind: "default", ID: "rules-safe", Effect: Allow, Reason: "Core proved this operation is read-only"}
		}
		if unknown {
			if !profileRestricted(profile) && stablePrincipal(facts.Binding.Principal) && facts.OneShotEligible {
				return DecisionSource{Kind: "default", ID: "rules-opaque-review", Effect: Ask, Reason: "operation effects are opaque and require one-shot approval from the authenticated principal"}
			}
			return DecisionSource{Kind: "default", ID: "rules-unknown-deny", Effect: Deny, Reason: "rules mode cannot safely approve unknown effects for this principal/profile"}
		}
		return DecisionSource{Kind: "default", ID: "rules-review-side-effects", Effect: Ask, Reason: factsReason(facts, "known side effects require approval")}
	case Full:
		if unknown {
			return DecisionSource{Kind: "default", ID: "full-unclassified", Effect: Allow, Reason: "full mode permits this unclassified operation under the unrestricted compatibility profile"}
		}
		return DecisionSource{Kind: "default", ID: "full-allow", Effect: Allow, Reason: "full mode compatibility fallback allows this operation"}
	default:
		return DecisionSource{Kind: "default", ID: "invalid-mode", Effect: Deny, Reason: "invalid permission mode"}
	}
}

func stablePrincipal(principal AuthorizationPrincipal) bool {
	return principal.Authenticated && principal.Stable && strings.TrimSpace(principal.Kind) != "" && strings.TrimSpace(principal.ID) != ""
}

func approvalDecision(policy ApprovalPolicy, facts PermissionFacts) (DecisionSource, bool) {
	switch policy.Mode {
	case OnRequest:
		return DecisionSource{}, false
	case Never:
		return DecisionSource{Kind: "profile", ID: "approval-policy-never", Effect: Deny, Reason: "Approval Policy never does not accept approval requests"}, true
	case Granular:
		if policy.Granular == nil {
			return DecisionSource{Kind: "profile", ID: "approval-policy-invalid", Effect: Deny, Reason: "granular Approval Policy is missing category settings"}, true
		}
		categories := policy.Granular
		matched := false
		allowed := true
		check := func(applies, permitted bool) {
			if !applies {
				return
			}
			matched = true
			if !permitted {
				allowed = false
			}
		}
		check(facts.Filesystem == FileWrite, categories.FileWrites)
		check(facts.Commands, categories.Commands)
		check(facts.Network, categories.Network)
		check(facts.MCP, categories.MCP)
		check(facts.Management, categories.Management)
		check(facts.Other, categories.Other)
		if !matched {
			allowed = categories.Other
		}
		if !allowed {
			return DecisionSource{Kind: "profile", ID: "approval-policy-granular", Effect: Deny, Reason: "Approval Policy does not accept every applicable category for this request"}, true
		}
		return DecisionSource{}, false
	default:
		return DecisionSource{Kind: "profile", ID: "approval-policy-invalid", Effect: Deny, Reason: "invalid Approval Policy"}, true
	}
}

func AuditBindingFor(binding PermissionBinding) AuditSafeBinding {
	return AuditSafeBinding{
		PrincipalKind: boundedUTF8(strings.TrimSpace(binding.Principal.Kind), 128),
		PrincipalHash: opaqueHash("principal", binding.Principal.ID),
		Source:        boundedUTF8(strings.TrimSpace(binding.Source), 128),
		WorkspaceHash: opaqueHash("workspace", binding.WorkspaceID),
		SessionHash:   opaqueHash("acp-session", binding.ACPSessionID),
		ProviderHash:  opaqueHash("provider", binding.Provider),
	}
}

func opaqueHash(domain, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(domain + "\x00" + value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func appendSource(sources []DecisionSource, source DecisionSource) []DecisionSource {
	if len(sources) >= MaxSources {
		return sources
	}
	source.Kind = boundedUTF8(strings.TrimSpace(source.Kind), 128)
	source.ID = boundedUTF8(strings.TrimSpace(source.ID), 256)
	source.Effect = boundedUTF8(strings.TrimSpace(source.Effect), 64)
	source.Reason = boundedUTF8(strings.TrimSpace(source.Reason), MaxReasonBytes)
	return append(sources, source)
}

func factsReason(facts PermissionFacts, fallback string) string {
	if reason := strings.TrimSpace(facts.Reason); reason != "" {
		return boundedUTF8(reason, MaxReasonBytes)
	}
	return fallback
}

func boundedUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}
