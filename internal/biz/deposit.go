package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	depositv1 "github.com/go-kratos/kratos-layout/api/deposit/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const (
	DefaultDepositAmount         int64         = 9900
	DepositExemptionApplyTTL                   = 120 * time.Second
)

var (
	ErrDepositInvalidArgument         = errors.BadRequest(depositv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid deposit argument")
	ErrDepositAlreadyPaid             = errors.BadRequest(depositv1.ErrorReason_DEPOSIT_ALREADY_PAID.String(), "deposit already paid")
	ErrDepositOrderNotFound           = errors.NotFound(depositv1.ErrorReason_DEPOSIT_ORDER_NOT_FOUND.String(), "deposit order not found")
	ErrDepositExemptionRejected       = errors.BadRequest(depositv1.ErrorReason_DEPOSIT_EXEMPTION_REJECTED.String(), "deposit exemption rejected")
	ErrDepositExemptionAlreadyActive  = errors.BadRequest(depositv1.ErrorReason_DEPOSIT_EXEMPTION_ALREADY_ACTIVE.String(), "deposit exemption already active")
	ErrDepositDuplicateRequest        = errors.BadRequest(depositv1.ErrorReason_DEPOSIT_DUPLICATE_REQUEST.String(), "duplicate deposit request")
)

type DepositStatus int32

const (
	DepositStatusRequired DepositStatus = 1
	DepositStatusPaid     DepositStatus = 2
	DepositStatusExempt   DepositStatus = 3
)

type ExemptionProvider string

const (
	ExemptionProviderAlipayCredit ExemptionProvider = "ALIPAY_CREDIT"
	ExemptionProviderWechatCredit ExemptionProvider = "WECHAT_CREDIT"
)

type ExemptionStatus int32

const (
	ExemptionStatusPending  ExemptionStatus = 1
	ExemptionStatusApproved ExemptionStatus = 2
	ExemptionStatusRejected ExemptionStatus = 3
)

type DepositProfile struct {
	UID               string
	Status            DepositStatus
	DepositAmount     int64
	Paid              bool
	Exempt            bool
	ActiveOrderNo     string
	ExemptProvider    ExemptionProvider
	ExemptExpireAt    time.Time
}

type DepositOrder struct {
	DepositOrderNo string
	UID            string
	OutTradeNo     string
	ClientReqID    string
	Channel        string
	PayMode        PayMode
	Amount         int64
	Status         PayStatus
	CodeURL        string
	PayURL         string
	JSAPIParams    map[string]string
	CreatedAt      time.Time
}

type DepositExemptionRecord struct {
	ExemptionID string
	UID         string
	ClientReqID string
	Provider    ExemptionProvider
	CreditScore int32
	Status      ExemptionStatus
	Reason      string
	ExpireAt    time.Time
	CreatedAt   time.Time
}

type DepositRecord struct {
	RecordType       string
	RecordID         string
	DepositOrderNo   string
	OutTradeNo       string
	OrderStatus      PayStatus
	Amount           int64
	Channel          string
	ExemptionStatus  ExemptionStatus
	ExemptionProvider ExemptionProvider
	CreditScore      int32
	Reason           string
	CreatedAt        time.Time
	ExpireAt         time.Time
}

type DepositAccess struct {
	Allowed       bool
	DepositAmount int64
	Mode          DepositStatus
}

type CreditDecision struct {
	Provider    ExemptionProvider
	CreditScore int32
	Approved    bool
	Reason      string
	ExpireAt    time.Time
}

type CreateDepositOrderInput struct {
	UID         string
	Channel     string
	PayMode     PayMode
	ClientReqID string
	OpenID      string
}

type ApplyDepositExemptionInput struct {
	UID         string
	Provider    ExemptionProvider
	ClientReqID string
}

type DepositRepo interface {
	GetOrCreateProfile(ctx context.Context, uid string, depositAmount int64) (*DepositProfile, error)
	FindDepositOrderByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*DepositOrder, error)
	CreateDepositOrder(ctx context.Context, order *DepositOrder) error
	MarkDepositOrderPaying(ctx context.Context, outTradeNo string) error
	FindExemptionByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*DepositExemptionRecord, error)
	AcquireExemptionIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error)
	CreateExemptionRecord(ctx context.Context, profile *DepositProfile, record *DepositExemptionRecord) error
	ListRecords(ctx context.Context, uid string, page, pageSize int32) ([]*DepositRecord, error)
}

