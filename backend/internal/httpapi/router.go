package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/templates"
)

type RouterConfig struct {
	JWTSecret      string
	CookieSecure   bool
	EnableDevLogin bool
}

// NewRouter creates the Gin HTTP API router.
func NewRouter(authRepository *auth.Repository, resumeRepository *resume.Repository, templateRepository *templates.Repository, config RouterConfig) *gin.Engine {
	router := gin.New()
	router.GET("/api/health", func(context *gin.Context) {
		writeJSON(context, http.StatusOK, map[string]string{"status": "ok"})
	})
	if authRepository == nil || resumeRepository == nil || templateRepository == nil {
		return router
	}

	service, err := auth.NewService(authRepository, config.JWTSecret)
	if err != nil {
		panic(err)
	}
	authHandler := newAuthHandler(service, config.CookieSecure)
	router.POST("/api/auth/register", authHandler.register)
	router.POST("/api/auth/login", authHandler.login)
	router.POST("/api/auth/logout", authHandler.logout)
	if config.EnableDevLogin {
		router.POST("/api/dev/login", authHandler.developmentLogin)
	}
	router.GET("/api/auth/me", requireAuthentication(service), authHandler.me)

	resumeHandler := newResumeHandler(resumeRepository)
	protected := router.Group("/api/resumes")
	protected.Use(requireAuthentication(service))
	protected.GET("", resumeHandler.list)
	protected.POST("", resumeHandler.create)
	protected.GET("/:id", resumeHandler.get)
	protected.PUT("/:id", resumeHandler.update)
	protected.DELETE("/:id", resumeHandler.delete)

	templateHandler := newTemplateHandler(templateRepository)
	admin := router.Group("/api/admin/templates")
	admin.Use(requireAuthentication(service), requireAdministrator())
	admin.GET("", templateHandler.listDisabled)
	admin.DELETE("/:templateID", templateHandler.disable)

	return router
}
