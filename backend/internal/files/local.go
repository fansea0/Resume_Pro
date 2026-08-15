package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const avatarURLPrefix = "/uploads/avatars/"

var supportedAvatarExtensions = map[string]struct{}{
	".jpeg": {},
	".jpg":  {},
	".png":  {},
	".webp": {},
}

// LocalFileStorage stores uploaded avatars beneath one configured directory.
// Its public URLs intentionally remain independent of the local directory so
// callers can keep their contract when switching to an object-storage adapter.
type LocalFileStorage struct {
	directory string
}

// NewLocalFileStorage creates a local adapter that writes avatar files to directory.
func NewLocalFileStorage(directory string) *LocalFileStorage {
	return &LocalFileStorage{directory: directory}
}

// Store persists one supported avatar file with a generated name. It writes to a
// temporary file in the destination directory and atomically renames it only
// after the complete content has been copied.
func (s *LocalFileStorage) Store(ctx context.Context, file File) (StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	extension, err := safeAvatarExtension(file.Name)
	if err != nil {
		return StoredFile{}, err
	}
	if file.Content == nil {
		return StoredFile{}, errors.New("file content is required")
	}
	if s.directory == "" {
		return StoredFile{}, errors.New("local storage directory is required")
	}
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		return StoredFile{}, fmt.Errorf("create avatar directory: %w", err)
	}

	temporary, err := os.CreateTemp(s.directory, ".avatar-*")
	if err != nil {
		return StoredFile{}, fmt.Errorf("create temporary avatar: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporary, file.Content); err != nil {
		temporary.Close()
		return StoredFile{}, fmt.Errorf("write avatar: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return StoredFile{}, fmt.Errorf("close temporary avatar: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}

	filename := uuid.NewString() + extension
	path := filepath.Join(s.directory, filename)
	if err := os.Rename(temporaryPath, path); err != nil {
		return StoredFile{}, fmt.Errorf("finalize avatar: %w", err)
	}

	return StoredFile{Path: path, URL: avatarURLPrefix + filename}, nil
}

func safeAvatarExtension(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) {
		return "", errors.New("file name must not contain a path")
	}
	extension := strings.ToLower(filepath.Ext(name))
	if _, supported := supportedAvatarExtensions[extension]; !supported {
		return "", fmt.Errorf("unsupported avatar file extension %q", extension)
	}
	return extension, nil
}
