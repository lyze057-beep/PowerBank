package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	borrowIdempotentKey       = "idemp:rent:borrow:%s:%s"
	orderExceptionIdempotentKey = "idemp:rent:exception:%s:%s"
)

type rentRepo struct {
	data *Data
	log  *log.Helper
}

func NewChargerRepo(data *Data, logger log.Logger) biz.ChargerRepo {
	return &rentRepo{data: data, log: log.NewHelper(log.With(logger, "module", "data/charger"))}
}

func NewOrderRepo(data *Data, logger log.Logger) biz.OrderRepo {
	return &rentRepo{data: data, log: log.NewHelper(log.With(logger, "module", "data/order"))}
}

func (r *rentRepo) AcquireBorrowIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(borrowIdempotentKey, uid, clientReqID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *rentRepo) FindRentOrderByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.RentOrder, error) {
	const query = rentOrderBaseQuery + ` WHERE uid = ? AND client_req_id = ? LIMIT 1`
	return r.queryOne(ctx, query, uid, clientReqID)
}

func (r *rentRepo) HasActiveOrder(ctx context.Context, uid string) (bool, error) {
	const query = `SELECT 1 FROM rent_orders WHERE uid = ? AND status IN (?, ?, ?, ?, ?) LIMIT 1`
	var one int
	if err := r.data.DB().QueryRowContext(ctx, query, uid,
		int32(biz.RentOrderStatusPendingBorrow),
		int32(biz.RentOrderStatusInUse),
		int32(biz.RentOrderStatusReturnPending),
		int32(biz.RentOrderStatusPayPending),
		int32(biz.RentOrderStatusException),
	).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *rentRepo) CreateBorrowOrder(ctx context.Context, uid, stationID, clientReqID string, depositAmount int64) (*biz.RentOrder, error) {
	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var (
		stationStatus int32
		pricingRuleID string
	)
	const stationQuery = `SELECT status, pricing_rule_id FROM stations WHERE station_id = ? LIMIT 1 FOR UPDATE`
	if err = tx.QueryRowContext(ctx, stationQuery, stationID).Scan(&stationStatus, &pricingRuleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, biz.ErrChargerStationNotFound
		}
		return nil, err
	}
	if stationStatus != 1 {
		return nil, biz.ErrChargerStationOffline
	}

	const slotQuery = `SELECT s.slot_id, s.powerbank_id
FROM station_slots s
WHERE s.station_id = ? AND s.status = 1 AND s.powerbank_id <> ''
ORDER BY s.id ASC
LIMIT 1 FOR UPDATE`
	var slotID, powerbankID string
	if err = tx.QueryRowContext(ctx, slotQuery, stationID).Scan(&slotID, &powerbankID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, biz.ErrChargerNoAvailableBank
		}
		return nil, err
	}

	rentOrderNo := biz.NewRentOrderNoForData(time.Now())
	const updateSlot = `UPDATE station_slots SET status = 2, updated_at = NOW(3) WHERE slot_id = ? AND status = 1`
	if _, err = tx.ExecContext(ctx, updateSlot, slotID); err != nil {
		return nil, err
	}
	const insertOrder = `INSERT INTO rent_orders(uid, rent_order_no, client_req_id, station_id, powerbank_id, borrow_slot_id, pricing_rule_id, status, pay_status, deposit_amount)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err = tx.ExecContext(ctx, insertOrder,
		uid,
		rentOrderNo,
		clientReqID,
		stationID,
		powerbankID,
		slotID,
		pricingRuleID,
		int32(biz.RentOrderStatusPendingBorrow),
		int32(biz.RentPayStatusNotRequired),
		depositAmount,
	); err != nil {
		if isDuplicateErr(err) {
			return nil, biz.ErrChargerDuplicateRequest
		}
		return nil, err
	}
	if err = r.insertOrderEvent(ctx, tx, rentOrderNo, "BORROW_RESERVED", "borrow:"+rentOrderNo+":reserved", map[string]any{"station_id": stationID, "slot_id": slotID, "powerbank_id": powerbankID}); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetOrderDetail(ctx, uid, rentOrderNo)
}

func (r *rentRepo) HandleBorrowResult(ctx context.Context, rentOrderNo string, success bool, reason string) (*biz.RentOrder, error) {
	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	order, err := r.queryOneTx(ctx, tx, rentOrderBaseQuery+` WHERE rent_order_no = ? LIMIT 1 FOR UPDATE`, rentOrderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, nil
	}
	if order.Status != biz.RentOrderStatusPendingBorrow {
		_ = tx.Rollback()
		return order, nil
	}

	eventKey := fmt.Sprintf("borrow:%s:%t", rentOrderNo, success)
	if err = r.insertOrderEvent(ctx, tx, rentOrderNo, "BORROW_CALLBACK", eventKey, map[string]any{"success": success, "reason": reason}); err != nil {
		if isDuplicateErr(err) {
			_ = tx.Rollback()
			return order, nil
		}
		return nil, err
	}

	if success {
		const clearSlot = `UPDATE station_slots SET status = 3, powerbank_id = '', updated_at = NOW(3) WHERE slot_id = ?`
		if _, err = tx.ExecContext(ctx, clearSlot, order.BorrowSlotID); err != nil {
			return nil, err
		}
		const updatePowerbank = `UPDATE powerbanks SET status = 2, current_station_id = '', current_slot_id = '', updated_at = NOW(3) WHERE powerbank_id = ?`
		if _, err = tx.ExecContext(ctx, updatePowerbank, order.PowerbankID); err != nil {
			return nil, err
		}
		const updateOrder = `UPDATE rent_orders SET status = ?, borrowed_at = NOW(3), updated_at = NOW(3) WHERE rent_order_no = ?`
		if _, err = tx.ExecContext(ctx, updateOrder, int32(biz.RentOrderStatusInUse), rentOrderNo); err != nil {
			return nil, err
		}
	} else {
		const restoreSlot = `UPDATE station_slots SET status = 1, updated_at = NOW(3) WHERE slot_id = ?`
		if _, err = tx.ExecContext(ctx, restoreSlot, order.BorrowSlotID); err != nil {
			return nil, err
		}
		const updateOrder = `UPDATE rent_orders SET status = ?, exception_desc = ?, updated_at = NOW(3) WHERE rent_order_no = ?`
		if _, err = tx.ExecContext(ctx, updateOrder, int32(biz.RentOrderStatusCancelled), reason, rentOrderNo); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.queryOne(ctx, rentOrderBaseQuery+` WHERE rent_order_no = ? LIMIT 1`, rentOrderNo)
}

func (r *rentRepo) HandleReturnResult(ctx context.Context, rentOrderNo, stationID, slotID string, success bool, reason string) (*biz.RentOrder, error) {
	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	order, err := r.queryOneTx(ctx, tx, rentOrderBaseQuery+` WHERE rent_order_no = ? LIMIT 1 FOR UPDATE`, rentOrderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, nil
	}
	if order.Status != biz.RentOrderStatusInUse && order.Status != biz.RentOrderStatusPayPending && order.Status != biz.RentOrderStatusCompleted {
		return nil, biz.ErrOrderReturnConflict
	}

	eventKey := fmt.Sprintf("return:%s:%s:%s", rentOrderNo, stationID, slotID)
	if err = r.insertOrderEvent(ctx, tx, rentOrderNo, "RETURN_CALLBACK", eventKey, map[string]any{"success": success, "reason": reason, "station_id": stationID, "slot_id": slotID}); err != nil {
		if isDuplicateErr(err) {
			_ = tx.Rollback()
			return order, nil
		}
		return nil, err
	}

	if !success {
		const updateOrder = `UPDATE rent_orders SET status = ?, exception_desc = ?, updated_at = NOW(3) WHERE rent_order_no = ?`
		if _, err = tx.ExecContext(ctx, updateOrder, int32(biz.RentOrderStatusException), reason, rentOrderNo); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return r.queryOne(ctx, rentOrderBaseQuery+` WHERE rent_order_no = ? LIMIT 1`, rentOrderNo)
	}

	var pricingName string
	var freeMinutes, unitMinutes int32
	var unitPrice, dailyCap int64
	const pricingQuery = `SELECT name, free_minutes, unit_minutes, unit_price, daily_cap FROM pricing_rules WHERE rule_id = ? LIMIT 1`
	if err = tx.QueryRowContext(ctx, pricingQuery, order.PricingRuleID).Scan(&pricingName, &freeMinutes, &unitMinutes, &unitPrice, &dailyCap); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pricingName = "default"
			freeMinutes = 0
			unitMinutes = 30
			unitPrice = 100
			dailyCap = 2999
		} else {
			return nil, err
		}
	}

	const updateSlot = `UPDATE station_slots SET station_id = ?, powerbank_id = ?, status = 1, updated_at = NOW(3) WHERE slot_id = ?`
	if _, err = tx.ExecContext(ctx, updateSlot, stationID, order.PowerbankID, slotID); err != nil {
		return nil, err
	}
	const updatePowerbank = `UPDATE powerbanks SET status = 1, current_station_id = ?, current_slot_id = ?, updated_at = NOW(3) WHERE powerbank_id = ?`
	if _, err = tx.ExecContext(ctx, updatePowerbank, stationID, slotID, order.PowerbankID); err != nil {
		return nil, err
	}

	returnedAt := time.Now()
	rentFee := calcRentFee(order.BorrowedAt, returnedAt, freeMinutes, unitMinutes, unitPrice, dailyCap)
	status := biz.RentOrderStatusCompleted
	payStatus := biz.RentPayStatusNotRequired
	if rentFee > 0 {
		status = biz.RentOrderStatusPayPending
		payStatus = biz.RentPayStatusUnpaid
	}
	const updateOrder = `UPDATE rent_orders
