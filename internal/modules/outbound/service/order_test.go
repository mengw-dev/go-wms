package service

import (
	"context"
	"errors"
	"testing"

	"gowms/internal/pkg/errcode"
)

func TestBatchOperKeepsPartialSuccessAndHidesDatabaseErrors(t *testing.T) {
	s := &Service{}
	var called []int64
	result := s.batchOper(context.Background(), []int64{1, 2, 3}, func(_ context.Context, id int64) error {
		called = append(called, id)
		switch id {
		case 1:
			return errors.New("database connection details must not reach the client")
		case 2:
			return errcode.ShipOrderStatusWrong
		default:
			return nil
		}
	})
	if len(called) != 3 || result.Success != 1 || result.Fail != 2 || len(result.Errors) != 2 {
		t.Fatalf("unexpected batch result: %+v, called=%v", result, called)
	}
	if result.Errors[0].Msg != errcode.Internal.Msg || result.Errors[1].Msg != errcode.ShipOrderStatusWrong.Msg {
		t.Fatalf("unexpected client errors: %+v", result.Errors)
	}
}
