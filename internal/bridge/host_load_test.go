package bridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	internalpluginhost "github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
)

func TestBuiltPluginLoadsIntoHost(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("c-shared host load test only runs on linux/amd64, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	pluginDir := t.TempDir()
	pluginPath := filepath.Join(pluginDir, "codex-retry-guard.so")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", pluginPath, "./cmd/plugin")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}

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
