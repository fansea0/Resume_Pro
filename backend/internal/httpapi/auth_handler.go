package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/auth"
)

type authHandler struct {
	service      *auth.Service
	cookieSecure bool
}

type credentialsInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type developmentLoginInput struct {
	Email string `json:"email"`
}

type publicUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	TenantID int64  `json:"tenantId"`
}

func newAuthHandler(service *auth.Service, cookieSecure bool) *authHandler {
	return &authHandler{service: service, cookieSecure: cookieSecure}
}

func (h *authHandler) register(context *gin.Context) {
	noStore(context)
	input, ok := decodeCredentials(context)
	if !ok {
		return
	}
	user, err := h.service.Register(context.Request.Context(), auth.CreateUserInput{Email: input.Email, Password: input.Password})
	if errors.Is(err, auth.ErrInvalidRegistration) {
		writeError(context, http.StatusBadRequest, "valid email and password of at least 12 characters are required")
		return
	}
	if errors.Is(err, auth.ErrEmailAlreadyRegistered) {
		writeError(context, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not register account")
		return
	}
	h.writeAuthenticated(context, http.StatusCreated, user)
}

func (h *authHandler) login(context *gin.Context) {
	noStore(context)
	input, ok := decodeCredentials(context)
	if !ok {
		return
	}
	user, err := h.service.Login(context.Request.Context(), input.Email, input.Password)
	if err != nil {
		writeError(context, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.writeAuthenticated(context, http.StatusOK, user)
}

func (h *authHandler) developmentLogin(context *gin.Context) {
	noStore(context)
	if !isLoopbackRequest(context.Request) {
		writeError(context, http.StatusForbidden, "development login requires a loopback request")
		return
	}
	input, ok := decodeDevelopmentLogin(context)
	if !ok {
		return
	}
	user, err := h.service.FindUserByEmail(context.Request.Context(), input.Email)
	if err != nil {
		writeError(context, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.writeAuthenticated(context, http.StatusOK, user)
}

func (h *authHandler) logout(context *gin.Context) {
	noStore(context)
	h.setCookie(context, "", -1, time.Unix(1, 0))
	context.Status(http.StatusNoContent)
}

func (h *authHandler) me(context *gin.Context) {
	noStore(context)
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(context, http.StatusOK, map[string]publicUser{"user": {ID: principal.UserID, Email: principal.Email, TenantID: principal.TenantID}})
}

func (h *authHandler) writeAuthenticated(context *gin.Context, status int, user auth.User) {
	token, err := h.service.IssueToken(user)
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not start session")
		return
	}
	h.setCookie(context, token, int((7 * 24 * time.Hour).Seconds()), time.Now().Add(7*24*time.Hour))
	writeJSON(context, status, map[string]publicUser{"user": {ID: user.ID, Email: user.Email, TenantID: user.TenantID}})
}

func (h *authHandler) setCookie(context *gin.Context, value string, maxAge int, expires time.Time) {
	http.SetCookie(context.Writer, &http.Cookie{Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: expires})
}

func decodeCredentials(context *gin.Context) (credentialsInput, bool) {
	decoder := json.NewDecoder(context.Request.Body)
	decoder.DisallowUnknownFields()
	var input credentialsInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(context, http.StatusBadRequest, "email and password are required")
		return credentialsInput{}, false
	}
	return input, true
}

func decodeDevelopmentLogin(context *gin.Context) (developmentLoginInput, bool) {
	decoder := json.NewDecoder(context.Request.Body)
	decoder.DisallowUnknownFields()
	var input developmentLoginInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !auth.ValidEmail(input.Email) {
		writeError(context, http.StatusBadRequest, "a valid email is required")
		return developmentLoginInput{}, false
	}
	return input, true
}

func isLoopbackRequest(request *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func noStore(context *gin.Context) { context.Header("Cache-Control", "no-store") }
