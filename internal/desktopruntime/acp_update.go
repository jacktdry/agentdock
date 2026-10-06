package desktopruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

const defaultAntigravityReleaseAPI = "https://api.github.com/repos/jacktdry/antigravity-acp/releases/latest"
const maxACPAdapterAssetBytes int64 = 128 << 20

type ACPUpdateSource struct {
	ReleaseAPI        string
	AllowedAssetHosts []string
}

type ACPAdapterUpdate struct {
	Supported      bool
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseTag     string
	AssetName      string
	AssetURL       string
	Digest         string
	TargetPath     string
	VersionState   string
	BlockedReason  string
}

type acpRelease struct {
	TagName string            `json:"tag_name"`
	Assets  []acpReleaseAsset `json:"assets"`
}

type acpReleaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func DefaultACPUpdateSource() ACPUpdateSource {
	return ACPUpdateSource{
		ReleaseAPI: defaultAntigravityReleaseAPI,
		AllowedAssetHosts: []string{
			"github.com",
			"objects.githubusercontent.com",
			"github-releases.githubusercontent.com",
		},
	}
}

func CheckACPProfileUpdate(ctx context.Context, client *http.Client, source ACPUpdateSource, runtimeRoot, preset string, profile ACPProfileSettings, probe ACPAdapterProbe) (ACPAdapterUpdate, error) {
	result := ACPAdapterUpdate{
		CurrentVersion: probe.InstalledVersion,
		TargetPath:     profile.Command,
		VersionState:   probe.VersionState,
	}
	if preset != "antigravity" {
		result.VersionState = "unsupported"
		result.BlockedReason = "shared_install_not_managed"
		return result, nil
	}
	if !NextManagedRoot(runtimeRoot) || !trustedAntigravityTarget(runtimeRoot, profile.Command) {
		result.VersionState = "unsupported"
		result.BlockedReason = "update_target_not_next_owned"
		return result, nil
	}
	if probe.Availability != "available" || probe.InstalledVersion == "" {
		result.VersionState = "unavailable"
		result.BlockedReason = "installed_version_unavailable"
		return result, nil
	}
	current, ok := parseAgentDockAdapterVersion(probe.InstalledVersion)
	if !ok {
		result.VersionState = "unavailable"
		result.BlockedReason = "installed_version_invalid"
		return result, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	release, status, err := fetchACPRelease(ctx, client, source.ReleaseAPI)
	if err != nil {
		if status == http.StatusNotFound {
			result.VersionState = "unavailable"
			result.BlockedReason = "trusted_release_unavailable"
			return result, nil
		}
		return result, err
	}
	latest, ok := parseAgentDockAdapterVersion(release.TagName)
	if !ok {
		result.VersionState = "unavailable"
		result.BlockedReason = "trusted_release_invalid"
		return result, nil
	}
	assetName, ok := antigravityAssetName(runtime.GOOS, runtime.GOARCH)
	if !ok {
		result.VersionState = "unsupported"
		result.BlockedReason = "platform_unsupported"
		return result, nil
	}
	asset, ok := findACPReleaseAsset(release.Assets, assetName)
	if !ok || !validSHA256Digest(asset.Digest) || !allowedACPAssetURL(asset.URL, source.AllowedAssetHosts) || asset.Size <= 0 || asset.Size > maxACPAdapterAssetBytes {
		result.VersionState = "unavailable"
		result.BlockedReason = "trusted_release_incomplete"
		return result, nil
	}
	result.Supported = true
	result.LatestVersion = latest.String()
	result.ReleaseTag = release.TagName
	result.AssetName = asset.Name
	result.AssetURL = asset.URL
	result.Digest = strings.ToLower(asset.Digest)
	if compareAgentDockAdapterVersions(latest, current) > 0 {
		result.Available = true
		result.VersionState = "update_available"
		result.BlockedReason = ""
	} else {
		result.VersionState = "current"
		result.BlockedReason = ""
	}
	return result, nil
}

func ApplyACPProfileUpdate(ctx context.Context, client *http.Client, source ACPUpdateSource, runtimeRoot string, update ACPAdapterUpdate) error {
	if !update.Supported || !update.Available || update.AssetURL == "" || !validSHA256Digest(update.Digest) || update.TargetPath == "" ||
		!trustedAntigravityTarget(runtimeRoot, update.TargetPath) || !allowedACPAssetURL(update.AssetURL, source.AllowedAssetHosts) {
		return errors.New("ACP adapter update plan is not actionable")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	data, err := downloadACPAsset(ctx, client, update.AssetURL, maxACPAdapterAssetBytes)
	if err != nil {
		return err
	}
	if !matchesSHA256(data, update.Digest) {
		return errors.New("ACP adapter update digest mismatch")
	}
	dir := filepath.Dir(update.TargetPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ACP adapter directory: %w", err)
	}
	staged, err := os.CreateTemp(dir, ".agentdock-acp-update-*")
	if err != nil {
		return fmt.Errorf("create ACP adapter staging file: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(0o700); err != nil {
		staged.Close()
		return fmt.Errorf("secure ACP adapter staging file: %w", err)
	}
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return fmt.Errorf("write ACP adapter staging file: %w", err)
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return fmt.Errorf("sync ACP adapter staging file: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close ACP adapter staging file: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(verifyCtx, stagedPath, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify ACP adapter staging binary: %w", err)
	}
	if normalizeACPVersion(string(output)) != update.LatestVersion {
		return errors.New("ACP adapter staging version mismatch")
	}
	if err := atomicfile.Write(update.TargetPath, data, 0o700); err != nil {
		return fmt.Errorf("promote ACP adapter update: %w", err)
	}
	return nil
}

func trustedAntigravityTarget(runtimeRoot, command string) bool {
	command = filepath.Clean(strings.TrimSpace(command))
	if command == "" || !filepath.IsAbs(command) || !NextManagedRoot(runtimeRoot) {
		return false
	}
	if info, err := os.Lstat(command); err == nil && !info.Mode().IsRegular() {
		return false
	}
	home, _ := os.UserHomeDir()
	roots := []string{filepath.Join(home, ".agentdock-next", "bin"), filepath.Join(runtimeRoot, "bin")}
	for _, root := range roots {
		root = filepath.Clean(root)
		relative, err := filepath.Rel(root, command)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			continue
		}
		base := strings.ToLower(filepath.Base(command))
		return base == "antigravity-acp" || base == "antigravity-acp.exe" || base == "antigravity-acp.com"
	}
	return false
}

func fetchACPRelease(ctx context.Context, client *http.Client, endpoint string) (acpRelease, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return acpRelease{}, 0, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "AgentDock-Next")
	response, err := client.Do(request)
	if err != nil {
		return acpRelease{}, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return acpRelease{}, response.StatusCode, fmt.Errorf("ACP release endpoint returned %s", response.Status)
	}
	limited := io.LimitReader(response.Body, 2<<20)
	var release acpRelease
	if err := json.NewDecoder(limited).Decode(&release); err != nil {
		return acpRelease{}, response.StatusCode, err
	}
	return release, response.StatusCode, nil
}

