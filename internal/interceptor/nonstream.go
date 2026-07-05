package interceptor

import (
	"encoding/json"
	"fmt"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

type NonStreamDecision struct {
	Matched        bool
	Retry          bool
	StatusCode     int
	ResponseBody   []byte
	Reasoning      int
	ReasoningFound bool
}

type nonStreamEnvelope struct {
	Usage struct {
		OutputTokensDetails struct {
			ReasoningTokens *int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens *int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Response struct {
		Usage struct {
			OutputTokensDetails struct {
				ReasoningTokens *int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
			CompletionTokensDetails struct {
				ReasoningTokens *int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
}

func InspectNonStream(cfg pluginconfig.Config, path string, body []byte, retryRemaining int) (NonStreamDecision, error) {
	var payload nonStreamEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return NonStreamDecision{}, err
	}
	reasoning, ok := extractNonStreamReasoning(payload)
	if !ok {
		return NonStreamDecision{}, nil
	}
	matched := pluginconfig.ReasoningMatched(cfg, reasoning)
	if !matched {
		return NonStreamDecision{Reasoning: reasoning, ReasoningFound: true}, nil
	}
	decision := NonStreamDecision{Matched: true, Reasoning: reasoning, ReasoningFound: true}
	if cfg.InterceptNonStreaming && retryRemaining > 0 {
		decision.Retry = true
		return decision, nil
	}
	decision.StatusCode = cfg.NonStreamStatusCode
	decision.ResponseBody = buildBlockedBody(path, reasoning, cfg.NonStreamStatusCode)
	return decision, nil
}

func extractNonStreamReasoning(payload nonStreamEnvelope) (int, bool) {
	pointers := []*int{
		payload.Usage.OutputTokensDetails.ReasoningTokens,
		payload.Usage.CompletionTokensDetails.ReasoningTokens,
		payload.Response.Usage.OutputTokensDetails.ReasoningTokens,
		payload.Response.Usage.CompletionTokensDetails.ReasoningTokens,
	}
	for _, ptr := range pointers {
		if ptr != nil {
			return *ptr, true
		}
	}
	return 0, false
}

func buildBlockedBody(path string, reasoning int, statusCode int) []byte {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message":          fmt.Sprintf("codex retry gateway blocked suspicious reasoning response on %s", path),
			"type":             "codex_retry_gateway",
			"code":             "reasoning_guard_triggered",
			"reasoning_tokens": reasoning,
			"status_code":      statusCode,
		},
	})
	return body
}
