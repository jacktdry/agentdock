package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
)

func candidateService(t *testing.T) *Service {
	t.Helper()
	home := t.TempDir()
	manager, err := pluginruntime.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := envstore.New(home)
	if err != nil {
		t.Fatal(err)
	}
	clients, err := mcpclient.NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{AgentDockHome: home}, manager, clients, envs, nil)
	t.Cleanup(func() { service.ReleaseDesktopCandidates(); service.ReleaseMCPLeases(); _ = clients.Close() })
	return service
}

func writeCandidatePlugin(t *testing.T, source, name, version string, withMCP bool) {
	t.Helper()
	writePluginServiceJSON(t, filepath.Join(source, "plugin.json"), map[string]any{
		"name": name, "version": version, "description": "Candidate Plugin",
		"provenance": map[string]any{"origin": "https://example.invalid/repo", "ref": "main"},
	})
	if withMCP {
		writePluginServiceJSON(t, filepath.Join(source, "mcp.json"), map[string]any{"mcpServers": map[string]any{
			"remote": map[string]any{
				"type": "streamable-http", "url": "https://example.invalid/mcp?secret=URL_CANARY",
				"headers": map[string]string{"Authorization": "${TOKEN}"},
			},
		}})
	}
}

func TestDesktopCandidateInstallIsImmutableDisabledAndSingleUse(t *testing.T) {
	s := candidateService(t)
	source := t.TempDir()
	writeCandidatePlugin(t, source, "candidate-demo", "1.0.0", true)
	view, err := s.PrepareDesktopCandidate(context.Background(), source, "install", "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(view)
	for _, forbidden := range []string{source, s.manager.Store().Home(), "review_token", "storage_key", "runtime_name", "URL_CANARY"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("candidate review leaked %q: %s", forbidden, data)
		}
	}
	if view.Kind != "install" || view.Review.Name != "candidate-demo" || view.Review.Version != "1.0.0" || view.Review.MCP[0].Endpoint != "https://example.invalid" {
		t.Fatalf("unexpected candidate: %#v", view)
	}

	writeCandidatePlugin(t, source, "candidate-demo", "9.9.9", true)
	registry, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	request := DesktopManageRequest{
		Action: "desktop_install_candidate", RequestID: strings.Repeat("a", 32),
		CandidateID: view.CandidateID, Name: view.Review.Name, ExpectedRegistryRevision: registry.Revision,
	}
	result, err := s.DesktopManage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result["completed"] != true || result["runtime_impact"] != "installed_disabled" {
		t.Fatalf("install result %#v", result)
	}
	installed, err := s.manager.Inspect("candidate-demo")
	if err != nil {
		t.Fatal(err)
	}
	if installed.Version != "1.0.0" || installed.Enabled {
		t.Fatalf("install did not use reviewed disabled snapshot: %#v", installed.State)
	}
	registry, _ = s.manager.Store().DesktopRegistrySnapshot()
	request.RequestID = strings.Repeat("b", 32)
	request.ExpectedRegistryRevision = registry.Revision
	if _, err := s.DesktopManage(context.Background(), request); err == nil {
		t.Fatal("consumed candidate was reusable")
	}
}

