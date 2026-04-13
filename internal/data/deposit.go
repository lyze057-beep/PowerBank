package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

const depositExemptionIdempotentKey = "idemp:deposit:exemption:%s:%s"

type depositRepo struct {
	data *Data
	log  *log.Helper
}

func NewDepositRepo(data *Data, logger log.Logger) biz.DepositRepo {
	return &depositRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/deposit")),
	}
}

func (r *depositRepo) GetOrCreateProfile(ctx context.Context, uid string, depositAmount int64) (*biz.DepositProfile, error) {
	const insertStmt = `INSERT INTO deposit_profiles(uid, status, deposit_amount, paid, exempt)
VALUES(?, ?, ?, 0, 0)
ON DUPLICATE KEY UPDATE uid = uid`
	if _, err := r.data.DB().ExecContext(ctx, insertStmt, uid, int32(biz.DepositStatusRequired), depositAmount); err != nil {
		return nil, err
	}
	const query = `SELECT uid, status, deposit_amount, paid, exempt, active_deposit_order_no, exempt_provider, exempt_expire_at
FROM deposit_profiles WHERE uid = ? LIMIT 1`
	row := r.data.DB().QueryRowContext(ctx, query, uid)
	return scanDepositProfile(row)
}

func (r *depositRepo) FindDepositOrderByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.DepositOrder, error) {
	const query = `SELECT deposit_order_no, uid, out_trade_no, client_req_id, channel, pay_mode, amount, status, created_at
FROM deposit_orders
WHERE uid = ? AND client_req_id = ?
LIMIT 1`
	row := r.data.DB().QueryRowContext(ctx, query, uid, clientReqID)
	return scanDepositOrder(row)
}

