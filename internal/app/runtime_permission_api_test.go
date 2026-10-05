package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/permission"
)

func TestPermissionReadAPIValidationAndNoControlSecret(t *testing.T) {
	rt := newPermissionRuntime(t)
	result, err := rt.RuntimePermissions()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), rt.DesktopPermissionControlCredential()) {
		t.Fatal("normal read leaked control credential")
	}
	for _, args := range []struct {
		id, status string
		limit      int
	}{{"", "unknown", 1}, {"", "", -1}, {"", "", 513}, {"missing", "invalid", 1}} {
		_, err := rt.RuntimeApprovals(args.id, args.status, args.limit)
		var te *ToolError
		if !errors.As(err, &te) || te.Category != "validation" {
			t.Fatalf("invalid approval query: %v", err)
		}
	}
	_, err = rt.RuntimeApprovals("missing", "", 1)
	var te *ToolError
	if !errors.As(err, &te) || te.Code != "APPROVAL_NOT_FOUND" {
		t.Fatal(err)
	}
	for _, params := range []string{`{"credential":"forged"}`, `null`, `{} {}`} {
		if _, err := rt.DesktopPermissionBootstrap([]byte(params)); !errors.Is(err, permission.ErrControlMutation) {
			t.Fatal("bootstrap accepted arbitrary params")
		}
	}
}
