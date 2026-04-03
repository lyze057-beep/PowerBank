package biz

import (
	"context"
	"strings"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/user/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultJWTIssuer = "powerbank-user-center"
	defaultJWTExpire = 24 * time.Hour
)

var (
	ErrAuthInvalidArgument  = errors.BadRequest(v1.ErrorReason_INVALID_ARGUMENT.String(), "invalid auth argument")
	ErrAuthUnauthorized     = errors.Unauthorized(v1.ErrorReason_UNAUTHORIZED.String(), "invalid mobile or password")
	ErrAuthUserStatus       = errors.Forbidden(v1.ErrorReason_USER_STATUS_INVALID.String(), "user status invalid")
	ErrAuthLoginState       = errors.Unauthorized(v1.ErrorReason_LOGIN_STATE_INVALID.String(), "login state invalid")
	ErrAuthMobileDuplicated = errors.BadRequest(v1.ErrorReason_INVALID_ARGUMENT.String(), "mobile already registered")
)

// AuthUser is auth-domain user model.
type AuthUser struct {
	ID           uint64
	UID          string
	Mobile       string
	PasswordHash string
	Nickname     string
	Avatar       string
	Status       int32
}

// CreateAuthUserInput is create-user payload.
type CreateAuthUserInput struct {
	UID          string
	Mobile       string
	PasswordHash string
	Nickname     string
	Avatar       string
}

// RegisterInput is register payload.
type RegisterInput struct {
	Mobile   string
	Password string
	Nickname string
	Avatar   string
}

// LoginInput is login payload.
type LoginInput struct {
	Mobile   string
	Password string
}

// AuthToken is auth reply payload.
type AuthToken struct {
	TokenType   string
	AccessToken string
	ExpiresIn   int64
	ExpireAt    int64
	UID         string
	JTI         string
}

// AuthRepo defines auth persistence behavior.
type AuthRepo interface {
	FindByMobile(ctx context.Context, mobile string) (*AuthUser, error)
	FindByUID(ctx context.Context, uid string) (*AuthUser, error)
	CreateUser(ctx context.Context, in CreateAuthUserInput) (*AuthUser, error)
	UpdateLastLoginAt(ctx context.Context, uid string) error
	SaveSession(ctx context.Context, uid, jti string, ttl time.Duration) error
	DeleteSession(ctx context.Context, uid, jti string) error
	CheckSession(ctx context.Context, uid, jti string) (bool, error)
}

// AuthUsecase handles register/login/logout/refresh.
type AuthUsecase struct {
	repo    AuthRepo
	log     *log.Helper
	secret  string
	issuer  string
	expire  time.Duration
	nowFunc func() time.Time
}

// NewAuthUsecase creates auth usecase.
func NewAuthUsecase(repo AuthRepo, jwtConf *conf.JWT, logger log.Logger) *AuthUsecase {
	secret := "secret"
	issuer := defaultJWTIssuer
	expire := defaultJWTExpire
	if jwtConf != nil {
		if strings.TrimSpace(jwtConf.Key) != "" {
			secret = jwtConf.Key
		}
		if strings.TrimSpace(jwtConf.Issuer) != "" {
			issuer = jwtConf.Issuer
		}
		if jwtConf.Expire != nil && jwtConf.Expire.AsDuration() > 0 {
			expire = jwtConf.Expire.AsDuration()
		}
	}
	return &AuthUsecase{
		repo:    repo,
		log:     log.NewHelper(log.With(logger, "module", "biz/auth")),
		secret:  secret,
		issuer:  issuer,
		expire:  expire,
		nowFunc: time.Now,
	}
}

// Register creates account then returns one access token.
func (uc *AuthUsecase) Register(ctx context.Context, in RegisterInput) (*AuthToken, error) {
	in.Mobile = strings.TrimSpace(in.Mobile)
	in.Password = strings.TrimSpace(in.Password)
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.Avatar = strings.TrimSpace(in.Avatar)
	if !isValidMobile(in.Mobile) || !isValidPassword(in.Password) {
		return nil, ErrAuthInvalidArgument
	}
	existing, err := uc.repo.FindByMobile(ctx, in.Mobile)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAuthMobileDuplicated
	}

	passwordHash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	if in.Nickname == "" {
		in.Nickname = defaultNickname(in.Mobile)
	}
	created, err := uc.repo.CreateUser(ctx, CreateAuthUserInput{
		UID:          newUID(),
		Mobile:       in.Mobile,
		PasswordHash: passwordHash,
		Nickname:     in.Nickname,
		Avatar:       in.Avatar,
	})
	if err != nil {
		return nil, err
	}
	return uc.issueForUser(ctx, created.UID)
}

