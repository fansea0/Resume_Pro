package main

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"resumepro/backend/internal/httpapi"
)

func main() {
	logger := logrus.New()
	logger.WithField("address", ":8080").Info("starting resume API server")

	if err := http.ListenAndServe(":8080", httpapi.NewRouter()); err != nil {
		logger.WithError(err).Fatal("resume API server stopped")
	}
}