SET return_station_id = ?, return_slot_id = ?, status = ?, pay_status = ?, rent_fee = ?, returned_at = ?, pricing_rule_id = ?, updated_at = NOW(3)
WHERE rent_order_no = ?`
	if _, err = tx.ExecContext(ctx, updateOrder, stationID, slotID, int32(status), int32(payStatus), rentFee, returnedAt, order.PricingRuleID, rentOrderNo); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	updated, err := r.queryOne(ctx, rentOrderBaseQuery+` WHERE rent_order_no = ? LIMIT 1`, rentOrderNo)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		updated.PricingRuleName = pricingName
	}
	return updated, nil
}

func (r *rentRepo) GetCurrentOrder(ctx context.Context, uid string) (*biz.RentOrder, error) {
	const query = rentOrderBaseQuery + ` WHERE uid = ? AND status IN (?, ?, ?, ?, ?) ORDER BY id DESC LIMIT 1`
	return r.queryOne(ctx, query, uid,
		int32(biz.RentOrderStatusPendingBorrow),
		int32(biz.RentOrderStatusInUse),
		int32(biz.RentOrderStatusReturnPending),
		int32(biz.RentOrderStatusPayPending),
		int32(biz.RentOrderStatusException),
	)
}

func (r *rentRepo) ListOrders(ctx context.Context, uid string, page, pageSize int32) ([]*biz.RentOrder, error) {
	offset := (page - 1) * pageSize
	const query = rentOrderBaseQuery + ` WHERE uid = ? ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := r.data.DB().QueryContext(ctx, query, uid, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*biz.RentOrder, 0, pageSize)
	for rows.Next() {
		order, scanErr := scanRentOrder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, order)
	}
	return items, rows.Err()
}

