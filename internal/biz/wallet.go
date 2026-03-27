package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	walletv1 "github.com/go-kratos/kratos-layout/api/wallet/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const (
	// WalletBalanceCacheTTL defines wallet balance cache ttl.
	WalletBalanceCacheTTL = 5 * time.Minute
)

var (
	ErrWalletInvalidArgument = errors.BadRequest(walletv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid wallet argument")
)

// WalletRepo defines wallet persistence + cache behavior.
type WalletRepo interface {
	GetBalance(ctx context.Context, uid string) (int64, error)
	GetCachedBalance(ctx context.Context, uid string) (int64, bool, error)
	SetCachedBalance(ctx context.Context, uid string, balance int64, ttl time.Duration) error
}

// CreateRechargeOrderInput is wallet recharge input.
type CreateRechargeOrderInput struct {
	UID         string
	Channel     string
	PayMode     PayMode
	Amount      int64
	ClientReqID string
	OpenID      string
}

// WalletRechargeOrder is wallet recharge output.
type WalletRechargeOrder struct {
	OutTradeNo  string
	Channel     string
	Status      PayStatus
	CodeURL     string
	PayURL      string
	JSAPIParams map[string]string
}

// WalletUsecase contains wallet and recharge operations.
type WalletUsecase struct {
	paymentRepo PaymentRepo
	walletRepo  WalletRepo
	wxGateway   WxPayGateway
	aliGateway  AlipayGateway
	log         *log.Helper
	nowFunc     func() time.Time
}

// NewWalletUsecase creates wallet usecase.
func NewWalletUsecase(paymentRepo PaymentRepo, walletRepo WalletRepo, wxGateway WxPayGateway, aliGateway AlipayGateway, logger log.Logger) *WalletUsecase {
	return &WalletUsecase{
		paymentRepo: paymentRepo,
		walletRepo:  walletRepo,
		wxGateway:   wxGateway,
		aliGateway:  aliGateway,
		log:         log.NewHelper(log.With(logger, "module", "biz/wallet")),
		nowFunc:     time.Now,
	}
}

// GetBalance gets balance from redis cache first, then mysql.
func (uc *WalletUsecase) GetBalance(ctx context.Context, uid string) (int64, error) {
	if strings.TrimSpace(uid) == "" {
		return 0, ErrWalletInvalidArgument
	}
	balance, found, err := uc.walletRepo.GetCachedBalance(ctx, uid)
	if err != nil {
		return 0, err
	}
	if found {
		return balance, nil
	}
	balance, err = uc.walletRepo.GetBalance(ctx, uid)
	if err != nil {
		return 0, err
	}
	if err = uc.walletRepo.SetCachedBalance(ctx, uid, balance, WalletBalanceCacheTTL); err != nil {
		uc.log.Warnf("set wallet cache failed: uid=%s err=%v", uid, err)
	}
	return balance, nil
}

// CreateRechargeOrder creates recharge payment order for wechat or alipay.
func (uc *WalletUsecase) CreateRechargeOrder(ctx context.Context, in CreateRechargeOrderInput) (*WalletRechargeOrder, error) {
	if err := validateRechargeInput(in); err != nil {
		return nil, err
	}

	ok, err := uc.paymentRepo.AcquireCreateIdempotent(ctx, in.UID, in.ClientReqID, CreatePayOrderIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.paymentRepo.FindByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing == nil {
			return nil, ErrPaymentStatusInvalid
		}
		return &WalletRechargeOrder{
			OutTradeNo:  existing.OutTradeNo,
			Channel:     existing.Channel,
			Status:      existing.Status,
			CodeURL:     existing.CodeURL,
			PayURL:      existing.CodeURL,
			JSAPIParams: existing.JSAPIParams,
		}, nil
	}

	outTradeNo := uc.newRechargeOutTradeNo(in.Channel)
	order := &PaymentOrder{
		UID:         in.UID,
		OutTradeNo:  outTradeNo,
		ClientReqID: in.ClientReqID,
		Channel:     in.Channel,
		PayMode:     in.PayMode,
		BizType:     BizTypeRecharge,
		BizOrderNo:  uc.newBizOrderNo(),
		Amount:      in.Amount,
		Status:      PayStatusInit,
	}
	if err = uc.paymentRepo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	var (
		prepayID    string
		codeURL     string
		jsapiParams map[string]string
	)
	if in.Channel == PayChannelWx {
		resp, wxErr := uc.wxGateway.CreateOrder(ctx, WxCreateOrderInput{
			OutTradeNo: outTradeNo,
			Amount:     in.Amount,
			PayMode:    in.PayMode,
			OpenID:     in.OpenID,
		})
		if wxErr != nil {
			return nil, wxErr
		}
		prepayID = resp.PrepayID
		codeURL = resp.CodeURL
		jsapiParams = resp.JSAPIParams
	} else {
		resp, aliErr := uc.aliGateway.CreateOrder(ctx, AlipayCreateOrderInput{
			OutTradeNo: outTradeNo,
			Amount:     in.Amount,
		})
		if aliErr != nil {
			return nil, aliErr
		}
		prepayID = resp.TradeNo
		codeURL = resp.PayURL
		jsapiParams = resp.PayParams
	}

	if err = uc.paymentRepo.UpdatePrepay(ctx, outTradeNo, prepayID, codeURL, jsapiParams); err != nil {
		return nil, err
	}
	created, err := uc.paymentRepo.GetByUIDAndOutTradeNo(ctx, in.UID, outTradeNo)
	if err != nil {
		return nil, err
	}
	return &WalletRechargeOrder{
		OutTradeNo:  created.OutTradeNo,
		Channel:     created.Channel,
		Status:      created.Status,
		CodeURL:     created.CodeURL,
		PayURL:      created.CodeURL,
		JSAPIParams: created.JSAPIParams,
	}, nil
}

func validateRechargeInput(in CreateRechargeOrderInput) error {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.ClientReqID) == "" || in.Amount <= 0 {
		return ErrWalletInvalidArgument
	}
	if in.Channel != PayChannelWx && in.Channel != PayChannelAl {
		return ErrWalletInvalidArgument
	}
	if in.Channel == PayChannelWx {
		if in.PayMode != PayModeNative && in.PayMode != PayModeJSAPI {
			return ErrWalletInvalidArgument
		}
		if in.PayMode == PayModeJSAPI && strings.TrimSpace(in.OpenID) == "" {
			return ErrWalletInvalidArgument
		}
	}
	return nil
}

func (uc *WalletUsecase) newRechargeOutTradeNo(channel string) string {
	prefix := "RC"
	if channel == PayChannelWx {
		prefix = "RW"
	}
	if channel == PayChannelAl {
		prefix = "RA"
	}
	return fmt.Sprintf("%s%s%s", prefix, uc.nowFunc().Format("20060102150405"), strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:12])
}

func (uc *WalletUsecase) newBizOrderNo() string {
	return "RECHARGE_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
}
