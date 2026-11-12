//go:build windows

package browser

import (
	"context"
	"testing"
)

// The native Windows Edge process/signature/owner attestor is not implemented.
// A synthetically valid macOS fixture must not grant access to a company
// profile or establish a transport fence on Windows.
func TestWindowsNativeEdgeProcessIdentityUnavailable(t *testing.T) {
	for _, endpoint := range []string{
		"ws://127.0.0.1:9222/devtools/browser/candidate",
		"ws://127.0.0.1:9223/devtools/browser/other",
	} {
		evidence, err := inspectNativeEdgeProcess(context.Background(), endpoint, `C:\Users\Example\AppData\Local\Microsoft\Edge\User Data`)
		if err != errEdgeProcessUnqualified || evidence != (edgeProcessPreflight{}) {
			t.Fatalf("Windows native Edge evidence was qualified or leaked: %v", err)
		}
	}
}
