package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
)

const minimumJWTSecretLength = 32

func main() {
	logger := logrus.New()
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < minimumJWTSecretLength {
		logger.Fatalf("JWT_SECRET must be at least %d characters", minimumJWTSecretLength)
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
	logger.WithFields(logrus.Fields{
		"address":       ":8080",
		"cookie_secure": cookieSecure,
		"database_path": databasePath,
	}).Info("starting resume API server")

	if err := http.ListenAndServe(":8080", httpapi.NewRouter(auth.NewRepository(db), resume.NewRepository(db), httpapi.RouterConfig{JWTSecret: jwtSecret, CookieSecure: cookieSecure})); err != nil {
		logger.WithError(err).Fatal("resume API server stopped")
	}
}
