package files

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const avatarURLPrefix = "/uploads/avatars/"

const maxAvatarsPerOwner = 20

const temporaryAvatarTTL = 24 * time.Hour

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
	mu        sync.Mutex
	now       func() time.Time
}

// NewLocalFileStorage creates a local adapter that writes avatar files to directory.
func NewLocalFileStorage(directory string) *LocalFileStorage {
	return &LocalFileStorage{directory: directory, now: time.Now}
}

// Store persists one supported avatar for owner and key. The pair maps to one
// opaque, stable path. A temporary file is atomically renamed over its previous
// avatar, while a per-owner count bound prevents unbounded unique keys.
func (s *LocalFileStorage) Store(ctx context.Context, owner, key string, file File) (StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	if !safeAvatarIdentifier(owner) || !safeAvatarIdentifier(key) {
		return StoredFile{}, errors.New("avatar owner or key is invalid")
	}
	if _, err := safeAvatarExtension(file.Name); err != nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.purgeExpiredTemporaryAvatars(); err != nil {
		return StoredFile{}, err
	}

	filename := avatarFilename(owner, key)
	path := filepath.Join(s.directory, filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := s.ensureOwnerHasAvatarCapacity(owner); err != nil {
			return StoredFile{}, err
		}
	} else if err != nil {
		return StoredFile{}, fmt.Errorf("inspect existing avatar: %w", err)
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

	if err := os.Rename(temporaryPath, path); err != nil {
		return StoredFile{}, fmt.Errorf("finalize avatar: %w", err)
	}

	return StoredFile{Path: path, URL: s.PublicURL(owner, key)}, nil
}

// PublicURL derives the stable public URL for one stored avatar.
func (s *LocalFileStorage) PublicURL(owner, key string) string {
	return avatarURLPrefix + avatarFilename(owner, key)
}

// Retain marks a successfully referenced upload as durable. New uploads are
// temporary until the associated resume create or update has completed.
func (s *LocalFileStorage) Retain(ctx context.Context, owner, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !safeAvatarIdentifier(owner) || !safeAvatarIdentifier(key) {
		return errors.New("avatar owner or key is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.directory, avatarFilename(owner, key))
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("find avatar to retain: %w", err)
	}
	marker, err := os.OpenFile(path+".retained", os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("mark avatar retained: %w", err)
	}
	return marker.Close()
}

// Release removes both the avatar and its retention marker. It is idempotent
// so a resume can safely clear a key that has already expired as temporary.
func (s *LocalFileStorage) Release(ctx context.Context, owner, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !safeAvatarIdentifier(owner) || !safeAvatarIdentifier(key) {
		return errors.New("avatar owner or key is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.directory, avatarFilename(owner, key))
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove avatar: %w", err)
	}
	if err := os.Remove(path + ".retained"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove avatar retention marker: %w", err)
	}
	return nil
}

func avatarFilename(owner, key string) string {
	ownerSum := sha256.Sum256([]byte(owner))
	keySum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x-%x.avatar", ownerSum[:], keySum[:])
}

func (s *LocalFileStorage) ensureOwnerHasAvatarCapacity(owner string) error {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return fmt.Errorf("list existing avatars: %w", err)
	}
	ownerSum := sha256.Sum256([]byte(owner))
	prefix := fmt.Sprintf("%x-", ownerSum[:])
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".avatar") {
			count++
		}
	}
	if count >= maxAvatarsPerOwner {
		return ErrAvatarQuotaExceeded
	}
	return nil
}

func (s *LocalFileStorage) purgeExpiredTemporaryAvatars() error {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return fmt.Errorf("list temporary avatars: %w", err)
	}
	now := s.now()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".avatar") {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		if _, err := os.Stat(path + ".retained"); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect avatar retention marker: %w", err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect temporary avatar: %w", err)
		}
		if now.Sub(info.ModTime()) > temporaryAvatarTTL {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove expired temporary avatar: %w", err)
			}
		}
	}
	return nil
}

func safeAvatarIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
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
