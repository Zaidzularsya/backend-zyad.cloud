package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrObjectNotFound = errors.New("storage object not found")

// ErrPresignUnsupported dikembalikan provider yang tidak mendukung presigned URL
// (mis. LocalProvider).
var ErrPresignUnsupported = errors.New("storage provider does not support presigned url")

// ObjectStorage menyimpan byte file di balik object key hasil BuildObjectKey.
type ObjectStorage interface {
	Put(ctx context.Context, key string, content io.Reader, contentType string, size int64) error
	Delete(ctx context.Context, key string) error
}

// PublicObjectReader membuka object berkelas public untuk dilayani lewat route
// publik backend. Implementasi wajib menolak object non-public dan path traversal.
type PublicObjectReader interface {
	OpenPublic(ctx context.Context, key string) (content io.ReadCloser, contentType string, err error)
}

// PresignedResult adalah hasil presign untuk satu operasi.
type PresignedResult struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// PresignedStorage menghasilkan URL bertanda tangan agar client meng-upload atau
// men-download object langsung ke/dari object storage tanpa melewati backend.
type PresignedStorage interface {
	PresignPut(ctx context.Context, key string, contentType string, size int64) (PresignedResult, error)
	PresignGet(ctx context.Context, key string) (PresignedResult, error)
}

// MediaStorage adalah gabungan kemampuan yang dibutuhkan modul media: menyimpan,
// menghapus, dan melayani object public.
type MediaStorage interface {
	ObjectStorage
	PublicObjectReader
}
