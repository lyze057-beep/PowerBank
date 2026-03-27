package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type mockWalletRepo struct {
	cachedBalance int64
	cacheFound    bool
	dbBalance     int64
}

func (m *mockWalletRepo) GetBalance(context.Context, string) (int64, error) {
	return m.dbBalance, nil
}
func (m *mockWalletRepo) GetCachedBalance(context.Context, string) (int64, bool, error) {
	return m.cachedBalance, m.cacheFound, nil
}
func (m *mockWalletRepo) SetCachedBalance(context.Context, string, int64, time.Duration) error {
	return nil
}

type mockAliGateway struct{}

func (mockAliGateway) CreateOrder(context.Context, AlipayCreateOrderInput) (*AlipayCreateOrderOutput, error) {
	return &AlipayCreateOrderOutput{
		TradeNo: "ali_trade_1",
		PayURL:  "https://alipay.mock/pay",
		PayParams: map[string]string{
			"trade_no": "ali_trade_1",
		},
	}, nil
}
func (mockAliGateway) VerifyNotify(context.Context, AlipayNotifyInput) error { return nil }
func (mockAliGateway) ParseNotify(context.Context, string) (*AlipayNotifyEvent, error) {
	return &AlipayNotifyEvent{}, nil
}
func (mockAliGateway) Heartbeat(context.Context) error { return nil }

func TestGetBalanceHitCache(t *testing.T) {
	uc := NewWalletUsecase(
		&mockPaymentRepo{},
		&mockWalletRepo{cachedBalance: 900, cacheFound: true},
		&mockWxGateway{},
		&mockAliGateway{},
		log.NewStdLogger(io.Discard),
	)
	balance, err := uc.GetBalance(context.Background(), "u1001")
	if err != nil {
		t.Fatalf("GetBalance() err=%v", err)
	}
	if balance != 900 {
		t.Fatalf("GetBalance()=%d, want 900", balance)
	}
}

func TestCreateRechargeOrderSupportAlipay(t *testing.T) {
	repo := &mockPaymentRepo{createLockOK: true}
	uc := NewWalletUsecase(
		repo,
		&mockWalletRepo{},
		&mockWxGateway{},
		&mockAliGateway{},
		log.NewStdLogger(io.Discard),
	)
	order, err := uc.CreateRechargeOrder(context.Background(), CreateRechargeOrderInput{
		UID:         "u1001",
		Channel:     PayChannelAl,
		PayMode:     PayModeNative,
		Amount:      1000,
		ClientReqID: "req-ali-1",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() err=%v", err)
	}
	if order.Channel != PayChannelAl {
		t.Fatalf("CreateRechargeOrder() channel=%s, want %s", order.Channel, PayChannelAl)
	}
}
