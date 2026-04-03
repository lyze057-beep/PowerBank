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
	"github.com/go-kratos/kratos/v2/transport/http"
	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, jwtConf *conf.JWT, greeter *service.GreeterService, auth *service.AuthService, user *service.UserService, payment *service.PaymentService, wallet *service.WalletService, notify *service.NotifyService, support *service.SupportService, logger log.Logger) *http.Server {
	secret := "secret"
	if jwtConf != nil && jwtConf.Key != "" {
		secret = jwtConf.Key
	}
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			DebugAuth(),
			selector.Server(
				jwt.Server(
					pkg.BuildJWTKeyFunc(secret),
					jwt.WithSigningMethod(jwtv5.SigningMethodHS256),
					jwt.WithClaims(func() jwtv5.Claims { return &pkg.UserClaims{} }),
				),
			).Match(needJWT).Build(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	v1.RegisterGreeterHTTPServer(srv, greeter)
	userv1.RegisterUserServiceHTTPServer(srv, user)
	paymentv1.RegisterPaymentServiceHTTPServer(srv, payment)
	walletv1.RegisterWalletServiceHTTPServer(srv, wallet)
	notifyv1.RegisterNotifyServiceHTTPServer(srv, notify)
	supportv1.RegisterSupportServiceHTTPServer(srv, support)
	registerAuthHTTPServer(srv, auth)
	return srv
}
