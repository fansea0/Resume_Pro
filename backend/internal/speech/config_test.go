package speech

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnv_DefaultsToDisabled(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "")
	cfg := LoadConfigFromEnv()
	if cfg.Enabled {
		t.Error("voice should be disabled by default")
	}
	if cfg.TTSBaseURL != "http://127.0.0.1:21100/v1" || cfg.ASRBaseURL != "http://127.0.0.1:21100/v1" {
		t.Errorf("default base urls = %q / %q", cfg.TTSBaseURL, cfg.ASRBaseURL)
	}
}

func TestLoadConfigFromEnv_ReadsBackoffAndAttempts(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "true")
	t.Setenv("VOICE_GENERATOR_BACKOFF", "1s,2s")
	t.Setenv("VOICE_GENERATOR_MAX_ATTEMPTS", "5")
	cfg := LoadConfigFromEnv()
	if !cfg.Enabled {
		t.Error("voice should be enabled")
	}
	if cfg.MaxAttempts != 5 {
		t.Errorf("max attempts = %d, want 5", cfg.MaxAttempts)
	}
	if len(cfg.Backoff) != 2 || cfg.Backoff[1] != 2*time.Second {
		t.Errorf("backoff = %v", cfg.Backoff)
	}
}
