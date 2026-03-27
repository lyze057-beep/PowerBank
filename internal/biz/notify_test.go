package biz

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	notifyv1 "github.com/go-kratos/kratos-layout/api/notify/v1"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type mockNotifyRepo struct {
	lockOK           bool
	existing         *MessageRecord
	created          *MessageRecord
	markSentID       uint64
	markFailedID     uint64
	markFailedReason string
}

func (m *mockNotifyRepo) AcquirePushIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return m.lockOK, nil
}

func (m *mockNotifyRepo) FindByUIDAndClientReqID(context.Context, string, string) (*MessageRecord, error) {
	return m.existing, nil
}

func (m *mockNotifyRepo) Create(_ context.Context, record *MessageRecord) error {
	record.ID = 1001
	m.created = record
	return nil
}

func (m *mockNotifyRepo) MarkSent(_ context.Context, id uint64) error {
	m.markSentID = id
	return nil
}

func (m *mockNotifyRepo) MarkFailed(_ context.Context, id uint64, reason string) error {
	m.markFailedID = id
	m.markFailedReason = reason
	return nil
}

func (m *mockNotifyRepo) ListByUID(context.Context, string, int32, int32) ([]*MessageRecord, error) {
	return nil, nil
}

type mockNotifier struct {
	err    error
	called bool
}

func (m *mockNotifier) Publish(context.Context, string, []byte) error {
	m.called = true
	return m.err
}

func TestPushMessageDuplicateReturnsExisting(t *testing.T) {
	repo := &mockNotifyRepo{
		lockOK: false,
		existing: &MessageRecord{
			ID:     99,
			UID:    "u1001",
			Title:  "dup",
			Status: MessageStatusSent,
		},
	}
	notifier := &mockNotifier{}
	uc := NewNotifyUsecase(repo, notifier, log.NewStdLogger(io.Discard))

	record, err := uc.PushMessage(context.Background(), PushMessageInput{
		UID:         "u1001",
		Title:       "hello",
		Content:     "world",
		ClientReqID: "req_dup",
	})
	if err != nil {
		t.Fatalf("PushMessage() err=%v", err)
	}
	if record.ID != 99 {
		t.Fatalf("PushMessage() id=%d, want 99", record.ID)
	}
	if repo.created != nil {
		t.Fatal("PushMessage() should not create new record on duplicate")
	}
	if notifier.called {
		t.Fatal("PushMessage() should not publish on duplicate")
	}
}

func TestPushMessagePublishFailedMarksFailed(t *testing.T) {
	repo := &mockNotifyRepo{lockOK: true}
	notifier := &mockNotifier{err: errors.New("mqtt down")}
	uc := NewNotifyUsecase(repo, notifier, log.NewStdLogger(io.Discard))

	_, err := uc.PushMessage(context.Background(), PushMessageInput{
		UID:         "u1001",
		Title:       "hello",
		Content:     "world",
		ClientReqID: "req_1",
	})
	if err == nil {
		t.Fatal("PushMessage() err=nil, want publish failed error")
	}
	se := kerrors.FromError(err)
	if se.Reason != notifyv1.ErrorReason_NOTIFY_PUBLISH_FAILED.String() {
		t.Fatalf("PushMessage() reason=%s, want %s", se.Reason, notifyv1.ErrorReason_NOTIFY_PUBLISH_FAILED.String())
	}
	if repo.markFailedID != 1001 {
		t.Fatalf("PushMessage() markFailed id=%d, want 1001", repo.markFailedID)
	}
}

func TestPushMessageSuccess(t *testing.T) {
	repo := &mockNotifyRepo{lockOK: true}
	notifier := &mockNotifier{}
	uc := NewNotifyUsecase(repo, notifier, log.NewStdLogger(io.Discard))

	record, err := uc.PushMessage(context.Background(), PushMessageInput{
		UID:         "u1001",
		Title:       "hello",
		Content:     "world",
		ClientReqID: "req_ok",
	})
	if err != nil {
		t.Fatalf("PushMessage() err=%v", err)
	}
	if !notifier.called {
		t.Fatal("PushMessage() notifier not called")
	}
	if repo.markSentID != 1001 {
		t.Fatalf("PushMessage() markSent id=%d, want 1001", repo.markSentID)
	}
	if record.Status != MessageStatusSent {
		t.Fatalf("PushMessage() status=%d, want %d", record.Status, MessageStatusSent)
	}
	if record.Topic != "u1001/notification" {
		t.Fatalf("PushMessage() topic=%s, want u1001/notification", record.Topic)
	}
}
