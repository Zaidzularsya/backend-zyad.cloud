package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

var ErrCreateUserRoleNotFound = errors.New("create user role not found")
var ErrUpdateUserNotFound = errors.New("update user not found")
var ErrDeleteUserNotFound = errors.New("delete user not found")
var ErrRestoreUserNotFound = errors.New("restore user not found")

const userInvitedEvent = "user.invited"
const maxBulkUserActionItems = 100

type UserListRepository interface {
	ListUsers(ctx context.Context, filter UserListFilter) ([]UserListRecord, int64, error)
	FindUserDetail(ctx context.Context, userID string, includeDeleted bool) (UserDetailRecord, error)
	CreateUser(ctx context.Context, user NewUser) (string, error)
	UpdateUser(ctx context.Context, update UserUpdate) error
	DeleteUser(ctx context.Context, userID string, metadata UserLifecycleMetadata, deletedAt time.Time) error
	RestoreUser(ctx context.Context, userID string, metadata UserLifecycleMetadata, restoredAt time.Time) error
	UpdateUserStatus(ctx context.Context, change UserStatusChange) error
}

type UserListFilter struct {
	Page           int
	PerPage        int
	Offset         int
	Search         string
	Status         model.UserStatus
	Role           string
	OrganizationID string
	IncludeDeleted bool
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	Sort           string
	Direction      string
}

