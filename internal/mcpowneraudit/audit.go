// Package mcpowneraudit observes ancestry, never ownership or process control.
package mcpowneraudit

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

const MaxTableRows = 16384

type Options struct {
	RootPID  int
	MaxNodes int
	MaxDepth int
	Timeout  time.Duration
}

func (o Options) Valid() bool {
	return o.RootPID > 0 && int64(o.RootPID) <= math.MaxInt32 && o.MaxNodes >= 1 && o.MaxNodes <= 1024 && o.MaxDepth >= 1 && o.MaxDepth <= 32 && o.Timeout > 0 && o.Timeout <= 10*time.Second
}

type Node struct {
	PID              int    `json:"pid"`
	PPID             int    `json:"ppid"`
	UID              uint32 `json:"uid"`
	PGID             int    `json:"pgid"`
	BirthSec         int64  `json:"birth_sec"`
	BirthUsec        int64  `json:"birth_usec"`
	ExecutableFamily string `json:"executable_basename_family"`
	MCPFamily        string `json:"mcp_family"`
	Ancestry         string `json:"ancestry"`
	Owner            string `json:"owner"`
}

type Report struct {
	SchemaVersion int            `json:"schema_version"`
	Status        string         `json:"status"`
	Error         string         `json:"error,omitempty"`
	RootPID       int            `json:"root_pid"`
	Owner         string         `json:"owner"`
	Nodes         []Node         `json:"nodes"`
	Counts        map[string]int `json:"known_mcp_family_counts"`
	Limits        []string       `json:"limits"`
	MaxNodes      int            `json:"max_nodes"`
	MaxDepth      int            `json:"max_depth"`
	MaxTableRows  int            `json:"max_table_rows"`
	TimeoutMS     int64          `json:"timeout_ms"`
}

func Failure(o Options, code string) Report {
	return Report{SchemaVersion: 1, Status: "unavailable", Error: code, RootPID: o.RootPID, Owner: "owner_unverified", Nodes: []Node{}, Counts: map[string]int{}, MaxNodes: o.MaxNodes, MaxDepth: o.MaxDepth, MaxTableRows: MaxTableRows, TimeoutMS: o.Timeout.Milliseconds(), Limits: []string{
		"non_atomic_metadata_observation_only", "no_session_native_thread_or_transport_mapping",
		"no_managed_next_or_external_owner_attestation", "missing_or_reparented_descendants_not_attributable",
		"kernel_basename_truncated_interpreter_and_launcher_mcp_families_unknown",
		"family_labels_are_name_hints_not_protocol_identity", "no_orphan_health_or_cleanup_inference",
		"fixed_capacity_global_metadata_table", "deadline_does_not_cancel_inflight_kernel_call",
	}}
}

type row struct {
	id   process.BirthIdentity
	comm string
}

type source struct {
	table func(context.Context) ([]row, error)
	birth func(context.Context, int) (process.BirthIdentity, error)
}

// Observe requires an explicit root. At most one observation worker is used;
// timeout returns no evidence and the worker stops at its next context check.
func Observe(ctx context.Context, o Options) Report {
	if !o.Valid() || ctx == nil {
		return Failure(o, "invalid_options")
	}
	if !available {
		return Failure(o, "platform_unavailable")
	}
	return observe(ctx, o, source{table: readTable, birth: process.ObserveBirthIdentity})
}

func observe(ctx context.Context, o Options, s source) Report {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	ch := make(chan Report, 1)
	go func() { ch <- collect(ctx, o, s) }()
	select {
	case <-ctx.Done():
		return Failure(o, "deadline_or_cancellation")
	case r := <-ch:
		if ctx.Err() != nil {
			return Failure(o, "deadline_or_cancellation")
		}
		return r
	}
}

func validIdentity(id process.BirthIdentity) bool {
	return id.PID > 0 && id.ParentPID >= 0 && id.StartSec > 0 && id.StartUsec >= 0 && id.StartUsec < 1000000 && id.ProcessGroupID > 0
}