func (r *rentRepo) GetOrderDetail(ctx context.Context, uid, rentOrderNo string) (*biz.RentOrder, error) {
	const query = rentOrderBaseQuery + ` WHERE uid = ? AND rent_order_no = ? LIMIT 1`
	return r.queryOne(ctx, query, uid, rentOrderNo)
}

func (r *rentRepo) ReportException(ctx context.Context, in biz.ReportOrderExceptionInput) error {
	key := fmt.Sprintf(orderExceptionIdempotentKey, in.UID, in.ClientReqID)
	ok, err := r.data.Redis().SetNX(ctx, key, "1", 120*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return biz.ErrOrderDuplicate
	}

	tx, err := r.data.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const updateOrder = `UPDATE rent_orders
SET exception_reported = 1, exception_desc = ?, status = IF(status = ?, ?, status), updated_at = NOW(3)
WHERE uid = ? AND rent_order_no = ?`
	res, err := tx.ExecContext(ctx, updateOrder, in.Description, int32(biz.RentOrderStatusCompleted), int32(biz.RentOrderStatusException), in.UID, in.RentOrderNo)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return biz.ErrOrderNotFound
	}
	if err = r.insertOrderEvent(ctx, tx, in.RentOrderNo, "EXCEPTION_REPORTED", "exception:"+in.ClientReqID, map[string]any{"type": in.Type, "description": in.Description}); err != nil {
		return err
	}
	return tx.Commit()
}

