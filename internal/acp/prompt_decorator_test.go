package acp

import (
	"context"
	"testing"
)

func TestPromptDecoratorRunsOnceForFreshACPSubmission(t *testing.T) {
	workspace := t.TempDir()
	manager, err := newTestManagerWithPromptMode(t.TempDir(), workspace, "test-adapter", "steering_prompt_required")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	created, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	manager.opts.PromptDecorator = func(id string, blocks []ContentBlock) ([]ContentBlock, error) {
		if id != created.Session.ID || len(blocks) != 1 || blocks[0]["text"] != "my work" {
			t.Fatalf("wrong ACP prompt admission input: %s %+v", id, blocks)
		}
		count++
		return append(blocks, TextBlock("report advisory MCP usage, no process kills")), nil
	}
	started, err := manager.SubmitPrompt(context.Background(), created.Session.ID, []ContentBlock{TextBlock("my work")})
	if err != nil {
		t.Fatal(err)
	}
	state := waitForSettledRun(t, manager, started.RunID)
	if state.Status != RunCompleted || count != 1 {
		t.Fatalf("decorated run mismatch: %+v, callback count=%d", state, count)
	}
}

func TestPromptDecoratorCannotBypassACPContentValidation(t *testing.T) {
	workspace := t.TempDir()
	manager, err := newTestManagerWithPromptMode(t.TempDir(), workspace, "test-adapter", "steering_prompt_required")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	created, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The callback must not be allowed to attach an unsupported modality.
	// The helper Adapter advertises no audio capability.
	manager.opts.PromptDecorator = func(_ string, blocks []ContentBlock) ([]ContentBlock, error) {
		return append(blocks, ContentBlock{"type": "audio", "data": "x", "mimeType": "audio/wav"}), nil
	}
	if _, err := manager.SubmitPrompt(context.Background(), created.Session.ID, []ContentBlock{TextBlock("original")}); errorCode(err) != "ACP_CAPABILITY_UNSUPPORTED" {
		t.Fatalf("decorator bypassed ACP post-validation: %v", err)
	}
	if len(manager.activeRunBySession) != 0 {
		t.Fatal("invalid decorated content created ACP Run")
	}
}
