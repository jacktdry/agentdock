package desktopruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateLocalMCPURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8765/mcp", "http://localhost:8765/mcp", "http://[::1]:8765/mcp"} {
		if got, err := validateLocalMCPURL(raw); err != nil || got == "" {
			t.Fatalf("valid %q => %q err=%v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"https://127.0.0.1:8765/mcp", "http://192.168.1.2:8765/mcp", "http://127.0.0.1:8765/", "http://127.0.0.1/mcp",
		"http://user@127.0.0.1:8765/mcp", "http://127.0.0.1:8765/mcp?x=1", "http://127.0.0.1:8765/mcp#x",
		"http://127.0.0.1:0/mcp", "http://127.0.0.1:65536/mcp", "http://127.0.0.1:8765/mcp?", "http://127.0.0.1:8765/mcp#",
		"http://127.0.0.1:8765/%6dcp", "http://localhost.example:8765/mcp", "http://[::1%25lo0]:8765/mcp",
	} {
		if _, err := validateLocalMCPURL(raw); err == nil {
			t.Fatalf("invalid local MCP URL accepted: %q", raw)
		}
	}
}

func TestLocalCoreAccessDoesNotSerializeOrFormatToken(t *testing.T) {
	access := LocalCoreAccess{MCPURL: "http://127.0.0.1:8765/mcp", AuthToken: "private-core-token"}
	data, _ := json.Marshal(access)
	for _, value := range []string{string(data), fmt.Sprint(access), fmt.Sprintf("%+v", access), fmt.Sprintf("%#v", &access)} {
		if strings.Contains(value, access.AuthToken) {
			t.Fatal("local access leaks credential")
		}
	}
	_, err := validateLocalMCPURL("http://private-core-token@127.0.0.1:8765/mcp")
	if err == nil || strings.Contains(err.Error(), access.AuthToken) {
		t.Fatal("URL error leaks input")
	}
}
