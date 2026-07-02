package bridge

import (
	"encoding/json"
	"testing"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestPluginReconfigureUpdatesRuntimeConfigFromConfigYAML(t *testing.T) {
	state, err := NewPluginState(pluginconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPluginState() error = %v", err)
	}
	request, _ := json.Marshal(map[string]any{
		"config_yaml": []byte("guard_retry_attempts: 9\nlog_match: false\n"),
	})
	if _, err := HandleMethod(state, pluginabi.MethodPluginReconfigure, request); err != nil {
		t.Fatalf("HandleMethod() error = %v", err)
	}
	cfg := state.Runtime.Config()
	if cfg.GuardRetryAttempts != 9 {
		t.Fatalf("GuardRetryAttempts = %d, want 9", cfg.GuardRetryAttempts)
	}
	if cfg.LogMatch {
		t.Fatal("LogMatch = true, want false")
	}
}
