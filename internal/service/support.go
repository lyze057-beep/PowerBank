package service

import (
	"context"

	supportv1 "github.com/go-kratos/kratos-layout/api/support/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// SupportService handles user customer-support dialog.
type SupportService struct {
	supportv1.UnimplementedSupportServiceServer

	uc *biz.SupportUsecase
}

// NewSupportService creates support service.
func NewSupportService(uc *biz.SupportUsecase) *SupportService {
	return &SupportService{uc: uc}
}

// SendMessage sends user message and returns assistant reply.
func (s *SupportService) SendMessage(ctx context.Context, req *supportv1.SendMessageRequest) (*supportv1.SendMessageReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	turn, err := s.uc.SendMessage(ctx, biz.SendMessageInput{
		UID:         claims.UID,
		SessionID:   req.GetSessionId(),
		Content:     req.GetContent(),
		ClientReqID: req.GetClientReqId(),
	})
	if err != nil {
		return nil, err
	}
	return &supportv1.SendMessageReply{
		SessionId:    turn.SessionID,
		TurnId:       turn.ID,
		Intent:       toProtoIntent(turn.Intent),
		Reply:        turn.AssistantReply,
		UsedFallback: turn.UsedFallback,
	}, nil
}

// ListSessionTurns lists turn records for one conversation session.
func (s *SupportService) ListSessionTurns(ctx context.Context, req *supportv1.ListSessionTurnsRequest) (*supportv1.ListSessionTurnsReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	turns, err := s.uc.ListSessionTurns(ctx, claims.UID, req.GetSessionId(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	items := make([]*supportv1.SessionTurn, 0, len(turns))
	for _, turn := range turns {
		items = append(items, &supportv1.SessionTurn{
			Id:               turn.ID,
			SessionId:        turn.SessionID,
			UserMessage:      turn.UserMessage,
			AssistantMessage: turn.AssistantReply,
			Intent:           toProtoIntent(turn.Intent),
			UsedFallback:     turn.UsedFallback,
			CreatedAt:        turn.CreatedAt.Unix(),
		})
	}
	return &supportv1.ListSessionTurnsReply{Items: items}, nil
}

func toProtoIntent(intent biz.IntentType) supportv1.IntentType {
	switch intent {
	case biz.IntentGreeting:
		return supportv1.IntentType_INTENT_TYPE_GREETING
	case biz.IntentBorrowHelp:
		return supportv1.IntentType_INTENT_TYPE_BORROW_HELP
	case biz.IntentReturnHelp:
		return supportv1.IntentType_INTENT_TYPE_RETURN_HELP
	case biz.IntentOrderHelp:
		return supportv1.IntentType_INTENT_TYPE_ORDER_HELP
	case biz.IntentPayment:
		return supportv1.IntentType_INTENT_TYPE_PAYMENT_HELP
	case biz.IntentDeposit:
		return supportv1.IntentType_INTENT_TYPE_DEPOSIT_HELP
	case biz.IntentComplaint:
		return supportv1.IntentType_INTENT_TYPE_COMPLAINT_HELP
	default:
		return supportv1.IntentType_INTENT_TYPE_UNKNOWN
	}
}
