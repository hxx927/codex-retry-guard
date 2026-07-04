package runtime

import (
	"fmt"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

func TestStateStartsWithValidatedDefaultConfig(t *testing.T) {
	state, err := NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	cfg := state.Config()
	if !cfg.Enabled {
		t.Fatal("Config().Enabled = false, want true")
	}
	if cfg.GuardRetryAttempts != 3 {
		t.Fatalf("Config().GuardRetryAttempts = %d, want 3", cfg.GuardRetryAttempts)
	}
}

func TestReconfigureSwapsConfigAtomically(t *testing.T) {
	state, err := NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	next := state.Config()
	next.GuardRetryAttempts = 5
	next.LogMatch = false
	if err := state.Reconfigure(next); err != nil {
		t.Fatalf("Reconfigure() error = %v", err)
	}
	got := state.Config()
	if got.GuardRetryAttempts != 5 {
		t.Fatalf("Config().GuardRetryAttempts = %d, want 5", got.GuardRetryAttempts)
	}
	if got.LogMatch {
		t.Fatal("Config().LogMatch = true, want false")
	}
}

func TestMetricsSnapshotStartsEmpty(t *testing.T) {
	state, err := NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	snap := state.Metrics().Snapshot()
	if snap.TotalProxyRequestCount != 0 {
		t.Fatalf("Snapshot().TotalProxyRequestCount = %d, want 0", snap.TotalProxyRequestCount)
	}
	if snap.MatchedResponseCount != 0 {
		t.Fatalf("Snapshot().MatchedResponseCount = %d, want 0", snap.MatchedResponseCount)
	}
}

func TestCaptureRequestProfileSanitizesHeadersAndReasoning(t *testing.T) {
	state, err := NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	state.CaptureRequestProfile(map[string]string{
		"User-Agent":     " CodexDesktop/1.0 ",
		"Authorization":  "Bearer secret",
		"Content-Length": "123",
		"X-Test":         " ok ",
	}, "HIGH")
	snap := state.Metrics().Snapshot()
	if got := snap.RequestProfile.Headers["user-agent"]; got != "CodexDesktop/1.0" {
		t.Fatalf("user-agent = %q, want %q", got, "CodexDesktop/1.0")
	}
	if _, ok := snap.RequestProfile.Headers["authorization"]; ok {
		t.Fatal("authorization should be stripped from request profile")
	}
	if got := snap.RequestProfile.Headers["x-test"]; got != "ok" {
		t.Fatalf("x-test = %q, want %q", got, "ok")
	}
	if snap.RequestProfile.Reasoning == nil || snap.RequestProfile.Reasoning.Effort != "high" {
		t.Fatalf("reasoning effort = %#v, want high", snap.RequestProfile.Reasoning)
	}
}

func TestMetricsRetainsOnlyLatestLogEntries(t *testing.T) {
	metrics := NewMetrics()
	for i := 1; i <= 105; i++ {
		metrics.AppendLog("2026-07-04T00:00:00Z", fmt.Sprintf("entry-%03d", i))
	}

	logs := metrics.Snapshot().Logs
	if len(logs) != 100 {
		t.Fatalf("len(logs) = %d, want 100", len(logs))
	}
	if logs[0].Message != "entry-006" {
		t.Fatalf("first log = %q, want entry-006", logs[0].Message)
	}
	if logs[len(logs)-1].Message != "entry-105" {
		t.Fatalf("last log = %q, want entry-105", logs[len(logs)-1].Message)
	}
}
