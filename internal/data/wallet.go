package data

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

type walletRepo struct {
	data *Data
	log  *log.Helper
}

// NewWalletRepo creates wallet repository.
func NewWalletRepo(data *Data, logger log.Logger) biz.WalletRepo {
	return &walletRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/wallet")),
	}
}

func (r *walletRepo) GetBalance(ctx context.Context, uid string) (int64, error) {
	const query = `SELECT balance FROM user_wallets WHERE uid = ? LIMIT 1`
	var balance int64
	if err := r.data.DB().QueryRowContext(ctx, query, uid).Scan(&balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return balance, nil
}

func (r *walletRepo) GetCachedBalance(ctx context.Context, uid string) (int64, bool, error) {
	key := pkg.BuildWalletBalanceKey(uid)
	val, err := r.data.Redis().Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, false, nil
		}
		return 0, false, err
	}
	balance, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return balance, true, nil
}

func (r *walletRepo) SetCachedBalance(ctx context.Context, uid string, balance int64, ttl time.Duration) error {
	key := pkg.BuildWalletBalanceKey(uid)
	return r.data.Redis().Set(ctx, key, strconv.FormatInt(balance, 10), ttl).Err()
}
