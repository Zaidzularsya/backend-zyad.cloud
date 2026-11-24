package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
)

// MaxAvatarSizeBytes adalah batas ukuran file avatar (2 MB).
const MaxAvatarSizeBytes = 2 * 1024 * 1024

const avatarPublicPathPrefix = "/public/media/"

// avatarExtensions memetakan MIME hasil sniffing isi file ke ekstensi. SVG sengaja tidak ada.
var avatarExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// AvatarStorage dipenuhi oleh storage.ObjectStorage.
type AvatarStorage interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string, size int64) error
	Delete(ctx context.Context, key string) error
}

// SetAvatarStorage mengaktifkan upload avatar. Avatar disimpan sebagai object publik
// milik organisasi platform agar dilayani oleh GET /public/media/*.
func (s *UserService) SetAvatarStorage(store AvatarStorage, platformOrganizationID string) {
	s.avatarStorage = store
	s.avatarPrefix = "organizations/" + strings.TrimSpace(platformOrganizationID) + "/public/avatars/"
}

func (s *UserService) UploadSelfAvatar(ctx context.Context, userID string, file io.Reader, size int64, md UpdateUserMetadata) (dto.UserDetailResponse, error) {
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}
	if s.avatarStorage == nil || s.avatarPrefix == "organizations//public/avatars/" {
		return dto.UserDetailResponse{}, coreerrors.New("AVATAR_STORAGE_UNAVAILABLE", "avatar storage is not configured", http.StatusServiceUnavailable)
	}
	tooLarge := coreerrors.New("AVATAR_TOO_LARGE", "avatar must be at most 2 MB", http.StatusRequestEntityTooLarge)
	if size > MaxAvatarSizeBytes {
		return dto.UserDetailResponse{}, tooLarge
	}

	// Baca sampai batas + 1 supaya ukuran sebenarnya tidak bergantung pada nilai yang dilaporkan klien.
	data, err := io.ReadAll(io.LimitReader(file, MaxAvatarSizeBytes+1))
	if err != nil {
		return dto.UserDetailResponse{}, coreerrors.Wrap("AVATAR_READ_FAILED", "failed to read avatar", http.StatusBadRequest, err)
	}
	if len(data) > MaxAvatarSizeBytes {
		return dto.UserDetailResponse{}, tooLarge
	}
	contentType := http.DetectContentType(data)
	ext, ok := avatarExtensions[contentType]
	if !ok {
		return dto.UserDetailResponse{}, coreerrors.New("AVATAR_INVALID_TYPE", "avatar must be a JPEG, PNG or WebP image", http.StatusUnprocessableEntity)
	}

	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return dto.UserDetailResponse{}, coreerrors.Wrap("AVATAR_UPLOAD_FAILED", "failed to upload avatar", http.StatusInternalServerError, err)
	}
	key := s.avatarPrefix + hex.EncodeToString(token) + ext

	// Ambil avatar lama sebelum diganti supaya bisa dibersihkan setelah update sukses.
	var previous string
	if current, err := s.repo.FindUserDetail(ctx, userID, false); err == nil {
		previous = current.Profile.AvatarURL
	}

	if err := s.avatarStorage.Put(ctx, key, bytes.NewReader(data), contentType, int64(len(data))); err != nil {
		return dto.UserDetailResponse{}, coreerrors.Wrap("AVATAR_UPLOAD_FAILED", "failed to upload avatar", http.StatusInternalServerError, err)
	}

	avatarURL := avatarPublicPathPrefix + key
	user, err := s.UpdateUser(ctx, userID, dto.UpdateUserRequest{AvatarURL: &avatarURL}, md)
	if err != nil {
		if delErr := s.avatarStorage.Delete(ctx, key); delErr != nil {
			slog.Warn("cleanup uploaded avatar failed", "key", key, "error", delErr)
		}
		return dto.UserDetailResponse{}, err
	}

	if oldKey, ok := s.ownAvatarKey(previous); ok && oldKey != key {
		if err := s.avatarStorage.Delete(ctx, oldKey); err != nil {
			slog.Warn("delete previous avatar failed", "key", oldKey, "error", err)
		}
	}
	return user, nil
}

// ownAvatarKey mengembalikan object key bila avatarURL menunjuk ke prefix avatar platform;
// URL eksternal (mis. Google) atau path traversal diabaikan.
func (s *UserService) ownAvatarKey(avatarURL string) (string, bool) {
	if !strings.HasPrefix(avatarURL, avatarPublicPathPrefix) {
		return "", false
	}
	key := strings.TrimPrefix(avatarURL, avatarPublicPathPrefix)
	if !strings.HasPrefix(key, s.avatarPrefix) || strings.Contains(key, "..") ||
		path.Clean(key) != key || strings.Contains(strings.TrimPrefix(key, s.avatarPrefix), "/") {
		return "", false
	}
	return key, true
}
