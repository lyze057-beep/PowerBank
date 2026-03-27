package biz

import (
	"context"
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

type mockUserRepo struct {
	user      *User
	updated   *User
	loggedIn  bool
	findErr   error
	updateErr error
	stateErr  error
}

func (m *mockUserRepo) FindByUID(context.Context, string) (*User, error) {
	return m.user, m.findErr
}

func (m *mockUserRepo) UpdateProfile(context.Context, string, UpdateProfileInput) (*User, error) {
	return m.updated, m.updateErr
}

func (m *mockUserRepo) CheckLoginState(context.Context, string, string) (bool, error) {
	return m.loggedIn, m.stateErr
}

func TestGetProfile(t *testing.T) {
	uc := NewUserUsecase(&mockUserRepo{
		user: &User{
			UID:    "u1001",
			Status: 1,
		},
	}, log.NewStdLogger(io.Discard))

	user, err := uc.GetProfile(context.Background(), "u1001")
	if err != nil {
		t.Fatalf("GetProfile() unexpected error: %v", err)
	}
	if user.UID != "u1001" {
		t.Fatalf("GetProfile() uid = %s, want u1001", user.UID)
	}
}

func TestUpdateProfileInvalid(t *testing.T) {
	uc := NewUserUsecase(&mockUserRepo{}, log.NewStdLogger(io.Discard))
	_, err := uc.UpdateProfile(context.Background(), "u1001", UpdateProfileInput{})
	if err == nil {
		t.Fatal("UpdateProfile() expected error, got nil")
	}
}

func TestCheckLoginState(t *testing.T) {
	uc := NewUserUsecase(&mockUserRepo{loggedIn: true}, log.NewStdLogger(io.Discard))
	ok, err := uc.CheckLoginState(context.Background(), "u1001", "jti-1")
	if err != nil {
		t.Fatalf("CheckLoginState() unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("CheckLoginState() = false, want true")
	}
}