type UserListRecord struct {
	ID              string
	Name            string
	Email           string
	Username        string
	Phone           string
	Status          model.UserStatus
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	LastLoginAt     *time.Time
	Roles           []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type UserDetailRecord struct {
	UserListRecord
	Profile     UserProfileRecord
	Roles       []UserRoleRecord
	Permissions []UserPermissionRecord
	Sessions    UserSessionSummaryRecord
	Audit       UserAuditSummaryRecord
}

type UserProfileRecord struct {
	AvatarURL  string
	Bio        string
	JobTitle   string
	Department string
	Company    string
	Address    string
	Timezone   string
	Language   string
}

type UserRoleRecord struct {
	ID             string
	Name           string
	Slug           string
	OrganizationID string
	AssignedAt     time.Time
}

type UserPermissionRecord struct {
	ID             string
	PermissionID   string
	Slug           string
	Effect         model.PermissionEffect
	OrganizationID string
	AssignedAt     time.Time
}

type UserSessionSummaryRecord struct {
	Total        int64
	Active       int64
	Revoked      int64
	Expired      int64
	LastActiveAt *time.Time
}

type UserAuditSummaryRecord struct {
	Total         int64
	LastEvent     string
	LastEventAt   *time.Time
	LastActorID   string
	LastIPAddress string
}

type CreateUserMetadata struct {
	ActorUserID string
	IPAddress   string
	UserAgent   string
}

type NewUser struct {
	Name               string
	Email              string
	Username           string
	Phone              string
	PasswordHash       string
	Status             model.UserStatus
	Profile            UserProfileRecord
	Roles              []NewUserRole
	PasswordSetupToken *NewPasswordResetToken
	ActorUserID        string
	IPAddress          string
	UserAgent          string
	InvitationSent     bool
}

type NewUserRole struct {
	RoleID         string
	OrganizationID string
}

type UpdateUserMetadata struct {
	ActorUserID string
	IPAddress   string
	UserAgent   string
}

type UserUpdate struct {
	UserID      string
	Name        *string
	Email       *string
	Username    *string
	Phone       *string
	AvatarURL   *string
	Bio         *string
	JobTitle    *string
	Department  *string
	Company     *string
	Address     *string
	Timezone    *string
	Language    *string
	ActorUserID string
	IPAddress   string
	UserAgent   string
}

type UserLifecycleMetadata struct {
	ActorUserID string
	IPAddress   string
	UserAgent   string
}

type UserStatusChange struct {
	UserID      string
	Status      model.UserStatus
	Reason      string
	ActorUserID string
	IPAddress   string
	UserAgent   string
	ChangedAt   time.Time
}

type UserService struct {
	repo                  UserListRepository
	notificationPublisher NotificationEventPublisher
	resetTokenTTL         time.Duration
	passwordMinLength     int
	appName               string
	frontendURL           string
	notificationLocale    string
	notificationAttempts  int
	now                   func() time.Time
}

func NewUserService(repo UserListRepository) *UserService {
	return &UserService{
		repo:              repo,
		resetTokenTTL:     15 * time.Minute,
		passwordMinLength: 8,
		appName:           "Zyad Cloud",
		now:               time.Now,
	}
}

func (s *UserService) Configure(cfg config.Config) error {
	resetTokenTTL, err := parseConfigDuration(cfg.Auth.ResetTokenExpiresIn)
	if err != nil {
		return fmt.Errorf("parse user setup token ttl: %w", err)
	}
	s.resetTokenTTL = resetTokenTTL
	s.passwordMinLength = cfg.Auth.PasswordMinLength
	s.appName = cfg.App.Name
	s.frontendURL = cfg.App.FrontendURL
	s.notificationLocale = cfg.Notification.DefaultLocale
	s.notificationAttempts = cfg.Notification.MaxAttempts
	return nil
}

func (s *UserService) SetNotificationPublisher(publisher NotificationEventPublisher) {
	s.notificationPublisher = publisher
}

func (s *UserService) ListUsers(ctx context.Context, query dto.UserListQuery) ([]dto.UserListItem, dto.PaginationMeta, error) {
	filter, err := normalizeUserListQuery(query)
	if err != nil {
		return nil, dto.PaginationMeta{}, err
	}

	users, total, err := s.repo.ListUsers(ctx, filter)
	if err != nil {
		return nil, dto.PaginationMeta{}, coreerrors.Wrap("USER_LIST_FAILED", "failed to list users", http.StatusInternalServerError, err)
	}

	items := make([]dto.UserListItem, 0, len(users))
	for _, user := range users {
		items = append(items, dto.UserListItem{
			ID:              user.ID,
			Name:            user.Name,
			Email:           user.Email,
			Username:        user.Username,
			Phone:           user.Phone,
			Status:          string(user.Status),
			EmailVerifiedAt: formatTimePtr(user.EmailVerifiedAt),
			PhoneVerifiedAt: formatTimePtr(user.PhoneVerifiedAt),
			LastLoginAt:     formatTimePtr(user.LastLoginAt),
			Roles:           user.Roles,
			CreatedAt:       user.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:       user.UpdatedAt.UTC().Format(time.RFC3339),
			DeletedAt:       formatTimePtr(user.DeletedAt),
		})
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(filter.PerPage) - 1) / int64(filter.PerPage))
	}
	return items, dto.PaginationMeta{
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *UserService) GetUser(ctx context.Context, userID string, includeDeleted bool) (dto.UserDetailResponse, error) {
	userID = strings.TrimSpace(userID)
	var id pgtype.UUID
	if err := id.Scan(userID); err != nil || !id.Valid {
		return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}

	user, err := s.repo.FindUserDetail(ctx, userID, includeDeleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		}
		return dto.UserDetailResponse{}, coreerrors.Wrap("USER_DETAIL_FAILED", "failed to get user detail", http.StatusInternalServerError, err)
	}

	roles := make([]dto.UserRoleDetail, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, dto.UserRoleDetail{
			ID:             role.ID,
			Name:           role.Name,
			Slug:           role.Slug,
			OrganizationID: role.OrganizationID,
			AssignedAt:     role.AssignedAt.UTC().Format(time.RFC3339),
		})
	}
	permissions := make([]dto.UserPermissionDetail, 0, len(user.Permissions))
	for _, permission := range user.Permissions {
		permissions = append(permissions, dto.UserPermissionDetail{
			ID:             permission.ID,
			PermissionID:   permission.PermissionID,
			Slug:           permission.Slug,
			Effect:         string(permission.Effect),
			OrganizationID: permission.OrganizationID,
			AssignedAt:     permission.AssignedAt.UTC().Format(time.RFC3339),
		})
	}

	return dto.UserDetailResponse{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		Username:        user.Username,
		Phone:           user.Phone,
		Status:          string(user.Status),
		EmailVerifiedAt: formatTimePtr(user.EmailVerifiedAt),
		PhoneVerifiedAt: formatTimePtr(user.PhoneVerifiedAt),
		LastLoginAt:     formatTimePtr(user.LastLoginAt),
		Profile: dto.UserProfileDetail{
			AvatarURL:  user.Profile.AvatarURL,
			Bio:        user.Profile.Bio,
			JobTitle:   user.Profile.JobTitle,
			Department: user.Profile.Department,
			Company:    user.Profile.Company,
			Address:    user.Profile.Address,
			Timezone:   user.Profile.Timezone,
			Language:   user.Profile.Language,
		},
		Roles:       roles,
		Permissions: permissions,
		Sessions: dto.UserSessionSummary{
			Total:        user.Sessions.Total,
			Active:       user.Sessions.Active,
			Revoked:      user.Sessions.Revoked,
			Expired:      user.Sessions.Expired,
			LastActiveAt: formatTimePtr(user.Sessions.LastActiveAt),
		},
		Audit: dto.UserAuditSummary{
			Total:         user.Audit.Total,
			LastEvent:     user.Audit.LastEvent,
			LastEventAt:   formatTimePtr(user.Audit.LastEventAt),
			LastActorID:   user.Audit.LastActorID,
			LastIPAddress: user.Audit.LastIPAddress,
		},
		CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.UTC().Format(time.RFC3339),
		DeletedAt: formatTimePtr(user.DeletedAt),
	}, nil
}

