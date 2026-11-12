package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/mcpowneraudit"
)

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		code  int
		calls int
	}{
		{nil, 2, 0}, {[]string{"--root-pid", "0"}, 2, 0}, {[]string{"--root-pid", "2147483648"}, 2, 0},
		{[]string{"--kill=CANARY"}, 2, 0}, {[]string{"--root-pid", "CANARY"}, 2, 0},
		{[]string{"--root-pid", "10", "CANARY"}, 2, 0}, {[]string{"--root-pid", "10", "--max-nodes", "1025"}, 2, 0},
		{[]string{"--root-pid", "10", "--max-depth", "33"}, 2, 0}, {[]string{"--root-pid", "10", "--timeout", "11s"}, 2, 0},
		{[]string{"--help"}, 0, 0}, {[]string{"--root-pid", "10"}, 0, 1},
	} {
		var out bytes.Buffer
		calls := 0
		code := run(tc.args, &out, func(_ context.Context, o mcpowneraudit.Options) mcpowneraudit.Report {
			calls++
			if o.RootPID != 10 {
				t.Fatal(o)
			}
			r := mcpowneraudit.Failure(o, "")
			r.Status = "observed"
			return r
		})
		if code != tc.code || calls != tc.calls || !json.Valid(out.Bytes()) || strings.Contains(out.String(), "CANARY") {
			t.Fatalf("args %v code %d calls %d: %s", tc.args, code, calls, out.String())
		}
	}
	var out bytes.Buffer
	if code := run([]string{"--root-pid", "10"}, &out, func(_ context.Context, o mcpowneraudit.Options) mcpowneraudit.Report {
		return mcpowneraudit.Failure(o, "sampling_failed")
	}); code != 1 {
		t.Fatal(code)
	}
}

func TestActualCLIInvalidDoesNotSample(t *testing.T) {
	var out bytes.Buffer
	if run(nil, &out, mcpowneraudit.Observe) != 2 {
		t.Fatal(out.String())
	}
}
