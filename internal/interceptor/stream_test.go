package interceptor

import (
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

func TestStreamPassesThroughWhenNoReasoningMatchAppears(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	decision, err := InspectStreamChunk(cfg, "/responses", []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":777}}}\n\n"), nil, 2, false)
	if err != nil {
		t.Fatalf("InspectStreamChunk() error = %v", err)
	}
	if decision.Matched {
		t.Fatal("Matched = true, want false")
	}
	if decision.Retry {
		t.Fatal("Retry = true, want false")
	}
	if decision.DropChunk {
		t.Fatal("DropChunk = true, want false")
	}
}

func TestStreamRequestsRetryBeforeAnyChunkEscapes(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	decision, err := InspectStreamChunk(cfg, "/responses", []byte("data: {\"usage\":{\"completion_tokens_details\":{\"reasoning_tokens\":516}}}\n\n"), nil, 1, false)
	if err != nil {
		t.Fatalf("InspectStreamChunk() error = %v", err)
	}
	if !decision.Matched {
		t.Fatal("Matched = false, want true")
	}
	if !decision.Retry {
		t.Fatal("Retry = false, want true")
	}
	if decision.Reasoning != 516 {
		t.Fatalf("Reasoning = %d, want 516", decision.Reasoning)
	}
}

func TestStreamReturnsReplacementChunkWhenStrict502BlocksMatchedChunk(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	decision, err := InspectStreamChunk(cfg, "/responses", []byte("data: {\"response\":{\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":516}}}}\n\n"), nil, 0, false)
	if err != nil {
		t.Fatalf("InspectStreamChunk() error = %v", err)
	}
	if !decision.Matched {
		t.Fatal("Matched = false, want true")
	}
	if len(decision.ReplacementChunk) == 0 {
		t.Fatal("ReplacementChunk is empty, want blocked payload")
	}
	if !decision.CloseStream {
		t.Fatal("CloseStream = false, want true")
	}
}

func TestStreamDropsOnlyCurrentChunkWhenConfiguredDisconnectModeIsUsed(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.StreamAction = "disconnect"
	decision, err := InspectStreamChunk(cfg, "/responses", []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":516}}}\n\n"), [][]byte{[]byte("first")}, 0, true)
	if err != nil {
		t.Fatalf("InspectStreamChunk() error = %v", err)
	}
	if !decision.DropChunk {
		t.Fatal("DropChunk = false, want true")
	}
	if decision.Retry {
		t.Fatal("Retry = true, want false")
	}
}
