package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/commandpath"
	"github.com/uvwt/agentdock/internal/config"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
)

type acpHostCapabilityProvider struct {
	bridge     *toolbrowser.ACPBridge
	computer   *toolcomputer.ACPBridge
	profileID  string
	serverName string
	port       int
}

func (p acpHostCapabilityProvider) Servers(ctx context.Context, sessionID, cwd string, _ []string) ([]acpruntime.SessionMCPServer, error) {
	servers := make([]acpruntime.SessionMCPServer, 0, 2)
	token := ""
	if p.bridge != nil {
		var err error
		token, err = p.bridge.RegisterSession(sessionID, p.profileID, cwd)
		if err != nil {
			return nil, err
		}
		name := p.serverName
		if name == "" {
			name = "agentdock-browser"
		}
		servers = append(servers, acpruntime.SessionMCPServer{Name: name, Type: "http", URL: p.bridge.MCPServerURL(p.port), Headers: []acpruntime.SessionMCPHeader{{Name: "Authorization", Value: "Bearer " + token}}})
	}
	if p.computer != nil {
		computerToken, err := p.computer.RegisterSessionWithTokenAndWorkspace(sessionID, p.profileID, cwd, token)
		if err != nil {
			if p.bridge != nil {
				_ = p.bridge.ReleaseSession(ctx, sessionID)
			}
			return nil, err
		}
		if token == "" {
			token = computerToken
		}
		servers = append(servers, acpruntime.SessionMCPServer{Name: "agentdock-computer", Type: "http", URL: p.computer.MCPServerURL(p.port), Headers: []acpruntime.SessionMCPHeader{{Name: "Authorization", Value: "Bearer " + token}}})
	}
	return servers, nil
}

func (p acpHostCapabilityProvider) ReleaseSession(ctx context.Context, sessionID string) error {
	var failures []error
	if p.bridge != nil {
		failures = append(failures, p.bridge.ReleaseSession(ctx, sessionID))
	}
	if p.computer != nil {
		failures = append(failures, p.computer.ReleaseSession(ctx, sessionID))
	}
	return errors.Join(failures...)
}

func acpBrowserServerName(config.ACPProfile) string { return "agentdock-browser" }

func acpProfileUsesAntigravity(profile config.ACPProfile) bool {
	base := strings.ToLower(filepath.Base(profile.Command))
	return strings.Contains(base, "antigravity-acp") || base == "agy-acp" || profile.ID == "antigravity" || profile.ID == "agy"
}

// prepareACPBrowserEnvironment prevents adapter-owned Browser/Computer Control
// backends while preserving unrelated ACP configuration.
func prepareACPBrowserEnvironment(agentDockHome string, profile config.ACPProfile, env map[string]string, brokerAvailable bool) (map[string]string, error) {
	if env == nil {
		env = make(map[string]string)
	}
	if profile.Kind == "codex" {
		var err error
		env, err = prepareCodexACPHome(agentDockHome, profile, env)
		if err != nil {
			return nil, err
		}
		cfg := map[string]any{}
		if raw := strings.TrimSpace(env["CODEX_CONFIG"]); raw != "" {
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return nil, fmt.Errorf("ACP profile %s CODEX_CONFIG: %w", profile.ID, err)
			}
			if env["AGENTDOCK_NEXT_CODEX_MCP_ISOLATION"] == "source-only" && containsNativeMCPConfig(cfg, 0) {
				return nil, errNextCodexRegistryUnavailable
			}
		}
		if servers, ok := cfg["mcp_servers"].(map[string]any); ok {
			for _, key := range codexACPBlockedMCPServers {
				delete(servers, key)
			}
		}
		features, _ := cfg["features"].(map[string]any)
		if features == nil {
			features = make(map[string]any)
			cfg["features"] = features
		}
		for _, key := range []string{"in_app_browser", "browser_use_full_cdp_access", "browser_use_external", "in_app_local_automation"} {
			features[key] = false
		}
		plugins, _ := cfg["plugins"].(map[string]any)
		if plugins == nil {
			plugins = make(map[string]any)
			cfg["plugins"] = plugins
		}
		for _, key := range []string{"chrome@openai-bundled", "computer-use@openai-bundled", "unified-computer-use@openai-bundled"} {
			entry, _ := plugins[key].(map[string]any)
			if entry == nil {
				entry = make(map[string]any)
			}
			entry["enabled"] = false
			plugins[key] = entry
		}
		encoded, err := json.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		env["CODEX_CONFIG"] = string(encoded)
		delete(env, "DISABLE_MCP_CONFIG_FILTERING")
	}
	if acpProfileUsesAntigravity(profile) {
		if err := prepareNextAntigravityACPState(agentDockHome, env); err != nil {
			return nil, err
		}
		if brokerAvailable {
			env["AGENTDOCK_BROWSER_BROKER_REQUIRED"] = "1"
			delete(env, "AGENTDOCK_BROWSER_BACKENDS_DISABLED")
		} else {
			// ACP remains usable for coding when Browser Broker is disabled, but its
			// AGY children must still run in the browser-free sandbox instead of
			// inheriting the user's global ~/.gemini browser MCP/plugins.
			env["AGENTDOCK_BROWSER_BACKENDS_DISABLED"] = "1"
			delete(env, "AGENTDOCK_BROWSER_BROKER_REQUIRED")
		}
	}
	return env, nil
}

