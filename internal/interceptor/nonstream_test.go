package interceptor

import (
	"encoding/json"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

func TestNonStreamPassesWhenReasoningTokensDoNotMatch(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	body := []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":777}}}`)
	decision, err := InspectNonStream(cfg, "/v1/responses", body, 3)
	if err != nil {
		t.Fatalf("InspectNonStream() error = %v", err)
	}
	if decision.Matched {
		t.Fatal("Matched = true, want false")
	}
	if decision.Retry {
		t.Fatal("Retry = true, want false")
	}
	if decision.StatusCode != 0 {
		t.Fatalf("StatusCode = %d, want 0", decision.StatusCode)
	}
}

func TestNonStreamFormulaModeMatches518NMinus2ReasoningTokens(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.ReasoningMatchMode = "formula_518n_minus_2"
	body := []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":2070}}}`)
	decision, err := InspectNonStream(cfg, "/v1/responses", body, 1)
	if err != nil {
		t.Fatalf("InspectNonStream() error = %v", err)
	}
	if !decision.Matched {
		t.Fatal("Matched = false, want true for 2070 in formula mode")
	}
	if decision.Reasoning != 2070 {
		t.Fatalf("Reasoning = %d, want 2070", decision.Reasoning)
	}
}

func TestNonStreamRequestsRetryWhenReasoningTokensMatchAndBudgetRemains(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	body := []byte(`{"usage":{"completion_tokens_details":{"reasoning_tokens":516}}}`)
	decision, err := InspectNonStream(cfg, "/v1/responses", body, 2)
	if err != nil {
		t.Fatalf("InspectNonStream() error = %v", err)
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

func TestNonStreamReturnsGatewayPayloadWhenReasoningTokensMatchWithoutRetryBudget(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	body := []byte(`{"response":{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}}`)
	decision, err := InspectNonStream(cfg, "/v1/responses", body, 0)
	if err != nil {
		t.Fatalf("InspectNonStream() error = %v", err)
	}
	if decision.StatusCode != 502 {
		t.Fatalf("StatusCode = %d, want 502", decision.StatusCode)
	}
	var payload struct {
		Error struct {
			Type            string `json:"type"`
			Code            string `json:"code"`
			ReasoningTokens int    `json:"reasoning_tokens"`
			StatusCode      int    `json:"status_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(decision.ResponseBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Type != "codex_retry_gateway" {
		t.Fatalf("error.type = %q, want codex_retry_gateway", payload.Error.Type)
	}
	if payload.Error.Code != "reasoning_guard_triggered" {
		t.Fatalf("error.code = %q, want reasoning_guard_triggered", payload.Error.Code)
	}
	if payload.Error.ReasoningTokens != 516 {
		t.Fatalf("error.reasoning_tokens = %d, want 516", payload.Error.ReasoningTokens)
	}
	if payload.Error.StatusCode != 502 {
		t.Fatalf("error.status_code = %d, want 502", payload.Error.StatusCode)
	}
}