func collect(ctx context.Context, o Options, s source) Report {
	fail := func() Report { return Failure(o, "sampling_failed_or_identity_changed") }
	root, err := s.birth(ctx, o.RootPID)
	if err != nil || !validIdentity(root) || root.PID != o.RootPID {
		return fail()
	}
	rows, err := s.table(ctx)
	if err != nil || ctx.Err() != nil {
		return fail()
	}
	if len(rows) > MaxTableRows {
		return Failure(o, "table_limit_exceeded")
	}
	byPID := make(map[int]row, len(rows))
	children := make(map[int][]int)
	for _, r := range rows {
		if !validIdentity(r.id) {
			return fail()
		}
		if _, ok := byPID[r.id.PID]; ok {
			return fail()
		}
		byPID[r.id.PID] = r
		children[r.id.ParentPID] = append(children[r.id.ParentPID], r.id.PID)
	}
	if r, ok := byPID[o.RootPID]; !ok || r.id != root {
		return fail()
	}
	for pid := range children {
		sort.Ints(children[pid])
	}
	type item struct{ pid, depth int }
	queue := []item{{o.RootPID, 0}}
	seen := map[int]bool{o.RootPID: true}
	selected := []row{}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if len(selected) >= o.MaxNodes {
			return Failure(o, "node_limit_exceeded")
		}
		r := byPID[it.pid]
		selected = append(selected, r)
		for _, pid := range children[it.pid] {
			if seen[pid] {
				return fail()
			}
			if it.depth >= o.MaxDepth {
				return Failure(o, "depth_limit_exceeded")
			}
			if len(seen) >= o.MaxNodes {
				return Failure(o, "node_limit_exceeded")
			}
			seen[pid] = true
			queue = append(queue, item{pid, it.depth + 1})
		}
	}
	// Recheck every selected identity/relation, including root last. No partial
	// counts escape when an exit, PID reuse, reparenting or credential change occurs.
	for i := len(selected) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return fail()
		}
		id, err := s.birth(ctx, selected[i].id.PID)
		if err != nil || id != selected[i].id {
			return fail()
		}
	}
	report := Failure(o, "")
	report.Status = "observed"
	report.Counts = map[string]int{"codebase_memory": 0, "chrome_devtools": 0, "search_console": 0, "memory": 0, "github": 0, "gitlab": 0}
	for _, r := range selected {
		executable, family := classify(r.comm)
		ancestry := "observed_pid_ppid"
		if r.id.PID == o.RootPID {
			ancestry = "explicit_root"
		} else if family != "unknown" {
			report.Counts[family]++
		}
		report.Nodes = append(report.Nodes, Node{r.id.PID, r.id.ParentPID, r.id.EffectiveUID, r.id.ProcessGroupID, r.id.StartSec, r.id.StartUsec, executable, family, ancestry, "owner_unverified"})
	}
	return report
}

// Only fixed labels leave this package; arbitrary kernel names are never output.
func classify(comm string) (string, string) {
	switch comm {
	case "codebase-memory", "codebase-memory-":
		return "codebase_memory_name_hint", "codebase_memory"
	case "chrome-devtools-m":
		return "chrome_devtools_name_hint", "chrome_devtools"
	case "mcp-search-conso":
		return "search_console_name_hint", "search_console"
	case "mcp-memory-servi":
		return "memory_name_hint", "memory"
	case "github-mcp-serve":
		return "github_name_hint", "github"
	case "gitlab-mcp", "mcp-gitlab":
		return "gitlab_name_hint", "gitlab"
	case "node", "nodejs":
		return "node", "unknown"
	case "python", "python3", "Python":
		return "python", "unknown"
	case "npm", "npx":
		return "npm_launcher", "unknown"
	case "codex":
		return "codex", "unknown"
	case "codex-acp":
		return "codex_adapter", "unknown"
	default:
		return "unknown", "unknown"
	}
}

var errUnavailable = errors.New("metadata unavailable")
