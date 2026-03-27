package pkg

import (
	"context"
	"fmt"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	jwtmw "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
)

const (
	sessionKeyPattern = "session:user:%s:jti:%s"
)

// UserClaims carries user identity data inside JWT.
type UserClaims struct {
	UID string `json:"uid"`
	jwtv5.RegisteredClaims
}

// BuildJWTKeyFunc builds key parser used by kratos jwt middleware.
func BuildJWTKeyFunc(secret string) jwtv5.Keyfunc {
	return func(_ *jwtv5.Token) (interface{}, error) {
		return []byte(secret), nil
	}
}

// BuildSessionKey builds redis session key used by login-state validation.
func BuildSessionKey(uid, jti string) string {
	return fmt.Sprintf(sessionKeyPattern, uid, jti)
}

// ClaimsFromContext extracts UserClaims from current request context.
func ClaimsFromContext(ctx context.Context) (*UserClaims, error) {
	raw, ok := jwtmw.FromContext(ctx)
	if !ok {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "missing jwt claims")
	}
	claims, ok := raw.(*UserClaims)
	if !ok || claims == nil {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid jwt claims")
	}
	if claims.UID == "" {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "missing uid in jwt claims")
	}
	return claims, nil
}
