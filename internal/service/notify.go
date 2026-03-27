package service

import (
	"context"

	notifyv1 "github.com/go-kratos/kratos-layout/api/notify/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// NotifyService handles push notifications.
type NotifyService struct {
	notifyv1.UnimplementedNotifyServiceServer

	uc *biz.NotifyUsecase
}

// NewNotifyService creates notify service.
func NewNotifyService(uc *biz.NotifyUsecase) *NotifyService {
	return &NotifyService{uc: uc}
}

// PushMessage pushes one notification message and saves push record.
func (s *NotifyService) PushMessage(ctx context.Context, req *notifyv1.PushMessageRequest) (*notifyv1.PushMessageReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.uc.PushMessage(ctx, biz.PushMessageInput{
		UID:         claims.UID,
		Title:       req.GetTitle(),
		Content:     req.GetContent(),
		BizType:     req.GetBizType(),
		BizID:       req.GetBizId(),
		ClientReqID: req.GetClientReqId(),
		Topic:       req.GetTopic(),
	})
	if err != nil {
		return nil, err
	}
	return &notifyv1.PushMessageReply{
		Id:     record.ID,
		Status: notifyv1.MessageStatus(record.Status),
		Topic:  record.Topic,
	}, nil
}

// ListMessages lists push records for current user.
func (s *NotifyService) ListMessages(ctx context.Context, req *notifyv1.ListMessagesRequest) (*notifyv1.ListMessagesReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.uc.ListMessages(ctx, claims.UID, req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	replyItems := make([]*notifyv1.MessageRecord, 0, len(items))
	for _, item := range items {
		replyItems = append(replyItems, &notifyv1.MessageRecord{
			Id:           item.ID,
			Title:        item.Title,
			Content:      item.Content,
			BizType:      item.BizType,
			BizId:        item.BizID,
			Topic:        item.Topic,
			Status:       notifyv1.MessageStatus(item.Status),
			FailedReason: item.FailedReason,
			CreatedAt:    item.CreatedAt.Unix(),
		})
	}
	return &notifyv1.ListMessagesReply{Items: replyItems}, nil
}
