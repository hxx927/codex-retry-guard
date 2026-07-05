package config

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Enabled                bool     `yaml:"enabled" json:"enabled"`
	Models                 []string `yaml:"models" json:"models"`
	Endpoints              []string `yaml:"endpoints" json:"endpoints"`
	AutoIncludeStreamUsage bool     `yaml:"auto_include_stream_usage" json:"auto_include_stream_usage"`
	ReasoningEquals        IntList  `yaml:"reasoning_equals" json:"reasoning_equals"`
	ReasoningMatchMode     string   `yaml:"reasoning_match_mode" json:"reasoning_match_mode"`
	InterceptStreaming     bool     `yaml:"intercept_streaming" json:"intercept_streaming"`
	InterceptNonStreaming  bool     `yaml:"intercept_non_streaming" json:"intercept_non_streaming"`
	GuardRetryAttempts     int      `yaml:"guard_retry_attempts" json:"guard_retry_attempts"`
	NonStreamStatusCode    int      `yaml:"non_stream_status_code" json:"non_stream_status_code"`
	StreamAction           string   `yaml:"stream_action" json:"stream_action"`
	LogMatch               bool     `yaml:"log_match" json:"log_match"`
}

const (
	ReasoningMatchModeManual          = "manual"
	ReasoningMatchModeFormula518NSub2 = "formula_518n_minus_2"
)

func DefaultConfig() Config {
	return Config{
		Enabled:                true,
		Endpoints:              []string{"/responses", "/chat/completions", "/v1/responses", "/v1/chat/completions"},
		AutoIncludeStreamUsage: true,
		ReasoningEquals:        IntList{516, 1034, 1552},
		ReasoningMatchMode:     ReasoningMatchModeManual,
		InterceptStreaming:     true,
		InterceptNonStreaming:  true,
		GuardRetryAttempts:     3,
		NonStreamStatusCode:    502,
		StreamAction:           "strict_502",
		LogMatch:               true,
	}
}

func ParseYAML(data []byte) (Config, error) {
	cfg := DefaultConfig()
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return NormalizeAndValidate(cfg)
}

func NormalizeAndValidate(cfg Config) (Config, error) {
	cfg.Models = normalizeStringList(cfg.Models)
	cfg.Endpoints = normalizeStringList(cfg.Endpoints)
	cfg.ReasoningEquals = normalizeIntegerList(cfg.ReasoningEquals)
	cfg.ReasoningMatchMode = normalizeReasoningMatchMode(cfg.ReasoningMatchMode)
	cfg.StreamAction = strings.TrimSpace(cfg.StreamAction)
	if cfg.StreamAction == "" {
		cfg.StreamAction = "strict_502"
	}
	if len(cfg.Endpoints) == 0 {
		cfg.Endpoints = append([]string(nil), DefaultConfig().Endpoints...)
	}
	if len(cfg.ReasoningEquals) == 0 {
		return Config{}, fmt.Errorf("reasoning_equals cannot be empty")
	}
	if cfg.GuardRetryAttempts < 0 {
		return Config{}, fmt.Errorf("guard_retry_attempts must be >= 0")
	}
	if !cfg.InterceptStreaming && !cfg.InterceptNonStreaming {
		return Config{}, fmt.Errorf("intercept_streaming and intercept_non_streaming cannot both be false")
	}
	if cfg.NonStreamStatusCode <= 0 {
		cfg.NonStreamStatusCode = DefaultConfig().NonStreamStatusCode
	}
	return cfg, nil
}

func ReasoningMatched(cfg Config, reasoning int) bool {
	if cfg.ReasoningMatchMode == ReasoningMatchModeFormula518NSub2 {
		return reasoning >= 516 && (reasoning+2)%518 == 0
	}
	for _, value := range cfg.ReasoningEquals {
		if value == reasoning {
			return true
		}
	}
	return false
}

type IntList []int

func (values *IntList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		var single int
		if err := decodeYAMLInt(value, &single); err != nil {
			return err
		}
		*values = IntList{single}
		return nil
	}
	out := make([]int, 0, len(value.Content))
	for _, node := range value.Content {
		var item int
		if err := decodeYAMLInt(node, &item); err != nil {
			return err
		}
		out = append(out, item)
	}
	*values = out
	return nil
}

func decodeYAMLInt(node *yaml.Node, out *int) error {
	if node.Tag == "!!str" {
		value, err := strconv.Atoi(strings.TrimSpace(node.Value))
		if err != nil {
			return fmt.Errorf("invalid integer %q: %w", node.Value, err)
		}
		*out = value
		return nil
	}
	return node.Decode(out)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeReasoningMatchMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case ReasoningMatchModeFormula518NSub2:
		return ReasoningMatchModeFormula518NSub2
	case ReasoningMatchModeManual, "":
		return ReasoningMatchModeManual
	default:
		return ReasoningMatchModeManual
	}
}

func normalizeIntegerList(values IntList) IntList {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return IntList(out)
}
