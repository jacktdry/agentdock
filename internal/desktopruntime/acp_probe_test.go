package desktopruntime

import "testing"

func TestNormalizeACPVersionAcceptsOnlyRecognizedVersionShapes(t *testing.T) {
	cases := map[string]string{
		"1.2.0-agentdock.6":                      "1.2.0-agentdock.6",
		"v1.2.0-agentdock.6":                     "1.2.0-agentdock.6",
		"antigravity-acp 1.2.0-agentdock.6":      "1.2.0-agentdock.6",
		"@agentclientprotocol/codex-acp 2.1.1":   "2.1.1",
		"codex-acp v2.1.1":                       "2.1.1",
		"credential=abc123":                      "",
		"error code 1.2.3":                       "",
		"unknown-adapter 2.1.1":                  "",
		"Authorization: Basic123":                "",
		"antigravity-acp credential=abc123":      "",
		"antigravity-acp 1.2":                    "",
		"antigravity-acp 1.2.3 extra-diagnostic": "",
	}
	for raw, want := range cases {
		if got := normalizeACPVersion(raw); got != want {
			t.Errorf("normalizeACPVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestCappedACPVersionOutputBoundsStoredDiagnostics(t *testing.T) {
	var output cappedACPVersionOutput
	payload := make([]byte, 8192)
	for i := range payload {
		payload[i] = 'x'
	}
	n, err := output.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if len(output.String()) != 4096 {
		t.Fatalf("stored bytes = %d", len(output.String()))
	}
}
