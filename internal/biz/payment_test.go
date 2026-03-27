package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type mockPaymentRepo struct {
	createLockOK bool
	notifyLockOK bool
	existing     *PaymentOrder
	created      *PaymentOrder
	processed    bool
}

func (m *mockPaymentRepo) AcquireCreateIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return m.createLockOK, nil
}
func (m *mockPaymentRepo) AcquireNotifyLock(context.Context, string, time.Duration) (bool, error) {
	return m.notifyLockOK, nil
}
func (m *mockPaymentRepo) FindByUIDAndClientReqID(context.Context, string, string) (*PaymentOrder, error) {
	return m.existing, nil
}
func (m *mockPaymentRepo) CreateOrder(_ context.Context, order *PaymentOrder) error {
	m.created = order
	return nil
}
func (m *mockPaymentRepo) UpdatePrepay(_ context.Context, _ string, _ string, _ string, _ map[string]string) error {
	return nil
}
func (m *mockPaymentRepo) GetByUIDAndOutTradeNo(context.Context, string, string) (*PaymentOrder, error) {
	if m.created != nil {
		m.created.Status = PayStatusPaying
		return m.created, nil
	}
	return m.existing, nil
}
func (m *mockPaymentRepo) HandleWxNotify(context.Context, *WxNotifyEvent) (bool, error) {
	return m.processed, nil
}
func (m *mockPaymentRepo) HandleAlipayNotify(context.Context, *AlipayNotifyEvent) (bool, error) {
	return m.processed, nil
}

type mockWxGateway struct {
	verifyErr error
}

func (m *mockWxGateway) CreateOrder(context.Context, WxCreateOrderInput) (*WxCreateOrderOutput, error) {
	return &WxCreateOrderOutput{
		PrepayID: "mock_prepay",
		CodeURL:  "weixin://mock",
	}, nil
}
func (m *mockWxGateway) VerifyNotify(context.Context, WxNotifyInput) error {
	return m.verifyErr
}
func (m *mockWxGateway) ParseNotify(context.Context, string) (*WxNotifyEvent, error) {
	return &WxNotifyEvent{
		EventID:    "evt_1",
		OutTradeNo: "otn_1",
		TradeState: "SUCCESS",
		RawBody:    "{}",
	}, nil
}

type mockAliGatewayForPayment struct {
	verifyErr error
}

func (m *mockAliGatewayForPayment) CreateOrder(context.Context, AlipayCreateOrderInput) (*AlipayCreateOrderOutput, error) {
	return &AlipayCreateOrderOutput{}, nil
}
func (m *mockAliGatewayForPayment) VerifyNotify(context.Context, AlipayNotifyInput) error {
	return m.verifyErr
}
func (m *mockAliGatewayForPayment) ParseNotify(context.Context, string) (*AlipayNotifyEvent, error) {
	return &AlipayNotifyEvent{
		EventID:    "ali_evt_1",
		OutTradeNo: "ali_otn_1",
	}, nil
}
func (m *mockAliGatewayForPayment) Heartbeat(context.Context) error {
	return nil
}

func TestCreateWxPayOrderValidateMode(t *testing.T) {
	uc := NewPaymentUsecase(&mockPaymentRepo{createLockOK: true}, &mockWxGateway{}, &mockAliGatewayForPayment{}, log.NewStdLogger(io.Discard))
	_, err := uc.CreateWxPayOrder(context.Background(), CreateWxPayOrderInput{
		UID:         "u1001",
		PayMode:     PayMode(99),
		BizType:     BizTypeRecharge,
		BizOrderNo:  "bz_1",
		Amount:      100,
		ClientReqID: "req_1",
	})
	if err == nil {
		t.Fatal("CreateWxPayOrder() expected error, got nil")
	}
}

func TestCreateWxPayOrderDuplicateReturnsExisting(t *testing.T) {
	repo := &mockPaymentRepo{
		createLockOK: false,
		existing: &PaymentOrder{
			OutTradeNo: "otn_exist",
			Status:     PayStatusPaying,
		},
	}
	uc := NewPaymentUsecase(repo, &mockWxGateway{}, &mockAliGatewayForPayment{}, log.NewStdLogger(io.Discard))
	order, err := uc.CreateWxPayOrder(context.Background(), CreateWxPayOrderInput{
		UID:         "u1001",
		PayMode:     PayModeNative,
		BizType:     BizTypeRecharge,
		BizOrderNo:  "bz_1",
		Amount:      100,
		ClientReqID: "req_dup",
	})
	if err != nil {
		t.Fatalf("CreateWxPayOrder() unexpected error: %v", err)
	}
	if order.OutTradeNo != "otn_exist" {
		t.Fatalf("CreateWxPayOrder() out_trade_no=%s, want otn_exist", order.OutTradeNo)
	}
}

func TestHandleWxNotifyDuplicateOnlyOnce(t *testing.T) {
	repo := &mockPaymentRepo{
		notifyLockOK: true,
		processed:    false,
	}
	uc := NewPaymentUsecase(repo, &mockWxGateway{}, &mockAliGatewayForPayment{}, log.NewStdLogger(io.Discard))
	processed, err := uc.HandleWxNotify(context.Background(), WxNotifyInput{
		Body:      `{"event_id":"evt_1","out_trade_no":"otn_1","trade_state":"SUCCESS"}`,
		Timestamp: "1",
		Nonce:     "1",
		Signature: "sig",
	})
	if err != nil {
		t.Fatalf("HandleWxNotify() unexpected error: %v", err)
	}
	if processed {
		t.Fatal("HandleWxNotify() processed=true, want false for duplicated notify")
	}
}

func TestHandleAlipayNotifyDuplicateOnlyOnce(t *testing.T) {
	repo := &mockPaymentRepo{
		notifyLockOK: true,
		processed:    false,
	}
	uc := NewPaymentUsecase(repo, &mockWxGateway{}, &mockAliGatewayForPayment{}, log.NewStdLogger(io.Discard))
	processed, err := uc.HandleAlipayNotify(context.Background(), AlipayNotifyInput{
		Body:      `{"notify_id":"ali_evt_1","out_trade_no":"ali_otn_1","trade_status":"TRADE_SUCCESS"}`,
		Signature: "sig",
		NotifyID:  "ali_evt_1",
	})
	if err != nil {
		t.Fatalf("HandleAlipayNotify() unexpected error: %v", err)
	}
	if processed {
		t.Fatal("HandleAlipayNotify() processed=true, want false for duplicated notify")
	}
}
