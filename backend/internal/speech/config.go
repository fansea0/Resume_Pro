package speech

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是语音能力的运行配置。TTS 与 ASR 各自独立，互不影响。
type Config struct {
	// Enabled 为 false 时不创建 provider，路由仍注册并返回 503。
	Enabled bool

	TTSBaseURL string
	TTSAPIKey  string
	TTSModel   string
	TTSVoice   string
	TTSTimeout time.Duration

	ASRBaseURL string
	ASRAPIKey  string
	ASRModel   string
	ASRTimeout time.Duration

	// CorrectTimeout 是错别字修复的单次调用超时。
	CorrectTimeout time.Duration

	// MaxAttempts 是单题生成的尝试次数（含首次）。
	MaxAttempts int
	// Backoff 是失败重试的退避序列。
	Backoff []time.Duration

	// CacheDir 是题目音频的根目录。
	CacheDir string
}

// LoadConfigFromEnv 从环境变量读配置。语音功能默认关闭（opt-in），
// 避免未安装 mlx-audio 的部署启动即报错。
func LoadConfigFromEnv() Config {
	return Config{
		Enabled:        envBool("VOICE_ENABLED", false),
		TTSBaseURL:     envOr("VOICE_TTS_BASE_URL", "http://127.0.0.1:21100/v1"),
		TTSAPIKey:      os.Getenv("VOICE_TTS_API_KEY"),
		TTSModel:       envOr("VOICE_TTS_MODEL", "mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit"),
		TTSVoice:       envOr("VOICE_TTS_VOICE", "Vivian"),
		TTSTimeout:     envDuration("VOICE_TTS_TIMEOUT", 120*time.Second),
		ASRBaseURL:     envOr("VOICE_ASR_BASE_URL", "http://127.0.0.1:21100/v1"),
		ASRAPIKey:      os.Getenv("VOICE_ASR_API_KEY"),
		ASRModel:       envOr("VOICE_ASR_MODEL", "mlx-community/Qwen3-ASR-1.7B-8bit"),
		ASRTimeout:     envDuration("VOICE_ASR_TIMEOUT", 60*time.Second),
		CorrectTimeout: envDuration("VOICE_CORRECT_TIMEOUT", 30*time.Second),
		MaxAttempts:    envInt("VOICE_GENERATOR_MAX_ATTEMPTS", 3),
		Backoff:        envDurations("VOICE_GENERATOR_BACKOFF", []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}),
		CacheDir:       envOr("VOICE_CACHE_DIR", "./uploads/voice"),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && value > 0 {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key))); err == nil && value > 0 {
		return value
	}
	return fallback
}

func envDurations(key string, fallback []time.Duration) []time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var out []time.Duration
	for _, part := range strings.Split(raw, ",") {
		value, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return fallback
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
