package service

import (
	"context"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/payment/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

// PaymentService is wechat payment service.
type PaymentService struct {
	v1.UnimplementedPaymentServiceServer

	uc *biz.PaymentUsecase
}

// NewPaymentService creates payment service.
func NewPaymentService(uc *biz.PaymentUsecase) *PaymentService {
	return &PaymentService{uc: uc}
}

// CreateWxPayOrder creates wechat payment order.
func (s *PaymentService) CreateWxPayOrder(ctx context.Context, req *v1.CreateWxPayOrderRequest) (*v1.CreateWxPayOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.CreateWxPayOrder(ctx, biz.CreateWxPayOrderInput{
		UID:         claims.UID,
		PayMode:     biz.PayMode(req.GetPayMode()),
		BizType:     biz.BizType(req.GetBizType()),
		BizOrderNo:  req.GetBizOrderNo(),
		Amount:      req.GetAmount(),
		ClientReqID: req.GetClientReqId(),
		OpenID:      req.GetOpenid(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.CreateWxPayOrderReply{
		OutTradeNo:  order.OutTradeNo,
		Status:      v1.PayStatus(order.Status),
		CodeUrl:     order.CodeURL,
		JsapiParams: order.JSAPIParams,
	}, nil
}

// QueryPayOrder queries payment order with uid ownership.
func (s *PaymentService) QueryPayOrder(ctx context.Context, req *v1.QueryPayOrderRequest) (*v1.QueryPayOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.QueryPayOrder(ctx, claims.UID, req.GetOutTradeNo())
	if err != nil {
		return nil, err
	}
	return &v1.QueryPayOrderReply{
		OutTradeNo:    order.OutTradeNo,
		BizType:       v1.BizType(order.BizType),
		BizOrderNo:    order.BizOrderNo,
		Amount:        order.Amount,
		PayMode:       v1.PayMode(order.PayMode),
		Status:        v1.PayStatus(order.Status),
		TransactionId: order.TransactionID,
	}, nil
}

// WxPayNotify handles wechat callback, no JWT required.
func (s *PaymentService) WxPayNotify(ctx context.Context, req *v1.WxPayNotifyRequest) (*v1.WxPayNotifyReply, error) {
	processed, err := s.uc.HandleWxNotify(ctx, biz.WxNotifyInput{
		Body:      req.GetBody(),
		Timestamp: req.GetTimestamp(),
		Nonce:     req.GetNonce(),
		Signature: req.GetSignature(),
		Serial:    req.GetSerial(),
	})
	if err != nil {
		return nil, err
	}
	msg := "success"
	if !processed {
		msg = "duplicated_or_locked"
	}
	return &v1.WxPayNotifyReply{
		Code:    "SUCCESS",
		Message: msg,
	}, nil
}

// AlipayNotify handles alipay callback, no JWT required.
func (s *PaymentService) AlipayNotify(ctx context.Context, req *v1.AlipayNotifyRequest) (*v1.AlipayNotifyReply, error) {
	processed, err := s.uc.HandleAlipayNotify(ctx, biz.AlipayNotifyInput{
		Body:      req.GetBody(),
		Signature: req.GetSignature(),
		NotifyID:  req.GetNotifyId(),
		Timestamp: req.GetTimestamp(),
	})
	if err != nil {
		return nil, err
	}
	msg := "success"
	if !processed {
		msg = "duplicated_or_locked"
	}
	return &v1.AlipayNotifyReply{
		Code:    "SUCCESS",
		Message: msg,
	}, nil
}

// AlipayHeartbeat checks alipay gateway health.
func (s *PaymentService) AlipayHeartbeat(ctx context.Context, _ *v1.AlipayHeartbeatRequest) (*v1.AlipayHeartbeatReply, error) {
	if err := s.uc.AlipayHeartbeat(ctx); err != nil {
		return nil, err
	}
	return &v1.AlipayHeartbeatReply{
		Ok:        true,
		Message:   "ok",
		Timestamp: time.Now().Unix(),
	}, nil
}
