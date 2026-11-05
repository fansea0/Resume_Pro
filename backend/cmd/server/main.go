package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/envfile"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/llm"
	"resumepro/backend/internal/logging"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/speech"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

const minimumJWTSecretLength = 32
const localDevelopmentJWTSecret = "resume-pro-local-development-secret-change-this-value"

type serverOptions struct {
	JWTSecret       string
	CookieSecure    bool
	AppEnv          string
	UploadDirectory string
}

type aiBootstrap struct {
	LoadConfig   func() (llm.Config, error)
	NewChatModel func(llm.Config) (model.ChatModel, error)
}

var productionAIBootstrap = aiBootstrap{
	LoadConfig:   llm.LoadConfigFromEnv,
	NewChatModel: llm.NewChatModel,
}

func main() {
	logger := logging.NewLogrus(logrus.New())
	loadLocalEnvFile(logger)
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < minimumJWTSecretLength {
		if os.Getenv("APP_ENV") == "production" {
			logger.Fatalf("JWT_SECRET must be at least %d characters in production", minimumJWTSecretLength)
		}
		jwtSecret = localDevelopmentJWTSecret
		logger.Warn("JWT_SECRET is unset or too short; using the local development secret")
	}
	cookieSecure := os.Getenv("COOKIE_SECURE") == "true"
	if !cookieSecure {
		logger.Warn("COOKIE_SECURE is disabled; use COOKIE_SECURE=true behind HTTPS in production")
	}
	databasePath := os.Getenv("RESUME_DB_PATH")
	if databasePath == "" {
		databasePath = "./resume.db"
	}
	db, err := store.OpenSQLite(databasePath)
	if err != nil {
		logger.WithError(err).Fatal("open resume database")
	}
	defer db.Close()
	if err := store.Migrate(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("migrate resume database")
	}
	authRepository := auth.NewRepository(db)
	resumeRepository := resume.NewRepository(db)
	uploadDirectory := os.Getenv("UPLOAD_DIR")
	if uploadDirectory == "" {
		uploadDirectory = "./uploads"
	}
	logger.WithFields(logging.Fields{
		"address":       ":8080",
		"cookie_secure": cookieSecure,
		"database_path": databasePath,
	}).Info("starting resume API server")

	routerConfig, voiceGenerator, err := buildRouterConfig(db, resumeRepository, logger, serverOptions{
		JWTSecret:       jwtSecret,
		CookieSecure:    cookieSecure,
		AppEnv:          os.Getenv("APP_ENV"),
		UploadDirectory: uploadDirectory,
	}, productionAIBootstrap)
	if err != nil {
		logger.WithError(err).Fatal("configure resume API server")
	}

	server := &http.Server{
		Addr:              ":8080",
		Handler:           httpapi.NewRouter(authRepository, resumeRepository, templates.NewRepository(db), routerConfig),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// 优雅退出：收到 SIGINT/SIGTERM 时先关 HTTP，再停语音 worker。
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		graceful, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(graceful); err != nil {
			logger.WithError(err).Warn("http server shutdown failed")
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.WithError(err).Error("resume API server stopped")
	}
	// 语音 worker 收尾：取消在途 TTS 并限时等待 worker 退出。
	if voiceGenerator != nil {
		voiceGenerator.Close()
	}
}

// loadLocalEnvFile loads a local .env file so that secrets and AI settings do
// not have to be exported in every shell. Variables already present in the
// environment always win, so `export OPENAI_API_KEY=...` still overrides it.
func loadLocalEnvFile(logger logging.Logger) {
	paths, err := envfile.LoadFromPaths([]string{".", ".."})
	if err != nil {
		logger.WithError(err).Warn("load local .env file")
		return
	}
	if paths != "" {
		logger.WithFields(logging.Fields{"path": paths}).Info("loaded local .env file")
	}
}

// buildRouterConfig 装配 HTTP 路由配置。
//
// 第二个返回值是语音生成器（非 nil 表示语音已启用）：由 main 在进程退出时关闭，
// 避免在途 TTS 任务被硬杀。
func buildRouterConfig(db *sqlx.DB, resumeRepository *resume.Repository, logger logging.Logger, options serverOptions, bootstrap aiBootstrap) (httpapi.RouterConfig, *speech.Generator, error) {
	avatarDirectory := filepath.Join(options.UploadDirectory, "avatars")
	fileStorage := files.NewLocalFileStorage(avatarDirectory)
	targetJobService := targetjob.NewService(targetjob.NewRepository(db))
	config := httpapi.RouterConfig{
		JWTSecret:        options.JWTSecret,
		CookieSecure:     options.CookieSecure,
		EnableDevLogin:   options.AppEnv == "development",
		FileStorage:      fileStorage,
		AvatarDirectory:  avatarDirectory,
		TargetJobService: targetJobService,
		ResumesvcService: resumesvc.NewService(resumeRepository, fileStorage, targetJobService),
	}

	llmConfig, err := bootstrap.LoadConfig()
	if err != nil {
		if options.AppEnv == "production" {
			return config, nil, fmt.Errorf("AI features required in production (set OPENAI_API_KEY): %w", err)
		}
		logger.WithError(err).Warn("AI features disabled (set OPENAI_API_KEY to enable)")
		return config, nil, nil
	}
	chatModel, err := bootstrap.NewChatModel(llmConfig)
	if err != nil {
		return config, nil, fmt.Errorf("create LLM client: %w", err)
	}
	config.AIService = ai.NewService(
		chatModel,
		ai.NewHooks(logger),
		ai.WithMaxRetries(llmConfig.MaxRetries),
	)
	logger.WithFields(logging.Fields{"model": llmConfig.Model}).Info("AI features enabled")

	// 先建语音：生成器要注入面试服务，否则题目永远不会被入队。
	voiceService, voiceGenerator, err := buildVoiceServices(db, llmConfig, logger)
	if err != nil {
		// 语音装配失败不阻断面试主流程：按「语音未启用」继续（接口返回 503）。
		logger.WithError(err).Error("voice features disabled (assembly failed)")
		voiceService, voiceGenerator = nil, nil
	}
	var audioHooks interview.AudioHooks
	if voiceGenerator != nil {
		audioHooks = voiceGenerator
		config.VoiceService = voiceService
		// 重启重载：先把上次崩溃残留的 pending 归零，再扫描未就绪的题重新入队。
		// 必须在 Start 之前：worker 一旦开始消费，pending 就可能属于在途任务，
		// 此时归零会导致重复合成。Generator 用 ErrReloadAfterStart 挡住误用。
		if err := voiceGenerator.Reload(context.Background()); err != nil {
			logger.WithError(err).Warn("voice: reload pending question audio failed")
		}
		// 后台生成 worker：串行消费，不阻塞任何请求。
		voiceGenerator.Start(context.Background())
		logger.WithFields(logging.Fields{"cache_dir": voiceService.Store.Root()}).Info("voice features enabled")
	}

	interviewService, err := buildInterviewService(db, llmConfig, logger, options.AppEnv, audioHooks)
	if err != nil {
		return config, nil, err
	}
	config.InterviewService = interviewService
	return config, voiceGenerator, nil
}

// buildInterviewService 接线 AI 模拟面试：Redis checkpoint store + Responses 模型
// + 生题器 + 面试图 + Service。
//
// Redis 或 Responses 模型不可用时：生产环境直接返回错误（面试是必开功能）；
// 非生产环境打印告警并返回 nil（路由仍注册，接口统一返回 503，与 AI 功能降级一致）。
// audioHooks 为 nil 时语音链路整体缺席，面试照常工作。
func buildInterviewService(db *sqlx.DB, llmConfig llm.Config, logger logging.Logger, appEnv string, audioHooks interview.AudioHooks) (*interview.Service, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	redisDB := 0
	if raw := os.Getenv("REDIS_DB"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			redisDB = parsed
		}
	}
	redisStore, err := interview.NewRedisStore(redisAddr, os.Getenv("REDIS_PASSWORD"), redisDB)
	if err != nil {
		if appEnv == "production" {
			return nil, fmt.Errorf("interview requires Redis: %w", err)
		}
		logger.WithError(err).Warn("AI interview disabled (Redis unavailable)")
		return nil, nil
	}

	// 生题一次输出 N 道题（12 题正文约 2k token），评分单次秒级。
	// 思考档位修正为 reasoning.effort 后不再需要为隐藏思考预留 150s，留 90s 兜底
	// 高推理档位与网络抖动即可。
	interviewConfig := llmConfig
	interviewConfig.Timeout = 90 * time.Second
	questionModel, err := llm.NewResponsesModel(interviewConfig)
	if err != nil {
		if appEnv == "production" {
			return nil, fmt.Errorf("interview requires responses model: %w", err)
		}
		logger.WithError(err).Warn("AI interview disabled (responses model unavailable)")
		return nil, nil
	}
	responsesModel, err := llm.NewResponsesModel(interviewConfig)
	if err != nil {
		if appEnv == "production" {
			return nil, fmt.Errorf("interview requires responses model: %w", err)
		}
		logger.WithError(err).Warn("AI interview disabled (responses model unavailable)")
		return nil, nil
	}

	repo := interview.NewRepository(db)
	gen := interview.NewQuestionGenerator(questionModel, llmConfig.MaxRetries)
	graph, err := interview.BuildGraph(repo, responsesModel, redisStore)
	if err != nil {
		return nil, fmt.Errorf("build interview graph: %w", err)
	}
	logger.WithFields(logging.Fields{"redis": redisAddr}).Info("AI interview enabled")
	return interview.NewService(repo, redisStore, gen, graph, interview.WithAudioHooks(audioHooks)), nil
}

