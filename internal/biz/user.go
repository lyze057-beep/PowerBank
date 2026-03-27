package biz

import (
	"context"
	"strings"

	v1 "github.com/go-kratos/kratos-layout/api/user/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	// ErrProfileUserNotFound means requested user does not exist.
	ErrProfileUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
	// ErrProfileUserStatusInvalid means user is not active.
	ErrProfileUserStatusInvalid = errors.Forbidden(v1.ErrorReason_USER_STATUS_INVALID.String(), "user status invalid")
	// ErrProfileInvalidArgument means request contains invalid field values.
	ErrProfileInvalidArgument = errors.BadRequest(v1.ErrorReason_INVALID_ARGUMENT.String(), "invalid argument")
)

// User is personal-center user model.
type User struct {
	ID       uint64
	UID      string
	Mobile   string
	Nickname string
	Avatar   string
	Status   int32
}

// UpdateProfileInput is personal profile update payload.
type UpdateProfileInput struct {
	Nickname string
	Avatar   string
}

// UserRepo is user domain storage abstraction.
type UserRepo interface {
	FindByUID(ctx context.Context, uid string) (*User, error)
	UpdateProfile(ctx context.Context, uid string, in UpdateProfileInput) (*User, error)
	CheckLoginState(ctx context.Context, uid, jti string) (bool, error)
}

// UserUsecase handles personal-center business logic.
type UserUsecase struct {
	repo UserRepo
	log  *log.Helper
}

// NewUserUsecase creates user usecase.
func NewUserUsecase(repo UserRepo, logger log.Logger) *UserUsecase {
	return &UserUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "biz/user")),
	}
}

// GetProfile queries profile by uid in JWT.
func (uc *UserUsecase) GetProfile(ctx context.Context, uid string) (*User, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrProfileInvalidArgument
	}
	user, err := uc.repo.FindByUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrProfileUserNotFound
	}
	if user.Status != 1 {
		return nil, ErrProfileUserStatusInvalid
	}
	return user, nil
}

// UpdateProfile updates nickname/avatar by uid.
func (uc *UserUsecase) UpdateProfile(ctx context.Context, uid string, in UpdateProfileInput) (*User, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrProfileInvalidArgument
	}
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.Avatar = strings.TrimSpace(in.Avatar)
	if in.Nickname == "" && in.Avatar == "" {
		return nil, ErrProfileInvalidArgument
	}
	return uc.repo.UpdateProfile(ctx, uid, in)
}

// CheckLoginState validates redis session for current token jti.
func (uc *UserUsecase) CheckLoginState(ctx context.Context, uid, jti string) (bool, error) {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(jti) == "" {
		return false, ErrProfileInvalidArgument
	}
	return uc.repo.CheckLoginState(ctx, uid, jti)
}