func (s *UserService) CreateUser(ctx context.Context, req dto.CreateUserRequest, metadata CreateUserMetadata) (dto.UserDetailResponse, error) {
	user, setupToken, err := s.normalizeNewUser(req, metadata)
	if err != nil {
		return dto.UserDetailResponse{}, err
	}

	userID, err := s.repo.CreateUser(ctx, user)
	if err != nil {
		if errors.Is(err, ErrCreateUserRoleNotFound) {
			return dto.UserDetailResponse{}, coreerrors.New("USER_ROLE_NOT_FOUND", "one or more roles were not found", http.StatusUnprocessableEntity)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "idx_users_username_active_unique":
				return dto.UserDetailResponse{}, coreerrors.New("USER_USERNAME_ALREADY_EXISTS", "username already exists", http.StatusConflict)
			case "auth_identities_provider_user_unique":
				if user.Username != "" {
					return dto.UserDetailResponse{}, coreerrors.New("USER_USERNAME_ALREADY_EXISTS", "username already exists", http.StatusConflict)
				}
				return dto.UserDetailResponse{}, coreerrors.New("USER_EMAIL_ALREADY_EXISTS", "email already exists", http.StatusConflict)
			default:
				return dto.UserDetailResponse{}, coreerrors.New("USER_EMAIL_ALREADY_EXISTS", "email already exists", http.StatusConflict)
			}
		}
		return dto.UserDetailResponse{}, coreerrors.Wrap("USER_CREATE_FAILED", "failed to create user", http.StatusInternalServerError, err)
	}

	if setupToken != "" {
		s.publishUserInvitation(ctx, userID, user.Name, user.Email, setupToken, user.PasswordSetupToken.ExpiresAt)
	}
	return s.GetUser(ctx, userID, false)
}

func (s *UserService) UpdateUser(ctx context.Context, userID string, req dto.UpdateUserRequest, metadata UpdateUserMetadata) (dto.UserDetailResponse, error) {
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}
	if req.Status != nil {
		return dto.UserDetailResponse{}, coreerrors.New("USER_STATUS_UPDATE_REQUIRES_PERMISSION", "status must be changed through the status endpoint", http.StatusForbidden)
	}

	update, err := normalizeUserUpdate(userID, req, metadata)
	if err != nil {
		return dto.UserDetailResponse{}, err
	}
	if err := s.repo.UpdateUser(ctx, update); err != nil {
		if errors.Is(err, ErrUpdateUserNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "idx_users_username_active_unique":
				return dto.UserDetailResponse{}, coreerrors.New("USER_USERNAME_ALREADY_EXISTS", "username already exists", http.StatusConflict)
			case "auth_identities_provider_user_unique":
				if update.Username != nil && *update.Username != "" {
					return dto.UserDetailResponse{}, coreerrors.New("USER_USERNAME_ALREADY_EXISTS", "username already exists", http.StatusConflict)
				}
				return dto.UserDetailResponse{}, coreerrors.New("USER_EMAIL_ALREADY_EXISTS", "email already exists", http.StatusConflict)
			default:
				return dto.UserDetailResponse{}, coreerrors.New("USER_EMAIL_ALREADY_EXISTS", "email already exists", http.StatusConflict)
			}
		}
		return dto.UserDetailResponse{}, coreerrors.Wrap("USER_UPDATE_FAILED", "failed to update user", http.StatusInternalServerError, err)
	}
	return s.GetUser(ctx, userID, false)
}

