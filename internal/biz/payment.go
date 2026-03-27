package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/payment/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const (
	// CreatePayOrderIdempotentTTL defines create-order idempotency window.
	CreatePayOrderIdempotentTTL = 120 * time.Second
	// NotifyLockTTL defines notify processing lock duration.
	NotifyLockTTL = 10 * time.Second
)

const (
	PayChannelWx = "WECHAT"
	PayChannelAl = "ALIPAY"
)

// PayMode is unified pay mode.
type PayMode int32

const (
	PayModeUnspecified PayMode = 0
	PayModeNative      PayMode = 1
	PayModeJSAPI       PayMode = 2
)

// BizType is payment business type.
type BizType int32

const (
	BizTypeUnspecified BizType = 0
	BizTypeRecharge    BizType = 1
	BizTypeRentOrder   BizType = 2
)

// PayStatus is payment order status.
type PayStatus int32

const (
	PayStatusUnspecified PayStatus = 0
	PayStatusInit        PayStatus = 1
	PayStatusPaying      PayStatus = 2
	PayStatusSuccess     PayStatus = 3
	PayStatusFailed      PayStatus = 4
)

var (
	ErrPaymentInvalidArgument = errors.BadRequest(v1.ErrorReason_INVALID_ARGUMENT.String(), "invalid payment argument")
	ErrPaymentOrderNotFound   = errors.NotFound(v1.ErrorReason_PAYMENT_ORDER_NOT_FOUND.String(), "payment order not found")
	ErrPaymentSignVerifyFail  = errors.Unauthorized(v1.ErrorReason_PAYMENT_SIGN_VERIFY_FAILED.String(), "payment sign verify failed")
	ErrPaymentStatusInvalid   = errors.BadRequest(v1.ErrorReason_PAYMENT_STATUS_INVALID.String(), "payment status invalid")
	ErrPaymentHeartbeatFailed = errors.InternalServer(v1.ErrorReason_PAYMENT_HEARTBEAT_FAILED.String(), "payment heartbeat failed")
)

// PaymentOrder is unified payment order aggregate.
type PaymentOrder struct {
	ID            uint64
	UID           string
	OutTradeNo    string
	ClientReqID   string
	Channel       string
	PayMode       PayMode
	BizType       BizType
	BizOrderNo    string
	Amount        int64
	Status        PayStatus
	PrepayID      string
	CodeURL       string
	JSAPIParams   map[string]string
	TransactionID string
}

// CreateWxPayOrderInput is usecase input.
type CreateWxPayOrderInput struct {
	UID         string
	PayMode     PayMode
	BizType     BizType
	BizOrderNo  string
	Amount      int64
	ClientReqID string
	OpenID      string
}

// WxNotifyInput contains notify headers and body.
type WxNotifyInput struct {
	Body      string
	Timestamp string
	Nonce     string
	Signature string
	Serial    string
}

// WxNotifyEvent is parsed normalized callback payload.
type WxNotifyEvent struct {
	EventID       string
	EventType     string
	OutTradeNo    string
	TradeState    string
	TransactionID string
	RawBody       string
}

// AlipayNotifyInput contains callback headers and body.
type AlipayNotifyInput struct {
	Body      string
	Signature string
	NotifyID  string
	Timestamp string
}

// AlipayNotifyEvent is parsed callback payload from alipay.
type AlipayNotifyEvent struct {
	EventID     string
	OutTradeNo  string
	TradeStatus string
	TradeNo     string
	RawBody     string
}

// WxCreateOrderInput defines upstream create request.
type WxCreateOrderInput struct {
	OutTradeNo string
	Amount     int64
	PayMode    PayMode
	OpenID     string
}

// WxCreateOrderOutput defines upstream create response.
type WxCreateOrderOutput struct {
	PrepayID    string
	CodeURL     string
	JSAPIParams map[string]string
}

// AlipayCreateOrderInput defines alipay pre-create order input.
type AlipayCreateOrderInput struct {
	OutTradeNo string
	Amount     int64
}

// AlipayCreateOrderOutput defines alipay pre-create order output.
type AlipayCreateOrderOutput struct {
	TradeNo   string
	PayURL    string
	PayParams map[string]string
}

// WxPayGateway handles wechat pay integration.
type WxPayGateway interface {
	CreateOrder(ctx context.Context, in WxCreateOrderInput) (*WxCreateOrderOutput, error)
	VerifyNotify(ctx context.Context, in WxNotifyInput) error
	ParseNotify(ctx context.Context, body string) (*WxNotifyEvent, error)
}

// AlipayGateway handles alipay integration.
type AlipayGateway interface {
	CreateOrder(ctx context.Context, in AlipayCreateOrderInput) (*AlipayCreateOrderOutput, error)
	VerifyNotify(ctx context.Context, in AlipayNotifyInput) error
	ParseNotify(ctx context.Context, body string) (*AlipayNotifyEvent, error)
	Heartbeat(ctx context.Context) error
}

// PaymentRepo contains persistent operations for payment domain.
type PaymentRepo interface {
	AcquireCreateIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error)
	AcquireNotifyLock(ctx context.Context, eventID string, ttl time.Duration) (bool, error)
	FindByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*PaymentOrder, error)
	CreateOrder(ctx context.Context, order *PaymentOrder) error
	UpdatePrepay(ctx context.Context, outTradeNo, prepayID, codeURL string, jsapiParams map[string]string) error
	GetByUIDAndOutTradeNo(ctx context.Context, uid, outTradeNo string) (*PaymentOrder, error)
	HandleWxNotify(ctx context.Context, event *WxNotifyEvent) (bool, error)
	HandleAlipayNotify(ctx context.Context, event *AlipayNotifyEvent) (bool, error)
}

