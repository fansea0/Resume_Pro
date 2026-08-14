package main

import (
	"context"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

const minimumJWTSecretLength = 32
const localDevelopmentJWTSecret = "resume-pro-local-development-secret-change-this-value"

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
	db, err := sqlx.Open("sqlite", databasePath)
	if err != nil {
		logger.WithError(err).Fatal("open resume database")
	}
	defer db.Close()
	if err := store.Migrate(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("migrate resume database")
	}
	authRepository := auth.NewRepository(db)
	logger.WithFields(logrus.Fields{
		"address":       ":8080",
		"cookie_secure": cookieSecure,
		"database_path": databasePath,
	}).Info("starting resume API server")

	if err := httpapi.NewRouter(authRepository, resume.NewRepository(db), templates.NewRepository(db), httpapi.RouterConfig{JWTSecret: jwtSecret, CookieSecure: cookieSecure, EnableDevLogin: os.Getenv("APP_ENV") == "development"}).Run(":8080"); err != nil {
		logger.WithError(err).Fatal("resume API server stopped")
	}
}
