package bridge

import (
	"context"
	"path/filepath"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	internalpluginhost "github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
)

func TestBuiltPluginLoadsIntoHost(t *testing.T) {
	root := filepath.Clean("/opt/src/worktrees/CLIProxyAPI/codex-cpa-plugin-retry-guard-main/plugins/codex-retry-guard")
	pluginDir := filepath.Join(root, "dist")
	enabled := true
	host := internalpluginhost.New()
	t.Cleanup(host.ShutdownAll)
	host.ApplyConfig(context.Background(), &internalconfig.Config{
		Plugins: internalconfig.PluginsConfig{
			Enabled: true,
			Dir:     pluginDir,
			Configs: map[string]internalconfig.PluginInstanceConfig{
				"codex-retry-guard": {
					Enabled:  &enabled,
					Priority: 1,
				},
			},
		},
	})
	if !host.PluginLoaded("codex-retry-guard") {
		t.Fatal("PluginLoaded(codex-retry-guard) = false, want true")
	}
	if !host.PluginRegistered("codex-retry-guard") {
		t.Fatal("PluginRegistered(codex-retry-guard) = false, want true")
	}
	plugins := host.RegisteredPlugins()
	if len(plugins) != 1 {
		t.Fatalf("RegisteredPlugins() len = %d, want 1", len(plugins))
	}
	if plugins[0].ID != "codex-retry-guard" {
		t.Fatalf("RegisteredPlugins()[0].ID = %q, want codex-retry-guard", plugins[0].ID)
	}
}
