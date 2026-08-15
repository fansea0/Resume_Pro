package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalFileStorageStoreWritesAvatarAndReturnsPublicURL(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "uploads", "avatars")
	storage := NewLocalFileStorage(directory)
	contents := []byte("png avatar bytes")

	stored, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{
		Name:    "portrait.png",
		Content: bytes.NewReader(contents),
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if !strings.HasPrefix(stored.URL, "/uploads/avatars/") {
		t.Fatalf("Store() URL = %q, want /uploads/avatars/ prefix", stored.URL)
	}
	if filepath.Dir(stored.Path) != directory {
		t.Fatalf("Store() path = %q, want file directly below %q", stored.Path, directory)
	}
	if filepath.Ext(stored.Path) != ".avatar" {
		t.Fatalf("Store() path = %q, want .avatar extension", stored.Path)
	}
	if filepath.Base(stored.Path) == "portrait.png" || strings.Contains(filepath.Base(stored.Path), "user-42") {
		t.Fatalf("Store() path = %q, want opaque generated name", stored.Path)
	}

	written, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(written, contents) {
		t.Fatalf("stored content = %q, want %q", written, contents)
	}
}

func TestLocalFileStorageRetainAndReleaseLifecycle(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	stored, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{Name: "portrait.png", Content: strings.NewReader("avatar")})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Retain(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stored.Path + ".retained"); err != nil {
		t.Fatalf("retained marker missing: %v", err)
	}
	if err := storage.Release(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stored.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("avatar file still exists after release: %v", err)
	}
}

func TestLocalFileStoragePurgesExpiredTemporaryUploadsBeforeQuota(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	storage.now = func() time.Time { return time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC) }
	for index := 0; index < maxAvatarsPerOwner; index++ {
		key := fmt.Sprintf("expired-%d", index)
		stored, err := storage.Store(context.Background(), "user-42", key, File{Name: "portrait.png", Content: strings.NewReader("avatar")})
		if err != nil {
			t.Fatal(err)
		}
		expired := storage.now().Add(-temporaryAvatarTTL - time.Hour)
		if err := os.Chtimes(stored.Path, expired, expired); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := storage.Store(context.Background(), "user-42", "fresh", File{Name: "portrait.png", Content: strings.NewReader("avatar")}); err != nil {
		t.Fatalf("Store() did not purge expired temporary avatars: %v", err)
	}
}

func TestLocalFileStorageStoreAtomicallyOverwritesStableOwnerAvatar(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	first, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{Name: "portrait.png", Content: strings.NewReader("first")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{Name: "portrait.jpg", Content: strings.NewReader("second")})
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != second.Path || first.URL != second.URL {
		t.Fatalf("stable owner should keep one storage location: first=%+v second=%+v", first, second)
	}
	written, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != "second" {
		t.Fatalf("stored content = %q, want replacement", written)
	}
	entries, err := os.ReadDir(filepath.Dir(second.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("avatar directory entries = %d, want 1", len(entries))
	}
}

func TestLocalFileStorageStoreRejectsUnsafeOrUnsupportedFileNames(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	for _, name := range []string{"portrait.gif", "../portrait.png", "nested/portrait.png"} {
		t.Run(name, func(t *testing.T) {
			_, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{
				Name:    name,
				Content: strings.NewReader("file content"),
			})
			if err == nil {
				t.Fatalf("Store(%q) error = nil, want rejection", name)
			}
		})
	}
}

func TestLocalFileStorageStoreRejectsUnsafeOwnerKeys(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	for _, owner := range []string{"", "../user", "nested/user"} {
		t.Run(owner, func(t *testing.T) {
			_, err := storage.Store(context.Background(), owner, "550e8400-e29b-41d4-a716-446655440000", File{Name: "portrait.png", Content: strings.NewReader("file content")})
			if err == nil {
				t.Fatalf("Store(%q) error = nil, want rejection", owner)
			}
		})
	}
}

func TestLocalFileStorageStoreUsesOwnerAndKeyForDistinctAvatars(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	first, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440000", File{Name: "portrait.png", Content: strings.NewReader("first")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.Store(context.Background(), "user-42", "550e8400-e29b-41d4-a716-446655440001", File{Name: "portrait.png", Content: strings.NewReader("second")})
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path || first.URL == second.URL {
		t.Fatalf("different keys must produce distinct avatars: first=%+v second=%+v", first, second)
	}
}

func TestLocalFileStorageStoreBoundsAvatarsPerOwner(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	for index := 0; index < maxAvatarsPerOwner; index++ {
		key := fmt.Sprintf("avatar-%d", index)
		if _, err := storage.Store(context.Background(), "user-42", key, File{Name: "portrait.png", Content: strings.NewReader("avatar")}); err != nil {
			t.Fatalf("Store(%q) error = %v", key, err)
		}
	}
	if _, err := storage.Store(context.Background(), "user-42", "one-too-many", File{Name: "portrait.png", Content: strings.NewReader("avatar")}); err == nil {
		t.Fatal("Store() error = nil, want quota rejection")
	}
}
