package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	supportv1 "github.com/go-kratos/kratos-layout/api/support/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

const (
	// SupportSendIdempotentTTL is idempotency window for send message.
	SupportSendIdempotentTTL = 120 * time.Second
	// SupportRecentHistoryLimit is number of historical turns used for model context.
	SupportRecentHistoryLimit = 8
)

var (
	ErrSupportInvalidArgument = errors.BadRequest(supportv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid support argument")
	ErrSupportSessionNotFound = errors.NotFound(supportv1.ErrorReason_SUPPORT_SESSION_NOT_FOUND.String(), "support session not found")
	ErrSupportDuplicate       = errors.BadRequest(supportv1.ErrorReason_SUPPORT_DUPLICATE_REQUEST.String(), "duplicate support request")
	ErrSupportModelCallFailed = errors.InternalServer(supportv1.ErrorReason_SUPPORT_MODEL_CALL_FAILED.String(), "support model call failed")
)

// IntentType is classified user intent.
type IntentType string

const (
	IntentGreeting   IntentType = "GREETING"
	IntentBorrowHelp IntentType = "BORROW_HELP"
	IntentReturnHelp IntentType = "RETURN_HELP"
	IntentOrderHelp  IntentType = "ORDER_HELP"
	IntentPayment    IntentType = "PAYMENT_HELP"
	IntentDeposit    IntentType = "DEPOSIT_HELP"
	IntentComplaint  IntentType = "COMPLAINT_HELP"
	IntentUnknown    IntentType = "UNKNOWN"
)

// SessionTurn is one user-assistant turn record.
type SessionTurn struct {
	ID             uint64
	UID            string
	SessionID      string
	ClientReqID    string
	UserMessage    string
	AssistantReply string
	Intent         IntentType
	UsedFallback   bool
	ModelName      string
	PromptTokens   int32
	ReplyTokens    int32
	CreatedAt      time.Time
}

// SendMessageInput is support message input.
type SendMessageInput struct {
	UID         string
	SessionID   string
	Content     string
	ClientReqID string
}

// ModelMessage is one chat-completion role message.
type ModelMessage struct {
	Role    string
	Content string
}

// ModelChatInput is openai-compatible chat request.
type ModelChatInput struct {
	Messages []ModelMessage
	Tools    []ToolDefinition
}

// ModelChatOutput is normalized chat response.
type ModelChatOutput struct {
	Reply            string
	ToolCalls        []ToolCall
	Model            string
	PromptTokens     int32
	CompletionTokens int32
}

// ToolDefinition defines a function tool.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  any // JSON schema
}

// ToolCall represents a model's request to call a tool.
type ToolCall struct {
	ID       string
	Name     string
	Arguments string // JSON string
}

// SupportRepo is support session persistence abstraction.
type SupportRepo interface {
	AcquireSendIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error)
	FindTurnByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*SessionTurn, error)
	EnsureSession(ctx context.Context, uid, sessionID string) (string, error)
	SessionExists(ctx context.Context, uid, sessionID string) (bool, error)
	ListRecentTurns(ctx context.Context, uid, sessionID string, limit int32) ([]*SessionTurn, error)
	CreateTurn(ctx context.Context, turn *SessionTurn) error
	ListSessionTurns(ctx context.Context, uid, sessionID string, page, pageSize int32) ([]*SessionTurn, error)
}

// ModelChatGateway is openai-compatible model interface.
type ModelChatGateway interface {
	Chat(ctx context.Context, in ModelChatInput) (*ModelChatOutput, error)
}

// SupportUsecase handles customer-support conversations.
type SupportUsecase struct {
	repo    SupportRepo
	model   ModelChatGateway
	user    *UserUsecase
	wallet  *WalletUsecase
	deposit *DepositUsecase
	order   *OrderUsecase
	log     *log.Helper
	nowFunc func() time.Time
}

// NewSupportUsecase creates support usecase.
func NewSupportUsecase(repo SupportRepo, model ModelChatGateway, user *UserUsecase, wallet *WalletUsecase, deposit *DepositUsecase, order *OrderUsecase, logger log.Logger) *SupportUsecase {
	return &SupportUsecase{
		repo:    repo,
		model:   model,
		user:    user,
		wallet:  wallet,
		deposit: deposit,
		order:   order,
		log:     log.NewHelper(log.With(logger, "module", "biz/support")),
		nowFunc: time.Now,
	}
}

