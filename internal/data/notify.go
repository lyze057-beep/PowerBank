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

const notifyIdempotentKey = "idemp:notify:push:%s:%s"

type notifyRepo struct {
	data *Data
	log  *log.Helper
}

// NewNotifyRepo creates notification repository.
func NewNotifyRepo(data *Data, logger log.Logger) biz.NotifyRepo {
	return &notifyRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/notify")),
	}
}

func (r *notifyRepo) AcquirePushIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(notifyIdempotentKey, uid, clientReqID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *notifyRepo) FindByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.MessageRecord, error) {
	const query = `SELECT id, uid, title, content, biz_type, biz_id, client_req_id, topic, status, failed_reason, created_at
FROM notification_records
WHERE uid = ? AND client_req_id = ?
LIMIT 1`
	return r.scanOne(ctx, query, uid, clientReqID)
}

func (r *notifyRepo) Create(ctx context.Context, record *biz.MessageRecord) error {
	const stmt = `INSERT INTO notification_records
(uid, title, content, biz_type, biz_id, client_req_id, topic, status, failed_reason)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.data.DB().ExecContext(ctx, stmt,
		record.UID,
		record.Title,
		record.Content,
		record.BizType,
		record.BizID,
		record.ClientReqID,
		record.Topic,
		int32(record.Status),
		record.FailedReason,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	record.ID = uint64(id)
	record.CreatedAt = time.Now()
	return nil
}

func (r *notifyRepo) MarkSent(ctx context.Context, id uint64) error {
	const stmt = `UPDATE notification_records
SET status = ?, failed_reason = '', updated_at = NOW(3)
WHERE id = ?`
	_, err := r.data.DB().ExecContext(ctx, stmt, int32(biz.MessageStatusSent), id)
	return err
}

func (r *notifyRepo) MarkFailed(ctx context.Context, id uint64, reason string) error {
	const stmt = `UPDATE notification_records
SET status = ?, failed_reason = ?, updated_at = NOW(3)
WHERE id = ?`
	_, err := r.data.DB().ExecContext(ctx, stmt, int32(biz.MessageStatusFailed), reason, id)
	return err
}

func (r *notifyRepo) ListByUID(ctx context.Context, uid string, page, pageSize int32) ([]*biz.MessageRecord, error) {
	offset := (page - 1) * pageSize
	const query = `SELECT id, uid, title, content, biz_type, biz_id, client_req_id, topic, status, failed_reason, created_at
FROM notification_records
WHERE uid = ?
ORDER BY id DESC
LIMIT ? OFFSET ?`
	rows, err := r.data.DB().QueryContext(ctx, query, uid, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]*biz.MessageRecord, 0, pageSize)
	for rows.Next() {
		record := &biz.MessageRecord{}
		var status int32
		if err = rows.Scan(
			&record.ID,
			&record.UID,
			&record.Title,
			&record.Content,
			&record.BizType,
			&record.BizID,
			&record.ClientReqID,
			&record.Topic,
			&status,
			&record.FailedReason,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.Status = biz.MessageStatus(status)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *notifyRepo) scanOne(ctx context.Context, query string, args ...any) (*biz.MessageRecord, error) {
	row := r.data.DB().QueryRowContext(ctx, query, args...)
	record := &biz.MessageRecord{}
	var status int32
	if err := row.Scan(
		&record.ID,
		&record.UID,
		&record.Title,
		&record.Content,
		&record.BizType,
		&record.BizID,
		&record.ClientReqID,
		&record.Topic,
		&status,
		&record.FailedReason,
		&record.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	record.Status = biz.MessageStatus(status)
	return record, nil
}
