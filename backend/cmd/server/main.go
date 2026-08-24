package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/httpapi"
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
	return config, nil
}