// SendMessage sends one user message and gets assistant reply.
func (uc *SupportUsecase) SendMessage(ctx context.Context, in SendMessageInput) (*SessionTurn, error) {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.Content) == "" || strings.TrimSpace(in.ClientReqID) == "" {
		return nil, ErrSupportInvalidArgument
	}
	in.Content = strings.TrimSpace(in.Content)

	ok, err := uc.repo.AcquireSendIdempotent(ctx, in.UID, in.ClientReqID, SupportSendIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.repo.FindTurnByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, ErrSupportDuplicate
	}

	sessionID, err := uc.repo.EnsureSession(ctx, in.UID, strings.TrimSpace(in.SessionID))
	if err != nil {
		return nil, err
	}

	intent := uc.detectIntent(in.Content)
	history, err := uc.repo.ListRecentTurns(ctx, in.UID, sessionID, SupportRecentHistoryLimit)
	if err != nil {
		return nil, err
	}
	modelInput := uc.buildModelInput(history, in.Content, intent)
	modelInput.Tools = uc.buildTools()

	reply := ""
	modelName := ""
	var promptTokens int32
	var completionTokens int32
	usedFallback := false

	// Model Interaction Loop (handling tool calls)
	resp, err := uc.model.Chat(ctx, modelInput)
	if err != nil || (resp != nil && strings.TrimSpace(resp.Reply) == "" && len(resp.ToolCalls) == 0) {
		usedFallback = true
		reply = uc.fallbackReply(intent)
		uc.log.Warnf("model call failed, use fallback: uid=%s session=%s err=%v", in.UID, sessionID, err)
	} else {
		modelName = resp.Model
		promptTokens += resp.PromptTokens
		completionTokens += resp.CompletionTokens

		if len(resp.ToolCalls) > 0 {
			// Handle Tool Calls
			toolMessages := uc.handleToolCalls(ctx, in.UID, resp.ToolCalls)
			modelInput.Messages = append(modelInput.Messages, ModelMessage{
				Role:    "assistant",
				Content: resp.Reply, // Might be empty if only tools are called
			})
			modelInput.Messages = append(modelInput.Messages, toolMessages...)

			// Second call with tool results
			finalResp, finalErr := uc.model.Chat(ctx, modelInput)
			if finalErr != nil {
				usedFallback = true
				reply = uc.fallbackReply(intent)
			} else {
				reply = strings.TrimSpace(finalResp.Reply)
				promptTokens += finalResp.PromptTokens
				completionTokens += finalResp.CompletionTokens
			}
		} else {
			reply = strings.TrimSpace(resp.Reply)
		}
	}

	turn := &SessionTurn{
		UID:            in.UID,
		SessionID:      sessionID,
		ClientReqID:    in.ClientReqID,
		UserMessage:    in.Content,
		AssistantReply: reply,
		Intent:         intent,
		UsedFallback:   usedFallback,
		ModelName:      modelName,
		PromptTokens:   promptTokens,
		ReplyTokens:    completionTokens,
	}
	if err = uc.repo.CreateTurn(ctx, turn); err != nil {
		return nil, err
	}
	return turn, nil
}

// ListSessionTurns lists conversation turns in one session.
func (uc *SupportUsecase) ListSessionTurns(ctx context.Context, uid, sessionID string, page, pageSize int32) ([]*SessionTurn, error) {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, ErrSupportInvalidArgument
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	ok, err := uc.repo.SessionExists(ctx, uid, sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSupportSessionNotFound
	}
	return uc.repo.ListSessionTurns(ctx, uid, sessionID, page, pageSize)
}

func (uc *SupportUsecase) detectIntent(content string) IntentType {
	c := strings.ToLower(content)
	switch {
	case containsAny(c, "你好", "您好", "hello", "hi"):
		return IntentGreeting
	case containsAny(c, "租借", "借充电宝", "扫码借", "借机柜"):
		return IntentBorrowHelp
	case containsAny(c, "归还", "还充电宝", "还不了", "无法归还"):
		return IntentReturnHelp
	case containsAny(c, "订单", "账单", "扣费", "费用", "余额"):
		return IntentOrderHelp
	case containsAny(c, "支付", "微信", "支付宝", "支付失败"):
		return IntentPayment
	case containsAny(c, "押金", "免押", "退押金"):
		return IntentDeposit
	case containsAny(c, "投诉", "故障", "人工客服", "客服"):
		return IntentComplaint
	default:
		return IntentUnknown
	}
}

