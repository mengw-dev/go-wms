package service

import (
	"errors"
	"testing"

	"gowms/internal/modules/system/dto"
	"gowms/internal/pkg/errcode"
)

func TestUserConcurrentEditorsHaveOneWinner(t *testing.T) {
	s, _, ctx := userFixture(t)
	actor := newAdminActor(t, s, ctx)
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, nickname := range []string{"editor A", "editor B"} {
		go func() {
			<-start
			done <- s.UpdateUser(ctx, actor, 42, &dto.UserUpdateReq{ExpectedVersion: 1, Nickname: &nickname})
		}()
	}
	close(start)
	first, second := <-done, <-done
	if !((first == nil && errors.Is(second, errcode.StaleEdit)) || (second == nil && errors.Is(first, errcode.StaleEdit))) {
		t.Fatalf("edit results: %v / %v", first, second)
	}
	user, err := s.repo.GetUserByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if user.Version != 2 || user.TokenVersion != 1 || (user.Nickname != "editor A" && user.Nickname != "editor B") {
		t.Fatal("concurrent edit did not preserve revision/credential invariants")
	}
}
