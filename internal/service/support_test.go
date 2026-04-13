package service

import (
	"context"
	"io"
	"testing"
	"time"

	supportv1 "github.com/go-kratos/kratos-layout/api/support/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type supportSvcRepo struct{}

func (supportSvcRepo) AcquireSendIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (supportSvcRepo) FindTurnByUIDAndClientReqID(context.Context, string, string) (*biz.SessionTurn, error) {
	return nil, nil
}
func (supportSvcRepo) EnsureSession(context.Context, string, string) (string, error) {
	return "cs_1", nil
}
func (supportSvcRepo) SessionExists(context.Context, string, string) (bool, error) { return true, nil }
func (supportSvcRepo) ListRecentTurns(context.Context, string, string, int32) ([]*biz.SessionTurn, error) {
	return nil, nil
}
func (supportSvcRepo) CreateTurn(_ context.Context, turn *biz.SessionTurn) error {
	turn.ID = 1
	turn.CreatedAt = time.Now()
	return nil
}
func (supportSvcRepo) ListSessionTurns(context.Context, string, string, int32, int32) ([]*biz.SessionTurn, error) {
	return []*biz.SessionTurn{{ID: 1, SessionID: "cs_1"}}, nil
}

type supportSvcModel struct{}

func (supportSvcModel) Chat(context.Context, biz.ModelChatInput) (*biz.ModelChatOutput, error) {
	return &biz.ModelChatOutput{Reply: "ok"}, nil
}

// Dummy repos for Usecase dependencies
type supportSvcUserRepo struct{}

func (supportSvcUserRepo) FindByUID(context.Context, string) (*biz.User, error) { return nil, nil }
func (supportSvcUserRepo) UpdateProfile(context.Context, string, biz.UpdateProfileInput) (*biz.User, error) {
	return nil, nil
}
func (supportSvcUserRepo) CheckLoginState(context.Context, string, string) (bool, error) {
	return true, nil
}

type supportSvcWalletRepo struct{}

func (supportSvcWalletRepo) GetBalance(context.Context, string) (int64, error) { return 0, nil }
func (supportSvcWalletRepo) GetCachedBalance(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}
func (supportSvcWalletRepo) SetCachedBalance(context.Context, string, int64, time.Duration) error {
	return nil
}

type supportSvcPaymentRepo struct{}

func (supportSvcPaymentRepo) AcquireCreateIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (supportSvcPaymentRepo) AcquireNotifyLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (supportSvcPaymentRepo) FindByUIDAndClientReqID(context.Context, string, string) (*biz.PaymentOrder, error) {
	return nil, nil
}
func (supportSvcPaymentRepo) CreateOrder(context.Context, *biz.PaymentOrder) error { return nil }
func (supportSvcPaymentRepo) UpdatePrepay(context.Context, string, string, string, map[string]string) error {
	return nil
}
func (supportSvcPaymentRepo) GetByUIDAndOutTradeNo(context.Context, string, string) (*biz.PaymentOrder, error) {
	return nil, nil
}
func (supportSvcPaymentRepo) GetByOutTradeNo(context.Context, string) (*biz.PaymentOrder, error) {
	return nil, nil
}
func (supportSvcPaymentRepo) HandleWxNotify(context.Context, *biz.WxNotifyEvent) (bool, error) {
	return true, nil
}
func (supportSvcPaymentRepo) HandleAlipayNotify(context.Context, *biz.AlipayNotifyEvent) (bool, error) {
	return true, nil
}

func TestSupportSendMessageRequireJWT(t *testing.T) {
	userUC := biz.NewUserUsecase(supportSvcUserRepo{}, log.NewStdLogger(io.Discard))
	walletUC := biz.NewWalletUsecase(supportSvcPaymentRepo{}, supportSvcWalletRepo{}, nil, nil, log.NewStdLogger(io.Discard))
	uc := biz.NewSupportUsecase(supportSvcRepo{}, supportSvcModel{}, userUC, walletUC, nil, nil, log.NewStdLogger(io.Discard))
	svc := NewSupportService(uc)
	_, err := svc.SendMessage(context.Background(), &supportv1.SendMessageRequest{
		Content:     "你好",
		ClientReqId: "req_1",
	})
	if err == nil {
		t.Fatal("SendMessage() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("SendMessage() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}

func TestSupportListSessionTurnsRequireJWT(t *testing.T) {
	userUC := biz.NewUserUsecase(supportSvcUserRepo{}, log.NewStdLogger(io.Discard))
	walletUC := biz.NewWalletUsecase(supportSvcPaymentRepo{}, supportSvcWalletRepo{}, nil, nil, log.NewStdLogger(io.Discard))
	uc := biz.NewSupportUsecase(supportSvcRepo{}, supportSvcModel{}, userUC, walletUC, nil, nil, log.NewStdLogger(io.Discard))
	svc := NewSupportService(uc)
	_, err := svc.ListSessionTurns(context.Background(), &supportv1.ListSessionTurnsRequest{
		SessionId: "cs_1",
	})
	if err == nil {
		t.Fatal("ListSessionTurns() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("ListSessionTurns() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}
