package bridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/interceptor"
	"github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/management"
	pluginruntime "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/runtime"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	managementPluginPrefix = "/v0/management"
	resourcePluginPrefix   = "/v0/resource/plugins/codex-retry-guard"
)

type HostCallFunc func(string, any) (json.RawMessage, error)

type PluginState struct {
	Runtime  *pluginruntime.State
	CallHost HostCallFunc
}

type responseEnvelope struct {
	OK     bool `json:"ok"`
	Result any  `json:"result,omitempty"`
	Error  any  `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type registrationResponse struct {
	SchemaVersion uint32 `json:"schema_version"`
	Metadata      struct {
		Name             string                  `json:"Name"`
		Version          string                  `json:"Version"`
		Author           string                  `json:"Author"`
		GitHubRepository string                  `json:"GitHubRepository"`
		Logo             string                  `json:"Logo"`
		ConfigFields     []pluginapi.ConfigField `json:"ConfigFields"`
	} `json:"metadata"`
	Capabilities struct {
		RequestInterceptor     bool `json:"request_interceptor"`
		ResponseInterceptor    bool `json:"response_interceptor"`
		StreamChunkInterceptor bool `json:"response_stream_interceptor"`
		ManagementAPI          bool `json:"management_api"`
	} `json:"capabilities"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type managementRegistrationResult struct {
	Routes    []managementRouteDTO    `json:"routes,omitempty"`
	Resources []managementResourceDTO `json:"resources,omitempty"`
}

type managementRouteDTO struct {
	Method      string `json:"Method,omitempty"`
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type managementResourceDTO struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type hostLogRequest struct {
	Level   string         `json:"level,omitempty"`
	Message string         `json:"message,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

func NewPluginState(cfg pluginconfig.Config) (*PluginState, error) {
	runtimeState, err := pluginruntime.NewState(cfg)
	if err != nil {
		return nil, err
	}
	return &PluginState{Runtime: runtimeState}, nil
}

func HandleMethod(state *PluginState, method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister:
		return okEnvelope(buildRegistration())
	case pluginabi.MethodPluginReconfigure:
		return handleReconfigure(state, request)
	case pluginabi.MethodRequestInterceptBefore:
		return handleRequestIntercept(state, request, true)
	case pluginabi.MethodRequestInterceptAfter:
		return handleRequestIntercept(state, request, false)
	case pluginabi.MethodResponseInterceptAfter:
		return handleNonStreamIntercept(state, request)
	case pluginabi.MethodResponseInterceptStreamChunk:
		return handleStreamIntercept(state, request)
	case pluginabi.MethodManagementRegister:
		return handleManagementRegister(state)
	case pluginabi.MethodManagementHandle:
		return handleManagementRequest(state, request)
	default:
		return errorEnvelope("unknown_method", fmt.Sprintf("unknown method: %s", method))
	}
}

func handleReconfigure(state *PluginState, raw []byte) ([]byte, error) {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return errorEnvelope("bad_request", err.Error())
		}
	}
	next, err := pluginconfig.ParseYAML(req.ConfigYAML)
	if err != nil {
		return errorEnvelope("bad_config", err.Error())
	}
	if err := state.Runtime.Reconfigure(next); err != nil {
		return errorEnvelope("reconfigure_failed", err.Error())
	}
	return okEnvelope(buildRegistration())
}

func buildRegistration() registrationResponse {
	var resp registrationResponse
	resp.SchemaVersion = pluginabi.SchemaVersion
	resp.Metadata.Name = "codex-retry-guard"
	resp.Metadata.Version = "0.1.12"
	resp.Metadata.Author = "router-for-me"
	resp.Metadata.GitHubRepository = "https://github.com/hxx927/codex-retry-guard"
	resp.Metadata.Logo = "https://raw.githubusercontent.com/router-for-me/CLIProxyAPI/main/docs/logo.png"
	resp.Metadata.ConfigFields = []pluginapi.ConfigField{
		{
			Name:        "models",
			Type:        pluginapi.ConfigFieldTypeArray,
			Description: "只检查这些模型。留空表示检查所有模型。",
		},
		{
			Name:        "endpoints",
			Type:        pluginapi.ConfigFieldTypeArray,
			Description: "要检查的 CPA 请求路径。默认覆盖 Codex 常用的 responses 和 chat completions 接口。",
		},
		{
			Name:        "auto_include_stream_usage",
			Type:        pluginapi.ConfigFieldTypeBoolean,
			Description: "流式请求自动补 stream_options.include_usage=true，方便插件读取 reasoning_tokens。",
		},
		{
			Name:        "reasoning_equals",
			Type:        pluginapi.ConfigFieldTypeArray,
			Description: "手动模式下触发防降智的 reasoning_tokens 数值列表。默认是 516、1034、1552。",
		},
		{
			Name:        "reasoning_match_mode",
			Type:        pluginapi.ConfigFieldTypeEnum,
			EnumValues:  []string{pluginconfig.ReasoningMatchModeManual, pluginconfig.ReasoningMatchModeFormula518NSub2},
			Description: "reasoning_tokens 命中模式。manual 使用上面的列表；formula_518n_minus_2 匹配 516、1034、1552、2070 等 518*n-2 序列。",
		},
		{
			Name:        "intercept_streaming",
			Type:        pluginapi.ConfigFieldTypeBoolean,
			Description: "是否检查流式响应。Codex 常用流式输出时建议开启。",
		},
		{
			Name:        "intercept_non_streaming",
			Type:        pluginapi.ConfigFieldTypeBoolean,
			Description: "是否检查非流式响应。",
		},
		{
			Name:        "guard_retry_attempts",
			Type:        pluginapi.ConfigFieldTypeInteger,
			Description: "命中异常后最多内部重试次数。设为 0 时不重试，只按拦截策略返回。",
		},
		{
			Name:        "non_stream_status_code",
			Type:        pluginapi.ConfigFieldTypeInteger,
			Description: "非流式响应命中且无法重试时返回给客户端的 HTTP 状态码。",
		},
		{
			Name:        "stream_action",
			Type:        pluginapi.ConfigFieldTypeEnum,
			EnumValues:  []string{"strict_502"},
			Description: "流式响应命中后的处理方式。strict_502 表示终止并返回 502。",
		},
		{
			Name:        "log_match",
			Type:        pluginapi.ConfigFieldTypeBoolean,
			Description: "是否记录请求检查、命中和拦截日志。",
		},
	}
	resp.Capabilities.RequestInterceptor = true
	resp.Capabilities.ResponseInterceptor = true
	resp.Capabilities.StreamChunkInterceptor = true
	resp.Capabilities.ManagementAPI = true
	return resp
}

func handleRequestIntercept(state *PluginState, raw []byte, logRequest bool) ([]byte, error) {
	var req pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error())
	}
	reasoningEffort := metadataString(req.Metadata, cliproxyexecutor.ReasoningEffortMetadataKey)
	cfg := state.Runtime.Config()
	bodyModel := requestBodyModel(req.Body)
	modelAllowed := requestModelMatches(cfg, req.Model, req.RequestedModel, bodyModel)
	if modelAllowed {
		state.Runtime.CaptureRequestProfile(captureRequestHeaders(req.Headers), reasoningEffort)
		if logRequest {
			logRequestProfile(state, requestPath(req.Metadata, ""), reasoningEffort, effectiveRequestModel(req.Model, req.RequestedModel, bodyModel))
		}
	}
	resp := pluginapi.RequestInterceptResponse{}
	if modelAllowed {
		if body, ok := rewriteStreamUsageRequest(cfg, req); ok {
			resp.Body = body
			if logRequest {
				logStreamUsageRewrite(state, requestPath(req.Metadata, req.SourceFormat), effectiveRequestModel(req.Model, req.RequestedModel, bodyModel))
			}
		}
	}
	return okEnvelope(resp)
}

func rewriteStreamUsageRequest(cfg pluginconfig.Config, req pluginapi.RequestInterceptRequest) ([]byte, bool) {
	if !cfg.AutoIncludeStreamUsage || !req.Stream || len(req.Body) == 0 {
		return nil, false
	}
	if !endpointMatches(cfg, requestPath(req.Metadata, req.SourceFormat)) {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return nil, false
	}
	bodyModel, _ := payload["model"].(string)
	if !requestModelMatches(cfg, req.Model, req.RequestedModel, bodyModel) {
		return nil, false
	}
	streamOptions := map[string]any{}
	if raw, ok := payload["stream_options"]; ok && raw != nil {
		existing, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		streamOptions = existing
	}
	if includeUsage, ok := streamOptions["include_usage"].(bool); ok && includeUsage {
		return nil, false
	}
	streamOptions["include_usage"] = true
	payload["stream_options"] = streamOptions
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return body, true
}

func handleNonStreamIntercept(state *PluginState, raw []byte) ([]byte, error) {
	var req pluginapi.ResponseInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error())
	}
	cfg := state.Runtime.Config()
	if !modelMatches(cfg, req.Model) {
		return okEnvelope(pluginapi.ResponseInterceptResponse{})
	}
	path := requestPath(req.Metadata, req.SourceFormat)
	state.Runtime.Metrics().RecordProxyAttempt()
	decision, err := interceptor.InspectNonStream(cfg, path, req.Body, cfg.GuardRetryAttempts)
	if err != nil {
		return errorEnvelope("inspect_failed", err.Error())
	}
	state.Runtime.Metrics().RecordInspectedResponse(decision.Matched, false)
	if decision.Matched {
		action := "observe_only"
		if cfg.InterceptNonStreaming {
			if decision.Retry {
				action = fmt.Sprintf("internal_retry remaining=%d", cfg.GuardRetryAttempts)
			} else {
				action = fmt.Sprintf("return_status_%d", cfg.NonStreamStatusCode)
			}
			state.Runtime.Metrics().RecordBlockedResponse(false)
		}
		logMatch(state, "non-stream", path, decision.Reasoning, action)
	} else if decision.ReasoningFound {
		logInspect(state, "non-stream", path, decision.Reasoning, "pass")
	}
	if decision.Retry && state != nil && state.CallHost != nil {
		attemptsRemaining := cfg.GuardRetryAttempts
		for {
			result, err := state.CallHost(pluginabi.MethodHostModelExecute, pluginapi.HostModelExecutionRequest{
				EntryProtocol: req.SourceFormat,
				ExitProtocol:  req.SourceFormat,
				Model:         req.Model,
				Stream:        false,
				Body:          req.RequestBody,
				Headers:       req.RequestHeaders,
			})
			if err != nil {
				return errorEnvelope("host_retry_failed", err.Error())
			}
			var hostResp pluginapi.HostModelExecutionResponse
			if err := json.Unmarshal(result, &hostResp); err != nil {
				return errorEnvelope("host_retry_decode_failed", err.Error())
			}
			state.Runtime.Metrics().RecordProxyAttempt()
			nextDecision, err := interceptor.InspectNonStream(cfg, path, hostResp.Body, max(0, attemptsRemaining-1))
			if err != nil {
				resp := pluginapi.ResponseInterceptResponse{Body: hostResp.Body, Headers: cloneHeader(hostResp.Headers)}
				return okEnvelope(resp)
			}
			state.Runtime.Metrics().RecordInspectedResponse(nextDecision.Matched, false)
			if !nextDecision.Matched {
				if nextDecision.ReasoningFound {
					logInspect(state, "non-stream", path, nextDecision.Reasoning, "pass")
				}
				resp := pluginapi.ResponseInterceptResponse{Body: hostResp.Body, Headers: cloneHeader(hostResp.Headers)}
				return okEnvelope(resp)
			}
			action := "observe_only"
			if cfg.InterceptNonStreaming {
				if nextDecision.Retry && attemptsRemaining > 1 {
					action = fmt.Sprintf("internal_retry remaining=%d", attemptsRemaining-1)
				} else {
					action = fmt.Sprintf("return_status_%d", cfg.NonStreamStatusCode)
				}
				state.Runtime.Metrics().RecordBlockedResponse(false)
			}
			logMatch(state, "non-stream", path, nextDecision.Reasoning, action)
			attemptsRemaining--
			if nextDecision.Retry && attemptsRemaining > 0 {
				continue
			}
			resp := pluginapi.ResponseInterceptResponse{}
			if nextDecision.StatusCode != 0 && len(nextDecision.ResponseBody) > 0 {
				resp.Body = nextDecision.ResponseBody
				resp.Headers = http.Header{
					"Content-Type":                 []string{"application/json; charset=utf-8"},
					"X-Codex-Retry-Gateway-Reason": []string{"reasoning-guard-triggered"},
					"X-CPA-Status-Code":            []string{fmt.Sprintf("%d", nextDecision.StatusCode)},
				}
				resp.ClearHeaders = []string{"Content-Length"}
				return okEnvelope(resp)
			}
			resp.Body = hostResp.Body
			resp.Headers = cloneHeader(hostResp.Headers)
			return okEnvelope(resp)
		}
	}
	resp := pluginapi.ResponseInterceptResponse{}
	if decision.StatusCode != 0 && len(decision.ResponseBody) > 0 {
		resp.Body = decision.ResponseBody
		resp.Headers = http.Header{
			"Content-Type":                 []string{"application/json; charset=utf-8"},
			"X-Codex-Retry-Gateway-Reason": []string{"reasoning-guard-triggered"},
			"X-CPA-Status-Code":            []string{fmt.Sprintf("%d", decision.StatusCode)},
		}
		resp.ClearHeaders = []string{"Content-Length"}
	}
	return okEnvelope(resp)
}

func handleStreamIntercept(state *PluginState, raw []byte) ([]byte, error) {
	var req pluginapi.StreamChunkInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error())
	}
	if req.ChunkIndex == pluginapi.StreamChunkHeaderInitIndex {
		if state != nil {
			cfg := state.Runtime.Config()
			if !modelMatches(cfg, req.Model) {
				return okEnvelope(pluginapi.StreamChunkInterceptResponse{})
			}
			if cfg.InterceptStreaming && cfg.StreamAction != "disconnect" {
				return okEnvelope(pluginapi.StreamChunkInterceptResponse{Headers: http.Header{"X-CPA-Buffer-Stream": []string{"1"}}})
			}
		}
		return okEnvelope(pluginapi.StreamChunkInterceptResponse{})
	}
	cfg := state.Runtime.Config()
	if !modelMatches(cfg, req.Model) {
		return okEnvelope(pluginapi.StreamChunkInterceptResponse{})
	}
	path := requestPath(req.Metadata, req.SourceFormat)
	decision, err := interceptor.InspectStreamChunk(cfg, path, req.Body, req.HistoryChunks, cfg.GuardRetryAttempts, req.ChunkIndex > 0)
	if err != nil {
		return errorEnvelope("inspect_failed", err.Error())
	}
	if decision.ReasoningFound {
		state.Runtime.Metrics().RecordProxyAttempt()
		state.Runtime.Metrics().RecordInspectedResponse(decision.Matched, true)
	}
	if decision.Matched {
		action := "observe_only"
		if cfg.InterceptStreaming {
			if decision.Retry {
				action = fmt.Sprintf("internal_retry remaining=%d", cfg.GuardRetryAttempts)
			} else if cfg.StreamAction == "disconnect" && req.ChunkIndex > 0 {
				action = "disconnect"
			} else {
				action = fmt.Sprintf("return_status_%d", cfg.NonStreamStatusCode)
			}
			state.Runtime.Metrics().RecordBlockedResponse(true)
		}
		logMatch(state, "stream", path, decision.Reasoning, action)
	} else if decision.ReasoningFound {
		logInspect(state, "stream", path, decision.Reasoning, "pass")
	}
	if decision.Retry && state != nil && state.CallHost != nil {
		resp, err := replayStreamWithRetry(state, cfg, path, req)
		if err != nil {
			return errorEnvelope("host_retry_failed", err.Error())
		}
		return okEnvelope(resp)
	}
	resp := pluginapi.StreamChunkInterceptResponse{DropChunk: decision.DropChunk}
	if len(decision.ReplacementChunk) > 0 {
		resp.Body = decision.ReplacementChunk
		resp.Headers = http.Header{
			"Content-Type":                 []string{"application/json; charset=utf-8"},
			"X-Codex-Retry-Gateway-Reason": []string{"reasoning-guard-triggered"},
			"X-CPA-Status-Code":            []string{fmt.Sprintf("%d", cfg.NonStreamStatusCode)},
		}
		resp.ClearHeaders = []string{"Content-Length"}
	}
	return okEnvelope(resp)
}

func replayStreamWithRetry(state *PluginState, cfg pluginconfig.Config, path string, req pluginapi.StreamChunkInterceptRequest) (pluginapi.StreamChunkInterceptResponse, error) {
	attemptsRemaining := cfg.GuardRetryAttempts
	for {
		state.Runtime.Metrics().RecordProxyAttempt()
		result, err := state.CallHost(pluginabi.MethodHostModelExecuteStream, pluginapi.HostModelExecutionRequest{
			EntryProtocol: req.SourceFormat,
			ExitProtocol:  req.SourceFormat,
			Model:         req.Model,
			Stream:        true,
			Body:          req.RequestBody,
			Headers:       req.RequestHeaders,
		})
		if err != nil {
			return pluginapi.StreamChunkInterceptResponse{}, err
		}
		var streamResp pluginapi.HostModelStreamResponse
		if err := json.Unmarshal(result, &streamResp); err != nil {
			return pluginapi.StreamChunkInterceptResponse{}, fmt.Errorf("decode retried stream: %w", err)
		}
		resp, retryAgain, err := readRetriedStreamAttempt(state, cfg, path, streamResp.StreamID, attemptsRemaining)
		if err != nil {
			return pluginapi.StreamChunkInterceptResponse{}, err
		}
		if retryAgain {
			attemptsRemaining--
			if attemptsRemaining <= 0 {
				return resp, nil
			}
			continue
		}
		return resp, nil
	}
}

func readRetriedStreamAttempt(state *PluginState, cfg pluginconfig.Config, path string, streamID string, attemptsRemaining int) (pluginapi.StreamChunkInterceptResponse, bool, error) {
	defer func() {
		if state != nil && state.CallHost != nil {
			_, _ = state.CallHost(pluginabi.MethodHostModelStreamClose, pluginapi.HostModelStreamCloseRequest{StreamID: streamID})
		}
	}()
	chunks := make([][]byte, 0, 8)
	history := make([][]byte, 0, 8)
	inspectedRecorded := false
	for {
		readRaw, err := state.CallHost(pluginabi.MethodHostModelStreamRead, pluginapi.HostModelStreamReadRequest{StreamID: streamID})
		if err != nil {
			return pluginapi.StreamChunkInterceptResponse{}, false, fmt.Errorf("read retried stream: %w", err)
		}
		var readResp pluginapi.HostModelStreamReadResponse
		if err := json.Unmarshal(readRaw, &readResp); err != nil {
			return pluginapi.StreamChunkInterceptResponse{}, false, fmt.Errorf("decode retried stream chunk: %w", err)
		}
		if readResp.Error != "" {
			return pluginapi.StreamChunkInterceptResponse{}, false, fmt.Errorf("retried stream error: %s", readResp.Error)
		}
		if len(readResp.Payload) > 0 {
			payloadCopy := append([]byte(nil), readResp.Payload...)
			chunks = append(chunks, payloadCopy)
			decision, err := interceptor.InspectStreamChunk(cfg, path, payloadCopy, history, max(0, attemptsRemaining-1), false)
			if err == nil && !decision.Matched && decision.ReasoningFound {
				logInspect(state, "stream", path, decision.Reasoning, "pass")
			}
			if err == nil && decision.Matched {
				if !inspectedRecorded {
					state.Runtime.Metrics().RecordInspectedResponse(true, true)
					inspectedRecorded = true
				}
				action := "observe_only"
				if cfg.InterceptStreaming {
					if decision.Retry && attemptsRemaining > 1 {
						action = fmt.Sprintf("internal_retry remaining=%d", attemptsRemaining-1)
					} else if cfg.StreamAction == "disconnect" {
						action = "disconnect"
					} else {
						action = fmt.Sprintf("return_status_%d", cfg.NonStreamStatusCode)
					}
					state.Runtime.Metrics().RecordBlockedResponse(true)
				}
				logMatch(state, "stream", path, decision.Reasoning, action)
				if decision.Retry && attemptsRemaining > 1 {
					return pluginapi.StreamChunkInterceptResponse{}, true, nil
				}
				resp := pluginapi.StreamChunkInterceptResponse{DropChunk: decision.DropChunk}
				if len(decision.ReplacementChunk) > 0 {
					resp.Body = decision.ReplacementChunk
					resp.Headers = http.Header{
						"Content-Type":                 []string{"application/json; charset=utf-8"},
						"X-Codex-Retry-Gateway-Reason": []string{"reasoning-guard-triggered"},
						"X-CPA-Status-Code":            []string{fmt.Sprintf("%d", cfg.NonStreamStatusCode)},
					}
					resp.ClearHeaders = []string{"Content-Length"}
				}
				return resp, false, nil
			}
			history = append(history, payloadCopy)
		}
		if readResp.Done {
			if !inspectedRecorded {
				state.Runtime.Metrics().RecordInspectedResponse(false, true)
			}
			return pluginapi.StreamChunkInterceptResponse{Body: joinChunks(chunks)}, false, nil
		}
	}
}

func joinChunks(chunks [][]byte) []byte {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	joined := make([]byte, 0, total)
	for _, chunk := range chunks {
		joined = append(joined, chunk...)
	}
	return joined
}

func handleManagementRegister(state *PluginState) ([]byte, error) {
	registration := management.Register(state.Runtime)
	result := managementRegistrationResult{
		Routes:    make([]managementRouteDTO, 0, len(registration.Routes)),
		Resources: make([]managementResourceDTO, 0, len(registration.Resources)),
	}
	for _, route := range registration.Routes {
		result.Routes = append(result.Routes, managementRouteDTO{Method: route.Method, Path: route.Path, Menu: route.Menu, Description: route.Description})
	}
	for _, resource := range registration.Resources {
		result.Resources = append(result.Resources, managementResourceDTO{Path: resource.Path, Menu: resource.Menu, Description: resource.Description})
	}
	return okEnvelope(result)
}

func handleManagementRequest(state *PluginState, raw []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelope("bad_request", err.Error())
	}
	registration := management.Register(state.Runtime)
	normalizedPath := normalizePluginManagementPath(req.Path)
	for _, route := range registration.Routes {
		if route.Method == req.Method && route.Path == normalizedPath {
			forwarded := req
			forwarded.Path = normalizedPath
			resp, err := route.Handler.HandleManagement(nil, forwarded)
			if err != nil {
				return errorEnvelope("management_failed", err.Error())
			}
			return okEnvelope(resp)
		}
	}
	resourcePath := normalizePluginResourcePath(req.Path)
	for _, resource := range registration.Resources {
		if resourcePath == resource.Path || strings.HasSuffix(resourcePath, resource.Path) {
			forwarded := req
			forwarded.Path = resourcePath
			resp, err := resource.Handler.HandleManagement(nil, forwarded)
			if err != nil {
				return errorEnvelope("management_failed", err.Error())
			}
			return okEnvelope(resp)
		}
	}
	return errorEnvelope("not_found", "management route not found")
}

func okEnvelope(result any) ([]byte, error) {
	return json.Marshal(responseEnvelope{OK: true, Result: result})
}

func errorEnvelope(code, message string) ([]byte, error) {
	return json.Marshal(responseEnvelope{OK: false, Error: responseError{Code: code, Message: message}})
}

func normalizePluginManagementPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, managementPluginPrefix+"/") {
		path = strings.TrimPrefix(path, managementPluginPrefix)
	}
	return path
}

func normalizePluginResourcePath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, resourcePluginPrefix+"/") {
		path = strings.TrimPrefix(path, resourcePluginPrefix)
	}
	return path
}

func modelMatches(cfg pluginconfig.Config, model string) bool {
	if len(cfg.Models) == 0 {
		return true
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, candidate := range cfg.Models {
		if strings.EqualFold(candidate, model) {
			return true
		}
	}
	return false
}

func requestModelMatches(cfg pluginconfig.Config, models ...string) bool {
	for _, model := range models {
		if modelMatches(cfg, model) {
			return true
		}
	}
	return false
}

func requestBodyModel(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	model, _ := payload["model"].(string)
	return strings.TrimSpace(model)
}

func effectiveRequestModel(models ...string) string {
	for _, model := range models {
		if trimmed := strings.TrimSpace(model); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func endpointMatches(cfg pluginconfig.Config, path string) bool {
	if len(cfg.Endpoints) == 0 {
		return true
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	for _, endpoint := range cfg.Endpoints {
		if strings.EqualFold(strings.TrimSpace(endpoint), path) {
			return true
		}
	}
	return false
}

func requestPath(metadata map[string]any, fallback string) string {
	path := metadataString(metadata, cliproxyexecutor.RequestPathMetadataKey)
	if strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return "/responses"
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func captureRequestHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[key] = values[0]
	}
	return out
}

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	out := make(http.Header, len(src))
	for key, values := range src {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func logMatch(state *PluginState, streamKind, path string, reasoning int, action string) {
	cfg := state.Runtime.Config()
	if !cfg.LogMatch {
		return
	}
	message := fmt.Sprintf("[match] %s path=%s reasoning_tokens=%d action=%s", streamKind, path, reasoning, action)
	state.Runtime.Metrics().AppendLog(time.Now().UTC().Format(time.RFC3339), message)
	if state.CallHost == nil {
		return
	}
	_, _ = state.CallHost(pluginabi.MethodHostLog, hostLogRequest{
		Level:   "info",
		Message: message,
		Fields: map[string]any{
			"plugin": "codex-retry-guard",
			"path":   path,
			"stream": streamKind,
		},
	})
}

func logInspect(state *PluginState, streamKind, path string, reasoning int, action string) {
	cfg := state.Runtime.Config()
	if !cfg.LogMatch {
		return
	}
	message := fmt.Sprintf("[inspect] %s path=%s reasoning_tokens=%d action=%s", streamKind, path, reasoning, action)
	state.Runtime.Metrics().AppendLog(time.Now().UTC().Format(time.RFC3339), message)
	if state.CallHost == nil {
		return
	}
	_, _ = state.CallHost(pluginabi.MethodHostLog, hostLogRequest{
		Level:   "info",
		Message: message,
		Fields: map[string]any{
			"plugin": "codex-retry-guard",
			"path":   path,
			"stream": streamKind,
		},
	})
}

func logRequestProfile(state *PluginState, path string, reasoningEffort string, model string) {
	cfg := state.Runtime.Config()
	if !cfg.LogMatch {
		return
	}
	if strings.TrimSpace(path) == "" {
		path = "/"
	}
	if strings.TrimSpace(reasoningEffort) == "" {
		reasoningEffort = "unspecified"
	}
	message := fmt.Sprintf("[request] path=%s model=%s reasoning_effort=%s action=inspect", path, valueOrUnspecified(model), reasoningEffort)
	state.Runtime.Metrics().AppendLog(time.Now().UTC().Format(time.RFC3339), message)
	if state.CallHost == nil {
		return
	}
	_, _ = state.CallHost(pluginabi.MethodHostLog, hostLogRequest{
		Level:   "info",
		Message: message,
		Fields: map[string]any{
			"plugin": "codex-retry-guard",
			"path":   path,
		},
	})
}

func valueOrUnspecified(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unspecified"
	}
	return strings.TrimSpace(value)
}

func logStreamUsageRewrite(state *PluginState, path string, model string) {
	cfg := state.Runtime.Config()
	if !cfg.LogMatch {
		return
	}
	if strings.TrimSpace(path) == "" {
		path = "/"
	}
	message := fmt.Sprintf("[request] path=%s model=%s action=add_stream_usage", path, valueOrUnspecified(model))
	state.Runtime.Metrics().AppendLog(time.Now().UTC().Format(time.RFC3339), message)
	if state.CallHost == nil {
		return
	}
	_, _ = state.CallHost(pluginabi.MethodHostLog, hostLogRequest{
		Level:   "info",
		Message: message,
		Fields: map[string]any{
			"plugin": "codex-retry-guard",
			"path":   path,
		},
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
