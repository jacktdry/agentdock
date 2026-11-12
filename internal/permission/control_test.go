package permission

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestControlAuthorityUsesFreshPerRuntimeCredential(t *testing.T) {
	first, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if first.Credential() == "" || second.Credential() == "" || first.Credential() == second.Credential() {
		t.Fatalf("desktop control credentials are not fresh: first=%t second=%t equal=%t",
			first.Credential() != "", second.Credential() != "", first.Credential() == second.Credential())
	}
	if !first.Authenticate(first.Credential()) || first.Authenticate("normal-mcp-bearer") {
		t.Fatal("desktop control credential authentication boundary is invalid")
	}
}

func TestControlConfirmationIsExactAtMostOnceAndMismatchBurnsChallenge(t *testing.T) {
	authority, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	policy.GlobalMode = Rules
	request := ControlMutationRequest{Kind: ControlMutationUpdatePolicy, PolicyRevision: 1, Policy: &policy}
	challenge, err := authority.BeginConfirmation(authority.Credential(), request)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Kind != ControlMutationUpdatePolicy || challenge.PolicyRevision != 1 || challenge.ID == "" || challenge.SchemaVersion != SchemaVersion {
		t.Fatalf("challenge=%#v", challenge)
	}

	changed := policy
	changed.GlobalMode = ReadOnly
	wrong := ControlMutationRequest{Kind: ControlMutationUpdatePolicy, PolicyRevision: 1, Policy: &changed}
	if _, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, wrong); !errors.Is(err, ErrConfirmationMismatch) {
		t.Fatalf("wrong payload consume err=%v", err)
	}
	if _, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, request); !errors.Is(err, ErrConfirmationNotFound) {
		t.Fatalf("mismatched challenge was reusable: %v", err)
	}

	challenge, err = authority.BeginConfirmation(authority.Credential(), request)
	if err != nil {
		t.Fatal(err)
	}
	if normalized, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, request); err != nil {
		t.Fatalf("exact challenge consume: %v", err)
	} else if normalized.Kind != ControlMutationUpdatePolicy || normalized.Policy == nil || normalized.Policy.Revision != 2 {
		t.Fatalf("normalized consumed mutation=%#v", normalized)
	}
	if _, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, request); !errors.Is(err, ErrConfirmationNotFound) {
		t.Fatalf("challenge was reusable: %v", err)
	}
}

func TestControlConfirmationRejectsNormalCredentialsAndExpires(t *testing.T) {
	authority, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	request := ControlMutationRequest{
		Kind: ControlMutationApproveOnce, ApprovalID: "approval-1", ApprovalVersion: 2, PolicyRevision: 3,
	}
	if _, err := authority.BeginConfirmation("normal-mcp-bearer", request); !errors.Is(err, ErrControlUnauthorized) {
		t.Fatalf("normal MCP bearer began confirmation: %v", err)
	}
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	authority.now = func() time.Time { return now }
	authority.ttl = time.Minute
	challenge, err := authority.BeginConfirmation(authority.Credential(), request)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, request); !errors.Is(err, ErrConfirmationExpired) {
		t.Fatalf("expired challenge err=%v", err)
	}
}

func TestControlMutationNormalizationBindsApprovalIdentityAndPolicyRevision(t *testing.T) {
	authority, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	request := ControlMutationRequest{
		Kind: ControlMutationReject, ApprovalID: "approval-1", ApprovalVersion: 4, PolicyRevision: 7,
	}
	challenge, err := authority.BeginConfirmation(authority.Credential(), request)
	if err != nil {
		t.Fatal(err)
	}
	wrongVersion := request
	wrongVersion.ApprovalVersion++
	if _, err := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, wrongVersion); !errors.Is(err, ErrConfirmationMismatch) {
		t.Fatalf("approval version mismatch err=%v", err)
	}

	invalid := request
	invalid.PolicyRevision = 0
	if _, err := authority.BeginConfirmation(authority.Credential(), invalid); !errors.Is(err, ErrControlMutation) {
		t.Fatalf("missing policy revision err=%v", err)
	}
}

func TestControlConfirmationConcurrentConsumeAllowsExactlyOne(t *testing.T) {
	authority, err := NewControlAuthority()
	if err != nil {
		t.Fatal(err)
	}
	request := ControlMutationRequest{
		Kind: ControlMutationApproveOnce, ApprovalID: "approval-race", ApprovalVersion: 1, PolicyRevision: 1,
	}
	challenge, err := authority.BeginConfirmation(authority.Credential(), request)
	if err != nil {
		t.Fatal(err)
	}

	const contenders = 16
	results := make(chan error, contenders)
	var wg sync.WaitGroup
	wg.Add(contenders)
	for range contenders {
		go func() {
			defer wg.Done()
			_, consumeErr := authority.ConsumeConfirmation(authority.Credential(), challenge.ID, request)
			results <- consumeErr
		}()
	}
	wg.Wait()
	close(results)

	succeeded, notFound := 0, 0
	for consumeErr := range results {
		switch {
		case consumeErr == nil:
			succeeded++
		case errors.Is(consumeErr, ErrConfirmationNotFound):
			notFound++
		default:
			t.Fatalf("unexpected concurrent consume error: %v", consumeErr)
		}
	}
	if succeeded != 1 || notFound != contenders-1 {
		t.Fatalf("concurrent consume results: succeeded=%d not_found=%d", succeeded, notFound)
	}
}
