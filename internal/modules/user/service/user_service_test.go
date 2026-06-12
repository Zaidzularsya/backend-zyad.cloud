package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

func TestUserServiceListUsersUsesPaginationDefaults(t *testing.T) {
	repo := &fakeUserListRepository{
		users: []UserListRecord{
			{
				ID:        "user-1",
				Name:      "Jane Doe",
				Email:     "jane@example.test",
				Status:    model.UserStatusActive,
				Roles:     []string{"admin"},
				CreatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
			},
		},
		total: 41,
	}
	svc := NewUserService(repo)

	users, meta, err := svc.ListUsers(context.Background(), dto.UserListQuery{})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if repo.filter.Page != 1 || repo.filter.PerPage != 20 || repo.filter.Offset != 0 {
		t.Fatalf("filter pagination = %#v", repo.filter)
	}
	if repo.filter.Sort != "created_at" || repo.filter.Direction != "desc" {
		t.Fatalf("filter sorting = %#v", repo.filter)
	}
	if meta.Page != 1 || meta.PerPage != 20 || meta.Total != 41 || meta.TotalPages != 3 {
		t.Fatalf("meta = %#v", meta)
	}
	if len(users) != 1 || users[0].Email != "jane@example.test" {
		t.Fatalf("users = %#v", users)
	}
}

func TestUserServiceListUsersValidatesFilters(t *testing.T) {
	tests := []struct {
		name  string
		query dto.UserListQuery
	}{
		{name: "invalid status", query: dto.UserListQuery{Status: "unknown"}},
		{name: "invalid sort", query: dto.UserListQuery{Sort: "password_hash"}},
		{name: "invalid direction", query: dto.UserListQuery{Direction: "sideways"}},
		{name: "per page too large", query: dto.UserListQuery{PerPage: 101}},
		{name: "invalid organization", query: dto.UserListQuery{OrganizationID: "not-a-uuid"}},
		{name: "invalid date", query: dto.UserListQuery{CreatedFrom: "12-06-2026"}},
		{name: "reversed date", query: dto.UserListQuery{CreatedFrom: "2026-06-13", CreatedTo: "2026-06-12"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUserService(&fakeUserListRepository{})
			_, _, err := svc.ListUsers(context.Background(), tt.query)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var appErr *coreerrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
				t.Fatalf("error = %v, want VALIDATION_ERROR", err)
			}
		})
	}
}

func TestUserServiceGetUser(t *testing.T) {
	repo := &fakeUserListRepository{
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
				Name:      "Jane Doe",
				Email:     "jane@example.test",
				Status:    model.UserStatusActive,
				CreatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
			},
			Profile: UserProfileRecord{JobTitle: "Administrator"},
			Roles: []UserRoleRecord{
				{
					ID:         "role-1",
					Name:       "Admin",
					Slug:       "admin",
					AssignedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
				},
			},
			Permissions: []UserPermissionRecord{
				{
					ID:           "assignment-1",
					PermissionID: "permission-1",
					Slug:         "user.read",
					Effect:       model.PermissionEffectAllow,
					AssignedAt:   time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
				},
			},
			Sessions: UserSessionSummaryRecord{Total: 2, Active: 1, Revoked: 1},
			Audit:    UserAuditSummaryRecord{Total: 3, LastEvent: "password_changed"},
		},
	}
	svc := NewUserService(repo)

	user, err := svc.GetUser(context.Background(), "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", true)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if !repo.includeDeleted {
		t.Fatal("includeDeleted = false, want true")
	}
	if user.ID == "" || user.Profile.JobTitle != "Administrator" {
		t.Fatalf("user = %#v", user)
	}
	if len(user.Roles) != 1 || user.Roles[0].Slug != "admin" {
		t.Fatalf("roles = %#v", user.Roles)
	}
	if len(user.Permissions) != 1 || user.Permissions[0].Effect != "allow" {
		t.Fatalf("permissions = %#v", user.Permissions)
	}
}