func (uc *SupportUsecase) buildModelInput(history []*SessionTurn, userMessage string, intent IntentType) ModelChatInput {
	systemPrompt := fmt.Sprintf(
		"你是怪兽充电宝客服助手。你可以获取用户资料和钱包余额。请优先通过调用工具获取准确信息后再回答。当前意图=%s。",
		intent,
	)
	messages := make([]ModelMessage, 0, len(history)*2+2)
	messages = append(messages, ModelMessage{Role: "system", Content: systemPrompt})
	for _, turn := range history {
		if strings.TrimSpace(turn.UserMessage) != "" {
			messages = append(messages, ModelMessage{Role: "user", Content: turn.UserMessage})
		}
		if strings.TrimSpace(turn.AssistantReply) != "" {
			messages = append(messages, ModelMessage{Role: "assistant", Content: turn.AssistantReply})
		}
	}
	messages = append(messages, ModelMessage{Role: "user", Content: userMessage})
	return ModelChatInput{Messages: messages}
}

func (uc *SupportUsecase) buildTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "get_user_info",
			Description: "获取当前登录用户的基本资料，如手机号、昵称等",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_wallet_balance",
			Description: "查询当前用户的账户余额（单位：分）",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_deposit_status",
			Description: "查询当前用户的押金和免押状态",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_current_order",
			Description: "查询当前用户进行中的租借订单",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (uc *SupportUsecase) handleToolCalls(ctx context.Context, uid string, calls []ToolCall) []ModelMessage {
	results := make([]ModelMessage, 0, len(calls))
	for _, call := range calls {
		var resContent string
		switch call.Name {
		case "get_user_info":
			u, err := uc.user.GetProfile(ctx, uid)
			if err != nil {
				resContent = fmt.Sprintf("Error: %v", err)
			} else {
				resContent = fmt.Sprintf("User: mobile=%s, nickname=%s", u.Mobile, u.Nickname)
			}
		case "get_wallet_balance":
			balance, err := uc.wallet.GetBalance(ctx, uid)
			if err != nil {
				resContent = fmt.Sprintf("Error: %v", err)
			} else {
				resContent = fmt.Sprintf("Balance: %d cents", balance)
			}
		case "get_deposit_status":
			if uc.deposit == nil {
				resContent = "Deposit status unavailable"
			} else {
				profile, err := uc.deposit.GetStatus(ctx, uid)
				if err != nil {
					resContent = fmt.Sprintf("Error: %v", err)
				} else {
					resContent = fmt.Sprintf("Deposit: status=%d paid=%t exempt=%t amount=%d expire_at=%d", profile.Status, profile.Paid, profile.Exempt, profile.DepositAmount, profile.ExemptExpireAt.Unix())
				}
			}
		case "get_current_order":
			if uc.order == nil {
				resContent = "Current order unavailable"
			} else {
				order, err := uc.order.GetCurrentOrder(ctx, uid)
				if err != nil {
					resContent = fmt.Sprintf("Error: %v", err)
				} else if order == nil {
					resContent = "No active order"
				} else {
					resContent = fmt.Sprintf("Order: no=%s status=%d pay_status=%d rent_fee=%d powerbank=%s", order.RentOrderNo, order.Status, order.PayStatus, order.RentFee, order.PowerbankID)
				}
			}
		default:
			resContent = "Unknown tool"
		}
		results = append(results, ModelMessage{
			Role:    "tool",
			Content: resContent,
		})
	}
	return results
}

func (uc *SupportUsecase) fallbackReply(intent IntentType) string {
	switch intent {
	case IntentBorrowHelp:
		return "可以先确认机柜是否在线，再重新扫码借出；若连续失败，请提供设备编号，我来帮你转人工处理。"
	case IntentReturnHelp:
		return "请先确认归还仓门是否打开，再尝试重新插入；若仍失败，请提供订单号，我为你发起异常归还处理。"
	case IntentOrderHelp:
		return "请在订单列表核对开始时间和归还时间；若扣费异常，请把订单号发我，我帮你核查。"
	case IntentPayment:
		return "建议先确认微信/支付宝账户状态和网络，再重新支付；如果已扣款未生效，请提供支付单号。"
	case IntentDeposit:
		return "押金问题可先查看账户页押金状态；若退款超时，请提供手机号和订单号以便排查。"
	case IntentComplaint:
		return "我已收到你的反馈。请补充设备编号、地点和时间，我会整理后提交给人工客服优先处理。"
	case IntentGreeting:
		return "你好，我是怪兽充电宝客服助手。你可以直接告诉我借还、订单、支付或押金问题。"
	default:
		return "我先帮你记录问题。请补充订单号/设备编号/发生时间，我会给你更准确的处理建议。"
	}
}

func containsAny(content string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (o *ModelChatOutput) GetReply() string {
	if o == nil {
		return ""
	}
	return o.Reply
}