// Use the existing desktop marker and exact Next home; stable and custom homes
// retain their adapter configuration. Parent ACP HOME is never changed here.
func prepareNextAntigravityACPState(agentDockHome string, env map[string]string) error {
	if runtime.GOOS != "darwin" || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve Next Antigravity ACP home: %w", err)
	}
	if !filepath.IsAbs(home) || agentDockHome != filepath.Join(home, ".agentdock-next") {
		return nil
	}
	state := filepath.Join(agentDockHome, "acp", "antigravity")
	expected := map[string]string{"AGY_ACP_STATE_DIR": state, "AGY_ACP_CHILD_ISOLATION": "required"}
	for key, value := range expected {
		if supplied, exists := env[key]; exists && supplied != value {
			return fmt.Errorf("conflicting Next Antigravity ACP %s", key)
		}
	}
	for _, dir := range []string{agentDockHome, filepath.Join(agentDockHome, "acp"), state} {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			// Create one component at a time, never following or repairing an
			// unsafe existing parent. Revalidate concurrent creation too.
			if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create Next Antigravity ACP directory: %w", err)
			}
			info, err = os.Lstat(dir)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !nativeStateOwnedByCurrentUser(info) {
			return fmt.Errorf("unsafe Next Antigravity ACP directory: %s", dir)
		}
	}
	for key, value := range expected {
		env[key] = value
	}
	return nil
}

var codexACPBlockedMCPServers = []string{"chrome-devtools", "node_repl", "computer-use"}

