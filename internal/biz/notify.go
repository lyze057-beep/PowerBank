package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	notifyv1 "github.com/go-kratos/kratos-layout/api/notify/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

const (
	// NotifyPushIdempotentTTL is idempotent window for message push.
	NotifyPushIdempotentTTL = 120 * time.Second
)

var (
	ErrNotifyInvalidArgument = errors.BadRequest(notifyv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid notify argument")
	ErrNotifyDuplicate       = errors.BadRequest(notifyv1.ErrorReason_NOTIFY_DUPLICATE_REQUEST.String(), "duplicate notify request")
	ErrNotifyPublishFailed   = errors.InternalServer(notifyv1.ErrorReason_NOTIFY_PUBLISH_FAILED.String(), "notify publish failed")
)

// MessageStatus indicates push status.
type MessageStatus int32

const (
	MessageStatusInit   MessageStatus = 1
	MessageStatusSent   MessageStatus = 2
	MessageStatusFailed MessageStatus = 3
)

// MessageRecord is notify persistence model.
type MessageRecord struct {
	ID           uint64
	UID          string
	Title        string
	Content      string
	BizType      string
	BizID        string
	ClientReqID  string
	Topic        string
	Status       MessageStatus
	FailedReason string
	CreatedAt    time.Time
}

// PushMessageInput is push request.
type PushMessageInput struct {
	UID         string
	Title       string
	Content     string
	BizType     string
	BizID       string
	ClientReqID string
	Topic       string
}

// NotifyRepo defines notification persistence and idempotency.
type NotifyRepo interface {
	AcquirePushIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error)
	FindByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*MessageRecord, error)
	Create(ctx context.Context, record *MessageRecord) error
	MarkSent(ctx context.Context, id uint64) error
	MarkFailed(ctx context.Context, id uint64, reason string) error
	ListByUID(ctx context.Context, uid string, page, pageSize int32) ([]*MessageRecord, error)
}

// Notifier publishes payload through MQTT.
type Notifier interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// NotifyUsecase handles message pushing and persistence.
type NotifyUsecase struct {
	repo     NotifyRepo
	notifier Notifier
	log      *log.Helper
}

// NewNotifyUsecase creates notify usecase.
func NewNotifyUsecase(repo NotifyRepo, notifier Notifier, logger log.Logger) *NotifyUsecase {
	return &NotifyUsecase{
		repo:     repo,
		notifier: notifier,
		log:      log.NewHelper(log.With(logger, "module", "biz/notify")),
	}
}

// PushMessage pushes message via MQTT and saves push record.
func (uc *NotifyUsecase) PushMessage(ctx context.Context, in PushMessageInput) (*MessageRecord, error) {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.Title) == "" ||
		strings.TrimSpace(in.Content) == "" || strings.TrimSpace(in.ClientReqID) == "" {
		return nil, ErrNotifyInvalidArgument
	}
	if strings.TrimSpace(in.Topic) == "" {
		in.Topic = in.UID + "/notification"
	}

	ok, err := uc.repo.AcquirePushIdempotent(ctx, in.UID, in.ClientReqID, NotifyPushIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.repo.FindByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, ErrNotifyDuplicate
	}

	record := &MessageRecord{
		UID:          in.UID,
		Title:        in.Title,
		Content:      in.Content,
		BizType:      in.BizType,
		BizID:        in.BizID,
		ClientReqID:  in.ClientReqID,
		Topic:        in.Topic,
		Status:       MessageStatusInit,
		FailedReason: "",
	}
	if err = uc.repo.Create(ctx, record); err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]any{
		"title":    in.Title,
		"content":  in.Content,
		"biz_type": in.BizType,
		"biz_id":   in.BizID,
		"uid":      in.UID,
		"ts":       time.Now().Unix(),
	})
	if err = uc.notifier.Publish(ctx, in.Topic, payload); err != nil {
		_ = uc.repo.MarkFailed(ctx, record.ID, err.Error())
		record.Status = MessageStatusFailed
		record.FailedReason = err.Error()
		uc.log.Warnf("notify publish failed but record saved: id=%d err=%v", record.ID, err)
		return nil, ErrNotifyPublishFailed
	} else {
		if err = uc.repo.MarkSent(ctx, record.ID); err != nil {
			return nil, err
		}
		record.Status = MessageStatusSent
	}
	return record, nil
}

// ListMessages lists persisted push records.
func (uc *NotifyUsecase) ListMessages(ctx context.Context, uid string, page, pageSize int32) ([]*MessageRecord, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrNotifyInvalidArgument
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	return uc.repo.ListByUID(ctx, uid, page, pageSize)
}
