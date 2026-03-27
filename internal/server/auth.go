package server

import (
	"context"
	"strings"

	paymentv1 "github.com/go-kratos/kratos-layout/api/payment/v1"
)

func needJWT(_ context.Context, operation string) bool {
	if strings.HasPrefix(operation, "/user.v1.UserService/") {
		return true
	}
	if strings.HasPrefix(operation, "/payment.v1.PaymentService/") && operation != paymentv1.OperationPaymentServiceWxPayNotify {
		if operation == paymentv1.OperationPaymentServiceAlipayNotify || operation == paymentv1.OperationPaymentServiceAlipayHeartbeat {
			return false
		}
		return true
	}
	if strings.HasPrefix(operation, "/wallet.v1.WalletService/") {
		return true
	}
	if strings.HasPrefix(operation, "/notify.v1.NotifyService/") {
		return true
	}
	if strings.HasPrefix(operation, "/support.v1.SupportService/") {
		return true
	}
	return false
}