func downloadACPAsset(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "AgentDock-Next")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ACP adapter asset returned %s", response.Status)
	}
	if response.ContentLength > limit {
		return nil, errors.New("ACP adapter asset exceeds size limit")
	}
	reader := io.LimitReader(response.Body, limit+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("ACP adapter asset exceeds size limit")
	}
	return data, nil
}

func allowedACPAssetURL(rawURL string, allowedHosts []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range allowedHosts {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func findACPReleaseAsset(assets []acpReleaseAsset, name string) (acpReleaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return acpReleaseAsset{}, false
}

func antigravityAssetName(goos, goarch string) (string, bool) {
	arch := ""
	switch goarch {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x64"
	default:
		return "", false
	}
	switch goos {
	case "darwin":
		return "agy-acp-darwin-" + arch, true
	case "linux":
		return "agy-acp-linux-" + arch, true
	case "windows":
		return "agy-acp-windows-" + arch + ".exe", true
	default:
		return "", false
	}
}

func validSHA256Digest(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	raw := strings.TrimPrefix(value, "sha256:")
	if len(raw) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}

func matchesSHA256(data []byte, digest string) bool {
	if !validSHA256Digest(digest) {
		return false
	}
	sum := sha256.Sum256(data)
	return "sha256:"+hex.EncodeToString(sum[:]) == strings.ToLower(strings.TrimSpace(digest))
}

type agentDockAdapterVersion struct {
	major    int
	minor    int
	patch    int
	revision int
}

func (v agentDockAdapterVersion) String() string {
	return fmt.Sprintf("%d.%d.%d-agentdock.%d", v.major, v.minor, v.patch, v.revision)
}

func parseAgentDockAdapterVersion(raw string) (agentDockAdapterVersion, bool) {
	value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(value, "-agentdock.")
	if len(parts) != 2 {
		return agentDockAdapterVersion{}, false
	}
	base := strings.Split(parts[0], ".")
	if len(base) != 3 {
		return agentDockAdapterVersion{}, false
	}
	numbers := make([]int, 0, 4)
	for _, rawNumber := range append(base, parts[1]) {
		number, err := strconv.Atoi(rawNumber)
		if err != nil || number < 0 {
			return agentDockAdapterVersion{}, false
		}
		numbers = append(numbers, number)
	}
	return agentDockAdapterVersion{major: numbers[0], minor: numbers[1], patch: numbers[2], revision: numbers[3]}, true
}

func compareAgentDockAdapterVersions(left, right agentDockAdapterVersion) int {
	l := []int{left.major, left.minor, left.patch, left.revision}
	r := []int{right.major, right.minor, right.patch, right.revision}
	for index := range l {
		if l[index] < r[index] {
			return -1
		}
		if l[index] > r[index] {
			return 1
		}
	}
	return 0
}
