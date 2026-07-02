package bridge

import (
	"encoding/json"
	"net/http"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestNonStreamSkipsModelOutsideAllowList(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.Models = []string{"gpt-5.5"}
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.ResponseInterceptRequest{
		SourceFormat: "openai",
		Model:        "gpt-4.1",
		Body:         []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`),
		RequestBody:  []byte(`{"model":"gpt-4.1"}`),
		Metadata:     map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptAfter, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Body    []byte              `json:"Body"`
			Headers map[string][]string `json:"Headers"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("env.OK = false: %#v", env)
	}
	if len(env.Result.Body) != 0 || len(env.Result.Headers) != 0 {
		t.Fatalf("result = %#v, want empty passthrough response", env.Result)
	}
	snap := state.Runtime.Metrics().Snapshot()
	if snap.InspectedResponseCount != 0 || snap.MatchedResponseCount != 0 || snap.BlockedResponseCount != 0 {
		t.Fatalf("metrics snapshot = %#v, want no response inspection", snap)
	}
}

func TestNonStreamRetryUsesHostModelExecute(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	var called string
	var reqBody pluginapi.HostModelExecutionRequest
	state.CallHost = func(method string, payload any) (json.RawMessage, error) {
		if method == pluginabi.MethodHostLog {
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		}
		called = method
		req, ok := payload.(pluginapi.HostModelExecutionRequest)
		if !ok {
			t.Fatalf("payload type = %T, want pluginapi.HostModelExecutionRequest", payload)
		}
		reqBody = req
		resp, _ := json.Marshal(pluginapi.HostModelExecutionResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)})
		return resp, nil
	}
	request, _ := json.Marshal(pluginapi.ResponseInterceptRequest{
		SourceFormat: "openai",
		Model:        "gpt-5.5",
		Stream:       false,
		Body:         []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`),
		RequestBody:  []byte(`{"model":"gpt-5.5"}`),
		Metadata:     map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptAfter, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	if called != pluginabi.MethodHostModelExecute {
		t.Fatalf("called method = %q, want %q", called, pluginabi.MethodHostModelExecute)
	}
	if reqBody.Stream {
		t.Fatal("HostModelExecutionRequest.Stream = true, want false")
	}
	if string(reqBody.Body) != `{"model":"gpt-5.5"}` {
		t.Fatalf("HostModelExecutionRequest.Body = %q, want original request body", string(reqBody.Body))
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Body []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if string(env.Result.Body) != `{"ok":true}` {
		t.Fatalf("result body = %q, want retried upstream body", string(env.Result.Body))
	}
}

func TestNonStreamRetryLoopsUntilResponseStopsMatching(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	calls := 0
	state.CallHost = func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostLog:
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		case pluginabi.MethodHostModelExecute:
			calls++
			if calls == 1 {
				raw, _ := json.Marshal(pluginapi.HostModelExecutionResponse{StatusCode: 200, Body: []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`)})
				return raw, nil
			}
			raw, _ := json.Marshal(pluginapi.HostModelExecutionResponse{StatusCode: 200, Body: []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":128}},"ok":true}`)})
			return raw, nil
		default:
			raw, _ := json.Marshal(map[string]any{})
			return raw, nil
		}
	}
	request, _ := json.Marshal(pluginapi.ResponseInterceptRequest{
		SourceFormat: "openai",
		Model:        "gpt-5.5",
		Body:         []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`),
		RequestBody:  []byte(`{"model":"gpt-5.5"}`),
		Metadata:     map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptAfter, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("host model execute calls = %d, want 2", calls)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Body []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if string(env.Result.Body) != `{"usage":{"output_tokens_details":{"reasoning_tokens":128}},"ok":true}` {
		t.Fatalf("result body = %q", string(env.Result.Body))
	}
}

func TestNonStreamBlockedResponseAddsGatewayReasonHeaderAndLog(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.GuardRetryAttempts = 0
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	hostLogs := make([]hostLogRequest, 0, 1)
	state.CallHost = func(method string, payload any) (json.RawMessage, error) {
		if method == pluginabi.MethodHostLog {
			req, ok := payload.(hostLogRequest)
			if !ok {
				t.Fatalf("host log payload type = %T", payload)
			}
			hostLogs = append(hostLogs, req)
		}
		raw, _ := json.Marshal(map[string]any{})
		return raw, nil
	}
	request, _ := json.Marshal(pluginapi.ResponseInterceptRequest{
		SourceFormat:   "openai",
		Model:          "gpt-5.5",
		Stream:         false,
		RequestHeaders: http.Header{"User-Agent": []string{"CodexDesktop/1.0"}},
		Body:           []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`),
		RequestBody:    []byte(`{"model":"gpt-5.5"}`),
		Metadata:       map[string]any{cliproxyexecutor.RequestPathMetadataKey: "/responses"},
	})
	raw, err := HandleMethod(state, pluginabi.MethodResponseInterceptAfter, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Headers map[string][]string `json:"Headers"`
			Body    []byte              `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := env.Result.Headers["X-Codex-Retry-Gateway-Reason"]; len(got) != 1 || got[0] != "reasoning-guard-triggered" {
		t.Fatalf("gateway reason header = %#v, want reasoning-guard-triggered", got)
	}
	if len(hostLogs) != 1 {
		t.Fatalf("host log count = %d, want 1", len(hostLogs))
	}
	want := "[match] non-stream path=/responses reasoning_tokens=516 action=return_status_502"
	if hostLogs[0].Message != want {
		t.Fatalf("host log message = %q, want %q", hostLogs[0].Message, want)
	}
	snap := state.Runtime.Metrics().Snapshot()
	if snap.TotalProxyRequestCount != 1 || snap.InspectedResponseCount != 1 || snap.MatchedResponseCount != 1 || snap.BlockedResponseCount != 1 {
		t.Fatalf("metrics snapshot = %#v", snap)
	}
}
