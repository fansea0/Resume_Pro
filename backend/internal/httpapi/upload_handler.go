package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
)

const (
	maxAvatarUploadBytes  = 5 << 20 // 5 MiB, including multipart envelope.
	maxAvatarPixels       = 100_000_000
	maxStoredAvatarPixels = 4_000_000
)

type uploadHandler struct {
	storage    files.FileStorage
	repository *resume.Repository
}

func newUploadHandler(storage files.FileStorage, repository *resume.Repository) *uploadHandler {
	return &uploadHandler{storage: storage, repository: repository}
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
		writeError(context, http.StatusBadRequest, "avatar image is required", err)
		return
	}
	defer file.Close()
	parsedKey, err := uuid.Parse(context.PostForm("key"))
	if err != nil {
		writeError(context, http.StatusBadRequest, "avatar key must be a UUID", err)
		return
	}
	key := parsedKey.String()

	prepared, err := prepareAvatar(file)
	if err != nil {
		writeError(context, http.StatusBadRequest, "avatar must be a valid PNG, JPEG, or WebP image", err)
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	resumeID, err := optionalResumeID(context.PostForm("resumeId"))
	if err != nil {
		writeError(context, http.StatusBadRequest, "resume ID must be a positive integer", err)
		return
	}
	if err := h.repository.ReserveAvatarUpload(context.Request.Context(), principal.TenantID, key, avatarOwner(principal), resumeID); err != nil {
		if errors.Is(err, resume.ErrAvatarKeyInUse) || errors.Is(err, resume.ErrAvatarKeyReserved) {
			writeError(context, http.StatusConflict, "avatar image belongs to another resume")
			return
		}
		writeError(context, http.StatusInternalServerError, "could not reserve avatar image", err)
		return
	}
	stored, err := h.storage.Store(context.Request.Context(), "user-"+strconv.FormatInt(principal.UserID, 10), key, prepared)
	if err != nil {
		if errors.Is(err, files.ErrAvatarQuotaExceeded) {
			writeError(context, http.StatusTooManyRequests, "avatar upload quota exceeded")
			return
		}
		writeError(context, http.StatusInternalServerError, "could not store avatar image", err)
		return
	}
	writeJSON(context, http.StatusCreated, map[string]string{"url": stored.URL})
}

// optionalResumeID accepts the saved resume identity when an existing avatar
// is replaced, while keeping uploads for unsaved drafts possible.
func optionalResumeID(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid resume ID")
	}
	return id, nil
}

func prepareAvatar(file io.ReadSeeker) (files.File, error) {
	config, format, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxAvatarPixels {
		return files.File{}, errors.New("invalid avatar dimensions")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return files.File{}, err
	}
	decoded, _, err := image.Decode(file)
	if err != nil {
		return files.File{}, err
	}
	if int64(config.Width)*int64(config.Height) > maxStoredAvatarPixels {
		scale := math.Sqrt(float64(maxStoredAvatarPixels) / float64(config.Width*config.Height))
		width := max(1, int(math.Round(float64(config.Width)*scale)))
		height := max(1, int(math.Round(float64(config.Height)*scale)))
		resized := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
		var compressed bytes.Buffer
		if err := jpeg.Encode(&compressed, resized, &jpeg.Options{Quality: 90}); err != nil {
			return files.File{}, err
		}
		return files.File{Name: "avatar.jpg", Content: bytes.NewReader(compressed.Bytes())}, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return files.File{}, err
	}
	switch format {
	case "png":
		return files.File{Name: "avatar.png", Content: file}, nil
	case "jpeg", "jpg":
		return files.File{Name: "avatar.jpg", Content: file}, nil
	case "webp":
		return files.File{Name: "avatar.webp", Content: file}, nil
	default:
		return files.File{}, fmt.Errorf("unsupported image format %q", format)
	}
}

func writeAvatarUploadError(context *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(context, http.StatusRequestEntityTooLarge, "avatar image must be 5 MiB or smaller", err)
		return
	}
	writeError(context, http.StatusBadRequest, "avatar image is required", err)
}
