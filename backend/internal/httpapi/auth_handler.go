package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

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

func (h *authHandler) register(writer http.ResponseWriter, request *http.Request) {
	noStore(writer)
	input, ok := decodeCredentials(writer, request)
	if !ok {
		return
	}
	user, err := h.service.Register(request.Context(), auth.CreateUserInput{Email: input.Email, Password: input.Password})
	if errors.Is(err, auth.ErrInvalidRegistration) {
		writeError(writer, http.StatusBadRequest, "valid email and password of at least 12 characters are required")
		return
	}
	if errors.Is(err, auth.ErrEmailAlreadyRegistered) {
		writeError(writer, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not register account")
		return
	}
	h.writeAuthenticated(writer, http.StatusCreated, user)
}

func (h *authHandler) login(writer http.ResponseWriter, request *http.Request) {
	noStore(writer)
	input, ok := decodeCredentials(writer, request)
	if !ok {
		return
	}
	user, err := h.service.Login(request.Context(), input.Email, input.Password)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.writeAuthenticated(writer, http.StatusOK, user)
}

func (h *authHandler) developmentLogin(writer http.ResponseWriter, request *http.Request) {
	noStore(writer)
	if !isLoopbackRequest(request) {
		writeError(writer, http.StatusForbidden, "development login requires a loopback request")
		return
	}
	input, ok := decodeDevelopmentLogin(writer, request)
	if !ok {
		return
	}
	user, err := h.service.FindUserByEmail(request.Context(), input.Email)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.writeAuthenticated(writer, http.StatusOK, user)
}

func (h *authHandler) logout(writer http.ResponseWriter, _ *http.Request) {
	noStore(writer)
	h.setCookie(writer, "", -1, time.Unix(1, 0))
	writer.WriteHeader(http.StatusNoContent)
}

func (h *authHandler) me(writer http.ResponseWriter, request *http.Request) {
	noStore(writer)
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]publicUser{"user": {ID: principal.UserID, Email: principal.Email, TenantID: principal.TenantID}})
}

func (h *authHandler) writeAuthenticated(writer http.ResponseWriter, status int, user auth.User) {
	token, err := h.service.IssueToken(user)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not start session")
		return
	}
	h.setCookie(writer, token, int((7 * 24 * time.Hour).Seconds()), time.Now().Add(7*24*time.Hour))
	writeJSON(writer, status, map[string]publicUser{"user": {ID: user.ID, Email: user.Email, TenantID: user.TenantID}})
}

func (h *authHandler) setCookie(writer http.ResponseWriter, value string, maxAge int, expires time.Time) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: expires})
}

func decodeCredentials(writer http.ResponseWriter, request *http.Request) (credentialsInput, bool) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input credentialsInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(writer, http.StatusBadRequest, "email and password are required")
		return credentialsInput{}, false
	}
	return input, true
}

func decodeDevelopmentLogin(writer http.ResponseWriter, request *http.Request) (developmentLoginInput, bool) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input developmentLoginInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !auth.ValidEmail(input.Email) {
		writeError(writer, http.StatusBadRequest, "a valid email is required")
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

func noStore(writer http.ResponseWriter) { writer.Header().Set("Cache-Control", "no-store") }
