package process

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Snapshot is a best-effort, non-atomic observation of a PID, not a process
// identity or proof of application responsiveness. It grants no control rights.
// Nil metrics mean unavailable, never a measured zero. State is alive, zombie,
// not_found, or unknown. A zombie exists but Alive is false.
// DescendantCount counts all reachable descendants, excluding the root.
// TreeRSSBytes sums root and descendant RSS; shared pages may be counted twice.
// PID reuse, reparenting, exits, and platform visibility can affect the result.
type Snapshot struct {
	RootPID         int       `json:"root_pid"`
	Alive           *bool     `json:"alive"`
	State           string    `json:"state"`
	DescendantCount *int      `json:"descendant_count"`
	RSSBytes        *uint64   `json:"rss_bytes"`
	TreeRSSBytes    *uint64   `json:"tree_rss_bytes"`
	ObservedAt      time.Time `json:"observed_at"`
	Error           string    `json:"error,omitempty"`
}

// Observe snapshots a positive root PID without attaching a Controller, sending
// signals, or acquiring termination handles. The context bounds OS collection.
// Collection failures are represented in Error with unavailable metrics.
func Observe(ctx context.Context, rootPID int) Snapshot {
	s := Snapshot{RootPID: rootPID, State: "unknown"}
	if rootPID <= 0 || uint64(rootPID) > math.MaxInt32 {
		s.Error = "invalid root PID"
	} else if ctx == nil {
		s.Error = "nil context"
	} else if err := ctx.Err(); err != nil {
		s.Error = err.Error()
	} else {
		rows, err := observeProcesses(ctx)
		if err != nil {
			s.Error = err.Error()
		} else {
			s = aggregate(rootPID, rows)
		}
	}
	s.ObservedAt = time.Now().UTC()
	return s
}

type observedProcess struct {
	pid, parent int
	zombie      bool
	rss         *uint64
}

func aggregate(root int, rows []observedProcess) Snapshot {
	s := Snapshot{RootPID: root, State: "not_found"}
	byPID := make(map[int]observedProcess, len(rows))
	children := make(map[int][]int)
	for _, row := range rows {
		if row.pid <= 0 {
			continue
		}
		if _, exists := byPID[row.pid]; exists {
			return Snapshot{RootPID: root, State: "unknown", Error: fmt.Sprintf("duplicate PID %d", row.pid)}
		}
		byPID[row.pid] = row
		children[row.parent] = append(children[row.parent], row.pid)
	}
	alive := false
	s.Alive = &alive
	r, exists := byPID[root]
	if !exists {
		return s
	}
	s.State = "alive"
	alive = !r.zombie
	if r.zombie {
		s.State = "zombie"
	}
	s.RSSBytes = r.rss
	seen := map[int]bool{root: true}
	queue := []int{root}
	count := 0
	total := uint64(0)
	available := true
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		row := byPID[pid]
		if row.rss == nil || (row.rss != nil && *row.rss > math.MaxUint64-total) {
			available = false
		} else {
			total += *row.rss
		}
		for _, child := range children[pid] {
			if seen[child] {
				continue
			}
			seen[child] = true
			count++
			queue = append(queue, child)
		}
	}
	s.DescendantCount = &count
	if available {
		s.TreeRSSBytes = &total
	} else {
		s.Error = "RSS unsupported, unavailable, or sum overflow"
	}
	return s
}
