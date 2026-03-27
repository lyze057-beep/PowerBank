package data

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/log"
)

type userRepo struct {
	data *Data
	log  *log.Helper
}

// NewUserRepo creates mysql+redis backed user repository.
func NewUserRepo(data *Data, logger log.Logger) biz.UserRepo {
	return &userRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/user")),
	}
}

func (r *userRepo) FindByUID(ctx context.Context, uid string) (*biz.User, error) {
	const query = `SELECT id, uid, mobile, nickname, avatar, status
FROM users WHERE uid = ? AND deleted_at IS NULL LIMIT 1`

	row := r.data.DB().QueryRowContext(ctx, query, uid)
	user := &biz.User{}
	if err := row.Scan(&user.ID, &user.UID, &user.Mobile, &user.Nickname, &user.Avatar, &user.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *userRepo) UpdateProfile(ctx context.Context, uid string, in biz.UpdateProfileInput) (*biz.User, error) {
	const query = `UPDATE users
SET nickname = COALESCE(NULLIF(?, ''), nickname),
    avatar = COALESCE(NULLIF(?, ''), avatar),
    updated_at = NOW(3)
WHERE uid = ? AND deleted_at IS NULL`

	res, err := r.data.DB().ExecContext(ctx, query, in.Nickname, in.Avatar, uid)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, biz.ErrProfileUserNotFound
	}
	return r.FindByUID(ctx, uid)
}

func (r *userRepo) CheckLoginState(ctx context.Context, uid, jti string) (bool, error) {
	key := pkg.BuildSessionKey(uid, jti)
	exists, err := r.data.Redis().Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
