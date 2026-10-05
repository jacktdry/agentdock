package runtimeapi

import (
	"context"
	"errors"
	"testing"

	"github.com/uvwt/agentdock/internal/app"
)

func TestNormalDispatchCannotReachDesktopControl(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		_, err := Dispatch(context.Background(), &runtimeStub{}, Request{Method: method, Path: "/internal/desktop-control/permission-confirmations", Body: []byte(`{}`)})
		var te *app.ToolError
		if !errors.As(err, &te) || te.Code != "NOT_FOUND" {
			t.Fatalf("normal dispatch accepted Desktop route: %v", err)
		}
	}
}
func TestPermissionReadDispatchValidationAndUnsupported(t *testing.T) {
	for _, path := range []string{"/internal/runtime/permissions", "/internal/runtime/approvals"} {
		_, err := Dispatch(context.Background(), &runtimeStub{}, Request{Method: "GET", Path: path})
		var te *app.ToolError
		if !errors.As(err, &te) || te.Code != "PERMISSION_UNSUPPORTED" {
			t.Fatalf("missing permission capability: %v", err)
		}
	}
	for _, raw := range []string{"0", "-1", "513", "invalid"} {
		if _, err := parseRuntimeApprovalLimit(raw); err == nil {
			t.Fatalf("limit accepted: %s", raw)
		}
	}
}
