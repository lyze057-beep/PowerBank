package service

import (
	"context"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// AuthService handles register/login/logout/refresh.
type AuthService struct {
	uc *biz.AuthUsecase
}

// NewAuthService creates auth service.
func NewAuthService(uc *biz.AuthUsecase) *AuthService {
	return &AuthService{uc: uc}
}

// RegisterRequest is register payload.
type RegisterRequest struct {
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// LoginRequest is login payload.
type LoginRequest struct {
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
}

// LogoutRequest is logout payload.
type LogoutRequest struct{}

// RefreshTokenRequest is refresh payload.
type RefreshTokenRequest struct{}

// AuthReply is auth success payload.
type AuthReply struct {
	TokenType   string `json:"tokenType"`
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"`
	ExpireAt    int64  `json:"expireAt"`
	Uid         string `json:"uid"`
}

// LogoutReply is logout result payload.
type LogoutReply struct {
	Success bool `json:"success"`
}

// Register creates account and returns token.
func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (*AuthReply, error) {
	token, err := s.uc.Register(ctx, biz.RegisterInput{
		Mobile:   req.Mobile,
		Password: req.Password,
		Nickname: req.Nickname,
		Avatar:   req.Avatar,
	})
	if err != nil {
		return nil, err
	}
	return toAuthReply(token), nil
}

// Login validates account/password and returns token.
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*AuthReply, error) {
	token, err := s.uc.Login(ctx, biz.LoginInput{
		Mobile:   req.Mobile,
		Password: req.Password,
	})
	if err != nil {
		return nil, err
	}
	return toAuthReply(token), nil
}

// Logout clears current login session.
func (s *AuthService) Logout(ctx context.Context, _ *LogoutRequest) (*LogoutReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.uc.Logout(ctx, claims.UID, claims.ID); err != nil {
		return nil, err
	}
	return &LogoutReply{Success: true}, nil
}

// RefreshToken rotates session jti and returns a new token.
func (s *AuthService) RefreshToken(ctx context.Context, _ *RefreshTokenRequest) (*AuthReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.uc.Refresh(ctx, claims.UID, claims.ID)
	if err != nil {
		return nil, err
	}
	return toAuthReply(token), nil
}

func toAuthReply(token *biz.AuthToken) *AuthReply {
	return &AuthReply{
		TokenType:   token.TokenType,
		AccessToken: token.AccessToken,
		ExpiresIn:   token.ExpiresIn,
		ExpireAt:    token.ExpireAt,
		Uid:         token.UID,
	}
}