func (s *UserService) DeleteUser(ctx context.Context, userID string, metadata UserLifecycleMetadata) error {
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}
	if err := s.repo.DeleteUser(ctx, userID, normalizeLifecycleMetadata(metadata), s.now().UTC()); err != nil {
		if errors.Is(err, ErrDeleteUserNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		}
		return coreerrors.Wrap("USER_DELETE_FAILED", "failed to delete user", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *UserService) RestoreUser(ctx context.Context, userID string, metadata UserLifecycleMetadata) (dto.UserDetailResponse, error) {
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}
	if err := s.repo.RestoreUser(ctx, userID, normalizeLifecycleMetadata(metadata), s.now().UTC()); err != nil {
		if errors.Is(err, ErrRestoreUserNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "idx_users_username_active_unique" {
				return dto.UserDetailResponse{}, coreerrors.New("USER_USERNAME_ALREADY_EXISTS", "username is already used by another active user", http.StatusConflict)
			}
			return dto.UserDetailResponse{}, coreerrors.New("USER_EMAIL_ALREADY_EXISTS", "email is already used by another active user", http.StatusConflict)
		}
		return dto.UserDetailResponse{}, coreerrors.Wrap("USER_RESTORE_FAILED", "failed to restore user", http.StatusInternalServerError, err)
	}
	return s.GetUser(ctx, userID, false)
}

func (s *UserService) BulkAction(ctx context.Context, req dto.BulkUserActionRequest, metadata UserLifecycleMetadata) (dto.BulkUserActionResponse, error) {
	action := strings.TrimSpace(req.Action)
	if action != "delete" && action != "restore" && action != "update_status" {
		return dto.BulkUserActionResponse{}, validationError("action is invalid")
	}
	if len(req.UserIDs) == 0 {
		return dto.BulkUserActionResponse{}, validationError("user_ids must not be empty")
	}
	if len(req.UserIDs) > maxBulkUserActionItems {
		return dto.BulkUserActionResponse{}, validationError(fmt.Sprintf("user_ids must not exceed %d items", maxBulkUserActionItems))
	}

	status := model.UserStatus(strings.TrimSpace(req.Payload.Status))
	reason := strings.TrimSpace(req.Payload.Reason)
	if action == "update_status" {
		var err error
		status, reason, err = normalizeStatusChange(req.Payload.Status, req.Payload.Reason)
		if err != nil {
			return dto.BulkUserActionResponse{}, err
		}
	}

	metadata = normalizeLifecycleMetadata(metadata)
	now := s.now().UTC()
	response := dto.BulkUserActionResponse{
		Action:  action,
		Total:   len(req.UserIDs),
		Results: make([]dto.BulkUserActionResult, 0, len(req.UserIDs)),
	}
	seen := make(map[string]struct{}, len(req.UserIDs))
	for _, rawUserID := range req.UserIDs {
		userID := strings.TrimSpace(rawUserID)
		var itemErr error
		if !validUUID(userID) {
			itemErr = coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		} else if _, duplicate := seen[userID]; duplicate {
			itemErr = coreerrors.New("DUPLICATE_USER_ID", "user_id is duplicated in the request", http.StatusUnprocessableEntity)
		} else {
			seen[userID] = struct{}{}
			switch action {
			case "delete":
				itemErr = s.repo.DeleteUser(ctx, userID, metadata, now)
				if errors.Is(itemErr, ErrDeleteUserNotFound) {
					itemErr = coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
				}
			case "restore":
				itemErr = s.repo.RestoreUser(ctx, userID, metadata, now)
				if errors.Is(itemErr, ErrRestoreUserNotFound) {
					itemErr = coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
				}
			case "update_status":
				itemErr = s.repo.UpdateUserStatus(ctx, UserStatusChange{
					UserID:      userID,
					Status:      status,
					Reason:      reason,
					ActorUserID: metadata.ActorUserID,
					IPAddress:   metadata.IPAddress,
					UserAgent:   metadata.UserAgent,
					ChangedAt:   now,
				})
				if errors.Is(itemErr, ErrUpdateUserNotFound) {
					itemErr = coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
				}
			}
		}

		result := dto.BulkUserActionResult{UserID: userID, Success: itemErr == nil}
		if itemErr != nil {
			result.Error = bulkUserActionError(itemErr)
			response.Failed++
		} else {
			response.Succeeded++
		}
		response.Results = append(response.Results, result)
	}
	return response, nil
}

