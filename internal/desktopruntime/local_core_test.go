package desktopruntime

import "testing"

func TestValidateLocalMCPURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8765/mcp", "http://localhost:8765/mcp", "http://[::1]:8765/mcp"} {
		if got, err := validateLocalMCPURL(raw); err != nil || got == "" {
			t.Fatalf("valid %q => %q err=%v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"https://127.0.0.1:8765/mcp", "http://192.168.1.2:8765/mcp", "http://127.0.0.1:8765/", "http://127.0.0.1/mcp",
		"http://user@127.0.0.1:8765/mcp", "http://127.0.0.1:8765/mcp?x=1", "http://127.0.0.1:8765/mcp#x",
	} {
		if _, err := validateLocalMCPURL(raw); err == nil {
			t.Fatalf("invalid local MCP URL accepted: %q", raw)
		}
	}
}
