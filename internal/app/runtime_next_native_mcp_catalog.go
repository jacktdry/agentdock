package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const nextNativeMCPCatalogLimit = 64 * 1024

var errNextNativeMCPUnavailable = errors.New("Next native MCP server unavailable")
var nextNativeMCPServerID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type nextNativeMCPServer struct {
	id, executable string
	args           []string
}

func sameNextNativeMCPConfig(a, b nextCodexMCPConfig) bool {
	return a.ServerID == b.ServerID && a.ConfigDigest == b.ConfigDigest &&
		a.WrapperExecutable == b.WrapperExecutable && a.BackendExecutable == b.BackendExecutable &&
		slices.Equal(a.BackendArgs, b.BackendArgs)
}

// Version 1 deliberately has one executable namespace: Helpers/native-mcp/<id>.
// No path expansion, interpreter/script paths, environment or dynamic argv.
func parseNextNativeMCPCatalog(raw []byte) ([]nextNativeMCPServer, error) {
	value, err := nextNativeMCPStrictJSON(raw)
	root, ok := value.(map[string]any)
	if err != nil || !ok || len(root) != 2 || root["schema_version"] != json.Number("1") {
		return nil, errNextNativeMCPUnavailable
	}
	entries, ok := root["servers"].([]any)
	if !ok || len(entries) > 32 {
		return nil, errNextNativeMCPUnavailable
	}
	servers := make([]nextNativeMCPServer, 0, len(entries))
	seen := make(map[string]bool)
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok || len(entry) < 2 || len(entry) > 3 {
			return nil, errNextNativeMCPUnavailable
		}
		for key := range entry {
			if key != "id" && key != "executable" && key != "args" {
				return nil, errNextNativeMCPUnavailable
			}
		}
		id, _ := entry["id"].(string)
		executable, _ := entry["executable"].(string)
		if !nextNativeMCPServerID.MatchString(id) || seen[id] || executable != "Helpers/native-mcp/"+id {
			return nil, errNextNativeMCPUnavailable
		}
		// Even a re-signed shell/interpreter must not become a catalog backend.
		switch id {
		case "sh", "bash", "zsh", "dash", "fish", "csh", "tcsh", "env", "python", "python3", "perl", "ruby", "node", "osascript":
			return nil, errNextNativeMCPUnavailable
		}
		seen[id] = true
		server := nextNativeMCPServer{id: id, executable: executable, args: []string{}}
		if args, present := entry["args"]; present {
			values, ok := args.([]any)
			if !ok || len(values) > 64 {
				return nil, errNextNativeMCPUnavailable
			}
			for _, value := range values {
				arg, ok := value.(string)
				if !ok || len(arg) > 1024 || strings.ContainsAny(arg, "\x00\r\n") {
					return nil, errNextNativeMCPUnavailable
				}
				server.args = append(server.args, arg)
			}
		}
		servers = append(servers, server)
	}
	return servers, nil
}

// Token decoding rejects duplicate keys, trailing values and replacement of
// invalid Unicode escapes. Exact map keys avoid encoding/json's case folding.
func nextNativeMCPStrictJSON(raw []byte) (any, error) {
	if len(raw) > nextNativeMCPCatalogLimit || !utf8.Valid(raw) {
		return nil, errNextNativeMCPUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 8 {
			return nil, errNextNativeMCPUnavailable
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, errNextNativeMCPUnavailable
		}
		switch token {
		case json.Delim('{'):
			object := make(map[string]any)
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || strings.ContainsRune(name, utf8.RuneError) {
					return nil, errNextNativeMCPUnavailable
				}
				if _, duplicate := object[name]; duplicate {
					return nil, errNextNativeMCPUnavailable
				}
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				object[name] = value
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, errNextNativeMCPUnavailable
			}
			return object, nil
		case json.Delim('['):
			array := []any{}
			for decoder.More() {
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, errNextNativeMCPUnavailable
			}
			return array, nil
		}
		if value, ok := token.(string); ok && strings.ContainsRune(value, utf8.RuneError) {
			return nil, errNextNativeMCPUnavailable
		}
		return token, nil
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errNextNativeMCPUnavailable
	}
	return value, nil
}
