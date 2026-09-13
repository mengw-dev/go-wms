package errcode

import (
	"errors"
	"fmt"
	"testing"
)

func TestFromRecognizesWrappedBusinessError(t *testing.T) {
	err := fmt.Errorf("service: %w", OrderNotFound)
	got := From(err)
	if got.Code != OrderNotFound.Code {
		t.Fatalf("got code %d, want %d", got.Code, OrderNotFound.Code)
	}
}

func TestIsConflictRecognizesWrappedError(t *testing.T) {
	err := fmt.Errorf("transaction: %w", Conflict)
	if !IsConflict(err) {
		t.Fatal("wrapped conflict error should be retryable")
	}
}

func TestWrapPreservesCause(t *testing.T) {
	cause := errors.New("deadlock")
	err := Wrap(cause, Conflict)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped error lost its cause")
	}
	if !IsConflict(err) {
		t.Fatal("wrapped conflict should remain retryable")
	}
}