func (s *UserService) ChangeUserStatus(ctx context.Context, userID string, req dto.UpdateUserStatusRequest, metadata UserLifecycleMetadata) (dto.UserDetailResponse, error) {
	userID = strings.TrimSpace(userID)
	if !validUUID(userID) {
		return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
	}
	status, reason, err := normalizeStatusChange(req.Status, req.Reason)
	if err != nil {
		return dto.UserDetailResponse{}, err
	}
	metadata = normalizeLifecycleMetadata(metadata)
	if err := s.repo.UpdateUserStatus(ctx, UserStatusChange{
		UserID:      userID,
		Status:      status,
		Reason:      reason,
		ActorUserID: metadata.ActorUserID,
		IPAddress:   metadata.IPAddress,
		UserAgent:   metadata.UserAgent,
		ChangedAt:   s.now().UTC(),
	}); err != nil {
		if errors.Is(err, ErrUpdateUserNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return dto.UserDetailResponse{}, coreerrors.New("USER_NOT_FOUND", "user not found", http.StatusNotFound)
		}
		return dto.UserDetailResponse{}, coreerrors.Wrap("USER_STATUS_UPDATE_FAILED", "failed to update user status", http.StatusInternalServerError, err)
	}
	return s.GetUser(ctx, userID, false)
}

func normalizeStatusChange(rawStatus string, rawReason string) (model.UserStatus, string, error) {
	status := model.UserStatus(strings.TrimSpace(rawStatus))
	reason := strings.TrimSpace(rawReason)
	if !status.IsValid() || status == model.UserStatusDeleted {
		return "", "", validationError("status is invalid")
	}
	if (status == model.UserStatusSuspended || status == model.UserStatusBanned) && reason == "" {
		return "", "", validationError("reason is required for suspended or banned status")
	}
	return status, reason, nil
}

func bulkUserActionError(err error) *dto.BulkUserActionError {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		return &dto.BulkUserActionError{Code: appErr.Code, Message: appErr.Message}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		code := "USER_EMAIL_ALREADY_EXISTS"
		message := "email is already used by another active user"
		if pgErr.ConstraintName == "idx_users_username_active_unique" {
			code = "USER_USERNAME_ALREADY_EXISTS"
			message = "username is already used by another active user"
		}
		return &dto.BulkUserActionError{Code: code, Message: message}
	}
	return &dto.BulkUserActionError{Code: "USER_BULK_ACTION_FAILED", Message: "failed to process user"}
}

func normalizeLifecycleMetadata(metadata UserLifecycleMetadata) UserLifecycleMetadata {
	return UserLifecycleMetadata{
		ActorUserID: strings.TrimSpace(metadata.ActorUserID),
		IPAddress:   strings.TrimSpace(metadata.IPAddress),
		UserAgent:   strings.TrimSpace(metadata.UserAgent),
	}
}

