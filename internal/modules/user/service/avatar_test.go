package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
)

const (
	avatarTestUserID   = "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	avatarTestOtherID  = "2f0e0d4c-5b7a-4b8e-9c1d-0a1b2c3d4e5f"
	avatarTestPlatform = "11111111-1111-1111-1111-111111111111"
	avatarTestBaseURL  = "https://api.example.test"
	avatarTestPrefix   = "organizations/" + avatarTestPlatform + "/public/avatars/"
)

type fakeAvatarStorage struct {
	putKeys     []string
	putType     string
	putBody     []byte
	deletedKeys []string
	deleteErr   error
}

func (f *fakeAvatarStorage) Put(_ context.Context, key string, r io.Reader, contentType string, _ int64) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.putKeys = append(f.putKeys, key)
	f.putType = contentType
	f.putBody = body
	return nil
}

func (f *fakeAvatarStorage) Delete(_ context.Context, key string) error {
	f.deletedKeys = append(f.deletedKeys, key)
	return f.deleteErr
}

func avatarTestService(repo *fakeUserListRepository, store *fakeAvatarStorage) *UserService {
	svc := NewUserService(repo)
	svc.SetAvatarStorage(store, avatarTestPlatform, avatarTestBaseURL+"/")
	return svc
}

func pngBytes() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)
}

func uploadAvatar(svc *UserService, data []byte) error {
	_, err := svc.UploadSelfAvatar(context.Background(), avatarTestUserID, bytes.NewReader(data), int64(len(data)), UpdateUserMetadata{ActorUserID: avatarTestUserID})
	return err
}

func TestUploadAvatarAcceptsPNG(t *testing.T) {
	repo := &fakeUserListRepository{}
	store := &fakeAvatarStorage{}
	if err := uploadAvatar(avatarTestService(repo, store), pngBytes()); err != nil {
		t.Fatalf("error = %v", err)
	}
	prefix := avatarTestPrefix + avatarTestUserID + "/"
	if len(store.putKeys) != 1 || !strings.HasPrefix(store.putKeys[0], prefix) || !strings.HasSuffix(store.putKeys[0], ".png") ||
		strings.Contains(strings.TrimPrefix(store.putKeys[0], prefix), "/") {
		t.Fatalf("put keys = %v", store.putKeys)
	}
	if store.putType != "image/png" {
		t.Fatalf("content type = %q", store.putType)
	}
	if repo.updated.AvatarURL == nil || *repo.updated.AvatarURL != avatarTestBaseURL+"/public/media/"+store.putKeys[0] {
		t.Fatalf("avatar_url = %v", repo.updated.AvatarURL)
	}
}

func TestUploadAvatarRejectsSVG(t *testing.T) {
	repo := &fakeUserListRepository{}
	store := &fakeAvatarStorage{}
	err := uploadAvatar(avatarTestService(repo, store), []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AVATAR_INVALID_TYPE" || appErr.Status != 422 {
		t.Fatalf("error = %v, want AVATAR_INVALID_TYPE 422", err)
	}
	if len(store.putKeys) != 0 || repo.updateCalled {
		t.Fatal("nothing must be stored or updated")
	}
}

func TestUploadAvatarRejectsOver2MB(t *testing.T) {
	repo := &fakeUserListRepository{}
	store := &fakeAvatarStorage{}
	data := append(pngBytes(), make([]byte, MaxAvatarSizeBytes)...)
	err := uploadAvatar(avatarTestService(repo, store), data)
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AVATAR_TOO_LARGE" || appErr.Status != 413 {
		t.Fatalf("error = %v, want AVATAR_TOO_LARGE 413", err)
	}
	if len(store.putKeys) != 0 {
		t.Fatal("nothing must be stored")
	}
}

func TestUploadAvatarDeletesPreviousOwnObject(t *testing.T) {
	own := avatarTestPrefix + avatarTestUserID + "/old.png"
	// Bentuk absolut (baru) dan relatif (lama) sama-sama dikenali sebagai milik sendiri.
	for _, old := range []string{avatarTestBaseURL + "/public/media/" + own, "/public/media/" + own} {
		repo := &fakeUserListRepository{}
		repo.detail.Profile.AvatarURL = old
		store := &fakeAvatarStorage{deleteErr: errors.New("boom")}
		if err := uploadAvatar(avatarTestService(repo, store), pngBytes()); err != nil {
			t.Fatalf("delete failure must not fail request: %v", err)
		}
		if len(store.deletedKeys) != 1 || store.deletedKeys[0] != own {
			t.Fatalf("old %q deleted = %v", old, store.deletedKeys)
		}
	}

	for _, old := range []string{
		"https://lh3.googleusercontent.com/a/x",
		"/public/media/" + avatarTestPrefix + avatarTestUserID + "/../../private/x.png",
		"/public/media/" + avatarTestPrefix + "legacy-flat.png",
		"/public/media/" + avatarTestPrefix + avatarTestOtherID + "/theirs.png",
		avatarTestBaseURL + "/public/media/" + avatarTestPrefix + avatarTestOtherID + "/theirs.png",
		"https://evil.example/public/media/" + avatarTestPrefix + avatarTestUserID + "/x.png",
	} {
		repo := &fakeUserListRepository{}
		repo.detail.Profile.AvatarURL = old
		store := &fakeAvatarStorage{}
		if err := uploadAvatar(avatarTestService(repo, store), pngBytes()); err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(store.deletedKeys) != 0 {
			t.Fatalf("must not delete %q: %v", old, store.deletedKeys)
		}
	}
}

