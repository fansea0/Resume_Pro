// Package files defines storage primitives for user-uploaded files.
package files

import (
	"context"
	"io"
)

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

// FileStorage persists uploads and returns the resulting file metadata.
// Implementations may write to the local filesystem, object storage, or another
// backend without changing their callers.
type FileStorage interface {
	Store(ctx context.Context, file File) (StoredFile, error)
}
