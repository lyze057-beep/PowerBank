package data

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

type wxPayGateway struct {
	cfg *conf.Payment_Wechat
	log *log.Helper
}

// NewWxPayGateway creates mock wechat payment gateway.
func NewWxPayGateway(c *conf.Payment, logger log.Logger) biz.WxPayGateway {
	var wc *conf.Payment_Wechat
	if c != nil {
		wc = c.Wechat
	}
	if wc == nil {
		wc = &conf.Payment_Wechat{
			MockEnabled: true,
			ApiV3Key:    "mock_api_v3_key",
		}
	}
	return &wxPayGateway{
		cfg: wc,
		log: log.NewHelper(log.With(logger, "module", "data/wxpay")),
	}
}

func (g *wxPayGateway) CreateOrder(_ context.Context, in biz.WxCreateOrderInput) (*biz.WxCreateOrderOutput, error) {
	if !g.cfg.MockEnabled {
		return nil, errors.New(501, "PAYMENT_UNIMPLEMENTED", "real wechat pay integration is not implemented yet")
	}

	prepayID := "mock_prepay_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	resp := &biz.WxCreateOrderOutput{
		PrepayID: prepayID,
	}
	switch in.PayMode {
	case biz.PayModeNative:
		resp.CodeURL = fmt.Sprintf("weixin://wxpay/bizpayurl?pr=%s", strings.ToUpper(uuid.NewString())[:8])
	case biz.PayModeJSAPI:
		nonceStr := strings.ReplaceAll(uuid.NewString(), "-", "")
		timeStamp := fmt.Sprintf("%d", time.Now().Unix())
		pkgStr := "prepay_id=" + prepayID
		payload := strings.Join([]string{
			g.cfg.AppId,
			timeStamp,
			nonceStr,
			pkgStr,
		}, "\n")
		signature := mockSign(g.cfg.ApiV3Key, payload)
		resp.JSAPIParams = map[string]string{
			"appId":     g.cfg.AppId,
			"timeStamp": timeStamp,
			"nonceStr":  nonceStr,
			"package":   pkgStr,
			"signType":  "MOCK-HMAC-SHA256",
			"paySign":   signature,
		}
	default:
		return nil, biz.ErrPaymentInvalidArgument
	}
	return resp, nil
}

func (g *wxPayGateway) VerifyNotify(_ context.Context, in biz.WxNotifyInput) error {
	if !g.cfg.MockEnabled {
		return errors.New(501, "PAYMENT_UNIMPLEMENTED", "real wechat sign verify is not implemented yet")
	}
	if strings.TrimSpace(in.Signature) == "" {
		return biz.ErrPaymentSignVerifyFail
	}
	signPayload := strings.Join([]string{in.Timestamp, in.Nonce, in.Body, ""}, "\n")
	expect := mockSign(g.cfg.ApiV3Key, signPayload)
	if !strings.EqualFold(expect, strings.TrimSpace(in.Signature)) {
		return biz.ErrPaymentSignVerifyFail
	}
	return nil
}

func (g *wxPayGateway) ParseNotify(_ context.Context, body string) (*biz.WxNotifyEvent, error) {
	type notifyPayload struct {
		EventID       string `json:"event_id"`
		EventType     string `json:"event_type"`
		OutTradeNo    string `json:"out_trade_no"`
		TradeState    string `json:"trade_state"`
		TransactionID string `json:"transaction_id"`
	}
	var payload notifyPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.EventID) == "" || strings.TrimSpace(payload.OutTradeNo) == "" {
		return nil, biz.ErrPaymentInvalidArgument
	}
	return &biz.WxNotifyEvent{
		EventID:       payload.EventID,
		EventType:     payload.EventType,
		OutTradeNo:    payload.OutTradeNo,
		TradeState:    payload.TradeState,
		TransactionID: payload.TransactionID,
		RawBody:       body,
	}, nil
}

func mockSign(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(payload))
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}
