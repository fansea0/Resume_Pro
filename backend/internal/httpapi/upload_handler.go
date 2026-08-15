package httpapi

import (
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"

	"resumepro/backend/internal/files"
)

const (
	maxAvatarUploadBytes = 5 << 20 // 5 MiB, including multipart envelope.
	maxAvatarPixels      = 4_000_000
)

type uploadHandler struct{ storage files.FileStorage }

func newUploadHandler(storage files.FileStorage) *uploadHandler {
	return &uploadHandler{storage: storage}
}

func (h *uploadHandler) avatar(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxAvatarUploadBytes)
	fileHeader, err := context.FormFile("avatar")
	if err != nil {
		writeAvatarUploadError(context, err)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(context, http.StatusBadRequest, "avatar image is required")
		return
	}
	defer file.Close()
	parsedKey, err := uuid.Parse(context.PostForm("key"))
	if err != nil {
		writeError(context, http.StatusBadRequest, "avatar key must be a UUID")
		return
	}
	key := parsedKey.String()

	filename, err := validatedAvatarName(file)
	if err != nil {
		writeError(context, http.StatusBadRequest, "avatar must be a valid PNG, JPEG, or WebP image")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(context, http.StatusBadRequest, "avatar image could not be read")
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	stored, err := h.storage.Store(context.Request.Context(), "user-"+strconv.FormatInt(principal.UserID, 10), key, files.File{Name: filename, Content: file})
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not store avatar image")
		return
	}
	writeJSON(context, http.StatusCreated, map[string]string{"url": stored.URL})
}

func validatedAvatarName(file io.ReadSeeker) (string, error) {
	config, format, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxAvatarPixels {
		return "", errors.New("invalid avatar dimensions")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if _, _, err := image.Decode(file); err != nil {
		return "", err
	}
	switch format {
	case "png":
		return "avatar.png", nil
	case "jpeg":
		return "avatar.jpg", nil
	case "webp":
		return "avatar.webp", nil
	default:
		return "", fmt.Errorf("unsupported image format %q", format)
	}
}

func writeAvatarUploadError(context *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(context, http.StatusRequestEntityTooLarge, "avatar image must be 5 MiB or smaller")
		return
	}
	writeError(context, http.StatusBadRequest, "avatar image is required")
}
