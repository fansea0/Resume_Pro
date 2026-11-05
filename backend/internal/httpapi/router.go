package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/speech"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

type RouterConfig struct {
	JWTSecret      string
	CookieSecure   bool
	EnableDevLogin bool
	// FileStorage is optional so deployments can opt into avatar uploads only
	// after configuring a durable storage backend.
	FileStorage files.FileStorage
	// AvatarDirectory is the local public directory mounted at /uploads/avatars.
	// It is ignored unless explicitly configured.
	AvatarDirectory string
	// TargetJobService is optional until target-job management is enabled.
	TargetJobService *targetjob.Service
	// ResumesvcService owns resume mutation business logic.
	ResumesvcService *resumesvc.Service
	// AIService is nil when AI features are disabled. The authenticated routes
	// remain registered so callers receive an explicit 503 response.
	AIService *ai.Service
	// InterviewService is nil when Redis or the Responses model is unavailable;
	// the authenticated routes remain registered and return an explicit 503.
	InterviewService *interview.Service
	// VoiceService is nil when voice features are disabled (VOICE_ENABLED 未开)
	// or the speech provider is unavailable; the authenticated routes remain
	// registered and return an explicit 503.
	VoiceService *speech.VoiceService
}

var avatarFilenamePattern = regexp.MustCompile(`^[a-f0-9]{64}-[a-f0-9]{64}\.avatar$`)

// NewRouter creates the Gin HTTP API router.
func NewRouter(authRepository *auth.Repository, resumeRepository *resume.Repository, templateRepository *templates.Repository, config RouterConfig) *gin.Engine {
	router := gin.New()
	// multipart 解析的内存阈值压到音频上限：超过上限的部分本来就会被拒绝，
	// 不该先落成磁盘临时文件（用户录音不落盘是硬约束）。
	router.MaxMultipartMemory = speech.MaxAudioBytes
	router.GET("/api/health", func(context *gin.Context) {
		writeJSON(context, http.StatusOK, map[string]string{"status": "ok"})
	})
	if config.AvatarDirectory != "" {
		router.GET("/uploads/avatars/:filename", serveAvatarFile(config.AvatarDirectory))
	}
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
	if config.FileStorage != nil {
		uploadHandler := newUploadHandler(config.FileStorage, resumeRepository)
		router.POST("/api/uploads/avatar", requireAuthentication(service), uploadHandler.avatar)
	}

	resumeHandler := newResumeHandler(config.ResumesvcService, resumeRepository, config.FileStorage)
	protected := router.Group("/api/resumes")
	protected.Use(requireAuthentication(service))
	protected.GET("", resumeHandler.list)
	protected.POST("", resumeHandler.create)
	protected.GET("/:id", resumeHandler.get)
	protected.PUT("/:id", resumeHandler.update)
	protected.DELETE("/:id", resumeHandler.delete)

	templateHandler := newTemplateHandler(templateRepository)
	router.GET("/api/templates", templateHandler.listDisabled)
	admin := router.Group("/api/admin/templates")
	admin.Use(requireAuthentication(service), requireAdministrator(authRepository))
	admin.GET("", templateHandler.listDisabled)
	admin.DELETE("/:templateID", templateHandler.disable)

	if config.TargetJobService != nil {
		targetHandler := newTargetJobHandler(config.TargetJobService)
		targetJobs := router.Group("/api/target-jobs")
		targetJobs.Use(requireAuthentication(service))
		targetJobs.GET("", targetHandler.list)
		targetJobs.POST("", targetHandler.create)
		targetJobs.GET("/:id", targetHandler.get)
		targetJobs.PUT("/:id", targetHandler.update)
		targetJobs.DELETE("/:id", targetHandler.delete)
	}

	aiHandler := newAIHandler(config.AIService, resumeRepository, config.TargetJobService)
	aiRoutes := router.Group("/api/ai")
	aiRoutes.Use(requireAuthentication(service))
	aiRoutes.POST("/resumes/generate", aiHandler.generateResume)
	aiRoutes.POST("/resumes/:id/section/rewrite", aiHandler.rewriteSection)

	voiceEnabled := config.VoiceService != nil
	interviewHandler := newInterviewHandler(config.InterviewService, resumeRepository, config.TargetJobService, voiceEnabled)
	interviews := router.Group("/api/interviews")
	interviews.Use(requireAuthentication(service))
	interviews.POST("", interviewHandler.create)
	interviews.GET("", interviewHandler.list)
	interviews.GET("/:id", interviewHandler.get)
	interviews.POST("/:id/answer", interviewHandler.answer)
	interviews.GET("/:id/questions", interviewHandler.questions)
	interviews.POST("/:id/abort", interviewHandler.abort)

	// 语音路由无条件注册：语音未启用或依赖缺失时由 handler 返回 503，
	// 而不是让调用方拿到「路由不存在」的 404。
	voiceHandler := newVoiceHandler(config.InterviewService, config.VoiceService)
	interviews.GET("/:id/questions/:qid/audio", voiceHandler.audio)
	interviews.POST("/:id/questions/:qid/transcribe", voiceHandler.transcribe)

	return router
}

func serveAvatarFile(directory string) gin.HandlerFunc {
	return func(context *gin.Context) {
		filename := context.Param("filename")
		if filename == "" || filename != filepath.Base(filename) || !avatarFilenamePattern.MatchString(filename) {
			context.Status(http.StatusNotFound)
			return
		}
		path := filepath.Join(directory, filename)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			context.Status(http.StatusNotFound)
			return
		}
		http.ServeFile(context.Writer, context.Request, path)
	}
}
