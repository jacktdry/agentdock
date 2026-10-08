package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/uvwt/agentdock/internal/nexusbridge"
)

func TestNexusPairCLIRequiresExplicitReplaceAndGenerationFence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTDOCK_HOME", home)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"node":{"id":"node_%d"},"device_token":"token_%d"}`, n, n)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	args := []string{"--endpoint", server.URL, "--code", "first-code", "--name", "Next"}
	if err := runNexusPairCommand(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d", requests.Load())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runNexusPairCommand(context.Background(), []string{"--endpoint", server.URL, "--code", "second-code"}, &stdout, &stderr); err == nil {
		t.Fatal("re-pair without --replace succeeded")
	}
	if requests.Load() != 1 {
		t.Fatalf("one-time code reached network without confirmation: requests=%d", requests.Load())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runNexusPairCommand(context.Background(), []string{"--endpoint", server.URL, "--code", "second-code", "--replace"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d", requests.Load())
	}
	identity, err := nexusbridge.Load(home)
	if err != nil || identity.NodeID != "node_2" || identity.DeviceToken != "token_2" {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
}
