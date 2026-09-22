package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	s := &Service{loginAttempts: make(map[string]loginAttempt)}
	key := "admin|127.0.0.1"
	for i := 0; i < maxLoginAttempts; i++ {
		if !s.beginLoginAttempt(key) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if s.beginLoginAttempt(key) {
		t.Fatal("attempt above limit should be blocked")
	}
	s.clearLoginAttempts(key)
	if !s.beginLoginAttempt(key) {
		t.Fatal("successful login should clear failures")
	}
}

func TestLoginLimiterExpires(t *testing.T) {
	s := &Service{loginAttempts: map[string]loginAttempt{
		"admin|127.0.0.1": {Attempts: maxLoginAttempts, ResetAt: time.Now().Add(-time.Second)},
	}}
	if !s.beginLoginAttempt("admin|127.0.0.1") {
		t.Fatal("expired login window should be allowed")
	}
}

func TestLoginLimiterReservesConcurrentAttempts(t *testing.T) {
	s := &Service{loginAttempts: make(map[string]loginAttempt)}
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if s.beginLoginAttempt("tester|127.0.0.1") {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != maxLoginAttempts {
		t.Fatalf("accepted=%d want=%d", accepted.Load(), maxLoginAttempts)
	}
}

func TestLoginLimiterBoundsMemoryAndSweepsOtherKeys(t *testing.T) {
	s := &Service{loginAttempts: make(map[string]loginAttempt), loginLastSweep: time.Now()}
	for i := range maxTrackedLogins {
		if !s.beginLoginAttempt(fmt.Sprintf("user-%d|127.0.0.1", i)) {
			t.Fatalf("rejected before capacity at %d", i)
		}
	}
	if s.beginLoginAttempt("new-user|127.0.0.1") || len(s.loginAttempts) != maxTrackedLogins {
		t.Fatal("new username bypassed the memory limit")
	}
	if !s.beginLoginAttempt("user-0|127.0.0.1") {
		t.Fatal("capacity limit blocked an existing tracked account")
	}
	for key, item := range s.loginAttempts {
		item.ResetAt = time.Now().Add(-time.Second)
		s.loginAttempts[key] = item
	}
	s.loginLastSweep = time.Now().Add(-loginSweepInterval)
	if !s.beginLoginAttempt("new-user|127.0.0.1") || len(s.loginAttempts) != 1 {
		t.Fatalf("expired accounts not reclaimed: size=%d", len(s.loginAttempts))
	}
}
