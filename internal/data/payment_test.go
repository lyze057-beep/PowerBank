package data

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos/v2/log"
	mysqlerr "github.com/go-sql-driver/mysql"
)

func TestHandleWxNotifyDuplicateEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() err=%v", err)
	}
	defer db.Close()

	repo := &paymentRepo{
		data: &Data{db: db},
		log:  log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO payment_events").
		WillReturnError(&mysqlerr.MySQLError{Number: 1062, Message: "duplicate"})
	mock.ExpectRollback()

	processed, err := repo.HandleWxNotify(context.Background(), &biz.WxNotifyEvent{
		EventID:    "evt_1",
		OutTradeNo: "otn_1",
		TradeState: "SUCCESS",
		RawBody:    "{}",
	})
	if err != nil {
		t.Fatalf("HandleWxNotify() err=%v", err)
	}
	if processed {
		t.Fatal("HandleWxNotify() processed=true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() err=%v", err)
	}
}

func TestHandleAlipayNotifyDuplicateEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() err=%v", err)
	}
	defer db.Close()

	repo := &paymentRepo{
		data: &Data{db: db},
		log:  log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO payment_events").
		WillReturnError(&mysqlerr.MySQLError{Number: 1062, Message: "duplicate"})
	mock.ExpectRollback()

	processed, err := repo.HandleAlipayNotify(context.Background(), &biz.AlipayNotifyEvent{
		EventID:     "ali_evt_1",
		OutTradeNo:  "ali_otn_1",
		TradeStatus: "TRADE_SUCCESS",
		RawBody:     "{}",
	})
	if err != nil {
		t.Fatalf("HandleAlipayNotify() err=%v", err)
	}
	if processed {
		t.Fatal("HandleAlipayNotify() processed=true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() err=%v", err)
	}
}

func TestHandleWxNotifyRollbackOnUpdateFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() err=%v", err)
	}
	defer db.Close()

	repo := &paymentRepo{
		data: &Data{db: db},
		log:  log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO payment_events").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT uid, biz_type, biz_order_no, amount").
		WillReturnRows(sqlmock.NewRows([]string{"uid", "biz_type", "biz_order_no", "amount"}).AddRow("u1001", 1, "biz_1", 100))
	mock.ExpectExec("UPDATE payment_orders").
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	_, err = repo.HandleWxNotify(context.Background(), &biz.WxNotifyEvent{
		EventID:    "evt_2",
		OutTradeNo: "otn_2",
		TradeState: "FAILED",
		RawBody:    "{}",
	})
	if err == nil {
		t.Fatal("HandleWxNotify() err=nil, want non-nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() err=%v", err)
	}
}