func TestUserServiceGetUserReturnsNotFound(t *testing.T) {
	repo := &fakeUserListRepository{detailErr: pgx.ErrNoRows}
	svc := NewUserService(repo)

	_, err := svc.GetUser(context.Background(), "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", false)
	if err == nil {
		t.Fatal("expected not found error")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "USER_NOT_FOUND" {
		t.Fatalf("error = %v, want USER_NOT_FOUND", err)
	}
}

func TestUserServiceCreateUserHashesPassword(t *testing.T) {
	repo := &fakeUserListRepository{
		createID: "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
				Name:      "Jane Doe",
				Email:     "jane@example.test",
				Status:    model.UserStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	svc := NewUserService(repo)

	user, err := svc.CreateUser(context.Background(), dto.CreateUserRequest{
		Name:     " Jane Doe ",
		Email:    "JANE@example.test",
		Username: "jane",
		Password: "temporary-secret",
		Status:   "active",
	}, CreateUserMetadata{ActorUserID: "a73d7c06-96e9-4ab2-bb32-e025dde5660c"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.ID != repo.createID || repo.created.Email != "jane@example.test" {
		t.Fatalf("user = %#v, created = %#v", user, repo.created)
	}
	if !coreauth.VerifyPassword("temporary-secret", repo.created.PasswordHash) {
		t.Fatal("password was not hashed")
	}
	if repo.created.PasswordSetupToken != nil {
		t.Fatal("unexpected setup token")
	}
}

func TestUserServiceCreateUserWithoutPasswordCreatesInvitation(t *testing.T) {
	repo := &fakeUserListRepository{
		createID: "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b",
				Name:      "Jane Doe",
				Email:     "jane@example.test",
				Status:    model.UserStatusInvited,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	publisher := &fakeNotificationPublisher{}
	svc := NewUserService(repo)
	svc.frontendURL = "https://app.example.test"
	svc.SetNotificationPublisher(publisher)

	_, err := svc.CreateUser(context.Background(), dto.CreateUserRequest{
		Name:  "Jane Doe",
		Email: "jane@example.test",
	}, CreateUserMetadata{ActorUserID: "a73d7c06-96e9-4ab2-bb32-e025dde5660c"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if repo.created.Status != model.UserStatusInvited || repo.created.PasswordSetupToken == nil {
		t.Fatalf("created = %#v", repo.created)
	}
	if !publisher.called || publisher.event.Type != userInvitedEvent {
		t.Fatalf("event = %#v", publisher.event)
	}
	invitationURL, _ := publisher.event.Payload["invitation_url"].(string)
	if !strings.Contains(invitationURL, "/reset-password?token=") {
		t.Fatalf("invitation_url = %q", invitationURL)
	}
}

func TestUserServiceUpdateUserPartial(t *testing.T) {
	userID := "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	repo := &fakeUserListRepository{
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        userID,
				Name:      "Jane Updated",
				Email:     "jane@example.test",
				Status:    model.UserStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Profile: UserProfileRecord{JobTitle: "Senior Editor"},
		},
	}
	svc := NewUserService(repo)
	jobTitle := " Senior Editor "

	user, err := svc.UpdateUser(context.Background(), userID, dto.UpdateUserRequest{
		JobTitle: &jobTitle,
	}, UpdateUserMetadata{ActorUserID: "a73d7c06-96e9-4ab2-bb32-e025dde5660c"})
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if repo.updated.JobTitle == nil || *repo.updated.JobTitle != "Senior Editor" {
		t.Fatalf("updated = %#v", repo.updated)
	}
	if user.Profile.JobTitle != "Senior Editor" {
		t.Fatalf("user = %#v", user)
	}
}

func TestUserServiceUpdateUserRejectsStatus(t *testing.T) {
	repo := &fakeUserListRepository{}
	svc := NewUserService(repo)
	status := "inactive"

	_, err := svc.UpdateUser(context.Background(), "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", dto.UpdateUserRequest{
		Status: &status,
	}, UpdateUserMetadata{})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "USER_STATUS_UPDATE_REQUIRES_PERMISSION" {
		t.Fatalf("error = %v", err)
	}
	if repo.updateCalled {
		t.Fatal("repository should not be called")
	}
}

func TestUserServiceDeleteUser(t *testing.T) {
	repo := &fakeUserListRepository{}
	svc := NewUserService(repo)
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	err := svc.DeleteUser(context.Background(), "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", UserLifecycleMetadata{
		ActorUserID: "a73d7c06-96e9-4ab2-bb32-e025dde5660c",
	})
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if !repo.deleteCalled || !repo.deletedAt.Equal(now) {
		t.Fatalf("deleteCalled=%v deletedAt=%v", repo.deleteCalled, repo.deletedAt)
	}
}

func TestUserServiceRestoreUser(t *testing.T) {
	userID := "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	repo := &fakeUserListRepository{
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        userID,
				Name:      "Restored User",
				Email:     "restored@example.test",
				Status:    model.UserStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	svc := NewUserService(repo)

	user, err := svc.RestoreUser(context.Background(), userID, UserLifecycleMetadata{})
	if err != nil {
		t.Fatalf("RestoreUser() error = %v", err)
	}
	if !repo.restoreCalled || user.Status != "active" {
		t.Fatalf("restoreCalled=%v user=%#v", repo.restoreCalled, user)
	}
}

func TestUserServiceBulkActionReportsPartialFailure(t *testing.T) {
	firstID := "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	secondID := "a73d7c06-96e9-4ab2-bb32-e025dde5660c"
	repo := &fakeUserListRepository{
		deleteErrors: map[string]error{secondID: ErrDeleteUserNotFound},
	}
	svc := NewUserService(repo)

	result, err := svc.BulkAction(context.Background(), dto.BulkUserActionRequest{
		Action:  "delete",
		UserIDs: []string{firstID, secondID},
	}, UserLifecycleMetadata{})
	if err != nil {
		t.Fatalf("BulkAction() error = %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 1 || len(result.Results) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Results[1].Error == nil || result.Results[1].Error.Code != "USER_NOT_FOUND" {
		t.Fatalf("failed result = %#v", result.Results[1])
	}
}

func TestUserServiceBulkActionValidatesLimit(t *testing.T) {
	svc := NewUserService(&fakeUserListRepository{})
	userIDs := make([]string, maxBulkUserActionItems+1)

	_, err := svc.BulkAction(context.Background(), dto.BulkUserActionRequest{
		Action:  "delete",
		UserIDs: userIDs,
	}, UserLifecycleMetadata{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("error = %v", err)
	}
}

func TestUserServiceBulkUpdateStatus(t *testing.T) {
	userID := "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	repo := &fakeUserListRepository{}
	svc := NewUserService(repo)

	result, err := svc.BulkAction(context.Background(), dto.BulkUserActionRequest{
		Action:  "update_status",
		UserIDs: []string{userID},
		Payload: dto.BulkUserActionPayload{Status: "suspended", Reason: "Security review"},
	}, UserLifecycleMetadata{})
	if err != nil {
		t.Fatalf("BulkAction() error = %v", err)
	}
	if result.Succeeded != 1 || len(repo.statusChanges) != 1 {
		t.Fatalf("result=%#v changes=%#v", result, repo.statusChanges)
	}
	if repo.statusChanges[0].Status != model.UserStatusSuspended || repo.statusChanges[0].Reason != "Security review" {
		t.Fatalf("change = %#v", repo.statusChanges[0])
	}
}

func TestUserServiceChangeUserStatus(t *testing.T) {
	userID := "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b"
	repo := &fakeUserListRepository{
		detail: UserDetailRecord{
			UserListRecord: UserListRecord{
				ID:        userID,
				Name:      "Suspended User",
				Email:     "suspended@example.test",
				Status:    model.UserStatusSuspended,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	svc := NewUserService(repo)

	user, err := svc.ChangeUserStatus(context.Background(), userID, dto.UpdateUserStatusRequest{
		Status: "suspended",
		Reason: " Security review ",
	}, UserLifecycleMetadata{ActorUserID: "a73d7c06-96e9-4ab2-bb32-e025dde5660c"})
	if err != nil {
		t.Fatalf("ChangeUserStatus() error = %v", err)
	}
	if user.Status != "suspended" || len(repo.statusChanges) != 1 {
		t.Fatalf("user=%#v changes=%#v", user, repo.statusChanges)
	}
	if repo.statusChanges[0].Reason != "Security review" {
		t.Fatalf("change = %#v", repo.statusChanges[0])
	}
}

func TestUserServiceChangeUserStatusRequiresReason(t *testing.T) {
	repo := &fakeUserListRepository{}
	svc := NewUserService(repo)

	_, err := svc.ChangeUserStatus(context.Background(), "7ea7b1cb-04eb-456a-a19f-e41d590d2a3b", dto.UpdateUserStatusRequest{
		Status: "banned",
	}, UserLifecycleMetadata{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("error = %v", err)
	}
	if len(repo.statusChanges) != 0 {
		t.Fatal("repository should not be called")
	}
}

type fakeUserListRepository struct {
	filter         UserListFilter
	users          []UserListRecord
	total          int64
	err            error
	detail         UserDetailRecord
	includeDeleted bool
	detailErr      error
	created        NewUser
	createID       string
	createErr      error
	updateCalled   bool
	updated        UserUpdate
	updateErr      error
	deleteCalled   bool
	deletedAt      time.Time
	deleteErr      error
	restoreCalled  bool
	restoredAt     time.Time
	restoreErr     error
	deleteErrors   map[string]error
	statusChanges  []UserStatusChange
	statusErrors   map[string]error
}

func (r *fakeUserListRepository) DeleteUser(_ context.Context, userID string, _ UserLifecycleMetadata, deletedAt time.Time) error {
	r.deleteCalled = true
	r.deletedAt = deletedAt
	if r.deleteErrors != nil {
		if err := r.deleteErrors[userID]; err != nil {
			return err
		}
	}
	return r.deleteErr
}

func (r *fakeUserListRepository) RestoreUser(_ context.Context, _ string, _ UserLifecycleMetadata, restoredAt time.Time) error {
	r.restoreCalled = true
	r.restoredAt = restoredAt
	return r.restoreErr
}

func (r *fakeUserListRepository) UpdateUserStatus(_ context.Context, change UserStatusChange) error {
	r.statusChanges = append(r.statusChanges, change)
	if r.statusErrors != nil {
		return r.statusErrors[change.UserID]
	}
	return nil
}

func (r *fakeUserListRepository) UpdateUser(_ context.Context, update UserUpdate) error {
	r.updateCalled = true
	r.updated = update
	return r.updateErr
}

func (r *fakeUserListRepository) CreateUser(_ context.Context, user NewUser) (string, error) {
	r.created = user
	return r.createID, r.createErr
}

func (r *fakeUserListRepository) ListUsers(_ context.Context, filter UserListFilter) ([]UserListRecord, int64, error) {
	r.filter = filter
	return r.users, r.total, r.err
}

func (r *fakeUserListRepository) FindUserDetail(_ context.Context, _ string, includeDeleted bool) (UserDetailRecord, error) {
	r.includeDeleted = includeDeleted
	return r.detail, r.detailErr
}
