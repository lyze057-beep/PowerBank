package biz

import "github.com/google/wire"

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewGreeterUsecase,
	NewAuthUsecase,
	NewUserUsecase,
	NewPaymentUsecase,
	NewWalletUsecase,
	NewNotifyUsecase,
	NewDepositUsecase,
	NewOrderUsecase,
	NewChargerUsecase,
	NewSupportUsecase,
)
