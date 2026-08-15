// Package files defines storage primitives for user-uploaded files.
package files

import (
	"context"
	"errors"
	"io"
)

// ErrAvatarQuotaExceeded is returned when an owner has exhausted the bounded
// number of retained or still-active temporary avatar uploads.
var ErrAvatarQuotaExceeded = errors.New("avatar quota exceeded")

// File is an upload ready to be persisted by a FileStorage implementation.
// Name is used only to derive a safe output extension; Content supplies bytes.
type File struct {
	Name    string
	Content io.Reader
}

// StoredFile identifies a file after it has been persisted.
type StoredFile struct {
	Path string
	URL  string
}

// FileStorage persists one owner's upload and returns its file metadata.
// Implementations may write to the local filesystem, object storage, or another
// backend without changing their callers. owner and key must form a stable,
// safe identity so implementations can overwrite an earlier avatar instead of
// accumulating files.
type FileStorage interface {
	Store(ctx context.Context, owner, key string, file File) (StoredFile, error)
	Retain(ctx context.Context, owner, key string) error
	Release(ctx context.Context, owner, key string) error
}
