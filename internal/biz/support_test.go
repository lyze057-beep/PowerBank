package biz

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type mockSupportRepo struct {
	lockOK     bool
	existing   *SessionTurn
	sessionID  string
	sessionOK  bool
	created    *SessionTurn
	recentTurn []*SessionTurn
}

func (m *mockSupportRepo) AcquireSendIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return m.lockOK, nil
}

func (m *mockSupportRepo) FindTurnByUIDAndClientReqID(context.Context, string, string) (*SessionTurn, error) {
	return m.existing, nil
}

func (m *mockSupportRepo) EnsureSession(context.Context, string, string) (string, error) {
	if m.sessionID == "" {
		return "cs_1", nil
	}
	return m.sessionID, nil
}

func (m *mockSupportRepo) SessionExists(context.Context, string, string) (bool, error) {
	return m.sessionOK, nil
}

func (m *mockSupportRepo) ListRecentTurns(context.Context, string, string, int32) ([]*SessionTurn, error) {
	return m.recentTurn, nil
}

func (m *mockSupportRepo) CreateTurn(_ context.Context, turn *SessionTurn) error {
	turn.ID = 100
	m.created = turn
	return nil
}

func (m *mockSupportRepo) ListSessionTurns(context.Context, string, string, int32, int32) ([]*SessionTurn, error) {
	return []*SessionTurn{{ID: 1}}, nil
}

type mockModelGateway struct {
	out *ModelChatOutput
	err error
}

func (m *mockModelGateway) Chat(context.Context, ModelChatInput) (*ModelChatOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.out, nil
}

func TestSupportSendMessageDuplicate(t *testing.T) {
	repo := &mockSupportRepo{
		lockOK:   false,
		existing: &SessionTurn{ID: 8, SessionID: "cs_dup"},
	}
	userUC := NewUserUsecase(&mockUserRepo{}, log.NewStdLogger(io.Discard))
	walletUC := NewWalletUsecase(&mockPaymentRepo{}, &mockWalletRepo{}, nil, nil, log.NewStdLogger(io.Discard))
	uc := NewSupportUsecase(repo, &mockModelGateway{}, userUC, walletUC, log.NewStdLogger(io.Discard))
	turn, err := uc.SendMessage(context.Background(), SendMessageInput{
		UID:         "u1001",
		Content:     "你好",
		ClientReqID: "req_dup",
	})
	if err != nil {
		t.Fatalf("SendMessage() err=%v", err)
	}
	if turn.ID != 8 {
		t.Fatalf("SendMessage() id=%d, want 8", turn.ID)
	}
	if repo.created != nil {
		t.Fatal("SendMessage() should not create turn on duplicate request")
	}
}

func TestSupportSendMessageModelFailedFallback(t *testing.T) {
	repo := &mockSupportRepo{
		lockOK:    true,
		sessionID: "cs_1",
	}
	userUC := NewUserUsecase(&mockUserRepo{}, log.NewStdLogger(io.Discard))
	walletUC := NewWalletUsecase(&mockPaymentRepo{}, &mockWalletRepo{}, nil, nil, log.NewStdLogger(io.Discard))
	uc := NewSupportUsecase(repo, &mockModelGateway{err: errors.New("timeout")}, userUC, walletUC, log.NewStdLogger(io.Discard))
	turn, err := uc.SendMessage(context.Background(), SendMessageInput{
		UID:         "u1001",
		SessionID:   "cs_1",
		Content:     "支付失败了怎么办",
		ClientReqID: "req_1",
	})
	if err != nil {
		t.Fatalf("SendMessage() err=%v", err)
	}
	if !turn.UsedFallback {
		t.Fatal("SendMessage() used_fallback=false, want true")
	}
	if turn.Intent != IntentPayment {
		t.Fatalf("SendMessage() intent=%s, want %s", turn.Intent, IntentPayment)
	}
}

func TestSupportListSessionTurnsSessionNotFound(t *testing.T) {
	repo := &mockSupportRepo{sessionOK: false}
	userUC := NewUserUsecase(&mockUserRepo{}, log.NewStdLogger(io.Discard))
	walletUC := NewWalletUsecase(&mockPaymentRepo{}, &mockWalletRepo{}, nil, nil, log.NewStdLogger(io.Discard))
	uc := NewSupportUsecase(repo, &mockModelGateway{}, userUC, walletUC, log.NewStdLogger(io.Discard))
	_, err := uc.ListSessionTurns(context.Background(), "u1001", "cs_x", 1, 20)
	if err == nil {
		t.Fatal("ListSessionTurns() err=nil, want non-nil")
	}
}

func TestSupportSendMessageWithToolCalls(t *testing.T) {
	repo := &mockSupportRepo{
		lockOK:    true,
		sessionID: "cs_tool",
	}
	model := &mockModelGateway{
		out: &ModelChatOutput{
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "get_wallet_balance", Arguments: "{}"},
			},
			Model: "gpt-4",
		},
	}
	// On second call, return the final response
	cnt := 0
	modelChatMock := func(ctx context.Context, in ModelChatInput) (*ModelChatOutput, error) {
		cnt++
		if cnt == 1 {
			return model.out, nil
		}
		return &ModelChatOutput{
			Reply: "您的余额是 1000 分。",
			Model: "gpt-4",
		}, nil
	}

	userUC := NewUserUsecase(&mockUserRepo{user: &User{UID: "u1001", Status: 1, Mobile: "138", Nickname: "Test"}}, log.NewStdLogger(io.Discard))
	walletRepo := &mockWalletRepo{dbBalance: 1000}
	walletUC := NewWalletUsecase(&mockPaymentRepo{}, walletRepo, nil, nil, log.NewStdLogger(io.Discard))
	
	// Custom gateway to handle variable response
	uc := NewSupportUsecase(repo, &functionalMockGateway{chatFunc: modelChatMock}, userUC, walletUC, log.NewStdLogger(io.Discard))

	turn, err := uc.SendMessage(context.Background(), SendMessageInput{
		UID:         "u1001",
		Content:     "查下余额",
		ClientReqID: "req_tool",
	})
	if err != nil {
		t.Fatalf("SendMessage() err=%v", err)
	}
	if cnt != 2 {
		t.Fatalf("Expected 2 model calls (one for tools, one for final reply), got %d", cnt)
	}
	if !strings.Contains(turn.AssistantReply, "1000") {
		t.Fatalf("AssistantReply doesn't contain balance: %s", turn.AssistantReply)
	}
	if turn.UsedFallback {
		t.Fatal("SendMessage() used fallback unexpectedly")
	}
}

type functionalMockGateway struct {
	chatFunc func(context.Context, ModelChatInput) (*ModelChatOutput, error)
}

func (m *functionalMockGateway) Chat(ctx context.Context, in ModelChatInput) (*ModelChatOutput, error) {
	return m.chatFunc(ctx, in)
}