func TestUpdateSelfAvatarClearsWithEmptyString(t *testing.T) {
	repo := &fakeUserListRepository{}
	svc := avatarTestService(repo, &fakeAvatarStorage{})
	empty := ""
	if _, err := svc.UpdateSelfAvatar(context.Background(), avatarTestUserID, dto.UpdateAvatarRequest{AvatarURL: &empty}, UpdateUserMetadata{ActorUserID: avatarTestUserID}); err != nil {
		t.Fatalf("error = %v", err)
	}
	if repo.updated.AvatarURL == nil || *repo.updated.AvatarURL != "" {
		t.Fatalf("avatar_url = %v, want empty string", repo.updated.AvatarURL)
	}
}

func TestUpdateSelfAvatarClearDeletesOwnObject(t *testing.T) {
	prefix := avatarTestPrefix + avatarTestUserID + "/"
	empty := ""
	clear := func(old string, store *fakeAvatarStorage) {
		repo := &fakeUserListRepository{}
		repo.detail.Profile.AvatarURL = old
		svc := avatarTestService(repo, store)
		if _, err := svc.UpdateSelfAvatar(context.Background(), avatarTestUserID, dto.UpdateAvatarRequest{AvatarURL: &empty}, UpdateUserMetadata{ActorUserID: avatarTestUserID}); err != nil {
			t.Fatalf("error = %v", err)
		}
	}

	store := &fakeAvatarStorage{deleteErr: errors.New("boom")}
	clear("/public/media/"+prefix+"old.png", store)
	if len(store.deletedKeys) != 1 || store.deletedKeys[0] != prefix+"old.png" {
		t.Fatalf("deleted = %v", store.deletedKeys)
	}

	store = &fakeAvatarStorage{}
	clear("https://lh3.googleusercontent.com/a/x", store)
	if len(store.deletedKeys) != 0 {
		t.Fatalf("must not delete external URL: %v", store.deletedKeys)
	}
}

func TestUpdateSelfAvatarClearNeverDeletesAnotherUsersObject(t *testing.T) {
	empty := ""
	theirs := avatarTestPrefix + avatarTestOtherID + "/theirs.png"
	repo := &fakeUserListRepository{}
	repo.detail.Profile.AvatarURL = avatarTestBaseURL + "/public/media/" + theirs
	store := &fakeAvatarStorage{}
	svc := avatarTestService(repo, store)
	if _, err := svc.UpdateSelfAvatar(context.Background(), avatarTestUserID, dto.UpdateAvatarRequest{AvatarURL: &empty}, UpdateUserMetadata{ActorUserID: avatarTestUserID}); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(store.deletedKeys) != 0 {
		t.Fatalf("must not delete another user's avatar: %v", store.deletedKeys)
	}
}

func TestUpdateSelfAvatarRejectsAnotherUsersAvatarURL(t *testing.T) {
	theirs := avatarTestPrefix + avatarTestOtherID + "/theirs.png"
	for _, value := range []string{
		avatarTestBaseURL + "/public/media/" + theirs,
		"/public/media/" + theirs,
		"/public/media/" + avatarTestPrefix + "legacy-flat.png",
	} {
		repo := &fakeUserListRepository{}
		svc := avatarTestService(repo, &fakeAvatarStorage{})
		v := value
		_, err := svc.UpdateSelfAvatar(context.Background(), avatarTestUserID, dto.UpdateAvatarRequest{AvatarURL: &v}, UpdateUserMetadata{ActorUserID: avatarTestUserID})
		var appErr *coreerrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" || appErr.Status != 422 {
			t.Fatalf("value %q: error = %v, want VALIDATION_ERROR 422", value, err)
		}
		if repo.updateCalled {
			t.Fatalf("value %q: must not update", value)
		}
	}

	// Avatar milik sendiri dan URL eksternal tetap boleh.
	for _, value := range []string{
		avatarTestBaseURL + "/public/media/" + avatarTestPrefix + avatarTestUserID + "/mine.png",
		"https://lh3.googleusercontent.com/a/x",
	} {
		repo := &fakeUserListRepository{}
		svc := avatarTestService(repo, &fakeAvatarStorage{})
		v := value
		if _, err := svc.UpdateSelfAvatar(context.Background(), avatarTestUserID, dto.UpdateAvatarRequest{AvatarURL: &v}, UpdateUserMetadata{ActorUserID: avatarTestUserID}); err != nil {
			t.Fatalf("value %q: error = %v", value, err)
		}
	}
}
