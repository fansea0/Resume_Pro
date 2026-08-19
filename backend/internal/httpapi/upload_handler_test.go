package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

func TestAvatarUploadAcceptsDecodedPNGAndJPEG(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		filename string
		content  []byte
	}{
		{name: "PNG", filename: "avatar.png", content: encodedPNG(t)},
		{name: "JPEG", filename: "avatar.jpg", content: encodedJPEG(t)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			storage := &recordingFileStorage{stored: files.StoredFile{URL: "/uploads/avatars/saved-" + fixture.filename}}
			router, session := newAvatarUploadTestRouter(t, storage)

			response := multipartUploadRequest(t, router, fixture.filename, "application/octet-stream", fixture.content, session)
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var body struct {
				URL string `json:"url"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.URL != storage.stored.URL {
				t.Fatalf("url = %q, want %q", body.URL, storage.stored.URL)
			}
			if storage.file.Name != fixture.filename || !bytes.Equal(storage.content, fixture.content) {
				t.Fatal("storage did not receive the uploaded file unchanged")
			}
			if storage.key != "550e8400-e29b-41d4-a716-446655440000" {
				t.Fatalf("storage key = %q, want canonical multipart key", storage.key)
			}
		})
	}
}

func TestAvatarUploadRequiresAuthentication(t *testing.T) {
	storage := &recordingFileStorage{}
	router, _ := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "avatar.png", "image/png", encodedPNG(t), nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if storage.calls != 0 {
		t.Fatal("storage was called for an unauthenticated request")
	}
}

func TestAvatarUploadRejectsMislabeledOrUndecodableImage(t *testing.T) {
	storage := &recordingFileStorage{}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "avatar.png", "image/png", []byte("not an image"), session)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if storage.calls != 0 {
		t.Fatal("storage was called for an undecodable image")
	}
}

func TestAvatarUploadRequiresUUIDKey(t *testing.T) {
	storage := &recordingFileStorage{}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequestWithKey(t, router, "avatar.png", "image/png", encodedPNG(t), "not-a-uuid", session)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if storage.calls != 0 {
		t.Fatal("storage was called without a valid key")
	}
}

func TestAvatarUploadMapsStorageQuotaToTooManyRequests(t *testing.T) {
	storage := &recordingFileStorage{storeError: files.ErrAvatarQuotaExceeded}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "avatar.png", "image/png", encodedPNG(t), session)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAvatarUploadIncludesStorageErrorForDebugging(t *testing.T) {
	storage := &recordingFileStorage{storeError: errors.New("JPEG uploads are blocked by the storage provider")}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "avatar.jpg", "image/jpeg", encodedJPEG(t), session)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["server_err"] != "JPEG uploads are blocked by the storage provider" {
		t.Fatalf("server_err = %q", body["server_err"])
	}
}

func TestAvatarUploadRejectsBodiesOverFiveMiB(t *testing.T) {
	storage := &recordingFileStorage{}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "avatar.png", "image/png", bytes.Repeat([]byte{'x'}, 5<<20), session)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if storage.calls != 0 {
		t.Fatal("storage was called for an oversized upload")
	}
}

func TestConfiguredUploadsAreServedStatically(t *testing.T) {
	directory := t.TempDir()
	filename := strings.Repeat("a", 64) + "-" + strings.Repeat("b", 64) + ".avatar"
	path := filepath.Join(directory, "avatars", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encodedPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	storage := &recordingFileStorage{}
	router, _ := newAvatarUploadTestRouterWithConfig(t, storage, RouterConfig{AvatarDirectory: filepath.Join(directory, "avatars")})

	request := httptest.NewRequest(http.MethodGet, "/uploads/avatars/"+filename, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("static upload status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Equal(response.Body.Bytes(), encodedPNG(t)) {
		t.Fatal("static upload response did not serve the configured file")
	}
	for _, path := range []string{"/uploads", "/uploads/avatars", "/uploads/avatars/"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("directory %q status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
	request = httptest.NewRequest(http.MethodGet, "/uploads/avatars/"+filename+".retained", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sidecar status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestAvatarUploadCompressesImagesOverFourMillionPixels(t *testing.T) {
	storage := &recordingFileStorage{}
	router, session := newAvatarUploadTestRouter(t, storage)

	response := multipartUploadRequest(t, router, "large.png", "image/png", encodedPNGWithDimensions(t, 2001, 2000), session)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(storage.content))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || int64(config.Width)*int64(config.Height) > maxStoredAvatarPixels || storage.file.Name != "avatar.jpg" {
		t.Fatalf("stored image = %s %dx%d named %q", format, config.Width, config.Height, storage.file.Name)
	}
}

func TestAvatarUploadOnlyReplacesKeyBoundToItsExactResume(t *testing.T) {
	storage := &recordingFileStorage{stored: files.StoredFile{URL: "/uploads/avatars/avatar.avatar"}}
	router, session := newAvatarUploadTestRouter(t, storage)
	key := "550e8400-e29b-41d4-a716-446655440000"
	original := encodedPNG(t)
	if response := multipartUploadRequestWithKeyAndResumeID(t, router, "avatar.png", original, key, "", session); response.Code != http.StatusCreated {
		t.Fatalf("initial upload status = %d: %s", response.Code, response.Body.String())
	}
	if response := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("First", "minimal"), session); response.Code != http.StatusCreated {
		t.Fatalf("first resume status = %d: %s", response.Code, response.Body.String())
	}
	profile := `{"fullName":"Ada","headline":"Engineer","email":"","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/avatar.avatar","avatarStorageKey":"` + key + `"}`
	body := `{"title":"Second","templateId":"minimal","content":` + validV2ContentWithProfile("Second", "minimal", profile) + `}`
	if response := requestJSON(t, router, http.MethodPost, "/api/resumes", body, session); response.Code != http.StatusCreated {
		t.Fatalf("second resume status = %d: %s", response.Code, response.Body.String())
	}
	for _, resumeID := range []string{"", "1"} {
		response := multipartUploadRequestWithKeyAndResumeID(t, router, "replacement.jpg", encodedJPEG(t), key, resumeID, session)
		if response.Code != http.StatusConflict {
			t.Errorf("replacement with resumeId %q status = %d, want %d: %s", resumeID, response.Code, http.StatusConflict, response.Body.String())
		}
		if storage.calls != 1 || !bytes.Equal(storage.content, original) {
			t.Errorf("blocked replacement changed stored bytes or called storage: calls=%d content=%q", storage.calls, storage.content)
		}
	}
	response := multipartUploadRequestWithKeyAndResumeID(t, router, "replacement.jpg", encodedJPEG(t), key, "2", session)
	if response.Code != http.StatusCreated {
		t.Fatalf("same-resume replacement status = %d: %s", response.Code, response.Body.String())
	}
	if storage.calls != 2 || bytes.Equal(storage.content, original) {
		t.Fatalf("same-resume replacement did not store new bytes: calls=%d", storage.calls)
	}
}

type recordingFileStorage struct {
	stored     files.StoredFile
	file       files.File
	content    []byte
	calls      int
	owner      string
	key        string
	storeError error
}

func (s *recordingFileStorage) Store(_ context.Context, owner, key string, file files.File) (files.StoredFile, error) {
	s.calls++
	s.owner = owner
	s.key = key
	s.file = file
	content, err := io.ReadAll(file.Content)
	if err != nil {
		return files.StoredFile{}, err
	}
	s.content = content
	if s.storeError != nil {
		return files.StoredFile{}, s.storeError
	}
	return s.stored, nil
}

func (s *recordingFileStorage) PublicURL(owner, key string) string {
	return "/uploads/avatars/avatar.avatar"
}

func (s *recordingFileStorage) Retain(context.Context, string, string) error { return nil }

func (s *recordingFileStorage) Release(context.Context, string, string) error { return nil }

func newAvatarUploadTestRouter(t *testing.T, storage files.FileStorage) (http.Handler, *http.Cookie) {
	t.Helper()
	return newAvatarUploadTestRouterWithConfig(t, storage, RouterConfig{})
}

func newAvatarUploadTestRouterWithConfig(t *testing.T, storage files.FileStorage, config RouterConfig) (http.Handler, *http.Cookie) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	config.JWTSecret = "avatar-upload-test-secret-that-is-long-enough"
	config.FileStorage = storage
	router := NewRouter(auth.NewRepository(db), resume.NewRepository(db), templates.NewRepository(db), config)
	return router, registerAuthUser(t, router, "avatar@example.com")
}

func multipartUploadRequest(t *testing.T, router http.Handler, filename, contentType string, content []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	return multipartUploadRequestWithKey(t, router, filename, contentType, content, "550e8400-e29b-41d4-a716-446655440000", cookie)
}

func multipartUploadRequestWithKey(t *testing.T, router http.Handler, filename, contentType string, content []byte, key string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return multipartUploadRequestWithKeyAndResumeID(t, router, filename, content, key, "", cookie)
}

func multipartUploadRequestWithKeyAndResumeID(t *testing.T, router http.Handler, filename string, content []byte, key, resumeID string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("key", key); err != nil {
		t.Fatal(err)
	}
	if resumeID != "" {
		if err := writer.WriteField("resumeId", resumeID); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func encodedPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodedPNGWithDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodedJPEG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.White)
	if err := jpeg.Encode(&output, picture, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
