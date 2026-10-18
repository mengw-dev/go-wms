package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
)

func TestPutawayAndDisableLocationSerialize(t *testing.T) {
	for _, disableFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "disable commits before putaway", false: "putaway locks before disable"}[disableFirst], func(t *testing.T) {
			st := newPutawayIdemStack(t)
			ctx, cancel := context.WithTimeout(st.ctx, 8*time.Second)
			defer cancel()
			arrived, release := make(chan struct{}), make(chan struct{})
			var once, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			const name = "p1:putaway_location_gate"
			if err := st.db.Callback().Query().After("gorm:query").Register(name, func(q *gorm.DB) {
				if q.Statement.Table != "wms_location" || q.Error != nil {
					return
				}
				_, locked := q.Statement.Clauses["FOR"]
				if locked == disableFirst {
					return
				}
				once.Do(func() {
					close(arrived)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.db.Callback().Query().Remove(name) }()
			var workers sync.WaitGroup
			defer func() { unblock(); cancel(); workers.Wait() }()
			putaway := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				putaway <- st.s.Putaway(ctx, st.task.ID, st.location, 4, "test", "location-race")
			}()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("putaway did not reach location query")
			}
			basic := st.s.basic.(*basicservice.Service)
			if disableFirst {
				if err := basic.UpdateLocationStatus(ctx, st.location, basicmodel.LocationStatusDisabled); err != nil {
					t.Fatal(err)
				}
				unblock()
				select {
				case err := <-putaway:
					if !errors.Is(err, errcode.LocationDisabled) {
						t.Fatalf("new putaway must reject committed disable: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("putaway timeout")
				}
				if inv := st.inventory(t); inv.StockQuantity != 0 {
					t.Fatalf("disabled location gained stock: %+v", inv)
				}
				if st.transCount(t) != 0 {
					t.Fatal("rejected putaway wrote inventory transaction")
				}
				var records int64
				if err := st.db.WithContext(st.ctx).Model(&idempotency.Record{}).Count(&records).Error; err != nil {
					t.Fatal(err)
				}
				if records != 0 {
					t.Fatal("rejected putaway recorded success")
				}
			} else {
				disabled := make(chan error, 1)
				workers.Add(1)
				go func() {
					defer workers.Done()
					disabled <- basic.UpdateLocationStatus(ctx, st.location, basicmodel.LocationStatusDisabled)
				}()
				select {
				case err := <-disabled:
					t.Fatalf("disable bypassed held location lock: %v", err)
				case <-time.After(100 * time.Millisecond):
				}
				unblock()
				for _, done := range []chan error{putaway, disabled} {
					select {
					case err := <-done:
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal("operation timeout")
					}
				}
				if inv := st.inventory(t); inv.StockQuantity != 4 {
					t.Fatalf("stock=%+v", inv)
				}
				if err := st.s.Putaway(ctx, st.task.ID, st.location, 4, "test", "location-race"); err != nil {
					t.Fatalf("successful key must replay after disable: %v", err)
				}
			}
			var location basicmodel.Location
			if err := st.db.WithContext(st.ctx).First(&location, st.location).Error; err != nil {
				t.Fatal(err)
			}
			if location.Status != basicmodel.LocationStatusDisabled {
				t.Fatalf("disable was overwritten: status=%d", location.Status)
			}
			task, err := st.s.taskAPI.Get(st.ctx, st.task.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := 4
			if disableFirst {
				want = 0
			}
			if task.DoneQty != want {
				t.Fatalf("task progress=%d want=%d", task.DoneQty, want)
			}
		})
	}
}
