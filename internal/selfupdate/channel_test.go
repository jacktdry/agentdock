package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNextDisabledReasonIsRecognizableWithoutBypass(t *testing.T) {
	if !IsNextChannelDisabled(errNextChannelDisabled) || !IsNextChannelDisabled(
		fmt.Errorf("wrapped: %w", errNextChannelDisabled)) {
		t.Fatal("expected deliberate Next channel disablement")
	}
	if IsNextChannelDisabled(errors.New("network timeout")) || IsNextChannelDisabled(nil) {
		t.Fatal("unrelated errors must not look like intentional disablement")
	}
}

type channelTransport func(*http.Request) (*http.Response, error)

func (f channelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNextChannelBlocksCheckAndApplyBeforeHTTP(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		for _, source := range []string{"marker", "rc", "tag", "desktop-rc", "path", "conflict", "unknown"} {
			t.Run(goos+"/"+source, func(t *testing.T) {
				t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "")
				opts := options{CurrentVersion: "0.9.0", GOOS: goos, GOARCH: "amd64", ReleaseAPI: defaultReleaseAPI, Output: io.Discard}
				switch source {
				case "marker":
					t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
				case "rc":
					opts.CurrentVersion = "0.9.1-rc.1"
				case "tag":
					opts.CurrentVersion = "next-v0.9.1-rc.1"
				case "desktop-rc":
					opts.DesktopCurrentVersion = "0.9.1-rc.1"
				case "path":
					opts.ExecutablePath = `/fixture/AgentDock Next.app/Contents/Helpers/agentdock`
				case "conflict":
					t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
					opts.CurrentVersion = "0.9.1-rc.1"
					opts.DesktopTargetPath = "/Applications/AgentDock.app"
				case "unknown":
					t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "NEXT")
				}
				calls, mutations := 0, 0
				opts.HTTPClient = &http.Client{Transport: channelTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, errors.New("unexpected HTTP")
				})}
				opts.Apply = func(context.Context, applyRequest) (applyResult, error) { mutations++; return applyResult{}, nil }
				opts.VerifyBinary = func(context.Context, string, string) error { mutations++; return nil }
				opts.ExtractDesktop = func(context.Context, []byte, string, string) (string, error) { mutations++; return "", nil }
				for _, endpoint := range []string{defaultReleaseAPI, "", "https://fixture.invalid/release", "http://127.0.0.1:1/release"} {
					opts.ReleaseAPI = endpoint
					_, checkErr := checkWithOptions(context.Background(), opts)
					applyErr := run(context.Background(), opts)
					for _, err := range []error{checkErr, applyErr} {
						if source == "unknown" {
							if err == nil || !strings.Contains(err.Error(), "unknown update identity") {
								t.Fatalf("unknown marker: %v", err)
							}
						} else if !errors.Is(err, errNextChannelDisabled) {
							t.Fatalf("channel error: %v", err)
						}
					}
				}
				if calls != 0 || mutations != 0 {
					t.Fatalf("HTTP=%d mutations=%d", calls, mutations)
				}
			})
		}
	}
}

func TestUpdateChannelRuntimeSelection(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "")
	if got := newOptions(io.Discard, "/fixture/agentdock", "0.9.0", "").ReleaseAPI; got != defaultReleaseAPI {
		t.Fatal(got)
	}
	if got := newOptions(io.Discard, "/fixture/agentdock", "0.9.1-rc.1", "").ReleaseAPI; got != "" {
		t.Fatal(got)
	}
	for _, path := range []string{`C:\Users\fixture\AppData\Local\AgentDock Next\agentdock.exe`, "/fixture/.agentdock-next/runtime", "/fixture/AgentDock-Next/agentdock"} {
		if err := validateUpdateChannel(options{ExecutablePath: path}); !errors.Is(err, errNextChannelDisabled) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	if got := newOptions(io.Discard, "/fixture/agentdock", "0.9.0", "").ReleaseAPI; got != "" {
		t.Fatal(got)
	}
	if _, err := runtimeOptions(io.Discard); !errors.Is(err, errNextChannelDisabled) {
		t.Fatal(err)
	}
	if _, err := CheckForRuntime(context.Background(), "/fixture/nonexistent"); !errors.Is(err, errNextChannelDisabled) {
		t.Fatal(err)
	}
}

func TestNextFixtureCannotSelectExternalFeed(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	for _, endpoint := range []string{defaultReleaseAPI, "https://fixture.invalid", "http://localhost/release", ""} {
		if err := validateUpdateChannel(options{ReleaseAPI: endpoint, nextChannelFixture: true}); !errors.Is(err, errNextChannelDisabled) {
			t.Fatalf("%s: %v", endpoint, err)
		}
	}
	if err := validateUpdateChannel(options{ReleaseAPI: "http://127.0.0.1:1234/release", nextChannelFixture: true}); err != nil {
		t.Fatal(err)
	}
}