func normalizeUserUpdate(userID string, req dto.UpdateUserRequest, metadata UpdateUserMetadata) (UserUpdate, error) {
	if req.Name == nil && req.Email == nil && req.Username == nil && req.Phone == nil &&
		req.AvatarURL == nil && req.Bio == nil && req.JobTitle == nil && req.Department == nil &&
		req.Company == nil && req.Address == nil && req.Timezone == nil && req.Language == nil {
		return UserUpdate{}, validationError("at least one field must be provided")
	}

	trim := func(value *string) *string {
		if value == nil {
			return nil
		}
		trimmed := strings.TrimSpace(*value)
		return &trimmed
	}
	req.Name = trim(req.Name)
	req.Email = trim(req.Email)
	req.Username = trim(req.Username)
	req.Phone = trim(req.Phone)
	req.AvatarURL = trim(req.AvatarURL)
	req.Bio = trim(req.Bio)
	req.JobTitle = trim(req.JobTitle)
	req.Department = trim(req.Department)
	req.Company = trim(req.Company)
	req.Address = trim(req.Address)
	req.Timezone = trim(req.Timezone)
	req.Language = trim(req.Language)

	if req.Name != nil && *req.Name == "" {
		return UserUpdate{}, validationError("name must not be empty")
	}
	if req.Email != nil {
		*req.Email = strings.ToLower(*req.Email)
		address, err := mail.ParseAddress(*req.Email)
		if err != nil || !strings.EqualFold(address.Address, *req.Email) {
			return UserUpdate{}, validationError("email is invalid")
		}
	}

	return UserUpdate{
		UserID:      userID,
		Name:        req.Name,
		Email:       req.Email,
		Username:    req.Username,
		Phone:       req.Phone,
		AvatarURL:   req.AvatarURL,
		Bio:         req.Bio,
		JobTitle:    req.JobTitle,
		Department:  req.Department,
		Company:     req.Company,
		Address:     req.Address,
		Timezone:    req.Timezone,
		Language:    req.Language,
		ActorUserID: strings.TrimSpace(metadata.ActorUserID),
		IPAddress:   strings.TrimSpace(metadata.IPAddress),
		UserAgent:   strings.TrimSpace(metadata.UserAgent),
	}, nil
}

func (s *UserService) normalizeNewUser(req dto.CreateUserRequest, metadata CreateUserMetadata) (NewUser, string, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Name == "" || req.Email == "" {
		return NewUser{}, "", validationError("name and email are required")
	}
	address, err := mail.ParseAddress(req.Email)
	if err != nil || !strings.EqualFold(address.Address, req.Email) {
		return NewUser{}, "", validationError("email is invalid")
	}

	status := model.UserStatus(strings.TrimSpace(req.Status))
	if status == "" {
		if req.Password == "" {
			status = model.UserStatusInvited
		} else {
			status = model.UserStatusActive
		}
	}
	if !status.IsValid() || status == model.UserStatusDeleted {
		return NewUser{}, "", validationError("status is invalid")
	}

	passwordHash := ""
	if req.Password != "" {
		if len(req.Password) < s.passwordMinLength {
			return NewUser{}, "", validationError(fmt.Sprintf("password must be at least %d characters", s.passwordMinLength))
		}
		passwordHash, err = coreauth.HashPassword(req.Password)
		if err != nil {
			return NewUser{}, "", coreerrors.Wrap("USER_PASSWORD_HASH_FAILED", "failed to secure user password", http.StatusInternalServerError, err)
		}
	}

	roles := make([]NewUserRole, 0, len(req.Roles))
	seenRoles := make(map[string]struct{}, len(req.Roles))
	for _, role := range req.Roles {
		roleID := strings.TrimSpace(role.RoleID)
		if !validUUID(roleID) {
			return NewUser{}, "", validationError("role_id is invalid")
		}
		if _, exists := seenRoles[roleID]; exists {
			return NewUser{}, "", validationError("role_id must be unique")
		}
		seenRoles[roleID] = struct{}{}
		organizationID := ""
		if role.OrganizationID != nil {
			organizationID = strings.TrimSpace(*role.OrganizationID)
			if organizationID != "" && !validUUID(organizationID) {
				return NewUser{}, "", validationError("organization_id is invalid")
			}
		}
		roles = append(roles, NewUserRole{RoleID: roleID, OrganizationID: organizationID})
	}

	now := s.now().UTC()
	needsInvitation := req.Password == "" || req.SendInvitation
	var resetToken *NewPasswordResetToken
	plainToken := ""
	if needsInvitation {
		plainToken, err = coreauth.NewRandomToken(32)
		if err != nil {
			return NewUser{}, "", coreerrors.Wrap("USER_SETUP_TOKEN_FAILED", "failed to generate password setup token", http.StatusInternalServerError, err)
		}
		resetToken = &NewPasswordResetToken{
			TokenHash: coreauth.HashToken(plainToken),
			ExpiresAt: now.Add(s.resetTokenTTL),
		}
	}

	return NewUser{
		Name:         req.Name,
		Email:        req.Email,
		Username:     req.Username,
		Phone:        req.Phone,
		PasswordHash: passwordHash,
		Status:       status,
		Profile: UserProfileRecord{
			AvatarURL:  strings.TrimSpace(req.Profile.AvatarURL),
			Bio:        strings.TrimSpace(req.Profile.Bio),
			JobTitle:   strings.TrimSpace(req.Profile.JobTitle),
			Department: strings.TrimSpace(req.Profile.Department),
			Company:    strings.TrimSpace(req.Profile.Company),
			Address:    strings.TrimSpace(req.Profile.Address),
			Timezone:   strings.TrimSpace(req.Profile.Timezone),
			Language:   strings.TrimSpace(req.Profile.Language),
		},
		Roles:              roles,
		PasswordSetupToken: resetToken,
		ActorUserID:        strings.TrimSpace(metadata.ActorUserID),
		IPAddress:          strings.TrimSpace(metadata.IPAddress),
		UserAgent:          strings.TrimSpace(metadata.UserAgent),
		InvitationSent:     needsInvitation,
	}, plainToken, nil
}

