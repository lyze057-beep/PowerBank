package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

type alipayGateway struct {
	cfg *conf.Payment_Alipay
	log *log.Helper
}

// NewAlipayGateway creates mock alipay gateway.
func NewAlipayGateway(c *conf.Payment, logger log.Logger) biz.AlipayGateway {
	var ac *conf.Payment_Alipay
	if c != nil {
		ac = c.Alipay
	}
	if ac == nil {
		ac = &conf.Payment_Alipay{
			MockEnabled: true,
		}
	}
	return &alipayGateway{
		cfg: ac,
		log: log.NewHelper(log.With(logger, "module", "data/alipay")),
	}
}

func (g *alipayGateway) CreateOrder(_ context.Context, in biz.AlipayCreateOrderInput) (*biz.AlipayCreateOrderOutput, error) {
	if !g.cfg.MockEnabled {
		return nil, errors.New(501, "PAYMENT_UNIMPLEMENTED", "real alipay integration is not implemented yet")
	}
	tradeNo := "mock_ali_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	payURL := fmt.Sprintf("https://openapi.alipaydev.com/gateway.do?out_trade_no=%s&amount=%d", in.OutTradeNo, in.Amount)
	return &biz.AlipayCreateOrderOutput{
		TradeNo: tradeNo,
		PayURL:  payURL,
		PayParams: map[string]string{
			"trade_no":     tradeNo,
			"out_trade_no": in.OutTradeNo,
			"total_amount": fmt.Sprintf("%.2f", float64(in.Amount)/100),
			"subject":      "wallet recharge",
			"sign_type":    "MOCK-RSA2",
		},
	}, nil
}

func (g *alipayGateway) VerifyNotify(_ context.Context, in biz.AlipayNotifyInput) error {
	if !g.cfg.MockEnabled {
		return errors.New(501, "PAYMENT_UNIMPLEMENTED", "real alipay verify is not implemented yet")
	}
	if strings.EqualFold(in.Signature, "MOCK_SIGN") {
		return nil
	}
	if strings.TrimSpace(in.Signature) == "" || strings.TrimSpace(in.NotifyID) == "" {
		return biz.ErrPaymentSignVerifyFail
	}
	expect := mockAliSign(g.cfg.AppId, in.NotifyID, in.Body)
	if !strings.EqualFold(expect, strings.TrimSpace(in.Signature)) {
		return biz.ErrPaymentSignVerifyFail
	}
	return nil
}

func (g *alipayGateway) ParseNotify(_ context.Context, body string) (*biz.AlipayNotifyEvent, error) {
	type notifyPayload struct {
		NotifyID    string `json:"notify_id"`
		OutTradeNo  string `json:"out_trade_no"`
		TradeStatus string `json:"trade_status"`
		TradeNo     string `json:"trade_no"`
	}
	var payload notifyPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.NotifyID) == "" || strings.TrimSpace(payload.OutTradeNo) == "" {
		return nil, biz.ErrPaymentInvalidArgument
	}
	return &biz.AlipayNotifyEvent{
		EventID:     payload.NotifyID,
		OutTradeNo:  payload.OutTradeNo,
		TradeStatus: payload.TradeStatus,
		TradeNo:     payload.TradeNo,
		RawBody:     body,
	}, nil
}

func (g *alipayGateway) Heartbeat(_ context.Context) error {
	if !g.cfg.MockEnabled {
		return errors.New(501, "PAYMENT_UNIMPLEMENTED", "real alipay heartbeat is not implemented yet")
	}
	// Mock heartbeat: app id is required to consider gateway healthy.
	if strings.TrimSpace(g.cfg.AppId) == "" {
		return errors.New(500, "ALIPAY_HEARTBEAT_FAILED", "missing alipay app id")
	}
	return nil
}

func mockAliSign(appID, notifyID, body string) string {
	return strings.ToUpper(fmt.Sprintf("ALI-%s-%s-%d", appID, notifyID, len(body)))
}
