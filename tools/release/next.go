package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type nextInput struct {
	Version, Tag, Commit, BuildDate, BundleBuild string
}

type nextManifest struct {
	Schema          string   `json:"schema"`
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	Tag             string   `json:"candidate_tag"`
	Commit          string   `json:"commit"`
	BuildDate       string   `json:"build_date"`
	Channel         string   `json:"channel"`
	MacName         string   `json:"macos_name"`
	MacBundleID     string   `json:"macos_bundle_id"`
	MacShortVersion string   `json:"cfbundle_short_version"`
	MacBuildVersion string   `json:"cfbundle_version"`
	WindowsVersion  string   `json:"windows_version"`
	CoreVersion     string   `json:"required_core_version"`
	CoreCommit      string   `json:"required_core_commit"`
	Artifacts       []string `json:"planned_artifacts"`
	UpdateFeed      string   `json:"update_feed"`
	UpdateURL       string   `json:"update_url"`
	ReleaseReady    bool     `json:"release_ready"`
	Blockers        []string `json:"remaining_gates"`
}

// This is a source contract, never evidence that binaries were built or published.
func nextContract(in nextInput) (nextManifest, error) {
	var m nextManifest
	parts := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.([1-9][0-9]*)$`).FindStringSubmatch(in.Version)
	if parts == nil {
		return m, errors.New("Next version must be canonical M.m.p-rc.N")
	}
	for _, part := range parts[1:] {
		n, err := strconv.ParseUint(part, 10, 16)
		if err != nil || n > 65535 {
			return m, errors.New("Windows version component exceeds 65535")
		}
	}
	if in.Tag != "next-v"+in.Version {
		return m, errors.New("candidate tag must exactly match next-v<version>")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(in.Commit) || in.Commit == strings.Repeat("0", 40) {
		return m, errors.New("commit must be a full nonzero lowercase Git SHA-1")
	}
	date, err := time.Parse(time.RFC3339, in.BuildDate)
	if err != nil || date.UTC().Format(time.RFC3339) != in.BuildDate {
		return m, errors.New("build date must be canonical UTC RFC3339")
	}
	if !regexp.MustCompile(`^[1-9][0-9]{0,3}$`).MatchString(in.BundleBuild) {
		return m, errors.New("explicit macOS bundle build ordinal must be 1..9999")
	}
	return nextManifest{
		Schema: "agentdock-next-candidate-v1", Mode: "source-only-dry-run",
		Version: in.Version, Tag: in.Tag, Commit: in.Commit, BuildDate: in.BuildDate,
		Channel: "next-rc", MacName: "AgentDock Next", MacBundleID: "dev.dropabit.agentdock.next",
		MacShortVersion: strings.Join(parts[1:4], "."), MacBuildVersion: in.BundleBuild + ".0.0",
		WindowsVersion: strings.Join(parts[1:], "."), CoreVersion: in.Version, CoreCommit: in.Commit,
		Artifacts:  []string{"AgentDock-Next-macos-arm64.zip", "AgentDock-Next-macos-arm64.dmg", "AgentDock-NextSetup-amd64.exe", "AgentDock-NextSetup-arm64.exe"},
		UpdateFeed: "disabled/unpublished", UpdateURL: "", ReleaseReady: false,
		Blockers: []string{"uiux-unfrozen", "custom/main-not-integrated", "windows-next-installer-not-isolated", "installer-metadata-and-architecture-not-wired", "hosted-ci-native-artifacts-not-verified", "p11-signing-updater-native-uat"},
	}, nil
}

// Read the canonical identity source without sourcing shell code or touching an app.
func verifyNextIdentity(source []byte) error {
	block := regexp.MustCompile(`(?s)\n  next\)\n(.*?)\n    ;;`).FindSubmatch(source)
	if block == nil {
		return errors.New("missing canonical Next identity block")
	}
	for key, value := range map[string]string{"APP_NAME": "AgentDock Next", "BUNDLE_ID": "dev.dropabit.agentdock.next", "ARTIFACT_PREFIX": "AgentDock-Next", "STATE_NAME": ".agentdock-next", "DEFAULT_PORT": "8767"} {
		matches := regexp.MustCompile(`(?m)^    `+key+`=(?:"([^"\n]*)"|([0-9]+))$`).FindAllSubmatch(block[1], -1)
		if len(matches) != 1 || string(matches[0][1])+string(matches[0][2]) != value {
			return fmt.Errorf("Next identity mismatch: %s", key)
		}
	}
	return nil
}

func runNextPreflight(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("next-preflight", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var in nextInput
	fs.StringVar(&in.Version, "version", "", "explicit RC version")
	fs.StringVar(&in.Tag, "tag", "", "candidate tag (never created)")
	fs.StringVar(&in.Commit, "commit", "", "expected HEAD SHA")
	fs.StringVar(&in.BuildDate, "build-date", "", "expected HEAD commit timestamp in UTC")
	fs.StringVar(&in.BundleBuild, "bundle-build", "", "explicit bundle build ordinal")
	verify := fs.String("verify", "", "compare an exact preflight JSON manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected next-preflight arguments")
	}
	m, err := nextContract(in)
	if err != nil {
		return err
	}
	// Fixed read-only Git queries; no user input is ever used as a command/ref.
	gitRead := func(args ...string) (string, error) {
		out, err := exec.Command("git", args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	root, err := gitRead("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	head, err := gitRead("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	stamp, err := gitRead("show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return err
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return err
	}
	if in.Commit != head || in.BuildDate != time.Unix(seconds, 0).UTC().Format(time.RFC3339) {
		return errors.New("commit/build-date mismatch with checked-out HEAD")
	}
	identity, err := os.ReadFile(filepath.Join(root, "packaging/macos/app-identity.sh"))
	if err != nil {
		return err
	}
	if err := verifyNextIdentity(identity); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if *verify != "" {
		expected, err := os.ReadFile(*verify)
		if err != nil {
			return err
		}
		// Exact canonical bytes reject unknown/duplicate keys as well as altered values.
		if !bytes.Equal(encoded, expected) {
			return errors.New("Next manifest mismatch (canonical JSON required)")
		}
	}
	_, err = stdout.Write(encoded)
	return err
}