func (s *UserService) publishUserInvitation(ctx context.Context, userID string, name string, email string, token string, expiresAt time.Time) {
	if s.notificationPublisher == nil {
		return
	}
	resetURL, err := url.Parse(strings.TrimRight(s.frontendURL, "/") + "/reset-password")
	if err != nil {
		return
	}
	query := resetURL.Query()
	query.Set("token", token)
	resetURL.RawQuery = query.Encode()
	_, _ = s.notificationPublisher.Publish(ctx, notificationpublisher.Event{
		Type:   userInvitedEvent,
		UserID: userID,
		Recipient: notificationdomain.NotificationRecipient{
			Type:   "user",
			UserID: userID,
			Name:   name,
			Email:  email,
		},
		Payload: map[string]any{
			"app_name":          s.appName,
			"organization_name": s.appName,
			"inviter_name":      "Administrator",
			"invitee_email":     email,
			"invitation_url":    resetURL.String(),
			"expired_at":        expiresAt.UTC().Format(time.RFC3339),
		},
		Locale:      s.notificationLocale,
		MaxAttempts: s.notificationAttempts,
	})
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil && id.Valid
}

func normalizeUserListQuery(query dto.UserListQuery) (UserListFilter, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		return UserListFilter{}, validationError("per_page must not exceed 100")
	}

	status := model.UserStatus(strings.TrimSpace(query.Status))
	if status != "" && !status.IsValid() {
		return UserListFilter{}, validationError("status is invalid")
	}

	sort := strings.TrimSpace(query.Sort)
	if sort == "" {
		sort = "created_at"
	}
	switch sort {
	case "name", "email", "status", "created_at", "updated_at":
	default:
		return UserListFilter{}, validationError("sort is invalid")
	}

	direction := strings.ToLower(strings.TrimSpace(query.Direction))
	if direction == "" {
		direction = "desc"
	}
	if direction != "asc" && direction != "desc" {
		return UserListFilter{}, validationError("direction must be asc or desc")
	}

	createdFrom, err := parseDateFilter(query.CreatedFrom)
	if err != nil {
		return UserListFilter{}, validationError("created_from must use YYYY-MM-DD")
	}
	createdTo, err := parseDateFilter(query.CreatedTo)
	if err != nil {
		return UserListFilter{}, validationError("created_to must use YYYY-MM-DD")
	}
	if createdFrom != nil && createdTo != nil && createdTo.Before(*createdFrom) {
		return UserListFilter{}, validationError("created_to must not be before created_from")
	}
	organizationID := strings.TrimSpace(query.OrganizationID)
	if organizationID != "" {
		var id pgtype.UUID
		if err := id.Scan(organizationID); err != nil || !id.Valid {
			return UserListFilter{}, validationError("organization_id must be a valid UUID")
		}
	}

	return UserListFilter{
		Page:           page,
		PerPage:        perPage,
		Offset:         (page - 1) * perPage,
		Search:         strings.TrimSpace(query.Search),
		Status:         status,
		Role:           strings.TrimSpace(query.Role),
		OrganizationID: organizationID,
		IncludeDeleted: query.IncludeDeleted,
		CreatedFrom:    createdFrom,
		CreatedTo:      createdTo,
		Sort:           sort,
		Direction:      direction,
	}, nil
}

func parseDateFilter(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
