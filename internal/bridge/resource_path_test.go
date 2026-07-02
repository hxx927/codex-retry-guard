package bridge

import (
	"encoding/json"
	"net/http"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestManagementHandleMatchesAbsolutePluginPaths(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(map[string]any{
		"Method": http.MethodGet,
		"Path":   "/v0/management/plugins/codex-retry-guard/api/status",
	})
	raw, err := HandleMethod(state, pluginabi.MethodManagementHandle, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			StatusCode int    `json:"StatusCode"`
			Body       []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK || env.Result.StatusCode != http.StatusOK {
		t.Fatalf("management handle result = %#v", env)
	}
}

func TestResourceHandleMatchesAbsolutePluginPaths(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(map[string]any{
		"Method": http.MethodGet,
		"Path":   "/v0/resource/plugins/codex-retry-guard/status",
	})
	raw, err := HandleMethod(state, pluginabi.MethodManagementHandle, request)
	if err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			StatusCode int    `json:"StatusCode"`
			Body       []byte `json:"Body"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK || env.Result.StatusCode != http.StatusOK {
		t.Fatalf("resource handle result = %#v", env)
	}
}
