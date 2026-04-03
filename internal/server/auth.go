package server

import (
	"context"
	"strings"

	paymentv1 "github.com/go-kratos/kratos-layout/api/payment/v1"
	userv1 "github.com/go-kratos/kratos-layout/api/user/v1"
	jwtmw "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
)

func needJWT(ctx context.Context, operation string) bool {
	if _, ok := jwtmw.FromContext(ctx); ok {
		return false
	}
	if strings.HasPrefix(operation, "/auth.v1.AuthService/") {
		if operation == OperationAuthServiceRegister || operation == OperationAuthServiceLogin {
			return false
		}
		return true
	}
	if strings.HasPrefix(operation, "/user.v1.UserService/") {
		if operation == userv1.OperationUserServiceRegister ||
			operation == userv1.OperationUserServiceLogin ||
			operation == userv1.OperationUserServiceMockLogin {
			return false
		}
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