type CreditGateway interface {
	EvaluateExemption(ctx context.Context, uid string, provider ExemptionProvider) (*CreditDecision, error)
}

type DepositUsecase struct {
	depositRepo DepositRepo
	paymentRepo PaymentRepo
	wxGateway   WxPayGateway
	aliGateway  AlipayGateway
	credit      CreditGateway
	log         *log.Helper
	nowFunc     func() time.Time
}

func NewDepositUsecase(depositRepo DepositRepo, paymentRepo PaymentRepo, wxGateway WxPayGateway, aliGateway AlipayGateway, credit CreditGateway, logger log.Logger) *DepositUsecase {
	return &DepositUsecase{
		depositRepo: depositRepo,
		paymentRepo: paymentRepo,
		wxGateway:   wxGateway,
		aliGateway:  aliGateway,
		credit:      credit,
		log:         log.NewHelper(log.With(logger, "module", "biz/deposit")),
		nowFunc:     time.Now,
	}
}

func (uc *DepositUsecase) GetStatus(ctx context.Context, uid string) (*DepositProfile, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrDepositInvalidArgument
	}
	return uc.depositRepo.GetOrCreateProfile(ctx, uid, DefaultDepositAmount)
}

func (uc *DepositUsecase) HasDepositPrivilege(ctx context.Context, uid string) (*DepositAccess, error) {
	profile, err := uc.GetStatus(ctx, uid)
	if err != nil {
		return nil, err
	}
	access := &DepositAccess{Allowed: false, DepositAmount: profile.DepositAmount, Mode: profile.Status}
	if profile.Paid {
		access.Allowed = true
		access.Mode = DepositStatusPaid
		return access, nil
	}
	if profile.Exempt && (profile.ExemptExpireAt.IsZero() || profile.ExemptExpireAt.After(uc.nowFunc())) {
		access.Allowed = true
		access.Mode = DepositStatusExempt
		return access, nil
	}
	return access, nil
}

func (uc *DepositUsecase) CreateDepositOrder(ctx context.Context, in CreateDepositOrderInput) (*DepositOrder, error) {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.ClientReqID) == "" {
		return nil, ErrDepositInvalidArgument
	}
	if in.Channel != PayChannelWx && in.Channel != PayChannelAl {
		return nil, ErrDepositInvalidArgument
	}
	if in.Channel == PayChannelWx {
		if in.PayMode != PayModeNative && in.PayMode != PayModeJSAPI {
			return nil, ErrDepositInvalidArgument
		}
		if in.PayMode == PayModeJSAPI && strings.TrimSpace(in.OpenID) == "" {
			return nil, ErrDepositInvalidArgument
		}
	} else {
		in.PayMode = PayModeNative
	}

	profile, err := uc.depositRepo.GetOrCreateProfile(ctx, in.UID, DefaultDepositAmount)
	if err != nil {
		return nil, err
	}
	if profile.Paid {
		return nil, ErrDepositAlreadyPaid
	}
	if profile.Exempt && (profile.ExemptExpireAt.IsZero() || profile.ExemptExpireAt.After(uc.nowFunc())) {
		return nil, ErrDepositExemptionAlreadyActive
	}

	ok, err := uc.paymentRepo.AcquireCreateIdempotent(ctx, in.UID, in.ClientReqID, CreatePayOrderIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.depositRepo.FindDepositOrderByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, ErrDepositDuplicateRequest
	}

	outTradeNo := uc.newDepositOutTradeNo(in.Channel)
	depositOrder := &DepositOrder{
		DepositOrderNo: uc.newDepositOrderNo(),
		UID:            in.UID,
		OutTradeNo:     outTradeNo,
		ClientReqID:    in.ClientReqID,
		Channel:        in.Channel,
		PayMode:        in.PayMode,
		Amount:         profile.DepositAmount,
		Status:         PayStatusInit,
	}
	if err = uc.paymentRepo.CreateOrder(ctx, &PaymentOrder{
		UID:         in.UID,
		OutTradeNo:  outTradeNo,
		ClientReqID: in.ClientReqID,
		Channel:     in.Channel,
		PayMode:     in.PayMode,
		BizType:     BizTypeDeposit,
		BizOrderNo:  depositOrder.DepositOrderNo,
		Amount:      profile.DepositAmount,
		Status:      PayStatusInit,
	}); err != nil {
		return nil, err
	}
	if err = uc.depositRepo.CreateDepositOrder(ctx, depositOrder); err != nil {
		return nil, err
	}

	if in.Channel == PayChannelWx {
		resp, wxErr := uc.wxGateway.CreateOrder(ctx, WxCreateOrderInput{
			OutTradeNo: outTradeNo,
			Amount:     profile.DepositAmount,
			PayMode:    in.PayMode,
			OpenID:     in.OpenID,
		})
		if wxErr != nil {
			return nil, wxErr
		}
		depositOrder.CodeURL = resp.CodeURL
		depositOrder.JSAPIParams = resp.JSAPIParams
		if err = uc.paymentRepo.UpdatePrepay(ctx, outTradeNo, resp.PrepayID, resp.CodeURL, resp.JSAPIParams); err != nil {
			return nil, err
		}
	} else {
		resp, aliErr := uc.aliGateway.CreateOrder(ctx, AlipayCreateOrderInput{
			OutTradeNo: outTradeNo,
			Amount:     profile.DepositAmount,
		})
		if aliErr != nil {
			return nil, aliErr
		}
		depositOrder.PayURL = resp.PayURL
		depositOrder.CodeURL = resp.PayURL
		depositOrder.JSAPIParams = resp.PayParams
		if err = uc.paymentRepo.UpdatePrepay(ctx, outTradeNo, resp.TradeNo, resp.PayURL, resp.PayParams); err != nil {
			return nil, err
		}
	}
	if err = uc.depositRepo.MarkDepositOrderPaying(ctx, outTradeNo); err != nil {
		return nil, err
	}
	depositOrder.Status = PayStatusPaying
	return depositOrder, nil
}

