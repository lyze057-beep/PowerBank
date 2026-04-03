package server

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/pkg"

	"github.com/go-kratos/kratos/v2/middleware"
	jwtmw "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/transport"
)

// DebugAuth is a middleware that injects user claims from x-debug-uid header.
func DebugAuth() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			var uid string
			if tr, ok := transport.FromServerContext(ctx); ok {
				// Method A: Key is "x-debug-uid", Value is the UID (Best Practice)
				uid = tr.RequestHeader().Get("x-debug-uid")

				// Optional Method B: Key is "x-debug-<UID>" (For your screenshots' compatibility)
				if uid == "" {
					for _, key := range tr.RequestHeader().Keys() {
						lowerKey := strings.ToLower(key)
						if strings.HasPrefix(lowerKey, "x-debug-") && lowerKey != "x-debug-uid" {
							uid = key[8:] // Extract UID from key like x-debug-u1001
							break
						}
					}
				}
			}

			if uid != "" {
				// Inject claims into context so that Service can retrieve it using pkg.ClaimsFromContext.
				claims := &pkg.UserClaims{
					UID: uid,
				}
				ctx = jwtmw.NewContext(ctx, claims)
			}
			return handler(ctx, req)
		}
	}
}