// Login validates account/password then returns one access token.
func (uc *AuthUsecase) Login(ctx context.Context, in LoginInput) (*AuthToken, error) {
	in.Mobile = strings.TrimSpace(in.Mobile)
	in.Password = strings.TrimSpace(in.Password)
	if !isValidMobile(in.Mobile) || !isValidPassword(in.Password) {
		return nil, ErrAuthInvalidArgument
	}
	user, err := uc.repo.FindByMobile(ctx, in.Mobile)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrAuthUnauthorized
	}
	if user.Status != 1 {
		return nil, ErrAuthUserStatus
	}
	if !verifyPassword(user.PasswordHash, in.Password) {
		return nil, ErrAuthUnauthorized
	}
	return uc.issueForUser(ctx, user.UID)
}

// Logout invalidates one login session by uid+jti.
func (uc *AuthUsecase) Logout(ctx context.Context, uid, jti string) error {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(jti) == "" {
		return ErrAuthInvalidArgument
	}
	return uc.repo.DeleteSession(ctx, uid, jti)
}

// Refresh rotates jti then returns a new access token.
func (uc *AuthUsecase) Refresh(ctx context.Context, uid, oldJTI string) (*AuthToken, error) {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(oldJTI) == "" {
		return nil, ErrAuthInvalidArgument
	}
	ok, err := uc.repo.CheckSession(ctx, uid, oldJTI)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAuthLoginState
	}
	token, err := uc.newToken(uid)
	if err != nil {
		return nil, err
	}
	if err = uc.repo.SaveSession(ctx, uid, token.JTI, uc.expire); err != nil {
		return nil, err
	}
	if err = uc.repo.DeleteSession(ctx, uid, oldJTI); err != nil {
		return nil, err
	}
	return token, nil
}

// GenerateMockToken generates a signed token for given uid without saving session.
func (uc *AuthUsecase) GenerateMockToken(ctx context.Context, uid string) (*AuthToken, error) {
	if strings.TrimSpace(uid) == "" {
		uid = newUID()
	}
	return uc.newToken(uid)
}

func (uc *AuthUsecase) issueForUser(ctx context.Context, uid string) (*AuthToken, error) {
	token, err := uc.newToken(uid)
	if err != nil {
		return nil, err
	}
	if err = uc.repo.SaveSession(ctx, uid, token.JTI, uc.expire); err != nil {
		return nil, err
	}
	if err = uc.repo.UpdateLastLoginAt(ctx, uid); err != nil {
		uc.log.Warnf("update last login failed: uid=%s err=%v", uid, err)
	}
	return token, nil
}

func (uc *AuthUsecase) newToken(uid string) (*AuthToken, error) {
	now := uc.nowFunc()
	expireAt := now.Add(uc.expire)
	jti := strings.ReplaceAll(uuid.NewString(), "-", "")
	claims := &pkg.UserClaims{
		UID: uid,
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    uc.issuer,
			Subject:   uid,
			ID:        jti,
			IssuedAt:  jwtv5.NewNumericDate(now),
			NotBefore: jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(expireAt),
		},
	}
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(uc.secret))
	if err != nil {
		return nil, err
	}
	return &AuthToken{
		TokenType:   "Bearer",
		AccessToken: signed,
		ExpiresIn:   int64(uc.expire.Seconds()),
		ExpireAt:    expireAt.Unix(),
		UID:         uid,
		JTI:         jti,
	}, nil
}

func hashPassword(plain string) (string, error) {
	raw, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func verifyPassword(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}

func isValidMobile(mobile string) bool {
	l := len(mobile)
	return l >= 6 && l <= 20
}

func isValidPassword(password string) bool {
	l := len(password)
	return l >= 6 && l <= 64
}

func defaultNickname(mobile string) string {
	if len(mobile) >= 4 {
		return "用户" + mobile[len(mobile)-4:]
	}
	return "用户"
}

func newUID() string {
	return "u" + strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))
}
