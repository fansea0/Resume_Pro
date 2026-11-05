package speech

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnv_DefaultsToDisabled(t *testing.T) {
	// 逐个清空被断言的变量，避免本机/CI 导出的同名环境变量造成假失败。
	t.Setenv("VOICE_ENABLED", "")
	t.Setenv("VOICE_TTS_BASE_URL", "")
	t.Setenv("VOICE_ASR_BASE_URL", "")
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

// 未设置或非法值一律回落到默认超时，避免出现「永不超时」的客户端。
func TestLoadConfigFromEnv_FallsBackOnInvalidValues(t *testing.T) {
	t.Setenv("VOICE_TTS_TIMEOUT", "not-a-duration")
	t.Setenv("VOICE_ASR_TIMEOUT", "0")
	t.Setenv("VOICE_GENERATOR_MAX_ATTEMPTS", "-1")
	t.Setenv("VOICE_GENERATOR_BACKOFF", "1s,bad")
	cfg := LoadConfigFromEnv()
	if cfg.TTSTimeout != 120*time.Second {
		t.Errorf("tts timeout = %s, want 120s", cfg.TTSTimeout)
	}
	if cfg.ASRTimeout != 60*time.Second {
		t.Errorf("asr timeout = %s, want 60s", cfg.ASRTimeout)
	}
	if cfg.MaxAttempts != 3 {
		t.Errorf("max attempts = %d, want 3", cfg.MaxAttempts)
	}
	if len(cfg.Backoff) != 3 {
		t.Errorf("backoff = %v, want 3 entries", cfg.Backoff)
	}
}
