package management

import (
	"encoding/json"
	"net/http"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	pluginruntime "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/runtime"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementHandlerFunc func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error)

func (f managementHandlerFunc) HandleManagement(_ interface{}, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	return f(req)
}

func TestManagementRegistersStatusAndConfigRoutes(t *testing.T) {
	state, err := pluginruntime.NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	routes := Register(state)
	if len(routes.Routes) != 4 {
		t.Fatalf("len(Routes) = %d, want 4", len(routes.Routes))
	}
	if len(routes.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(routes.Resources))
	}
}

func TestStatusEndpointReturnsMetricsSnapshotAndRequestProfile(t *testing.T) {
	state, err := pluginruntime.NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	state.CaptureRequestProfile(map[string]string{"User-Agent": "CodexDesktop/1.0"}, "medium")
	routes := Register(state)
	resp, err := routes.Routes[0].Handler.HandleManagement(nil, pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/plugins/codex-retry-guard/api/status"})
	if err != nil {
		t.Fatalf("HandleManagement() error = %v", err)
	}
	var payload struct {
		Config struct {
			Enabled bool `json:"enabled"`
		} `json:"config"`
		Metrics struct {
			TotalProxyRequestCount int64 `json:"total_proxy_request_count"`
			RequestProfile         struct {
				Headers map[string]string `json:"headers"`
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
			} `json:"request_profile"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.Config.Enabled {
		t.Fatal("config.enabled = false, want true")
	}
	if payload.Metrics.TotalProxyRequestCount != 0 {
		t.Fatalf("metrics.total_proxy_request_count = %d, want 0", payload.Metrics.TotalProxyRequestCount)
	}
	if payload.Metrics.RequestProfile.Headers["user-agent"] != "CodexDesktop/1.0" {
		t.Fatalf("request profile user-agent = %q", payload.Metrics.RequestProfile.Headers["user-agent"])
	}
	if payload.Metrics.RequestProfile.Reasoning.Effort != "medium" {
		t.Fatalf("request profile reasoning effort = %q", payload.Metrics.RequestProfile.Reasoning.Effort)
	}
}

func TestLogsEndpointReturnsRecordedLogs(t *testing.T) {
	state, err := pluginruntime.NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	state.Metrics().AppendLog("2026-07-01T00:00:00Z", "[match] non-stream path=/responses reasoning_tokens=516 action=return_status_502")
	routes := Register(state)
	resp, err := routes.Routes[1].Handler.HandleManagement(nil, pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/plugins/codex-retry-guard/api/logs"})
	if err != nil {
		t.Fatalf("HandleManagement() error = %v", err)
	}
	var payload struct {
		TotalEntries int `json:"total_entries"`
		Entries []struct {
			Message string `json:"message"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.TotalEntries != 1 {
		t.Fatalf("total_entries = %d, want 1", payload.TotalEntries)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].Message == "" {
		t.Fatalf("entries = %#v", payload.Entries)
	}
}

func TestConfigEndpointRejectsInvalidReasoningList(t *testing.T) {
	state, err := pluginruntime.NewState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewState() error = %v", err)
	}
	routes := Register(state)
	resp, err := routes.Routes[3].Handler.HandleManagement(nil, pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/plugins/codex-retry-guard/config",
		Body:   []byte(`{"reasoning_equals":[]}`),
	})
	if err != nil {
		t.Fatalf("HandleManagement() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want 400", resp.StatusCode)
	}
}
