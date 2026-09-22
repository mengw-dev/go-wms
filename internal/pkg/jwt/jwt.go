// Package jwt 负责签发和严格校验用户身份 Token。
package jwt

import (
	"errors"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims is the authenticated identity carried by a signed access token.
type Claims struct {
	UserID       int64  `json:"uid"`
	Username     string `json:"username"`
	TokenVersion int    `json:"ver"`
	TenantID     int64  `json:"tid"` // 多租户：0 表示平台/默认租户（旁路隔离）
	jwtlib.RegisteredClaims
}

// ErrInvalidToken reports a malformed, expired, or otherwise invalid token.
var ErrInvalidToken = errors.New("invalid token")

// Generate signs an access token for the supplied user and tenant identity.
func Generate(secret string, expire time.Duration, userID int64, username string, tokenVersion int, tenantID int64) (string, error) {
	claims := Claims{
		UserID:       userID,
		Username:     username,
		TokenVersion: tokenVersion,
		TenantID:     tenantID,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(expire)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			Issuer:    "gowms",
		},
	}
	return jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Parse verifies the signature and required claims before returning identity data.
func Parse(secret, tokenStr string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(_ *jwtlib.Token) (any, error) {
		return []byte(secret), nil
	}, jwtlib.WithValidMethods([]string{"HS256"}), jwtlib.WithIssuer("gowms"), jwtlib.WithExpirationRequired())
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID <= 0 || claims.TokenVersion <= 0 || claims.TenantID < 0 {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
