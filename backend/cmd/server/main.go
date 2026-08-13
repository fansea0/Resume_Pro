package main

import (
	"context"
	"net/http"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
)

func main() {
	logger := logrus.New()
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
	logger.WithField("address", ":8080").Info("starting resume API server")

	if err := http.ListenAndServe(":8080", httpapi.NewRouter(resume.NewRepository(db))); err != nil {
		logger.WithError(err).Fatal("resume API server stopped")
	}
}
