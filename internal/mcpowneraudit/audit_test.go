package mcpowneraudit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

func fixtureRow(pid, ppid int, comm string) row {
	return row{process.BirthIdentity{PID: pid, ParentPID: ppid, StartSec: int64(100 + pid), StartUsec: 123, EffectiveUID: 501, ProcessGroupID: 10}, comm}
}

func fixtureSource(rows []row) source {
	return source{table: func(context.Context) ([]row, error) { return rows, nil }, birth: func(_ context.Context, pid int) (process.BirthIdentity, error) {
		for _, r := range rows {
			if r.id.PID == pid {
				return r.id, nil
			}
		}
		return process.BirthIdentity{}, errors.New("secret/path/argv")
	}}
}

func fixtureOptions() Options {
	return Options{RootPID: 10, MaxNodes: 16, MaxDepth: 4, Timeout: time.Second}
}

func TestClassificationPrivacyAndObservedAncestry(t *testing.T) {
	rows := []row{fixtureRow(10, 1, "codex"), fixtureRow(12, 11, "chrome-devtools-m"), fixtureRow(11, 10, "npm"), fixtureRow(13, 10, "python3"), fixtureRow(14, 999, "mcp-search-conso"), fixtureRow(15, 1, "codebase-memory"), fixtureRow(16, 10, "/private/token=CANARY argv secret")}
	r := observe(context.Background(), fixtureOptions(), fixtureSource(rows))
	if r.Status != "observed" || len(r.Nodes) != 5 || r.Counts["chrome_devtools"] != 1 || r.Counts["search_console"] != 0 || r.Counts["codebase_memory"] != 0 {
		t.Fatalf("unexpected attribution: %+v", r)
	}
	for _, n := range r.Nodes {
		if n.Owner != "owner_unverified" {
			t.Fatal("ancestry granted ownership")
		}
	}
	b, _ := json.Marshal(r)
	for _, secret := range []string{"CANARY", "/private", "argv secret", "native_thread_id", "session_id"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	for comm, family := range map[string]string{"codebase-memory": "codebase_memory", "chrome-devtools-m": "chrome_devtools", "mcp-search-conso": "search_console", "mcp-memory-servi": "memory", "github-mcp-serve": "github", "gitlab-mcp": "gitlab", "node": "unknown", "npm": "unknown", "python3": "unknown", "malicious-secret": "unknown"} {
		_, got := classify(comm)
		if got != family {
			t.Fatalf("%s: %s", comm, got)
		}
	}
}

func TestFailuresDiscardAllEvidence(t *testing.T) {
	base := []row{fixtureRow(10, 1, "codex"), fixtureRow(11, 10, "node")}
	for _, name := range []string{"table_error", "missing_root", "duplicate", "invalid_birth", "root_reuse", "child_reuse", "reparent", "uid_change", "pgid_change", "birth_failure", "nodes", "depth", "table_limit", "cycle"} {
		t.Run(name, func(t *testing.T) {
			rows := append([]row(nil), base...)
			o := fixtureOptions()
			s := fixtureSource(rows)
			switch name {
			case "table_error":
				s.table = func(context.Context) ([]row, error) { return nil, errors.New("CANARY /private argv") }
			case "missing_root":
				s.table = func(context.Context) ([]row, error) { return rows[1:], nil }
			case "duplicate":
				rows = append(rows, rows[1])
				s.table = fixtureSource(rows).table
			case "invalid_birth":
				rows[1].id.StartSec = 0
			case "nodes":
				o.MaxNodes = 1
			case "depth":
				o.MaxDepth = 1
				rows = append(rows, fixtureRow(12, 11, "node"))
				s = fixtureSource(rows)
			case "table_limit":
				s.table = func(context.Context) ([]row, error) { return make([]row, MaxTableRows+1), nil }
			case "cycle":
				rows[0].id.ParentPID = 11
			default:
				birth := s.birth
				calls := 0
				s.birth = func(ctx context.Context, pid int) (process.BirthIdentity, error) {
					calls++
					id, err := birth(ctx, pid)
					if calls > 1 {
						switch name {
						case "root_reuse":
							if pid == 10 {
								id.StartUsec++
							}
						case "child_reuse":
							if pid == 11 {
								id.StartSec++
							}
						case "reparent":
							id.ParentPID = 1
						case "uid_change":
							id.EffectiveUID++
						case "pgid_change":
							id.ProcessGroupID++
						case "birth_failure":
							return id, errors.New("CANARY /private argv")
						}
					}
					return id, err
				}
			}
			r := observe(context.Background(), o, s)
			b, _ := json.Marshal(r)
			if r.Status != "unavailable" || len(r.Nodes) != 0 || len(r.Counts) != 0 || strings.Contains(string(b), "CANARY") {
				t.Fatalf("failure published evidence: %s", b)
			}
		})
	}
}

func TestCancellationAndDeadline(t *testing.T) {
	s := fixtureSource([]row{fixtureRow(10, 1, "codex")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := observe(ctx, fixtureOptions(), s); r.Status != "unavailable" {
		t.Fatal(r)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.table = func(context.Context) ([]row, error) { close(entered); <-release; close(done); return nil, nil }
	o := fixtureOptions()
	o.Timeout = 20 * time.Millisecond
	r := observe(context.Background(), o, s)
	<-entered
	close(release)
	<-done
	if r.Status != "unavailable" || len(r.Nodes) != 0 {
		t.Fatal(r)
	}
}

func TestInvalidOptionsAndPlatform(t *testing.T) {
	if r := Observe(context.Background(), Options{}); r.Error != "invalid_options" {
		t.Fatal(r)
	}
	if r := Observe(nil, fixtureOptions()); r.Error != "invalid_options" {
		t.Fatal(r)
	}
	if !available {
		if r := Observe(context.Background(), fixtureOptions()); r.Error != "platform_unavailable" || len(r.Nodes) != 0 {
			t.Fatal(r)
		}
	}
}
