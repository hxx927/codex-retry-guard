package config

import "testing"

func TestDefaultConfigUsesGatewayParityValues(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if cfg.GuardRetryAttempts != 3 {
		t.Fatalf("GuardRetryAttempts = %d, want 3", cfg.GuardRetryAttempts)
	}
	if cfg.NonStreamStatusCode != 502 {
		t.Fatalf("NonStreamStatusCode = %d, want 502", cfg.NonStreamStatusCode)
	}
	if cfg.StreamAction != "strict_502" {
		t.Fatalf("StreamAction = %q, want strict_502", cfg.StreamAction)
	}
	want := []int{516, 1034, 1552}
	if len(cfg.ReasoningEquals) != len(want) {
		t.Fatalf("ReasoningEquals length = %d, want %d", len(cfg.ReasoningEquals), len(want))
	}
	for i, v := range want {
		if cfg.ReasoningEquals[i] != v {
			t.Fatalf("ReasoningEquals[%d] = %d, want %d", i, cfg.ReasoningEquals[i], v)
		}
	}
}

func TestValidateRejectsEmptyReasoningEquals(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReasoningEquals = nil
	if _, err := NormalizeAndValidate(cfg); err == nil {
		t.Fatal("NormalizeAndValidate error = nil, want error")
	}
}

func TestValidateRejectsBothInterceptModesDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InterceptStreaming = false
	cfg.InterceptNonStreaming = false
	if _, err := NormalizeAndValidate(cfg); err == nil {
		t.Fatal("NormalizeAndValidate error = nil, want error")
	}
}

func TestValidateNormalizesModelAllowList(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Models = []string{" gpt-5.5 ", "gpt-4.1", "gpt-5.5", ""}
	normalized, err := NormalizeAndValidate(cfg)
	if err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	want := []string{"gpt-5.5", "gpt-4.1"}
	if len(normalized.Models) != len(want) {
		t.Fatalf("Models length = %d, want %d: %#v", len(normalized.Models), len(want), normalized.Models)
	}
	for i, value := range want {
		if normalized.Models[i] != value {
			t.Fatalf("Models[%d] = %q, want %q", i, normalized.Models[i], value)
		}
	}
}

func TestParseYAMLAcceptsStringReasoningEqualsFromManagementForm(t *testing.T) {
	cfg, err := ParseYAML([]byte("reasoning_equals:\n  - \"516\"\n  - \"1034\"\n  - \"1552\"\nmodels:\n  - gpt-5.5\n  - gpt-5.4\n"))
	if err != nil {
		t.Fatalf("ParseYAML() error = %v", err)
	}
	want := []int{516, 1034, 1552}
	if len(cfg.ReasoningEquals) != len(want) {
		t.Fatalf("ReasoningEquals length = %d, want %d: %#v", len(cfg.ReasoningEquals), len(want), cfg.ReasoningEquals)
	}
	for i, value := range want {
		if cfg.ReasoningEquals[i] != value {
			t.Fatalf("ReasoningEquals[%d] = %d, want %d", i, cfg.ReasoningEquals[i], value)
		}
	}
	if len(cfg.Models) != 2 || cfg.Models[0] != "gpt-5.5" || cfg.Models[1] != "gpt-5.4" {
		t.Fatalf("Models = %#v", cfg.Models)
	}
}
