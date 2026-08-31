package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// LocalProvider menyimpan object di disk lokal di bawah satu root directory.
// Di VPS, root harus berada di shared/ (di luar release symlink) agar file
// tidak hilang saat deploy.
type LocalProvider struct {
	root string
}

func NewLocalProvider(root string) (*LocalProvider, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("storage root path is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalProvider{root: absRoot}, nil
}

func (p *LocalProvider) Put(
	_ context.Context,
	key string,
	content io.Reader,
	_ string,
	_ int64,
) error {
	target, err := p.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return fmt.Errorf("create temp storage file: %w", err)
	}
	tempName := temp.Name()
	if _, err := io.Copy(temp, content); err != nil {
		temp.Close()
		os.Remove(tempName)
		return fmt.Errorf("write storage file: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("close storage file: %w", err)
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("chmod storage file: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("finalize storage file: %w", err)
	}
	return nil
}

func (p *LocalProvider) Delete(_ context.Context, key string) error {
	target, err := p.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return fmt.Errorf("delete storage file: %w", err)
	}
	return nil
}

// ResolvePublic mengembalikan path absolut untuk object berkelas public.
// Dipakai route serving publik; object private/temp tidak pernah dilayani.
func (p *LocalProvider) ResolvePublic(key string) (string, error) {
	cleaned := cleanObjectKey(key)
	if cleaned == "" || !isPublicObjectKey(cleaned) {
		return "", ErrInvalidTenantObject
	}
	target, err := p.resolve(cleaned)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return "", ErrObjectNotFound
	}
	return target, nil
}

// OpenPublic membuka object berkelas public untuk dilayani route publik.
func (p *LocalProvider) OpenPublic(_ context.Context, key string) (io.ReadCloser, string, error) {
	target, err := p.ResolvePublic(key)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", fmt.Errorf("open storage file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(target))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return file, contentType, nil
}

func (p *LocalProvider) resolve(key string) (string, error) {
	cleaned := cleanObjectKey(key)
	if cleaned == "" {
		return "", ErrInvalidTenantObject
	}
	target := filepath.Join(p.root, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(target, p.root+string(os.PathSeparator)) {
		return "", ErrInvalidTenantObject
	}
	return target, nil
}
