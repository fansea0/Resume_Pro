package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
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
	logger := logrus.New()
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
	logger.WithFields(logrus.Fields{
		"address":       ":8080",
		"cookie_secure": cookieSecure,
		"database_path": databasePath,
	}).Info("starting resume API server")

	routerConfig, err := buildRouterConfig(db, resumeRepository, logger, serverOptions{
		JWTSecret:       jwtSecret,
		CookieSecure:    cookieSecure,
		AppEnv:          os.Getenv("APP_ENV"),
		UploadDirectory: uploadDirectory,
	}, productionAIBootstrap)
	if err != nil {
		logger.WithError(err).Fatal("configure resume API server")
	}

	if err := httpapi.NewRouter(authRepository, resumeRepository, templates.NewRepository(db), routerConfig).Run(":8080"); err != nil {
		logger.WithError(err).Fatal("resume API server stopped")
	}
}

// loadLocalEnvFile loads a local .env file so that secrets and AI settings do
// not have to be exported in every shell. Variables already present in the
// environment always win, so `export OPENAI_API_KEY=...` still overrides it.
func loadLocalEnvFile(logger *logrus.Logger) {
	paths, err := envfile.LoadFromPaths([]string{".", ".."})
	if err != nil {
		logger.WithError(err).Warn("load local .env file")
		return
	}
	if paths != "" {
		logger.WithField("path", paths).Info("loaded local .env file")
	}
}

func buildRouterConfig(db *sqlx.DB, resumeRepository *resume.Repository, logger *logrus.Logger, options serverOptions, bootstrap aiBootstrap) (httpapi.RouterConfig, error) {
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
			return config, fmt.Errorf("AI features required in production (set OPENAI_API_KEY): %w", err)
		}
		logger.WithError(err).Warn("AI features disabled (set OPENAI_API_KEY to enable)")
		return config, nil
	}
	chatModel, err := bootstrap.NewChatModel(llmConfig)
	if err != nil {
		return config, fmt.Errorf("create LLM client: %w", err)
	}
	config.AIService = ai.NewService(
		chatModel,
		ai.NewLogrusHooks(logger),
		ai.WithMaxRetries(llmConfig.MaxRetries),
	)
	logger.WithField("model", llmConfig.Model).Info("AI features enabled")

	interviewService, err := buildInterviewService(db, llmConfig, logger, options.AppEnv)
	if err != nil {
		return config, err
	}
	config.InterviewService = interviewService
	return config, nil
}

// buildInterviewService 接线 AI 模拟面试：Redis checkpoint store + Responses 模型
// + 生题器 + 面试图 + Service。
//
// Redis 或 Responses 模型不可用时：生产环境直接返回错误（面试是必开功能）；
// 非生产环境打印告警并返回 nil（路由仍注册，接口统一返回 503，与 AI 功能降级一致）。
func buildInterviewService(db *sqlx.DB, llmConfig llm.Config, logger *logrus.Logger, appEnv string) (*interview.Service, error) {
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

	// 生题一次输出 N 道题（12 题约 5~7k token），单题评分也偶发 50s+（含思考）；
	// 两者都用放宽后的超时，避免慢响应被当成失败。
	interviewConfig := llmConfig
	interviewConfig.Timeout = 150 * time.Second
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
	logger.WithField("redis", redisAddr).Info("AI interview enabled")
	return interview.NewService(repo, redisStore, gen, graph), nil
}
