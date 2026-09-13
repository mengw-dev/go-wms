package service

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	s := &Service{loginAttempts: make(map[string]loginAttempt)}
	key := "admin|127.0.0.1"
	for i := 0; i < maxLoginFailures; i++ {
		if !s.allowLogin(key) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		s.recordLoginFailure(key)
	}
	if s.allowLogin(key) {
		t.Fatal("attempt above limit should be blocked")
	}
	s.clearLoginFailures(key)
	if !s.allowLogin(key) {
		t.Fatal("successful login should clear failures")
	}
}

func TestLoginLimiterExpires(t *testing.T) {
	s := &Service{loginAttempts: map[string]loginAttempt{
		"admin|127.0.0.1": {Failures: maxLoginFailures, ResetAt: time.Now().Add(-time.Second)},
	}}
	if !s.allowLogin("admin|127.0.0.1") {
		t.Fatal("expired login window should be allowed")
	}
}
