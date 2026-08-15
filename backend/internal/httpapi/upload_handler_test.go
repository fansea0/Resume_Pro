package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
	path := filepath.Join(directory, "avatars", "saved.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encodedPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	storage := &recordingFileStorage{}
	router, _ := newAvatarUploadTestRouterWithConfig(t, storage, RouterConfig{UploadDirectory: directory})

	request := httptest.NewRequest(http.MethodGet, "/uploads/avatars/saved.png", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("static upload status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Equal(response.Body.Bytes(), encodedPNG(t)) {
		t.Fatal("static upload response did not serve the configured file")
	}
}

type recordingFileStorage struct {
	stored  files.StoredFile
	file    files.File
	content []byte
	calls   int
}

func (s *recordingFileStorage) Store(_ context.Context, file files.File) (files.StoredFile, error) {
	s.calls++
	s.file = file
	content, err := io.ReadAll(file.Content)
	if err != nil {
		return files.StoredFile{}, err
	}
	s.content = content
	return s.stored, nil
}

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
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
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
	request.Header.Set("X-Upload-Content-Type", contentType)
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
