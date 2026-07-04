package bridge

import (
	"encoding/json"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

type registration struct {
	SchemaVersion uint32 `json:"schema_version"`
	Metadata      struct {
		Name         string                  `json:"Name"`
		Version      string                  `json:"Version"`
		ConfigFields []pluginapi.ConfigField `json:"ConfigFields"`
	} `json:"metadata"`
	Capabilities struct {
		RequestInterceptor     bool `json:"request_interceptor"`
		ResponseInterceptor    bool `json:"response_interceptor"`
		StreamChunkInterceptor bool `json:"response_stream_interceptor"`
		ManagementAPI          bool `json:"management_api"`
	} `json:"capabilities"`
}

func TestPluginRegistersVisibleConfigFields(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	raw, err := HandleMethod(state, pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	fields := map[string]pluginapi.ConfigField{}
	for _, field := range reg.Metadata.ConfigFields {
		fields[field.Name] = field
	}
	want := map[string]pluginapi.ConfigFieldType{
		"models":                    pluginapi.ConfigFieldTypeArray,
		"auto_include_stream_usage": pluginapi.ConfigFieldTypeBoolean,
		"reasoning_equals":          pluginapi.ConfigFieldTypeArray,
		"guard_retry_attempts":      pluginapi.ConfigFieldTypeInteger,
		"intercept_streaming":       pluginapi.ConfigFieldTypeBoolean,
		"intercept_non_streaming":   pluginapi.ConfigFieldTypeBoolean,
		"non_stream_status_code":    pluginapi.ConfigFieldTypeInteger,
		"stream_action":             pluginapi.ConfigFieldTypeEnum,
		"log_match":                 pluginapi.ConfigFieldTypeBoolean,
	}
	for name, typ := range want {
		field, ok := fields[name]
		if !ok {
			t.Fatalf("ConfigFields missing %q; got %#v", name, reg.Metadata.ConfigFields)
		}
		if field.Type != typ {
			t.Fatalf("ConfigFields[%s].Type = %q, want %q", name, field.Type, typ)
		}
		if field.Description == "" {
			t.Fatalf("ConfigFields[%s].Description is empty", name)
		}
	}
	streamAction := fields["stream_action"]
	if len(streamAction.EnumValues) == 0 {
		t.Fatal("stream_action enum values are empty")
	}
}

func TestRequestInterceptAddsStreamUsageOptionForStreamRequest(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.RequestInterceptRequest{
		Model:          "gpt-5.5",
		RequestedModel: "gpt-5.5",
		Stream:         true,
		Body:           []byte(`{"model":"gpt-5.5","stream":true,"messages":[]}`),
	})
	raw, err := HandleMethod(state, pluginabi.MethodRequestInterceptBefore, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if len(resp.Body) == 0 {
		t.Fatal("RequestInterceptResponse.Body is empty, want rewritten body")
	}
	var rewritten struct {
		Stream        bool `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal(resp.Body, &rewritten); err != nil {
		t.Fatalf("json.Unmarshal(rewritten body) error = %v: %s", err, resp.Body)
	}
	if !rewritten.Stream {
		t.Fatal("rewritten stream = false, want true")
	}
	if !rewritten.StreamOptions.IncludeUsage {
		t.Fatalf("rewritten stream_options.include_usage = false, want true: %s", resp.Body)
	}
}

func TestRequestInterceptUsesBodyModelWhenInterceptorModelFieldsAreEmpty(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.Models = []string{"gpt-5.5"}
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.RequestInterceptRequest{
		Stream: true,
		Body:   []byte(`{"model":"gpt-5.5","stream":true,"messages":[]}`),
		Metadata: map[string]any{
			"request_path": "/v1/chat/completions",
		},
	})
	raw, err := HandleMethod(state, pluginabi.MethodRequestInterceptBefore, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if len(resp.Body) == 0 {
		t.Fatal("RequestInterceptResponse.Body is empty, want rewritten body from body model fallback")
	}
}

func TestRequestInterceptSkipsModelOutsideAllowListWithoutLogging(t *testing.T) {
	cfg := pluginconfig.DefaultConfig()
	cfg.Models = []string{"gpt-5.4", "gpt-5.5"}
	state, err := NewPluginState(cfg)
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.RequestInterceptRequest{
		Stream: true,
		Body:   []byte(`{"model":"gpt-5.4-mini","stream":true,"messages":[]}`),
		Metadata: map[string]any{
			"request_path": "/v1/chat/completions",
		},
	})
	raw, err := HandleMethod(state, pluginabi.MethodRequestInterceptBefore, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("RequestInterceptResponse.Body = %s, want empty for disallowed model", resp.Body)
	}
	if logs := state.Runtime.Metrics().Snapshot().Logs; len(logs) != 0 {
		t.Fatalf("logs = %#v, want none for disallowed model", logs)
	}
}

func TestRequestInterceptDoesNotRewriteNonStreamRequest(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(pluginapi.RequestInterceptRequest{
		Model:          "gpt-5.5",
		RequestedModel: "gpt-5.5",
		Stream:         false,
		Body:           []byte(`{"model":"gpt-5.5","messages":[]}`),
	})
	raw, err := HandleMethod(state, pluginabi.MethodRequestInterceptBefore, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("RequestInterceptResponse.Body = %s, want empty", resp.Body)
	}
}

func TestStreamChunksOnlyCountInspectedWhenReasoningUsageAppears(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	chunks := []pluginapi.StreamChunkInterceptRequest{
		{
			SourceFormat: "openai",
			Model:        "gpt-5.5",
			Body:         []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"),
			ChunkIndex:   0,
		},
		{
			SourceFormat: "openai",
			Model:        "gpt-5.5",
			Body:         []byte("data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":777}}}\n\n"),
			ChunkIndex:   1,
		},
		{
			SourceFormat: "openai",
			Model:        "gpt-5.5",
			Body:         []byte("data: [DONE]\n\n"),
			ChunkIndex:   2,
		},
	}
	for _, chunk := range chunks {
		request, _ := json.Marshal(chunk)
		if _, err := HandleMethod(state, pluginabi.MethodResponseInterceptStreamChunk, request); err != nil {
			t.Fatalf("HandleMethod() error = %v", err)
		}
	}
	snapshot := state.Runtime.Metrics().Snapshot()
	if snapshot.InspectedResponseCount != 1 {
		t.Fatalf("inspected_response_count = %d, want 1", snapshot.InspectedResponseCount)
	}
	if snapshot.TotalProxyRequestCount != 1 {
		t.Fatalf("total_proxy_request_count = %d, want 1", snapshot.TotalProxyRequestCount)
	}
}

func TestPluginRegistersRequestInterceptor(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	raw, err := HandleMethod(state, pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if !reg.Capabilities.RequestInterceptor {
		t.Fatal("request_interceptor = false, want true")
	}
}

func TestPluginRegistersResponseInterceptor(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	raw, err := HandleMethod(state, pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if !reg.Capabilities.ResponseInterceptor {
		t.Fatal("response_interceptor = false, want true")
	}
}

func TestPluginRegistersStreamInterceptor(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	raw, err := HandleMethod(state, pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if !reg.Capabilities.StreamChunkInterceptor {
		t.Fatal("response_stream_interceptor = false, want true")
	}
}

func TestPluginUsesManagementCapability(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	raw, err := HandleMethod(state, pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if !reg.Capabilities.ManagementAPI {
		t.Fatal("management_api = false, want true")
	}
}