func TestDesktopCandidateUpdatePreservesEnabledAndChangesIncarnation(t *testing.T) {
	s := candidateService(t)
	initial := t.TempDir()
	writeCandidatePlugin(t, initial, "candidate-update", "1.0.0", false)
	review := s.manager.Validate(initial)
	installed, err := s.manager.InstallReviewedSource(context.Background(), initial, true, review.ReviewToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.manager.FinalizeActivation(installed.Name); err != nil {
		t.Fatal(err)
	}
	before, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := pluginruntime.DesktopGeneration(before.States[0])

	next := t.TempDir()
	writeCandidatePlugin(t, next, "candidate-update", "2.0.0", false)
	candidate, err := s.PrepareDesktopCandidate(context.Background(), next, "update", "candidate-update", oldGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Current == nil || candidate.Current.Version != "1.0.0" || candidate.Review.Version != "2.0.0" {
		t.Fatalf("missing update comparison: %#v", candidate)
	}
	request := DesktopManageRequest{
		Action: "desktop_update_candidate", RequestID: strings.Repeat("c", 32), CandidateID: candidate.CandidateID, Name: "candidate-update",
		ExpectedRegistryRevision: before.Revision, ExpectedGeneration: oldGeneration,
	}
	result, err := s.DesktopManage(context.Background(), request)
	if err != nil || result["completed"] != true {
		t.Fatalf("update result %#v err=%v", result, err)
	}
	after, err := s.manager.Inspect("candidate-update")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != "2.0.0" || !after.Enabled {
		t.Fatalf("update did not preserve enabled state: %#v", after.State)
	}
	if pluginruntime.DesktopGeneration(after.State) == oldGeneration {
		t.Fatal("update reused Plugin incarnation")
	}
}

func TestDesktopCandidateTargetGenerationAndExpiryFailClosed(t *testing.T) {
	s := candidateService(t)
	initial := t.TempDir()
	writeCandidatePlugin(t, initial, "candidate-fence", "1.0.0", false)
	review := s.manager.Validate(initial)
	installed, err := s.manager.InstallReviewedSource(context.Background(), initial, false, review.ReviewToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.manager.FinalizeActivation(installed.Name); err != nil {
		t.Fatal(err)
	}
	registry, _ := s.manager.Store().DesktopRegistrySnapshot()
	generation := pluginruntime.DesktopGeneration(registry.States[0])
	next := t.TempDir()
	writeCandidatePlugin(t, next, "candidate-fence", "2.0.0", false)
	candidate, err := s.PrepareDesktopCandidate(context.Background(), next, "update", "candidate-fence", generation)
	if err != nil {
		t.Fatal(err)
	}
	request := DesktopManageRequest{Action: "desktop_update_candidate", RequestID: strings.Repeat("d", 32), CandidateID: candidate.CandidateID, Name: "candidate-fence", ExpectedRegistryRevision: registry.Revision, ExpectedGeneration: strings.Repeat("0", 64)}
	if _, err := s.DesktopManage(context.Background(), request); err == nil {
		t.Fatal("stale generation accepted")
	}
	request.ExpectedGeneration = generation
	request.RequestID = strings.Repeat("e", 32)
	if _, err := s.DesktopManage(context.Background(), request); err != nil {
		t.Fatalf("candidate consumed by rejected target: %v", err)
	}

	expiringSource := t.TempDir()
	writeCandidatePlugin(t, expiringSource, "candidate-expire", "1.0.0", false)
	expiring, err := s.PrepareDesktopCandidate(context.Background(), expiringSource, "install", "", "")
	if err != nil {
		t.Fatal(err)
	}
	s.desktopCandidateMu.Lock()
	s.desktopCandidates[expiring.CandidateID].expiresAt = time.Now().Add(-time.Second)
	s.desktopCandidateMu.Unlock()
	if err := s.DiscardDesktopCandidate(expiring.CandidateID); err == nil {
		t.Fatal("expired candidate remained available")
	}
	s.desktopCandidateMu.Lock()
	_, remains := s.desktopCandidates[expiring.CandidateID]
	s.desktopCandidateMu.Unlock()
	if remains {
		t.Fatal("expired candidate remained in registry")
	}
	entries, err := os.ReadDir(filepath.Join(s.manager.Store().Home(), "tmp", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expired candidate retained staging directories: %v", entries)
	}
}

func TestDecodeDesktopCandidateRequestStrict(t *testing.T) {
	generation := strings.Repeat("a", 64)
	candidateID := strings.Repeat("b", 64)
	valid := []string{
		`{"action":"prepare","source":"/private/plugin","kind":"install"}`,
		`{"action":"prepare","source":"/private/plugin","kind":"update","target_name":"demo","target_generation":"` + generation + `"}`,
		`{"action":"discard","candidate_id":"` + candidateID + `"}`,
	}
	for _, body := range valid {
		if _, err := DecodeDesktopCandidateRequest([]byte(body)); err != nil {
			t.Fatalf("valid request rejected: %s: %v", body, err)
		}
	}
	invalid := []string{
		`{"action":"prepare","source":"relative/plugin","kind":"install"}`,
		`{"action":"prepare","source":"https://example.invalid/plugin.zip","kind":"install"}`,
		`{"action":"prepare","source":"/private/plugin","kind":"install","target_name":"demo"}`,
		`{"action":"prepare","source":"/private/plugin","kind":"update","target_name":"demo"}`,
		`{"action":"prepare","source":"/private/plugin","kind":"update","target_name":"demo","target_generation":"` + generation + `","review_token":"secret"}`,
		`{"action":"discard","candidate_id":"short"}`,
	}
	for _, body := range invalid {
		if _, err := DecodeDesktopCandidateRequest([]byte(body)); err == nil {
			t.Fatalf("invalid request accepted: %s", body)
		}
	}
}
