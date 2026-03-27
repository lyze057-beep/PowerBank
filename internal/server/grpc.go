package server

import (
	v1 "github.com/go-kratos/kratos-layout/api/helloworld/v1"
	notifyv1 "github.com/go-kratos/kratos-layout/api/notify/v1"
	paymentv1 "github.com/go-kratos/kratos-layout/api/payment/v1"
	supportv1 "github.com/go-kratos/kratos-layout/api/support/v1"
	userv1 "github.com/go-kratos/kratos-layout/api/user/v1"
	walletv1 "github.com/go-kratos/kratos-layout/api/wallet/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/pkg"
	"github.com/go-kratos/kratos-layout/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server, jwtConf *conf.JWT, greeter *service.GreeterService, user *service.UserService, payment *service.PaymentService, wallet *service.WalletService, notify *service.NotifyService, support *service.SupportService, logger log.Logger) *grpc.Server {
	secret := "secret"
	if jwtConf != nil && jwtConf.Key != "" {
		secret = jwtConf.Key
	}
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			selector.Server(
				jwt.Server(
					pkg.BuildJWTKeyFunc(secret),
					jwt.WithSigningMethod(jwtv5.SigningMethodHS256),
					jwt.WithClaims(func() jwtv5.Claims { return &pkg.UserClaims{} }),
				),
			).Match(needJWT).Build(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	v1.RegisterGreeterServer(srv, greeter)
	userv1.RegisterUserServiceServer(srv, user)
	paymentv1.RegisterPaymentServiceServer(srv, payment)
	walletv1.RegisterWalletServiceServer(srv, wallet)
	notifyv1.RegisterNotifyServiceServer(srv, notify)
	supportv1.RegisterSupportServiceServer(srv, support)
	return srv
}
