package service

import (
	"context"
	"io"
	"testing"
	"time"

	notifyv1 "github.com/go-kratos/kratos-layout/api/notify/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type notifySvcRepo struct{}

func (notifySvcRepo) AcquirePushIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (notifySvcRepo) FindByUIDAndClientReqID(context.Context, string, string) (*biz.MessageRecord, error) {
	return nil, nil
}
func (notifySvcRepo) Create(_ context.Context, record *biz.MessageRecord) error {
	record.ID = 1
	return nil
}
func (notifySvcRepo) MarkSent(context.Context, uint64) error           { return nil }
func (notifySvcRepo) MarkFailed(context.Context, uint64, string) error { return nil }
func (notifySvcRepo) ListByUID(context.Context, string, int32, int32) ([]*biz.MessageRecord, error) {
	return nil, nil
}

type notifySvcNotifier struct{}

func (notifySvcNotifier) Publish(context.Context, string, []byte) error { return nil }

func TestPushMessageRequireJWT(t *testing.T) {
	uc := biz.NewNotifyUsecase(notifySvcRepo{}, notifySvcNotifier{}, log.NewStdLogger(io.Discard))
	svc := NewNotifyService(uc)
	_, err := svc.PushMessage(context.Background(), &notifyv1.PushMessageRequest{
		Title:       "t",
		Content:     "c",
		ClientReqId: "req_1",
	})
	if err == nil {
		t.Fatal("PushMessage() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("PushMessage() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}

func TestListMessagesRequireJWT(t *testing.T) {
	uc := biz.NewNotifyUsecase(notifySvcRepo{}, notifySvcNotifier{}, log.NewStdLogger(io.Discard))
	svc := NewNotifyService(uc)
	_, err := svc.ListMessages(context.Background(), &notifyv1.ListMessagesRequest{
		Page:     1,
		PageSize: 20,
	})
	if err == nil {
		t.Fatal("ListMessages() err=nil, want unauthorized")
	}
	se := kerrors.FromError(err)
	if se.Reason != "UNAUTHORIZED" {
		t.Fatalf("ListMessages() reason=%s, want UNAUTHORIZED", se.Reason)
	}
}
