package bridge

import (
	"encoding/json"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestStreamRetryUsesHostModelExecuteStream(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	calls := make([]string, 0, 6)
	state.CallHost = func(method string, payload any) (json.RawMessage, error) {
		calls = append(calls, method)
		switch method {
		case pluginabi.MethodHostModelExecuteStream:
			resp, _ := json.Marshal(pluginapi.HostModelStreamResponse{StatusCode: 200, StreamID: "stream-1"})
			return resp, nil
		case pluginabi.MethodHostModelStreamRead:
			resp, _ := json.Marshal(pluginapi.HostModelStreamReadResponse{Payload: []byte("data: retry\n\n"), Done: true})
			return resp, nil
		case pluginabi.MethodHostModelStreamClose:
			resp, _ := json.Marshal(map[string]any{})
			return resp, nil
		case pluginabi.MethodHostLog:
			resp, _ := json.Marshal(map[string]any{})
			return resp, nil
		default:
			resp, _ := json.Marshal(map[string]any{})
			return resp, nil
		}
	}
	request, _ := json.Marshal(pluginapi.StreamChunkInterceptRequest{
		SourceFormat: "openai",
		Model:        "gpt-5.5",
		Body:         []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":516}}}\n\n"),
		RequestBody:  []byte(`{"model":"gpt-5.5","stream":true}`),
		Metadata:     map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptStreamChunk, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	filtered := make([]string, 0, len(calls))
	for _, call := range calls {
		if call == pluginabi.MethodHostLog {
			continue
		}
		filtered = append(filtered, call)
	}
	if len(filtered) < 3 || filtered[0] != pluginabi.MethodHostModelExecuteStream || filtered[1] != pluginabi.MethodHostModelStreamRead || filtered[2] != pluginabi.MethodHostModelStreamClose {
		t.Fatalf("calls = %#v, want execute_stream then stream_read then stream_close", calls)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Body        []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if string(env.Result.Body) != "data: retry\n\n" {
		t.Fatalf("result body = %q, want retried stream payload", string(env.Result.Body))
	}
}

func TestStreamRetryLoopsUntilStreamStopsMatching(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	execCalls := 0
	readCalls := 0
	state.CallHost = func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostLog:
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		case pluginabi.MethodHostModelExecuteStream:
			execCalls++
			streamID := "stream-1"
			if execCalls == 2 {
				streamID = "stream-2"
			}
			raw, _ := json.Marshal(pluginapi.HostModelStreamResponse{StatusCode: 200, StreamID: streamID})
			return raw, nil
		case pluginabi.MethodHostModelStreamRead:
			readCalls++
			if readCalls == 1 {
				raw, _ := json.Marshal(pluginapi.HostModelStreamReadResponse{Payload: []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":516}}}\n\n"), Done: true})
				return raw, nil
			}
			raw, _ := json.Marshal(pluginapi.HostModelStreamReadResponse{Payload: []byte("data: ok\n\ndata: [DONE]\n\n"), Done: true})
			return raw, nil
		case pluginabi.MethodHostModelStreamClose:
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		default:
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		}
	}
	request, _ := json.Marshal(pluginapi.StreamChunkInterceptRequest{
		SourceFormat: "openai",
		Model:        "gpt-5.5",
		Body:         []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":516}}}\n\n"),
		RequestBody:  []byte(`{"model":"gpt-5.5","stream":true}`),
		Metadata:     map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptStreamChunk, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	if execCalls != 2 {
		t.Fatalf("execute_stream calls = %d, want 2", execCalls)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Body        []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if string(env.Result.Body) != "data: ok\n\ndata: [DONE]\n\n" {
		t.Fatalf("result body = %q", string(env.Result.Body))
	}
}

func TestStreamHeaderInitSkipsBufferingForModelOutsideAllowList(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.Models = []string{"gpt-5.5"}
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.StreamChunkInterceptRequest{ChunkIndex: pluginapi.StreamChunkHeaderInitIndex, Model: "gpt-4.1"})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptStreamChunk, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Headers map[string][]string `json:"Headers"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := env.Result.Headers["X-CPA-Buffer-Stream"]; len(got) != 0 {
		t.Fatalf("buffer header = %#v, want none", got)
	}
}

func TestStreamHeaderInitEnablesBufferedStrictMode(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.StreamChunkInterceptRequest{ChunkIndex: pluginapi.StreamChunkHeaderInitIndex})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptStreamChunk, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Headers map[string][]string `json:"Headers"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := env.Result.Headers["X-CPA-Buffer-Stream"]; len(got) != 1 || got[0] != "1" {
		t.Fatalf("buffer header = %#v, want 1", got)
	}
}