// buildVoiceServices 装配语音能力：provider、存储、生成器、错别字修复。
//
// VOICE_ENABLED 未开时返回 (nil, nil, nil)：语音链路整体缺席，
// 路由仍然注册，接口返回 503，面试照常工作。
func buildVoiceServices(db *sqlx.DB, llmConfig llm.Config, logger logging.Logger) (*speech.VoiceService, *speech.Generator, error) {
	cfg := speech.LoadConfigFromEnv()
	if !cfg.Enabled {
		logger.Warn("voice features disabled (set VOICE_ENABLED=true to enable)")
		return nil, nil, nil
	}
	provider := speech.NewOpenAICompatibleProvider(cfg)
	store := speech.NewAudioStore(cfg.CacheDir)

	// 错别字修复用独立模型实例：思考强度固定最低档（纯校对任务不需要长思考）。
	correctConfig := llmConfig
	correctConfig.ReasoningEffort = "low"
	correctConfig.Timeout = cfg.CorrectTimeout
	correctModel, err := llm.NewResponsesModel(correctConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create correction model: %w", err)
	}

	generator := speech.NewGenerator(provider, store, interview.NewRepository(db), cfg)
	corrector := speech.NewCorrector(correctModel, cfg.CorrectTimeout)
	return speech.NewVoiceService(store, provider, corrector), generator, nil
}
