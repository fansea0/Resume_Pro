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

	stored, err := storage.Store(context.Background(), File{
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
	if filepath.Ext(stored.Path) != ".png" {
		t.Fatalf("Store() path = %q, want .png extension", stored.Path)
	}
	if filepath.Base(stored.Path) == "portrait.png" {
		t.Fatalf("Store() path = %q, want generated name", stored.Path)
	}

	written, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(written, contents) {
		t.Fatalf("stored content = %q, want %q", written, contents)
	}
}

func TestLocalFileStorageStoreRejectsUnsafeOrUnsupportedFileNames(t *testing.T) {
	t.Parallel()

	storage := NewLocalFileStorage(filepath.Join(t.TempDir(), "uploads", "avatars"))
	for _, name := range []string{"portrait.gif", "../portrait.png", "nested/portrait.png"} {
		t.Run(name, func(t *testing.T) {
			_, err := storage.Store(context.Background(), File{
				Name:    name,
				Content: strings.NewReader("file content"),
			})
			if err == nil {
				t.Fatalf("Store(%q) error = nil, want rejection", name)
			}
		})
	}
}
