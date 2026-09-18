package jwt

import (
	"testing"
	"time"
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
