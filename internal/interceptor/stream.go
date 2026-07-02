package interceptor

import (
	"bytes"
	"encoding/json"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

type StreamDecision struct {
	Matched          bool
	HeaderMutations  map[string]string
	ReplacementChunk []byte
	DropChunk        bool
	Retry            bool
	CloseStream      bool
	Reasoning        int
	ReasoningFound   bool
}

type streamEnvelope struct {
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

func InspectStreamChunk(cfg pluginconfig.Config, path string, chunk []byte, history [][]byte, retryRemaining int, wroteAnyChunk bool) (StreamDecision, error) {
	payloads := extractSSEPayloads(chunk)
	reasoning := 0
	found := false
	for _, payload := range payloads {
		var env streamEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			continue
		}
		if value, ok := extractStreamReasoning(env); ok {
			reasoning = value
			found = true
		}
	}
	if !found {
		return StreamDecision{}, nil
	}
	matched := false
	for _, value := range cfg.ReasoningEquals {
		if value == reasoning {
			matched = true
			break
		}
	}
	if !matched {
		return StreamDecision{Reasoning: reasoning, ReasoningFound: true}, nil
	}
	decision := StreamDecision{Matched: true, Reasoning: reasoning, ReasoningFound: true}
	if cfg.InterceptStreaming && retryRemaining > 0 && (cfg.StreamAction != "disconnect" || !wroteAnyChunk) {
		decision.Retry = true
		return decision, nil
	}
	if cfg.StreamAction == "disconnect" && wroteAnyChunk {
		decision.DropChunk = true
		decision.CloseStream = true
		return decision, nil
	}
	decision.ReplacementChunk = buildBlockedBody(path, reasoning, cfg.NonStreamStatusCode)
	decision.CloseStream = true
	return decision, nil
}

func extractStreamReasoning(payload streamEnvelope) (int, bool) {
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

func extractSSEPayloads(chunk []byte) [][]byte {
	lines := bytes.Split(chunk, []byte("\n"))
	payloads := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		payloads = append(payloads, payload)
	}
	return payloads
}
