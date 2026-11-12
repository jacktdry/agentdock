package process

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func rss(n uint64) *uint64 { return &n }

func TestAggregateTree(t *testing.T) {
	rows := []observedProcess{{pid: 3, parent: 2, rss: rss(30)}, {pid: 1, parent: 3, rss: rss(10)}, {pid: 2, parent: 1, rss: rss(20)}, {pid: 4, parent: 0, rss: rss(999)}}
	s := aggregate(1, rows)
	if s.State != "alive" || s.Alive == nil || !*s.Alive || *s.DescendantCount != 2 || *s.RSSBytes != 10 || *s.TreeRSSBytes != 60 || s.Error != "" {
		t.Fatalf("snapshot = %+v", s)
	}
	rows[0].rss = nil
	s = aggregate(1, rows)
	if s.TreeRSSBytes != nil || s.RSSBytes == nil || s.Error == "" || *s.DescendantCount != 2 {
		t.Fatalf("partial = %+v", s)
	}
	rows[0].rss = rss(math.MaxUint64)
	if s := aggregate(1, rows); s.TreeRSSBytes != nil || s.Error == "" {
		t.Fatalf("overflow = %+v", s)
	}
	rows = append(rows, rows[0])
	if s := aggregate(1, rows); s.State != "unknown" || s.Alive != nil || s.Error == "" {
		t.Fatalf("duplicate = %+v", s)
	}
}

func TestAggregateStates(t *testing.T) {
	s := aggregate(9, nil)
	if s.State != "not_found" || s.Alive == nil || *s.Alive || s.RSSBytes != nil || s.DescendantCount != nil {
		t.Fatalf("missing = %+v", s)
	}
	s = aggregate(9, []observedProcess{{pid: 9, zombie: true, rss: rss(0)}})
	if s.State != "zombie" || *s.Alive || *s.DescendantCount != 0 || *s.TreeRSSBytes != 0 {
		t.Fatalf("zombie = %+v", s)
	}
	s = aggregate(9, []observedProcess{{pid: 9}})
	if s.RSSBytes != nil || s.TreeRSSBytes != nil || s.Error == "" {
		t.Fatalf("unsupported = %+v", s)
	}
}

func TestParsePS(t *testing.T) {
	rows, err := parsePS("  10 1 S+ 123\n11 10 Z 0\n\n")
	if err != nil || len(rows) != 2 || *rows[0].rss != 123*1024 || !rows[1].zombie {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	for _, input := range []string{"header", "1 0 S", "0 0 S 1", "x 0 S 1", "1 -1 S 1", "1 0 S -1", "1 0 S 18446744073709551615"} {
		if _, err := parsePS(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestParseProcStat(t *testing.T) {
	fields := make([]string, 22)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0], fields[1], fields[21] = "S", "7", "3"
	input := "12 (name ) with (spaces\nand newline)) " + strings.Join(fields, " ")
	row, err := parseProcStat(input, 4096)
	if err != nil || row.pid != 12 || row.parent != 7 || *row.rss != 12288 {
		t.Fatalf("row=%+v err=%v", row, err)
	}
	for _, invalid := range []string{"12 name S 0", "x (name) S 0", "12 (name) S 0", strings.Replace(input, " S ", " XX ", 1)} {
		if _, err := parseProcStat(invalid, 4096); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	for _, value := range []string{"-1", "18446744073709551615"} {
		fields[21] = value
		if _, err := parseProcStat("12 (name) "+strings.Join(fields, " "), 4096); err == nil {
			t.Fatalf("accepted RSS %s", value)
		}
	}
}

func TestObserveContract(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx context.Context
		pid int
	}{{ctx, os.Getpid()}, {context.Background(), 0}, {context.Background(), -1}, {nil, 1}} {
		s := Observe(tc.ctx, tc.pid)
		if s.RootPID != tc.pid || s.State != "unknown" || s.Error == "" || s.Alive != nil || s.RSSBytes != nil || s.TreeRSSBytes != nil || s.DescendantCount != nil || s.ObservedAt.IsZero() {
			t.Fatalf("invalid observation = %+v", s)
		}
		data, err := json.Marshal(s)
		if err != nil || !strings.Contains(string(data), `"rss_bytes":null`) || !strings.Contains(string(data), `"observed_at":`) {
			t.Fatalf("JSON = %s, %v", data, err)
		}
	}
}

func TestObserveSelf(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	before := time.Now()
	s := Observe(ctx, os.Getpid())
	if s.State == "unknown" && (strings.Contains(strings.ToLower(s.Error), "operation not permitted") || strings.Contains(strings.ToLower(s.Error), "permission denied")) {
		t.Skipf("process observation unavailable in this sandbox: %s", s.Error)
	}
	if s.RootPID != os.Getpid() || s.State != "alive" || s.Alive == nil || !*s.Alive || s.DescendantCount == nil || s.ObservedAt.Before(before) || s.ObservedAt.After(time.Now()) {
		t.Fatalf("self = %+v", s)
	}
	if runtime.GOOS == "windows" {
		if s.RSSBytes != nil || s.TreeRSSBytes != nil || s.Error == "" {
			t.Fatalf("Windows RSS must be unavailable: %+v", s)
		}
	} else if s.Error != "" || s.RSSBytes == nil || *s.RSSBytes == 0 || s.TreeRSSBytes == nil || *s.TreeRSSBytes < *s.RSSBytes {
		t.Fatalf("self RSS = %+v (PID %s)", s, strconv.Itoa(os.Getpid()))
	}
}
