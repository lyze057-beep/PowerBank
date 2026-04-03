package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	payv1 "github.com/go-kratos/kratos-layout/api/payment/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type svcRepo struct{}

func (svcRepo) AcquireCreateIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (svcRepo) AcquireNotifyLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (svcRepo) FindByUIDAndClientReqID(context.Context, string, string) (*biz.PaymentOrder, error) {
	return nil, nil
}
func (svcRepo) CreateOrder(context.Context, *biz.PaymentOrder) error { return nil }
func (svcRepo) UpdatePrepay(context.Context, string, string, string, map[string]string) error {
	return nil
}
func (svcRepo) GetByUIDAndOutTradeNo(context.Context, string, string) (*biz.PaymentOrder, error) {
	return &biz.PaymentOrder{OutTradeNo: "otn_1"}, nil
}
func (svcRepo) GetByOutTradeNo(context.Context, string) (*biz.PaymentOrder, error) {
	return &biz.PaymentOrder{OutTradeNo: "otn_1"}, nil
}
func (svcRepo) HandleWxNotify(context.Context, *biz.WxNotifyEvent) (bool, error) { return true, nil }
func (svcRepo) HandleAlipayNotify(context.Context, *biz.AlipayNotifyEvent) (bool, error) {
	return true, nil
}

type svcWx struct {
	verifyErr error
}

func (w svcWx) CreateOrder(context.Context, biz.WxCreateOrderInput) (*biz.WxCreateOrderOutput, error) {
	return &biz.WxCreateOrderOutput{}, nil
}
func (w svcWx) VerifyNotify(context.Context, biz.WxNotifyInput) error { return w.verifyErr }
func (w svcWx) ParseNotify(context.Context, string) (*biz.WxNotifyEvent, error) {
	return &biz.WxNotifyEvent{EventID: "evt_1", OutTradeNo: "otn_1", TradeState: "SUCCESS"}, nil
}

type svcAli struct {
	verifyErr    error
	heartbeatErr error
}

func (a svcAli) CreateOrder(context.Context, biz.AlipayCreateOrderInput) (*biz.AlipayCreateOrderOutput, error) {
	return &biz.AlipayCreateOrderOutput{}, nil
}
func (a svcAli) VerifyNotify(context.Context, biz.AlipayNotifyInput) error { return a.verifyErr }
func (a svcAli) ParseNotify(context.Context, string) (*biz.AlipayNotifyEvent, error) {
	return &biz.AlipayNotifyEvent{EventID: "ali_evt_1", OutTradeNo: "ali_otn_1", TradeStatus: "TRADE_SUCCESS"}, nil
}
func (a svcAli) Heartbeat(context.Context) error { return a.heartbeatErr }

func TestCreateWxPayOrderRequireJWT(t *testing.T) {
	uc := biz.NewPaymentUsecase(svcRepo{}, svcWx{}, svcAli{}, nil, log.NewStdLogger(io.Discard))
	svc := NewPaymentService(uc)
	_, err := svc.CreateWxPayOrder(context.Background(), &payv1.CreateWxPayOrderRequest{
		PayMode:     payv1.PayMode_PAY_MODE_NATIVE,
		BizType:     payv1.BizType_BIZ_TYPE_RECHARGE,
		BizOrderNo:  "biz_1",
		Amount:      100,
		ClientReqId: "req_1",
	})
	if err == nil {
		t.Fatal("CreateWxPayOrder() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("CreateWxPayOrder() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}

func TestWxPayNotifyNoJWTButVerifySign(t *testing.T) {
	uc := biz.NewPaymentUsecase(svcRepo{}, svcWx{verifyErr: errors.New("bad signature")}, svcAli{}, nil, log.NewStdLogger(io.Discard))
	svc := NewPaymentService(uc)
	_, err := svc.WxPayNotify(context.Background(), &payv1.WxPayNotifyRequest{
		Body:      `{"event_id":"evt_1","out_trade_no":"otn_1","trade_state":"SUCCESS"}`,
		Timestamp: "1",
		Nonce:     "1",
		Signature: "bad-sign",
	})
	if err == nil {
		t.Fatal("WxPayNotify() err=nil, want sign verify error")
	}
	se := kerrors.FromError(err)
	if se.Reason != payv1.ErrorReason_PAYMENT_SIGN_VERIFY_FAILED.String() {
		t.Fatalf("WxPayNotify() reason=%s, want %s", se.Reason, payv1.ErrorReason_PAYMENT_SIGN_VERIFY_FAILED.String())
	}
}

func TestAlipayNotifyNoJWTButVerifySign(t *testing.T) {
	uc := biz.NewPaymentUsecase(svcRepo{}, svcWx{}, svcAli{verifyErr: errors.New("bad signature")}, nil, log.NewStdLogger(io.Discard))
	svc := NewPaymentService(uc)
	_, err := svc.AlipayNotify(context.Background(), &payv1.AlipayNotifyRequest{
		Body:      `{"notify_id":"ali_evt_1","out_trade_no":"ali_otn_1","trade_status":"TRADE_SUCCESS"}`,
		Signature: "bad-sign",
		NotifyId:  "ali_evt_1",
	})
	if err == nil {
		t.Fatal("AlipayNotify() err=nil, want sign verify error")
	}
	se := kerrors.FromError(err)
	if se.Reason != payv1.ErrorReason_PAYMENT_SIGN_VERIFY_FAILED.String() {
		t.Fatalf("AlipayNotify() reason=%s, want %s", se.Reason, payv1.ErrorReason_PAYMENT_SIGN_VERIFY_FAILED.String())
	}
}

func TestAlipayHeartbeat(t *testing.T) {
	uc := biz.NewPaymentUsecase(svcRepo{}, svcWx{}, svcAli{}, nil, log.NewStdLogger(io.Discard))
	svc := NewPaymentService(uc)
	reply, err := svc.AlipayHeartbeat(context.Background(), &payv1.AlipayHeartbeatRequest{})
	if err != nil {
		t.Fatalf("AlipayHeartbeat() err=%v", err)
	}
	if !reply.GetOk() {
		t.Fatal("AlipayHeartbeat() ok=false, want true")
	}
}