func (uc *DepositUsecase) ApplyExemption(ctx context.Context, in ApplyDepositExemptionInput) (*DepositExemptionRecord, error) {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.ClientReqID) == "" || strings.TrimSpace(string(in.Provider)) == "" {
		return nil, ErrDepositInvalidArgument
	}
	profile, err := uc.depositRepo.GetOrCreateProfile(ctx, in.UID, DefaultDepositAmount)
	if err != nil {
		return nil, err
	}
	if profile.Paid {
		return nil, ErrDepositAlreadyPaid
	}
	if profile.Exempt && (profile.ExemptExpireAt.IsZero() || profile.ExemptExpireAt.After(uc.nowFunc())) {
		return nil, ErrDepositExemptionAlreadyActive
	}

	ok, err := uc.depositRepo.AcquireExemptionIdempotent(ctx, in.UID, in.ClientReqID, DepositExemptionApplyTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.depositRepo.FindExemptionByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, ErrDepositDuplicateRequest
	}

	decision, err := uc.credit.EvaluateExemption(ctx, in.UID, in.Provider)
	if err != nil {
		return nil, err
	}
	record := &DepositExemptionRecord{
		ExemptionID: uc.newExemptionID(),
		UID:         in.UID,
		ClientReqID: in.ClientReqID,
		Provider:    decision.Provider,
		CreditScore: decision.CreditScore,
		Status:      ExemptionStatusRejected,
		Reason:      decision.Reason,
		ExpireAt:    decision.ExpireAt,
	}
	if decision.Approved {
		record.Status = ExemptionStatusApproved
	}
	if err = uc.depositRepo.CreateExemptionRecord(ctx, profile, record); err != nil {
		return nil, err
	}
	if !decision.Approved {
		return nil, ErrDepositExemptionRejected
	}
	return record, nil
}

func (uc *DepositUsecase) ListRecords(ctx context.Context, uid string, page, pageSize int32) ([]*DepositRecord, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrDepositInvalidArgument
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	return uc.depositRepo.ListRecords(ctx, uid, page, pageSize)
}

func (uc *DepositUsecase) newDepositOrderNo() string {
	return "DP" + uc.nowFunc().Format("20060102150405") + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:10]
}

func (uc *DepositUsecase) newDepositOutTradeNo(channel string) string {
	prefix := "DW"
	if channel == PayChannelAl {
		prefix = "DA"
	}
	return fmt.Sprintf("%s%s%s", prefix, uc.nowFunc().Format("20060102150405"), strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:12])
}

func (uc *DepositUsecase) newExemptionID() string {
	return "EX" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:16]
}