// A strict Next-only ACP profile MUST use the already provisioned isolated
// home, WITHOUT copying global config.toml/auth.json or mutating native MCPs.
// The native TS adapter performs an additional OS owner/realpath check.
func containsNativeMCPConfig(value any, depth int) bool {
	if depth > 12 {
		return true
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for key, v := range obj {
		if key == "mcp_servers" || containsNativeMCPConfig(v, depth+1) {
			return true
		}
	}
	return false
}

// Mirrored in codex-acp/src/next/NativeMcpIsolation.ts. Only known native
// cache/state entries are allowed, inside the private Next-owned CODEX_HOME;
// executable scripts, symlinks, and config.toml remain prohibited.
var nextCodexNativeSQLite = regexp.MustCompile(`^(state|goals|logs|memories|queue)_[1-9][0-9]*\.sqlite(-wal|-shm)?$`)

func allowedNextCodexNativeEntry(home string, entry os.DirEntry) bool {
	name := entry.Name()
	info, err := os.Lstat(filepath.Join(home, name))
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	if !nativeStateOwnedByCurrentUser(info) {
		return false
	}
	switch name {
	case "auth.json":
		return info.Mode().IsRegular() && nativeStateSingleLink(info) && info.Mode().Perm()&0077 == 0
	case ".tmp", "tmp", "log", "skills", "sessions", "archived_sessions", "shell_snapshots":
		return info.IsDir()
	case "installation_id", "history.jsonl", "version.json", "models_cache.json":
		return info.Mode().IsRegular() && nativeStateSingleLink(info)
	default:
		return info.Mode().IsRegular() && nativeStateSingleLink(info) && nextCodexNativeSQLite.MatchString(name)
	}
}

func prepareCodexACPHome(agentDockHome string, profile config.ACPProfile, env map[string]string) (map[string]string, error) {
	if env["AGENTDOCK_NEXT_CODEX_MCP_ISOLATION"] == "source-only" {
		home, err := os.UserHomeDir()
		if err != nil || profile.ID != "codex" || profile.Kind != "codex" || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || os.Getenv(nextCodexMCPRegistrationEnv) != "source-only" || agentDockHome != filepath.Join(home, ".agentdock-next") || env["HOME"] != home || env["CODEX_HOME"] != filepath.Join(agentDockHome, "codex-mcp-isolated") {
			return nil, errNextCodexRegistryUnavailable
		}
		for _, dir := range []string{agentDockHome, env["CODEX_HOME"]} {
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
				return nil, errNextCodexRegistryUnavailable
			}
		}
		entries, err := os.ReadDir(env["CODEX_HOME"])
		if err != nil {
			return nil, errNextCodexRegistryUnavailable
		}
		for _, entry := range entries {
			if !allowedNextCodexNativeEntry(env["CODEX_HOME"], entry) {
				return nil, errNextCodexRegistryUnavailable
			}
		}
		return env, nil
	}
	codexPath, err := resolveCodexCLI(env)
	if err != nil {
		return nil, err
	}
	sourceHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if sourceHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve Codex source home: %w", err)
		}
		sourceHome = filepath.Join(home, ".codex")
	}
	if !filepath.IsAbs(sourceHome) {
		return nil, fmt.Errorf("Codex source home must be absolute: %s", sourceHome)
	}
	sandbox := filepath.Join(agentDockHome, "acp", profile.ID, "codex-home")
	if err := os.MkdirAll(sandbox, 0o700); err != nil {
		return nil, fmt.Errorf("create Codex ACP home: %w", err)
	}
	configSource := filepath.Join(sourceHome, "config.toml")
	configTarget := filepath.Join(sandbox, "config.toml")
	if _, err := os.Stat(configSource); err == nil {
		if err := copyACPFile(configSource, configTarget, 0o600, true); err != nil {
			return nil, fmt.Errorf("copy Codex config into ACP home: %w", err)
		}
	} else if os.IsNotExist(err) {
		if err := os.WriteFile(configTarget, nil, 0o600); err != nil {
			return nil, fmt.Errorf("create empty Codex ACP config: %w", err)
		}
	} else {
		return nil, fmt.Errorf("stat Codex config: %w", err)
	}
	for _, name := range []string{"auth.json", "AGENTS.md", "RTK.md", "hooks.json"} {
		src := filepath.Join(sourceHome, name)
		if _, err := os.Stat(src); err == nil {
			mode := os.FileMode(0o600)
			if strings.HasSuffix(name, ".md") {
				mode = 0o644
			}
			if err := copyACPFile(src, filepath.Join(sandbox, name), mode, false); err != nil {
				return nil, fmt.Errorf("sync Codex ACP %s: %w", name, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat Codex %s: %w", name, err)
		}
	}
	for _, server := range codexACPBlockedMCPServers {
		if err := runCodexMCPRemove(codexPath, sandbox, server); err != nil {
			return nil, err
		}
	}
	env["CODEX_HOME"] = sandbox
	return env, nil
}

func resolveCodexCLI(env map[string]string) (string, error) {
	if explicit, ok := env["CODEX_PATH"]; ok {
		return commandpath.ValidateExecutable(explicit)
	}
	resolved, err := commandpath.LookPath("codex")
	if err != nil {
		return "", fmt.Errorf("resolve codex CLI for ACP browser isolation: %w", err)
	}
	return resolved, nil
}

var runCodexMCPRemove = func(codexPath, home, server string) error {
	cmd := exec.Command(codexPath, "mcp", "remove", server) //nolint:gosec // executable comes from configured Codex path/PATH.
	cmd.Env = codexIsolationCommandEnv(home)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remove %s from Codex ACP MCP config: %w", server, err)
	}
	return nil
}

func codexIsolationCommandEnv(home string) []string {
	result := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CODEX_HOME=") || strings.HasPrefix(entry, "CODEX_CONFIG=") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "CODEX_HOME="+home)
}

func copyACPFile(src, dst string, mode os.FileMode, always bool) error {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	if !always {
		if targetInfo, err := os.Stat(dst); err == nil && !sourceInfo.ModTime().After(targetInfo.ModTime()) {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (r *Runtime) ACPBrowserMCPHandler() http.Handler {
	if r == nil || r.acpBrowser == nil {
		return nil
	}
	return r.acpBrowser.MCPHTTPHandler()
}

func (r *Runtime) ACPComputerMCPHandler() http.Handler {
	if r == nil || r.acpComputer == nil {
		return nil
	}
	return r.acpComputer.MCPHTTPHandler()
}