const rentOrderBaseQuery = `SELECT uid, rent_order_no, client_req_id, station_id, return_station_id, powerbank_id, borrow_slot_id, return_slot_id, pricing_rule_id,
status, pay_status, deposit_amount, rent_fee, payment_out_trade_no, borrowed_at, returned_at, created_at, exception_reported, exception_desc
FROM rent_orders`

func (r *rentRepo) queryOne(ctx context.Context, query string, args ...any) (*biz.RentOrder, error) {
	rows, err := r.data.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanRentOrder(rows)
	}
	return nil, rows.Err()
}

func (r *rentRepo) queryOneTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (*biz.RentOrder, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanRentOrder(rows)
	}
	return nil, rows.Err()
}

func (r *rentRepo) insertOrderEvent(ctx context.Context, tx *sql.Tx, rentOrderNo, eventType, eventKey string, payload map[string]any) error {
	body, _ := json.Marshal(payload)
	const stmt = `INSERT INTO rent_order_events(rent_order_no, event_type, event_key, payload) VALUES(?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, stmt, rentOrderNo, eventType, eventKey, string(body))
	return err
}

func scanRentOrder(rows scanner) (*biz.RentOrder, error) {
	order := &biz.RentOrder{}
	var (
		status            int32
		payStatus         int32
		borrowedAt        sql.NullTime
		returnedAt        sql.NullTime
		exceptionReported int32
	)
	if err := rows.Scan(
		&order.UID,
		&order.RentOrderNo,
		&order.ClientReqID,
		&order.StationID,
		&order.ReturnStationID,
		&order.PowerbankID,
		&order.BorrowSlotID,
		&order.ReturnSlotID,
		&order.PricingRuleID,
		&status,
		&payStatus,
		&order.DepositAmount,
		&order.RentFee,
		&order.PaymentOutTradeNo,
		&borrowedAt,
		&returnedAt,
		&order.CreatedAt,
		&exceptionReported,
		&order.ExceptionDesc,
	); err != nil {
		return nil, err
	}
	order.Status = biz.RentOrderStatus(status)
	order.PayStatus = biz.RentPayStatus(payStatus)
	order.ExceptionReported = exceptionReported == 1
	if borrowedAt.Valid {
		order.BorrowedAt = borrowedAt.Time
	}
	if returnedAt.Valid {
		order.ReturnedAt = returnedAt.Time
	}
	return order, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func calcRentFee(borrowedAt, returnedAt time.Time, freeMinutes, unitMinutes int32, unitPrice, dailyCap int64) int64 {
	if borrowedAt.IsZero() || returnedAt.Before(borrowedAt) {
		return 0
	}
	durationMinutes := int32(returnedAt.Sub(borrowedAt).Minutes())
	if durationMinutes <= freeMinutes {
		return 0
	}
	chargeMinutes := durationMinutes - freeMinutes
	if unitMinutes <= 0 {
		unitMinutes = 30
	}
	units := int64(math.Ceil(float64(chargeMinutes) / float64(unitMinutes)))
	fee := units * unitPrice
	if dailyCap > 0 && fee > dailyCap {
		fee = dailyCap
	}
	return fee
}
