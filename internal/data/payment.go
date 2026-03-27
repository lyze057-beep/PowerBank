package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/log"
	mysqlerr "github.com/go-sql-driver/mysql"
)

const (
	createOrderIDKey = "idemp:pay:create:%s:%s"
	notifyLockKey    = "lock:pay:notify:%s"
)

type paymentRepo struct {
	data *Data
	log  *log.Helper
}

// NewPaymentRepo creates unified payment repository.
func NewPaymentRepo(data *Data, logger log.Logger) biz.PaymentRepo {
	return &paymentRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/payment")),
	}
}

func (r *paymentRepo) AcquireCreateIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(createOrderIDKey, uid, clientReqID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *paymentRepo) AcquireNotifyLock(ctx context.Context, eventID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(notifyLockKey, eventID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *paymentRepo) FindByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.PaymentOrder, error) {
	const query = `SELECT id, uid, out_trade_no, client_req_id, channel, pay_mode, biz_type, biz_order_no, amount,
status, prepay_id, code_url, jsapi_params, transaction_id
FROM payment_orders
WHERE uid = ? AND client_req_id = ? AND deleted_at IS NULL
LIMIT 1`
	return r.scanOrder(ctx, query, uid, clientReqID)
}

func (r *paymentRepo) CreateOrder(ctx context.Context, order *biz.PaymentOrder) error {
	const stmt = `INSERT INTO payment_orders
(uid, out_trade_no, client_req_id, channel, pay_mode, biz_type, biz_order_no, amount, status)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.data.DB().ExecContext(ctx, stmt,
		order.UID,
		order.OutTradeNo,
		order.ClientReqID,
		order.Channel,
		int32(order.PayMode),
		int32(order.BizType),
		order.BizOrderNo,
		order.Amount,
		int32(order.Status),
	)
	if isDuplicateErr(err) {
		return biz.ErrPaymentStatusInvalid
	}
	return err
}

func (r *paymentRepo) UpdatePrepay(ctx context.Context, outTradeNo, prepayID, codeURL string, jsapiParams map[string]string) error {
	jsapiBytes, err := json.Marshal(jsapiParams)
	if err != nil {
		return err
	}
	const stmt = `UPDATE payment_orders
SET prepay_id = ?, code_url = ?, jsapi_params = ?, status = ?, updated_at = NOW(3)
WHERE out_trade_no = ? AND status = ?`
	res, err := r.data.DB().ExecContext(ctx, stmt,
		prepayID,
		codeURL,
		string(jsapiBytes),
		int32(biz.PayStatusPaying),
		outTradeNo,
		int32(biz.PayStatusInit),
	)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return biz.ErrPaymentStatusInvalid
	}
	return nil
}

func (r *paymentRepo) GetByUIDAndOutTradeNo(ctx context.Context, uid, outTradeNo string) (*biz.PaymentOrder, error) {
	const query = `SELECT id, uid, out_trade_no, client_req_id, channel, pay_mode, biz_type, biz_order_no, amount,
status, prepay_id, code_url, jsapi_params, transaction_id
FROM payment_orders
WHERE uid = ? AND out_trade_no = ? AND deleted_at IS NULL
LIMIT 1`
	return r.scanOrder(ctx, query, uid, outTradeNo)
}

func (r *paymentRepo) HandleWxNotify(ctx context.Context, event *biz.WxNotifyEvent) (bool, error) {
	return r.handleNotify(ctx, notifyHandleInput{
		channel:       biz.PayChannelWx,
		eventID:       event.EventID,
		eventType:     event.EventType,
		outTradeNo:    event.OutTradeNo,
		tradeState:    event.TradeState,
		transactionID: event.TransactionID,
		rawBody:       event.RawBody,
		failReason:    "wx notify failed",
		successStates: map[string]struct{}{
			"SUCCESS": {},
		},
	})
}

func (r *paymentRepo) HandleAlipayNotify(ctx context.Context, event *biz.AlipayNotifyEvent) (bool, error) {
	return r.handleNotify(ctx, notifyHandleInput{
		channel:       biz.PayChannelAl,
		eventID:       event.EventID,
		eventType:     "ALIPAY_NOTIFY",
		outTradeNo:    event.OutTradeNo,
		tradeState:    event.TradeStatus,
		transactionID: event.TradeNo,
		rawBody:       event.RawBody,
		failReason:    "alipay notify failed",
		successStates: map[string]struct{}{
			"TRADE_SUCCESS":  {},
			"TRADE_FINISHED": {},
		},
	})
}

type notifyHandleInput struct {
	channel       string
	eventID       string
	eventType     string
	outTradeNo    string
	tradeState    string
	transactionID string
	rawBody       string
	failReason    string
	successStates map[string]struct{}
}

func (r *paymentRepo) handleNotify(ctx context.Context, in notifyHandleInput) (bool, error) {
	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	var invalidateUID string
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const insertEvent = `INSERT INTO payment_events(channel, event_id, out_trade_no, event_type, trade_state, payload)
VALUES(?, ?, ?, ?, ?, ?)`
	if _, err = tx.ExecContext(ctx, insertEvent,
		in.channel,
		in.eventID,
		in.outTradeNo,
		in.eventType,
		in.tradeState,
		in.rawBody,
	); err != nil {
		if isDuplicateErr(err) {
			_ = tx.Rollback()
			return false, nil
		}
		return false, err
	}

	targetStatus := biz.PayStatusFailed
	failedReason := in.failReason
	if _, ok := in.successStates[strings.ToUpper(strings.TrimSpace(in.tradeState))]; ok {
		targetStatus = biz.PayStatusSuccess
		failedReason = ""
	}

	var (
		uid     string
		bizType int32
		amount  int64
	)
	const queryOrder = `SELECT uid, biz_type, amount
FROM payment_orders
WHERE out_trade_no = ?
LIMIT 1 FOR UPDATE`
	if err = tx.QueryRowContext(ctx, queryOrder, in.outTradeNo).Scan(&uid, &bizType, &amount); err != nil {
		return false, err
	}

	const updateOrder = `UPDATE payment_orders
SET status = ?, transaction_id = ?, failed_reason = ?, paid_at = IF(? = ?, NOW(3), paid_at), updated_at = NOW(3)
WHERE out_trade_no = ? AND status IN (?, ?)`
	res, execErr := tx.ExecContext(ctx, updateOrder,
		int32(targetStatus),
		in.transactionID,
		failedReason,
		int32(targetStatus),
		int32(biz.PayStatusSuccess),
		in.outTradeNo,
		int32(biz.PayStatusInit),
		int32(biz.PayStatusPaying),
	)
	if execErr != nil {
		err = execErr
		return false, err
	}
	affected, affErr := res.RowsAffected()
	if affErr != nil {
		err = affErr
		return false, err
	}
	if affected > 0 && targetStatus == biz.PayStatusSuccess && bizType == int32(biz.BizTypeRecharge) {
		const insertRechargeRecord = `INSERT INTO wallet_recharge_records(uid, out_trade_no, channel, amount, status)
VALUES(?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE out_trade_no = out_trade_no`
		if _, err = tx.ExecContext(ctx, insertRechargeRecord, uid, in.outTradeNo, in.channel, amount, 1); err != nil {
			return false, err
		}

		const upsertWallet = `INSERT INTO user_wallets(uid, balance)
VALUES(?, ?)
ON DUPLICATE KEY UPDATE balance = balance + VALUES(balance), updated_at = NOW(3)`
		if _, err = tx.ExecContext(ctx, upsertWallet, uid, amount); err != nil {
			return false, err
		}
		invalidateUID = uid
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	if invalidateUID != "" {
		if delErr := r.data.Redis().Del(ctx, pkg.BuildWalletBalanceKey(invalidateUID)).Err(); delErr != nil {
			r.log.Warnf("invalidate wallet cache failed: uid=%s err=%v", invalidateUID, delErr)
		}
	}
	return true, nil
}

func (r *paymentRepo) scanOrder(ctx context.Context, query string, args ...any) (*biz.PaymentOrder, error) {
	row := r.data.DB().QueryRowContext(ctx, query, args...)
	var (
		order      biz.PaymentOrder
		payMode    int32
		bizType    int32
		status     int32
		jsapiRaw   []byte
		jsapiStore sql.NullString
	)
	if err := row.Scan(
		&order.ID,
		&order.UID,
		&order.OutTradeNo,
		&order.ClientReqID,
		&order.Channel,
		&payMode,
		&bizType,
		&order.BizOrderNo,
		&order.Amount,
		&status,
		&order.PrepayID,
		&order.CodeURL,
		&jsapiStore,
		&order.TransactionID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	order.PayMode = biz.PayMode(payMode)
	order.BizType = biz.BizType(bizType)
	order.Status = biz.PayStatus(status)
	order.JSAPIParams = map[string]string{}
	if jsapiStore.Valid && jsapiStore.String != "" {
		jsapiRaw = []byte(jsapiStore.String)
		_ = json.Unmarshal(jsapiRaw, &order.JSAPIParams)
	}
	return &order, nil
}

func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	var me *mysqlerr.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