func (r *depositRepo) CreateDepositOrder(ctx context.Context, order *biz.DepositOrder) error {
	const stmt = `INSERT INTO deposit_orders(deposit_order_no, uid, out_trade_no, client_req_id, channel, pay_mode, amount, status)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.data.DB().ExecContext(ctx, stmt,
		order.DepositOrderNo,
		order.UID,
		order.OutTradeNo,
		order.ClientReqID,
		order.Channel,
		int32(order.PayMode),
		order.Amount,
		int32(order.Status),
	); err != nil {
		if isDuplicateErr(err) {
			return biz.ErrDepositDuplicateRequest
		}
		return err
	}
	const profileStmt = `UPDATE deposit_profiles SET active_deposit_order_no = ?, updated_at = NOW(3) WHERE uid = ?`
	_, err := r.data.DB().ExecContext(ctx, profileStmt, order.DepositOrderNo, order.UID)
	return err
}

func (r *depositRepo) MarkDepositOrderPaying(ctx context.Context, outTradeNo string) error {
	const stmt = `UPDATE deposit_orders SET status = ?, updated_at = NOW(3) WHERE out_trade_no = ? AND status = ?`
	_, err := r.data.DB().ExecContext(ctx, stmt, int32(biz.PayStatusPaying), outTradeNo, int32(biz.PayStatusInit))
	return err
}

func (r *depositRepo) FindExemptionByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.DepositExemptionRecord, error) {
	const query = `SELECT exemption_id, uid, client_req_id, provider, credit_score, status, reason, expire_at, created_at
FROM deposit_exemption_records
WHERE uid = ? AND client_req_id = ?
LIMIT 1`
	row := r.data.DB().QueryRowContext(ctx, query, uid, clientReqID)
	return scanDepositExemption(row)
}

func (r *depositRepo) AcquireExemptionIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(depositExemptionIdempotentKey, uid, clientReqID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *depositRepo) CreateExemptionRecord(ctx context.Context, profile *biz.DepositProfile, record *biz.DepositExemptionRecord) error {
	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const insertStmt = `INSERT INTO deposit_exemption_records(exemption_id, uid, client_req_id, provider, credit_score, status, reason, expire_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err = tx.ExecContext(ctx, insertStmt,
		record.ExemptionID,
		record.UID,
		record.ClientReqID,
		string(record.Provider),
		record.CreditScore,
		int32(record.Status),
		record.Reason,
		nullableTime(record.ExpireAt),
	); err != nil {
		if isDuplicateErr(err) {
			return biz.ErrDepositDuplicateRequest
		}
		return err
	}
	if record.Status == biz.ExemptionStatusApproved {
		const updateProfile = `UPDATE deposit_profiles
SET status = ?, paid = 0, exempt = 1, active_deposit_order_no = '', exempt_provider = ?, exempt_expire_at = ?, updated_at = NOW(3)
WHERE uid = ?`
		if _, err = tx.ExecContext(ctx, updateProfile,
			int32(biz.DepositStatusExempt),
			string(record.Provider),
			nullableTime(record.ExpireAt),
			record.UID,
		); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	record.CreatedAt = time.Now()
	return nil
}

func (r *depositRepo) ListRecords(ctx context.Context, uid string, page, pageSize int32) ([]*biz.DepositRecord, error) {
	offset := (page - 1) * pageSize
	const query = `
SELECT record_type, record_id, deposit_order_no, out_trade_no, order_status, amount, channel, exemption_status, exemption_provider, credit_score, reason, created_at, expire_at
FROM (
  SELECT 'ORDER' AS record_type,
         deposit_order_no AS record_id,
         deposit_order_no,
         out_trade_no,
         status AS order_status,
         amount,
         channel,
         0 AS exemption_status,
         '' AS exemption_provider,
         0 AS credit_score,
         '' AS reason,
         created_at,
         NULL AS expire_at
  FROM deposit_orders
  WHERE uid = ?
  UNION ALL
  SELECT 'EXEMPTION' AS record_type,
         exemption_id AS record_id,
         '' AS deposit_order_no,
         '' AS out_trade_no,
         0 AS order_status,
         0 AS amount,
         '' AS channel,
         status AS exemption_status,
         provider AS exemption_provider,
         credit_score,
         reason,
         created_at,
         expire_at
  FROM deposit_exemption_records
  WHERE uid = ?
) records
ORDER BY created_at DESC
LIMIT ? OFFSET ?`
	rows, err := r.data.DB().QueryContext(ctx, query, uid, uid, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*biz.DepositRecord, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanDepositRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanDepositProfile(row *sql.Row) (*biz.DepositProfile, error) {
	profile := &biz.DepositProfile{}
	var (
		status       int32
		paid         int32
		exempt       int32
		provider     string
		expireAtNull sql.NullTime
	)
	if err := row.Scan(&profile.UID, &status, &profile.DepositAmount, &paid, &exempt, &profile.ActiveOrderNo, &provider, &expireAtNull); err != nil {
		return nil, err
	}
	profile.Status = biz.DepositStatus(status)
	profile.Paid = paid == 1
	profile.Exempt = exempt == 1
	profile.ExemptProvider = biz.ExemptionProvider(provider)
	if expireAtNull.Valid {
		profile.ExemptExpireAt = expireAtNull.Time
	}
	return profile, nil
}

func scanDepositOrder(row *sql.Row) (*biz.DepositOrder, error) {
	order := &biz.DepositOrder{}
	var payMode, status int32
	if err := row.Scan(&order.DepositOrderNo, &order.UID, &order.OutTradeNo, &order.ClientReqID, &order.Channel, &payMode, &order.Amount, &status, &order.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	order.PayMode = biz.PayMode(payMode)
	order.Status = biz.PayStatus(status)
	return order, nil
}

func scanDepositExemption(row *sql.Row) (*biz.DepositExemptionRecord, error) {
	record := &biz.DepositExemptionRecord{}
	var (
		status       int32
		provider     string
		expireAtNull sql.NullTime
	)
	if err := row.Scan(&record.ExemptionID, &record.UID, &record.ClientReqID, &provider, &record.CreditScore, &status, &record.Reason, &expireAtNull, &record.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	record.Provider = biz.ExemptionProvider(provider)
	record.Status = biz.ExemptionStatus(status)
	if expireAtNull.Valid {
		record.ExpireAt = expireAtNull.Time
	}
	return record, nil
}

func scanDepositRecord(rows *sql.Rows) (*biz.DepositRecord, error) {
	item := &biz.DepositRecord{}
	var (
		orderStatus       int32
		exemptionStatus   int32
		exemptionProvider string
		expireAtNull      sql.NullTime
	)
	if err := rows.Scan(&item.RecordType, &item.RecordID, &item.DepositOrderNo, &item.OutTradeNo, &orderStatus, &item.Amount, &item.Channel, &exemptionStatus, &exemptionProvider, &item.CreditScore, &item.Reason, &item.CreatedAt, &expireAtNull); err != nil {
		return nil, err
	}
	item.OrderStatus = biz.PayStatus(orderStatus)
	item.ExemptionStatus = biz.ExemptionStatus(exemptionStatus)
	item.ExemptionProvider = biz.ExemptionProvider(exemptionProvider)
	if expireAtNull.Valid {
		item.ExpireAt = expireAtNull.Time
	}
	return item, nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
