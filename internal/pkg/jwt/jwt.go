package jwt

import (
	"errors"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID       int64  `json:"uid"`
	Username     string `json:"username"`
	TokenVersion int    `json:"ver"`
	jwtlib.RegisteredClaims
}

var ErrInvalidToken = errors.New("invalid token")

func Generate(secret string, expire time.Duration, userID int64, username string, tokenVersion int) (string, error) {
	claims := Claims{
		UserID:       userID,
		Username:     username,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(expire)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			Issuer:    "gowms",
		},
	}
	return jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func Parse(secret, tokenStr string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(t *jwtlib.Token) (any, error) {
		return []byte(secret), nil
	}, jwtlib.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
