package service

import (
	"context"

	v1 "github.com/go-kratos/kratos-layout/api/user/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// UserService is user personal-center service.
type UserService struct {
	v1.UnimplementedUserServiceServer

	uc *biz.UserUsecase
}

// NewUserService creates user service.
func NewUserService(uc *biz.UserUsecase) *UserService {
	return &UserService{uc: uc}
}

// GetProfile returns current user profile by jwt uid.
func (s *UserService) GetProfile(ctx context.Context, _ *v1.GetProfileRequest) (*v1.GetProfileReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.uc.GetProfile(ctx, claims.UID)
	if err != nil {
		return nil, err
	}
	return &v1.GetProfileReply{Profile: toProfile(user)}, nil
}

// UpdateProfile updates current user profile by jwt uid.
func (s *UserService) UpdateProfile(ctx context.Context, req *v1.UpdateProfileRequest) (*v1.UpdateProfileReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.uc.UpdateProfile(ctx, claims.UID, biz.UpdateProfileInput{
		Nickname: req.GetNickname(),
		Avatar:   req.GetAvatar(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.UpdateProfileReply{Profile: toProfile(user)}, nil
}

// CheckLoginState checks redis login state by uid+jti.
func (s *UserService) CheckLoginState(ctx context.Context, _ *v1.CheckLoginStateRequest) (*v1.CheckLoginStateReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	loggedIn, err := s.uc.CheckLoginState(ctx, claims.UID, claims.ID)
	if err != nil {
		return nil, err
	}
	var expireAt int64
	if claims.ExpiresAt != nil {
		expireAt = claims.ExpiresAt.Unix()
	}
	return &v1.CheckLoginStateReply{
		LoggedIn: loggedIn,
		ExpireAt: expireAt,
		Uid:      claims.UID,
	}, nil
}

func toProfile(user *biz.User) *v1.UserProfile {
	return &v1.UserProfile{
		Id:       user.ID,
		Uid:      user.UID,
		Mobile:   user.Mobile,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Status:   user.Status,
	}
}
