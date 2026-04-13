package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type mockChargerRepo struct {
	active  bool
	created *RentOrder
}

func (m *mockChargerRepo) AcquireBorrowIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (m *mockChargerRepo) FindRentOrderByUIDAndClientReqID(context.Context, string, string) (*RentOrder, error) {
	return m.created, nil
}
func (m *mockChargerRepo) HasActiveOrder(context.Context, string) (bool, error) { return m.active, nil }
func (m *mockChargerRepo) CreateBorrowOrder(_ context.Context, uid, stationID, clientReqID string, depositAmount int64) (*RentOrder, error) {
	m.created = &RentOrder{UID: uid, RentOrderNo: "ro_1", StationID: stationID, ClientReqID: clientReqID, DepositAmount: depositAmount, PowerbankID: "pb_1", BorrowSlotID: "slot_1", Status: RentOrderStatusPendingBorrow}
	return m.created, nil
}
func (m *mockChargerRepo) HandleBorrowResult(context.Context, string, bool, string) (*RentOrder, error) {
	return m.created, nil
}
func (m *mockChargerRepo) HandleReturnResult(context.Context, string, string, string, bool, string) (*RentOrder, error) {
	return m.created, nil
}

func TestScanBorrowRequiresDeposit(t *testing.T) {
	depositUC := NewDepositUsecase(&mockDepositRepo{profile: &DepositProfile{UID: "u1001", Status: DepositStatusRequired, DepositAmount: DefaultDepositAmount}}, &mockPaymentRepo{}, &mockWxGateway{}, &mockAliGatewayForPayment{}, mockCreditGateway{}, log.NewStdLogger(io.Discard))
	uc := NewChargerUsecase(&mockChargerRepo{}, depositUC, nil, log.NewStdLogger(io.Discard))
	_, err := uc.ScanBorrow(context.Background(), ScanBorrowInput{UID: "u1001", StationID: "st_1", ClientReqID: "req_1"})
	if err == nil {
		t.Fatal("ScanBorrow() err=nil, want deposit required")
	}
}

func TestScanBorrowAllowedAfterExemption(t *testing.T) {
	depositUC := NewDepositUsecase(&mockDepositRepo{profile: &DepositProfile{UID: "u1001", Status: DepositStatusExempt, DepositAmount: DefaultDepositAmount, Exempt: true, ExemptExpireAt: time.Now().Add(time.Hour)}}, &mockPaymentRepo{}, &mockWxGateway{}, &mockAliGatewayForPayment{}, mockCreditGateway{}, log.NewStdLogger(io.Discard))
	chargerRepo := &mockChargerRepo{}
	uc := NewChargerUsecase(chargerRepo, depositUC, nil, log.NewStdLogger(io.Discard))
	order, err := uc.ScanBorrow(context.Background(), ScanBorrowInput{UID: "u1001", StationID: "st_1", ClientReqID: "req_1"})
	if err != nil {
		t.Fatalf("ScanBorrow() err=%v", err)
	}
	if order.RentOrderNo == "" {
		t.Fatal("ScanBorrow() empty order no")
	}
}
