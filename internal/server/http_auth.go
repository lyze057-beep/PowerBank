package server

import (
	"context"

	"github.com/go-kratos/kratos-layout/internal/service"

	"github.com/go-kratos/kratos/v2/transport/http"
)

const (
	OperationAuthServiceRegister     = "/auth.v1.AuthService/Register"
	OperationAuthServiceLogin        = "/auth.v1.AuthService/Login"
	OperationAuthServiceLogout       = "/auth.v1.AuthService/Logout"
	OperationAuthServiceRefreshToken = "/auth.v1.AuthService/RefreshToken"
)

func registerAuthHTTPServer(srv *http.Server, auth *service.AuthService) {
	r := srv.Route("/")

	r.POST("/v1/auth/register", func(ctx http.Context) error {
		var in service.RegisterRequest
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		http.SetOperation(ctx, OperationAuthServiceRegister)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return auth.Register(ctx, req.(*service.RegisterRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		return ctx.Result(200, out)
	})

	r.POST("/v1/auth/login", func(ctx http.Context) error {
		var in service.LoginRequest
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		http.SetOperation(ctx, OperationAuthServiceLogin)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return auth.Login(ctx, req.(*service.LoginRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		return ctx.Result(200, out)
	})

	r.POST("/v1/auth/logout", func(ctx http.Context) error {
		http.SetOperation(ctx, OperationAuthServiceLogout)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return auth.Logout(ctx, req.(*service.LogoutRequest))
		})
		out, err := h(ctx, &service.LogoutRequest{})
		if err != nil {
			return err
		}
		return ctx.Result(200, out)
	})

	r.POST("/v1/auth/refresh", func(ctx http.Context) error {
		http.SetOperation(ctx, OperationAuthServiceRefreshToken)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return auth.RefreshToken(ctx, req.(*service.RefreshTokenRequest))
		})
		out, err := h(ctx, &service.RefreshTokenRequest{})
		if err != nil {
			return err
		}
		return ctx.Result(200, out)
	})
}