// PaymentUsecase handles payment domain operations.
type PaymentUsecase struct {
	repo    PaymentRepo
	wx      WxPayGateway
	alipay  AlipayGateway
	log     *log.Helper
	nowFunc func() time.Time
}

// NewPaymentUsecase creates payment usecase.
func NewPaymentUsecase(repo PaymentRepo, wx WxPayGateway, alipay AlipayGateway, logger log.Logger) *PaymentUsecase {
	return &PaymentUsecase{
		repo:    repo,
		wx:      wx,
		alipay:  alipay,
		log:     log.NewHelper(log.With(logger, "module", "biz/payment")),
		nowFunc: time.Now,
	}
}

// CreateWxPayOrder creates a payment order and prepay payload.
func (uc *PaymentUsecase) CreateWxPayOrder(ctx context.Context, in CreateWxPayOrderInput) (*PaymentOrder, error) {
	if err := validateCreateInput(in); err != nil {
		return nil, err
	}

	ok, err := uc.repo.AcquireCreateIdempotent(ctx, in.UID, in.ClientReqID, CreatePayOrderIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Idempotent replay: return existing order.
		existing, exErr := uc.repo.FindByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing == nil {
			return nil, ErrPaymentStatusInvalid
		}
		return existing, nil
	}

	outTradeNo := uc.newOutTradeNo()
	order := &PaymentOrder{
		UID:         in.UID,
		OutTradeNo:  outTradeNo,
		ClientReqID: in.ClientReqID,
		Channel:     PayChannelWx,
		PayMode:     in.PayMode,
		BizType:     in.BizType,
		BizOrderNo:  in.BizOrderNo,
		Amount:      in.Amount,
		Status:      PayStatusInit,
	}
	if err = uc.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	payResp, err := uc.wx.CreateOrder(ctx, WxCreateOrderInput{
		OutTradeNo: outTradeNo,
		Amount:     in.Amount,
		PayMode:    in.PayMode,
		OpenID:     in.OpenID,
	})
	if err != nil {
		return nil, err
	}

	if err = uc.repo.UpdatePrepay(ctx, outTradeNo, payResp.PrepayID, payResp.CodeURL, payResp.JSAPIParams); err != nil {
		return nil, err
	}
	return uc.repo.GetByUIDAndOutTradeNo(ctx, in.UID, outTradeNo)
}

// QueryPayOrder returns payment order by uid and outTradeNo.
func (uc *PaymentUsecase) QueryPayOrder(ctx context.Context, uid, outTradeNo string) (*PaymentOrder, error) {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(outTradeNo) == "" {
		return nil, ErrPaymentInvalidArgument
	}
	order, err := uc.repo.GetByUIDAndOutTradeNo(ctx, uid, outTradeNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPaymentOrderNotFound
	}
	return order, nil
}

// HandleWxNotify verifies and processes payment callback.
func (uc *PaymentUsecase) HandleWxNotify(ctx context.Context, in WxNotifyInput) (bool, error) {
	if strings.TrimSpace(in.Body) == "" {
		return false, ErrPaymentInvalidArgument
	}
	if err := uc.wx.VerifyNotify(ctx, in); err != nil {
		return false, ErrPaymentSignVerifyFail
	}
	event, err := uc.wx.ParseNotify(ctx, in.Body)
	if err != nil {
		return false, ErrPaymentInvalidArgument
	}
	lockOK, err := uc.repo.AcquireNotifyLock(ctx, event.EventID, NotifyLockTTL)
	if err != nil {
		return false, err
	}
	if !lockOK {
		return false, nil
	}
	return uc.repo.HandleWxNotify(ctx, event)
}

// HandleAlipayNotify verifies and processes alipay callback.
func (uc *PaymentUsecase) HandleAlipayNotify(ctx context.Context, in AlipayNotifyInput) (bool, error) {
	if strings.TrimSpace(in.Body) == "" {
		return false, ErrPaymentInvalidArgument
	}
	if err := uc.alipay.VerifyNotify(ctx, in); err != nil {
		return false, ErrPaymentSignVerifyFail
	}
	event, err := uc.alipay.ParseNotify(ctx, in.Body)
	if err != nil {
		return false, ErrPaymentInvalidArgument
	}
	lockOK, err := uc.repo.AcquireNotifyLock(ctx, event.EventID, NotifyLockTTL)
	if err != nil {
		return false, err
	}
	if !lockOK {
		return false, nil
	}
	return uc.repo.HandleAlipayNotify(ctx, event)
}

// AlipayHeartbeat checks alipay gateway health.
func (uc *PaymentUsecase) AlipayHeartbeat(ctx context.Context) error {
	if err := uc.alipay.Heartbeat(ctx); err != nil {
		return ErrPaymentHeartbeatFailed
	}
	return nil
}

func validateCreateInput(in CreateWxPayOrderInput) error {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.ClientReqID) == "" ||
		strings.TrimSpace(in.BizOrderNo) == "" || in.Amount <= 0 {
		return ErrPaymentInvalidArgument
	}
	if in.PayMode != PayModeNative && in.PayMode != PayModeJSAPI {
		return ErrPaymentInvalidArgument
	}
	if in.BizType != BizTypeRecharge && in.BizType != BizTypeRentOrder {
		return ErrPaymentInvalidArgument
	}
	if in.PayMode == PayModeJSAPI && strings.TrimSpace(in.OpenID) == "" {
		return ErrPaymentInvalidArgument
	}
	return nil
}

func (uc *PaymentUsecase) newOutTradeNo() string {
	return fmt.Sprintf("WX%s%s", uc.nowFunc().Format("20060102150405"), strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:12])
}
