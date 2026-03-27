package service

import (
	"context"

	walletv1 "github.com/go-kratos/kratos-layout/api/wallet/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// WalletService is wallet/recharge service.
type WalletService struct {
	walletv1.UnimplementedWalletServiceServer

	uc *biz.WalletUsecase
}

// NewWalletService creates wallet service.
func NewWalletService(uc *biz.WalletUsecase) *WalletService {
	return &WalletService{uc: uc}
}

// GetBalance returns current user wallet balance.
func (s *WalletService) GetBalance(ctx context.Context, _ *walletv1.GetBalanceRequest) (*walletv1.GetBalanceReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	balance, err := s.uc.GetBalance(ctx, claims.UID)
	if err != nil {
		return nil, err
	}
	return &walletv1.GetBalanceReply{
		Uid:     claims.UID,
		Balance: balance,
	}, nil
}

// CreateRechargeOrder creates recharge order by channel (wechat/alipay).
func (s *WalletService) CreateRechargeOrder(ctx context.Context, req *walletv1.CreateRechargeOrderRequest) (*walletv1.CreateRechargeOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}

	input := biz.CreateRechargeOrderInput{
		UID:         claims.UID,
		Channel:     toBizChannel(req.GetChannel()),
		PayMode:     toBizPayMode(req.GetPayMode()),
		Amount:      req.GetAmount(),
		ClientReqID: req.GetClientReqId(),
		OpenID:      req.GetOpenid(),
	}
	order, err := s.uc.CreateRechargeOrder(ctx, input)
	if err != nil {
		return nil, err
	}

	return &walletv1.CreateRechargeOrderReply{
		OutTradeNo:  order.OutTradeNo,
		Channel:     toProtoChannel(order.Channel),
		Status:      walletv1.RechargeOrderStatus(order.Status),
		CodeUrl:     order.CodeURL,
		PayUrl:      order.PayURL,
		JsapiParams: order.JSAPIParams,
	}, nil
}

// CreateAlipayRechargeOrder creates recharge order with alipay channel only.
func (s *WalletService) CreateAlipayRechargeOrder(ctx context.Context, req *walletv1.CreateAlipayRechargeOrderRequest) (*walletv1.CreateRechargeOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.CreateRechargeOrder(ctx, biz.CreateRechargeOrderInput{
		UID:         claims.UID,
		Channel:     biz.PayChannelAl,
		PayMode:     biz.PayModeNative,
		Amount:      req.GetAmount(),
		ClientReqID: req.GetClientReqId(),
	})
	if err != nil {
		return nil, err
	}
	return &walletv1.CreateRechargeOrderReply{
		OutTradeNo:  order.OutTradeNo,
		Channel:     walletv1.RechargeChannel_RECHARGE_CHANNEL_ALIPAY,
		Status:      walletv1.RechargeOrderStatus(order.Status),
		PayUrl:      order.PayURL,
		CodeUrl:     order.CodeURL,
		JsapiParams: order.JSAPIParams,
	}, nil
}

func toBizChannel(channel walletv1.RechargeChannel) string {
	switch channel {
	case walletv1.RechargeChannel_RECHARGE_CHANNEL_WECHAT:
		return biz.PayChannelWx
	case walletv1.RechargeChannel_RECHARGE_CHANNEL_ALIPAY:
		return biz.PayChannelAl
	default:
		return ""
	}
}

func toBizPayMode(mode walletv1.RechargePayMode) biz.PayMode {
	switch mode {
	case walletv1.RechargePayMode_RECHARGE_PAY_MODE_NATIVE:
		return biz.PayModeNative
	case walletv1.RechargePayMode_RECHARGE_PAY_MODE_JSAPI:
		return biz.PayModeJSAPI
	default:
		return biz.PayModeUnspecified
	}
}

func toProtoChannel(channel string) walletv1.RechargeChannel {
	switch channel {
	case biz.PayChannelWx:
		return walletv1.RechargeChannel_RECHARGE_CHANNEL_WECHAT
	case biz.PayChannelAl:
		return walletv1.RechargeChannel_RECHARGE_CHANNEL_ALIPAY
	default:
		return walletv1.RechargeChannel_RECHARGE_CHANNEL_UNSPECIFIED
	}
}
