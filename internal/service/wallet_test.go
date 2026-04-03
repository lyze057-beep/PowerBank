package service

import (
	"context"
	"io"
	"testing"
	"time"

	walletv1 "github.com/go-kratos/kratos-layout/api/wallet/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type walletSvcPayRepo struct{}

func (walletSvcPayRepo) AcquireCreateIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (walletSvcPayRepo) AcquireNotifyLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (walletSvcPayRepo) FindByUIDAndClientReqID(context.Context, string, string) (*biz.PaymentOrder, error) {
	return nil, nil
}
func (walletSvcPayRepo) CreateOrder(context.Context, *biz.PaymentOrder) error { return nil }
func (walletSvcPayRepo) UpdatePrepay(context.Context, string, string, string, map[string]string) error {
	return nil
}
func (walletSvcPayRepo) GetByUIDAndOutTradeNo(context.Context, string, string) (*biz.PaymentOrder, error) {
	return &biz.PaymentOrder{OutTradeNo: "otn_1", Status: biz.PayStatusPaying, Channel: biz.PayChannelWx}, nil
}
func (walletSvcPayRepo) GetByOutTradeNo(context.Context, string) (*biz.PaymentOrder, error) {
	return &biz.PaymentOrder{OutTradeNo: "otn_1", Status: biz.PayStatusPaying, Channel: biz.PayChannelWx}, nil
}
func (walletSvcPayRepo) HandleWxNotify(context.Context, *biz.WxNotifyEvent) (bool, error) {
	return true, nil
}
func (walletSvcPayRepo) HandleAlipayNotify(context.Context, *biz.AlipayNotifyEvent) (bool, error) {
	return true, nil
}

type walletSvcRepo struct{}

func (walletSvcRepo) GetBalance(context.Context, string) (int64, error) { return 0, nil }
func (walletSvcRepo) GetCachedBalance(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}
func (walletSvcRepo) SetCachedBalance(context.Context, string, int64, time.Duration) error {
	return nil
}

type walletSvcWx struct{}

func (walletSvcWx) CreateOrder(context.Context, biz.WxCreateOrderInput) (*biz.WxCreateOrderOutput, error) {
	return &biz.WxCreateOrderOutput{PrepayID: "wx_1", CodeURL: "weixin://mock"}, nil
}
func (walletSvcWx) VerifyNotify(context.Context, biz.WxNotifyInput) error           { return nil }
func (walletSvcWx) ParseNotify(context.Context, string) (*biz.WxNotifyEvent, error) { return nil, nil }

type walletSvcAli struct{}

func (walletSvcAli) CreateOrder(context.Context, biz.AlipayCreateOrderInput) (*biz.AlipayCreateOrderOutput, error) {
	return &biz.AlipayCreateOrderOutput{TradeNo: "ali_1", PayURL: "https://ali.mock/pay"}, nil
}
func (walletSvcAli) VerifyNotify(context.Context, biz.AlipayNotifyInput) error { return nil }
func (walletSvcAli) ParseNotify(context.Context, string) (*biz.AlipayNotifyEvent, error) {
	return &biz.AlipayNotifyEvent{}, nil
}
func (walletSvcAli) Heartbeat(context.Context) error { return nil }

func TestCreateRechargeOrderRequireJWT(t *testing.T) {
	uc := biz.NewWalletUsecase(walletSvcPayRepo{}, walletSvcRepo{}, walletSvcWx{}, walletSvcAli{}, log.NewStdLogger(io.Discard))
	svc := NewWalletService(uc)
	_, err := svc.CreateRechargeOrder(context.Background(), &walletv1.CreateRechargeOrderRequest{
		Channel:     walletv1.RechargeChannel_RECHARGE_CHANNEL_WECHAT,
		PayMode:     walletv1.RechargePayMode_RECHARGE_PAY_MODE_NATIVE,
		Amount:      100,
		ClientReqId: "req_1",
	})
	if err == nil {
		t.Fatal("CreateRechargeOrder() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("CreateRechargeOrder() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}

func TestCreateAlipayRechargeOrderRequireJWT(t *testing.T) {
	uc := biz.NewWalletUsecase(walletSvcPayRepo{}, walletSvcRepo{}, walletSvcWx{}, walletSvcAli{}, log.NewStdLogger(io.Discard))
	svc := NewWalletService(uc)
	_, err := svc.CreateAlipayRechargeOrder(context.Background(), &walletv1.CreateAlipayRechargeOrderRequest{
		Amount:      100,
		ClientReqId: "req_ali_1",
	})
	if err == nil {
		t.Fatal("CreateAlipayRechargeOrder() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("CreateAlipayRechargeOrder() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}
