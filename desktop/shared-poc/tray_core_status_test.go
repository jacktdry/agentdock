package main

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitTrayLabel(t *testing.T, output <-chan string) string {
	t.Helper()
	select {
	case label := <-output:
		return label
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a tray status update")
		return ""
	}
}

func TestCoreTrayLabelHonestEvidenceAndLocalization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reading coreTrayReading
		want    string
	}{
		{"healthy", coreTrayReading{running: true, healthy: true}, "運作正常"},
		{"running but unhealthy", coreTrayReading{running: true}, "健康異常"},
		{"stopped", coreTrayReading{}, "已停止"},
		{"failed read cannot look stopped", coreTrayReading{err: true}, "無法確認"},
		{"error overrides stale healthy flags", coreTrayReading{running: true, healthy: true, err: true}, "無法確認"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			labels := labelsForNativeMenu("zh-Hant")
			got := labels.trayCoreLabel(tc.reading, time.Date(2026, 10, 14, 11, 59, 10, 0, time.Local))
			if !strings.Contains(got, tc.want) || !strings.Contains(got, "上次觀察 11:59:10") {
				t.Fatalf("misleading status: %q, want %q with timestamp", got, tc.want)
			}
		})
	}
	for _, language := range []string{"en", "zh-Hans"} {
		labels := labelsForNativeMenu(language)
		if labels.trayInitial == "" || labels.trayExpired == "" || labels.trayRefresh == "" {
			t.Errorf("missing %s tray labels", language)
		}
	}
}

func TestCoreTrayManualRefreshExpiresWithoutPolling(t *testing.T) {
	labels := labelsForNativeMenu("zh-Hant")
	updates := make(chan string, 8)
	var reads atomic.Int32
	probe := newCoreTrayProbe(labels,
		func(context.Context) coreTrayReading {
			reads.Add(1)
			return coreTrayReading{running: true, healthy: true}
		},
		func(fn func()) { go fn() },
		func(label string) { updates <- label },
	)
	probe.staleAfter = 25 * time.Millisecond
	defer probe.Close()
	if labels.trayInitial != "核心服務：尚未檢查" {
		t.Fatal("initial menu must never claim healthy")
	}
	probe.Refresh()
	first := waitTrayLabel(t, updates)
	second := waitTrayLabel(t, updates)
	if first != labels.trayChecking && second != labels.trayChecking {
		t.Fatalf("refresh did not show pending: %q, %q", first, second)
	}
	if !strings.Contains(first+second, labels.trayHealthy) {
		t.Fatalf("no trustworthy healthy observation: %q, %q", first, second)
	}
	if got := waitTrayLabel(t, updates); got != labels.trayExpired {
		t.Fatalf("stale green wasn't invalidated: %q", got)
	}
	if got := reads.Load(); got != 1 {
		t.Errorf("expected only one explicit status read, got %d", got)
	}
}

func TestCoreTrayIgnoresDuplicateRefreshAndResultsAfterClose(t *testing.T) {
	labels := labelsForNativeMenu("en")
	updates := make(chan string, 8)
	release := make(chan struct{})
	started := make(chan struct{})
	var reads atomic.Int32
	probe := newCoreTrayProbe(labels, func(ctx context.Context) coreTrayReading {
		if reads.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return coreTrayReading{running: true, healthy: true}
		case <-ctx.Done():
			return coreTrayReading{err: true}
		}
	}, func(fn func()) { go fn() }, func(label string) { updates <- label })
	probe.Refresh()
	<-started
	probe.Refresh()
	if got := reads.Load(); got != 1 {
		t.Fatalf("duplicate read started: %d", got)
	}
	probe.Close()
	close(release)
	// The scheduled checking label may have run before Close. After closing
	// there must be no healthy, stopped, or unknown observation emitted.
	time.Sleep(30 * time.Millisecond)
	close(updates)
	for label := range updates {
		if label != labels.trayChecking {
			t.Fatalf("published a result after Close: %q", label)
		}
	}
	probe.Refresh()
	if reads.Load() != 1 {
		t.Fatal("closed tray performed another read")
	}
}
