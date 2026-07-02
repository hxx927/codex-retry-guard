package runtime

import (
	"strings"
	"sync/atomic"
	"time"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
)

type State struct {
	config  atomic.Pointer[pluginconfig.Config]
	metrics *Metrics
}

func NewState(cfg pluginconfig.Config) (*State, error) {
	normalized, err := pluginconfig.NormalizeAndValidate(cfg)
	if err != nil {
		return nil, err
	}
	state := &State{metrics: NewMetrics()}
	state.config.Store(&normalized)
	return state, nil
}

func (s *State) Config() pluginconfig.Config {
	if s == nil {
		return pluginconfig.Config{}
	}
	cfg := s.config.Load()
	if cfg == nil {
		return pluginconfig.Config{}
	}
	return *cfg
}

func (s *State) Reconfigure(next pluginconfig.Config) error {
	normalized, err := pluginconfig.NormalizeAndValidate(next)
	if err != nil {
		return err
	}
	s.config.Store(&normalized)
	return nil
}

func (s *State) Metrics() *Metrics {
	if s == nil || s.metrics == nil {
		return NewMetrics()
	}
	return s.metrics
}

func (s *State) CaptureRequestProfile(headers map[string]string, reasoningEffort string) {
	if s == nil || s.metrics == nil {
		return
	}
	sanitized := make(map[string]string)
	for key, value := range headers {
		headerName := strings.TrimSpace(strings.ToLower(key))
		headerValue := strings.TrimSpace(value)
		if headerName == "" || headerValue == "" {
			continue
		}
		if headerName == "authorization" || headerName == "content-length" || headerName == "host" {
			continue
		}
		sanitized[headerName] = headerValue
	}
	if _, ok := sanitized["user-agent"]; !ok {
		sanitized["user-agent"] = "codex-retry-gateway/active-probe"
	}
	profile := RequestProfile{
		Headers:    sanitized,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if effort := normalizeReasoningEffort(reasoningEffort); effort != "" {
		profile.Reasoning = &ReasoningProfile{Effort: effort}
	}
	s.metrics.SetRequestProfile(profile)
}

func normalizeReasoningEffort(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "minimal", "low", "medium", "high", "xhigh":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}
