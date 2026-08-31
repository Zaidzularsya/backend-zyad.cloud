package storage

import (
	"context"
	"errors"
	"io"
)

var ErrObjectNotFound = errors.New("storage object not found")

// ObjectStorage menyimpan byte file di balik object key hasil BuildObjectKey.
type ObjectStorage interface {
	Put(ctx context.Context, key string, content io.Reader, contentType string, size int64) error
	Delete(ctx context.Context, key string) error
}
