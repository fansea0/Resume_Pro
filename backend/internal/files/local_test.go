package files

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFileStorageStoreWritesAvatarAndReturnsPublicURL(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "uploads", "avatars")
	storage := NewLocalFileStorage(directory)
	contents := []byte("png avatar bytes")

	stored, err := storage.Store(context.Background(), "user-42", File{
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

func TestLocalFileStorageStoreAtomicallyOverwritesStableOwnerAvatar(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	first, err := storage.Store(context.Background(), "user-42", File{Name: "portrait.png", Content: strings.NewReader("first")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.Store(context.Background(), "user-42", File{Name: "portrait.jpg", Content: strings.NewReader("second")})
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
			_, err := storage.Store(context.Background(), "user-42", File{
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
			_, err := storage.Store(context.Background(), owner, File{Name: "portrait.png", Content: strings.NewReader("file content")})
			if err == nil {
				t.Fatalf("Store(%q) error = nil, want rejection", owner)
			}
		})
	}
}
