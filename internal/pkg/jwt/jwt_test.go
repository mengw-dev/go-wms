package jwt

import (
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndParseTokenVersion(t *testing.T) {
	token, err := Generate("test-secret-at-least-32-characters", time.Hour, 42, "tester", 7, 9)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := Parse("test-secret-at-least-32-characters", token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "tester" || claims.TokenVersion != 7 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.TenantID != 9 {
		t.Fatalf("unexpected tenant id: %d", claims.TenantID)
	}
}

func TestParseRejectsInvalidClaims(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Claims)
	}{
		{"missing expiry", func(c *Claims) { c.ExpiresAt = nil }},
		{"wrong issuer", func(c *Claims) { c.Issuer = "another-service" }},
		{"missing user", func(c *Claims) { c.UserID = 0 }},
		{"missing version", func(c *Claims) { c.TokenVersion = 0 }},
		{"negative tenant", func(c *Claims) { c.TenantID = -1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			claims := Claims{UserID: 1, TokenVersion: 1, RegisteredClaims: jwtlib.RegisteredClaims{Issuer: "gowms", ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour))}}
			tt.change(&claims)
			token, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse("test-secret", token); err == nil {
				t.Fatal("accepted invalid claims")
			}
		})
	}
}

func TestParseRejectsWrongSecretAndExpiredToken(t *testing.T) {
	token, err := Generate("test-secret-at-least-32-characters", -time.Minute, 1, "admin", 1, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := Parse("test-secret-at-least-32-characters", token); err == nil {
		t.Fatal("expired token should be rejected")
	}
	fresh, err := Generate("test-secret-at-least-32-characters", time.Hour, 1, "admin", 1, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := Parse("another-secret-at-least-32-characters", fresh); err == nil {
		t.Fatal("wrong secret should be rejected")
	}
}
