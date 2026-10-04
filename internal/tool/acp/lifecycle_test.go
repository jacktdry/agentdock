package acp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
)

func TestSessionLifecycleOptionsFromToolRequest(t *testing.T) {
	options, err := sessionLifecycleOptions(SessionRequest{})
	if err != nil || options.Policy != acpruntime.LifecyclePersistent || options.IdleCloseAfter != 0 {
		t.Fatalf("default options=%+v err=%v", options, err)
	}
	options, err = sessionLifecycleOptions(SessionRequest{LifecyclePolicy: string(acpruntime.LifecycleIdleManaged)})
	if err != nil || options.Policy != acpruntime.LifecycleIdleManaged || options.IdleCloseAfter != acpruntime.DefaultIdleCloseAfter {
		t.Fatalf("idle options=%+v err=%v", options, err)
	}
	custom := int((2 * time.Hour) / time.Millisecond)
	options, err = sessionLifecycleOptions(SessionRequest{LifecyclePolicy: string(acpruntime.LifecycleIdleManaged), IdleCloseAfterMS: &custom})
	if err != nil || options.IdleCloseAfter != 2*time.Hour {
		t.Fatalf("custom options=%+v err=%v", options, err)
	}
	invalidTTL := int(time.Hour / time.Millisecond)
	for _, request := range []SessionRequest{
		{IdleCloseAfterMS: &invalidTTL},
		{LifecyclePolicy: "future"},
		{LifecyclePolicy: string(acpruntime.LifecyclePersistent), IdleCloseAfterMS: &invalidTTL},
	} {
		if _, err := sessionLifecycleOptions(request); err == nil {
			t.Fatalf("request unexpectedly accepted: %+v", request)
		}
	}
}

func TestLifecycleUpdateDoesNotRequireAdapterProcess(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	manager, err := acpruntime.NewManager(acpruntime.Options{
		Home: home, DefaultCWD: cwd,
		Agent: acpruntime.AgentSpec{Name: "offline", Command: filepath.Join(t.TempDir(), "missing-adapter")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	record, err := manager.AttachRemoteSession(acpruntime.RemoteSession{RemoteSessionID: "remote-offline", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	service := NewMulti("offline", map[string]*acpruntime.Manager{"offline": manager})
	idle := int((90 * time.Minute) / time.Millisecond)
	result, err := service.Session(context.Background(), SessionRequest{
		Action: "update", SessionID: record.ID,
		LifecyclePolicy: string(acpruntime.LifecycleIdleManaged), IdleCloseAfterMS: &idle,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || updated.LifecyclePolicy != acpruntime.LifecycleIdleManaged || updated.IdleCloseAfterMS != int64(idle) {
		t.Fatalf("result=%#v", result)
	}
	change, _ := result["change"].(map[string]any)
	if change["field"] != "lifecycle" || change["after"] != string(acpruntime.LifecycleIdleManaged) || result["changed"] != true {
		t.Fatalf("change=%#v result=%#v", change, result)
	}
}

func TestLifecycleUpdateValidationIsExclusive(t *testing.T) {
	manager, err := acpruntime.NewManager(acpruntime.Options{
		Home: t.TempDir(), DefaultCWD: t.TempDir(),
		Agent: acpruntime.AgentSpec{Name: "offline", Command: filepath.Join(t.TempDir(), "missing-adapter")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	service := NewMulti("offline", map[string]*acpruntime.Manager{"offline": manager})
	for _, request := range []SessionRequest{
		{Action: "update", SessionID: "missing", LifecyclePolicy: "persistent", ModeID: "code"},
		{Action: "update", SessionID: "missing", LifecyclePolicy: "persistent", ConfigID: "safe", ConfigValue: true},
		{Action: "update", SessionID: "missing", IdleCloseAfterMS: intPtrTool(60_000)},
	} {
		_, callErr := service.Session(context.Background(), request)
		var toolErr *ToolError
		if !errors.As(callErr, &toolErr) || (toolErr.Code != "ACP_SESSION_UPDATE_INVALID" && toolErr.Code != "ACP_SESSION_LIFECYCLE_INVALID") {
			t.Fatalf("request=%+v err=%#v", request, callErr)
		}
	}
}

func TestLifecycleInputSchemaPublishesRuntimeBounds(t *testing.T) {
	schema, ok := InputSchema(ToolSession)
	if !ok {
		t.Fatal("ACP session schema missing")
	}
	props := schema["properties"].(map[string]any)
	policy := props["lifecycle_policy"].(map[string]any)
	values := policy["enum"].([]string)
	want := []string{"persistent", "ephemeral", "idle-managed"}
	if len(values) != len(want) {
		t.Fatalf("policy enum=%#v", values)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("policy enum=%#v", values)
		}
	}
	ttl := props["idle_close_after_ms"].(map[string]any)
	if ttl["minimum"] != int(acpruntime.MinIdleCloseAfter/time.Millisecond) || ttl["maximum"] != int(acpruntime.MaxIdleCloseAfter/time.Millisecond) {
		t.Fatalf("ttl schema=%#v", ttl)
	}
}

func intPtrTool(value int) *int { return &value }
