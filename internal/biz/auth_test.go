package biz

import (
	"context"
	"io"
	"testing"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/user/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/durationpb"
)

type mockAuthRepo struct {
	usersByMobile map[string]*AuthUser
	usersByUID    map[string]*AuthUser
	sessions      map[string]struct{}
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		usersByMobile: map[string]*AuthUser{},
		usersByUID:    map[string]*AuthUser{},
		sessions:      map[string]struct{}{},
	}
}

func (m *mockAuthRepo) FindByMobile(_ context.Context, mobile string) (*AuthUser, error) {
	return m.usersByMobile[mobile], nil
}

func (m *mockAuthRepo) FindByUID(_ context.Context, uid string) (*AuthUser, error) {
	return m.usersByUID[uid], nil
}

func (m *mockAuthRepo) CreateUser(_ context.Context, in CreateAuthUserInput) (*AuthUser, error) {
	user := &AuthUser{
		ID:           uint64(len(m.usersByUID) + 1),
		UID:          in.UID,
		Mobile:       in.Mobile,
		PasswordHash: in.PasswordHash,
		Nickname:     in.Nickname,
		Avatar:       in.Avatar,
		Status:       1,
	}
	m.usersByMobile[user.Mobile] = user
	m.usersByUID[user.UID] = user
	return user, nil
}

func (m *mockAuthRepo) UpdateLastLoginAt(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthRepo) SaveSession(_ context.Context, uid, jti string, _ time.Duration) error {
	m.sessions[uid+":"+jti] = struct{}{}
	return nil
}

func (m *mockAuthRepo) DeleteSession(_ context.Context, uid, jti string) error {
	delete(m.sessions, uid+":"+jti)
	return nil
}

func (m *mockAuthRepo) CheckSession(_ context.Context, uid, jti string) (bool, error) {
	_, ok := m.sessions[uid+":"+jti]
	return ok, nil
}

func TestAuthRegisterAndLogin(t *testing.T) {
	repo := newMockAuthRepo()
	uc := NewAuthUsecase(repo, &conf.JWT{
		Key:    "test-secret",
		Issuer: "test-issuer",
		Expire: durationpb.New(time.Hour),
	}, log.NewStdLogger(io.Discard))

	reg, err := uc.Register(context.Background(), RegisterInput{
		Mobile:   "13800138000",
		Password: "abc12345",
		Nickname: "tester",
	})
	if err != nil {
		t.Fatalf("Register() unexpected err: %v", err)
	}
	if reg.AccessToken == "" || reg.UID == "" || reg.JTI == "" {
		t.Fatal("Register() token payload is empty")
	}
	if _, ok := repo.sessions[reg.UID+":"+reg.JTI]; !ok {
		t.Fatal("Register() session was not saved")
	}

	login, err := uc.Login(context.Background(), LoginInput{
		Mobile:   "13800138000",
		Password: "abc12345",
	})
	if err != nil {
		t.Fatalf("Login() unexpected err: %v", err)
	}
	if login.AccessToken == "" || login.JTI == "" {
		t.Fatal("Login() token payload is empty")
	}
}

func TestAuthRefreshRotateSession(t *testing.T) {
	repo := newMockAuthRepo()
	uc := NewAuthUsecase(repo, &conf.JWT{
		Key:    "test-secret",
		Issuer: "test-issuer",
		Expire: durationpb.New(time.Hour),
	}, log.NewStdLogger(io.Discard))

	login, err := uc.Register(context.Background(), RegisterInput{
		Mobile:   "13900139000",
		Password: "abc12345",
	})
	if err != nil {
		t.Fatalf("Register() unexpected err: %v", err)
	}
	refreshed, err := uc.Refresh(context.Background(), login.UID, login.JTI)
	if err != nil {
		t.Fatalf("Refresh() unexpected err: %v", err)
	}
	if refreshed.JTI == login.JTI {
		t.Fatal("Refresh() jti not rotated")
	}
	if _, ok := repo.sessions[login.UID+":"+login.JTI]; ok {
		t.Fatal("Refresh() old session still exists")
	}
	if _, ok := repo.sessions[refreshed.UID+":"+refreshed.JTI]; !ok {
		t.Fatal("Refresh() new session missing")
	}
}

func TestAuthLoginBadPassword(t *testing.T) {
	repo := newMockAuthRepo()
	uc := NewAuthUsecase(repo, &conf.JWT{
		Key:    "test-secret",
		Issuer: "test-issuer",
		Expire: durationpb.New(time.Hour),
	}, log.NewStdLogger(io.Discard))

	_, err := uc.Register(context.Background(), RegisterInput{
		Mobile:   "13700137000",
		Password: "abc12345",
	})
	if err != nil {
		t.Fatalf("Register() unexpected err: %v", err)
	}

	_, err = uc.Login(context.Background(), LoginInput{
		Mobile:   "13700137000",
		Password: "wrong-pass",
	})
	if err == nil {
		t.Fatal("Login() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != v1.ErrorReason_UNAUTHORIZED.String() {
		t.Fatalf("Login() reason=%s, want %s", se.Reason, v1.ErrorReason_UNAUTHORIZED.String())
	}
}
