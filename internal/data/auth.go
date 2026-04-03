package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/log"
)

type authRepo struct {
	data *Data
	log  *log.Helper
}

// NewAuthRepo creates auth repository.
func NewAuthRepo(data *Data, logger log.Logger) biz.AuthRepo {
	return &authRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/auth")),
	}
}

func (r *authRepo) FindByMobile(ctx context.Context, mobile string) (*biz.AuthUser, error) {
	const query = `SELECT id, uid, mobile, password_hash, nickname, avatar, status
FROM users
WHERE mobile = ? AND deleted_at IS NULL
LIMIT 1`
	return r.scanOne(ctx, query, mobile)
}

func (r *authRepo) FindByUID(ctx context.Context, uid string) (*biz.AuthUser, error) {
	const query = `SELECT id, uid, mobile, password_hash, nickname, avatar, status
FROM users
WHERE uid = ? AND deleted_at IS NULL
LIMIT 1`
	return r.scanOne(ctx, query, uid)
}

func (r *authRepo) CreateUser(ctx context.Context, in biz.CreateAuthUserInput) (*biz.AuthUser, error) {
	const stmt = `INSERT INTO users(uid, mobile, password_hash, nickname, avatar, status)
VALUES(?, ?, ?, ?, ?, 1)`
	_, err := r.data.DB().ExecContext(ctx, stmt,
		in.UID,
		in.Mobile,
		in.PasswordHash,
		in.Nickname,
		in.Avatar,
	)
	if isDuplicateErr(err) {
		return nil, biz.ErrAuthMobileDuplicated
	}
	if err != nil {
		return nil, err
	}
	return r.FindByUID(ctx, in.UID)
}

func (r *authRepo) UpdateLastLoginAt(ctx context.Context, uid string) error {
	const stmt = `UPDATE users SET last_login_at = NOW(3), updated_at = NOW(3) WHERE uid = ? AND deleted_at IS NULL`
	_, err := r.data.DB().ExecContext(ctx, stmt, uid)
	return err
}

func (r *authRepo) SaveSession(ctx context.Context, uid, jti string, ttl time.Duration) error {
	key := pkg.BuildSessionKey(uid, jti)
	return r.data.Redis().Set(ctx, key, "1", ttl).Err()
}

func (r *authRepo) DeleteSession(ctx context.Context, uid, jti string) error {
	key := pkg.BuildSessionKey(uid, jti)
	return r.data.Redis().Del(ctx, key).Err()
}

func (r *authRepo) CheckSession(ctx context.Context, uid, jti string) (bool, error) {
	key := pkg.BuildSessionKey(uid, jti)
	exists, err := r.data.Redis().Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

func (r *authRepo) scanOne(ctx context.Context, query string, arg any) (*biz.AuthUser, error) {
	row := r.data.DB().QueryRowContext(ctx, query, arg)
	user := &biz.AuthUser{}
	if err := row.Scan(
		&user.ID,
		&user.UID,
		&user.Mobile,
		&user.PasswordHash,
		&user.Nickname,
		&user.Avatar,
		&user.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}
