package service

import (
	"context"
	"errors"
	"testing"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/pkg/errcode"
)

func TestLocationBatchRejectsUnsafeRangesBeforeDatabase(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, test := range []struct {
		name                     string
		from, to, colFrom, colTo int
		want                     error
	}{
		{"zero", 0, 1, 1, 1, errcode.ParamError},
		{"negative", -maxInt, maxInt, 1, 1, errcode.ParamError},
		{"reversed", 2, 1, 1, 1, errcode.ParamError},
		{"1001", 1, 1001, 1, 1, errcode.LocationBatchLimit},
		{"product", 1, 32, 1, 32, errcode.LocationBatchLimit},
		{"huge dimensions", 1, 2147483647, 1, 2147483647, errcode.LocationBatchLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			// nil dependencies make any database access fail this regression.
			s := &Service{}
			_, err := s.BatchCreateLocations(context.Background(), &dto.LocationBatchReq{RowFrom: test.from, RowTo: test.to, ColFrom: test.colFrom, ColTo: test.colTo})
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
		})
	}
	if maxInt > 2147483647 {
		if _, err := (&Service{}).BatchCreateLocations(context.Background(), &dto.LocationBatchReq{RowFrom: maxInt, RowTo: maxInt, ColFrom: 1, ColTo: 1}); !errors.Is(err, errcode.ParamError) {
			t.Fatalf("MaxInt endpoint: %v", err)
		}
	}
}

func TestLocationBatchBoundariesAndReplay(t *testing.T) {
	s, _, _, db := newDeleteTestService(t)
	warehouseID, _, _ := createDeleteFixtures(t, db)
	request := dto.LocationBatchReq{WarehouseID: warehouseID, Zone: "LIMIT", RowFrom: 1, RowTo: 10, ColFrom: 1, ColTo: 100}
	if count, err := s.BatchCreateLocations(context.Background(), &request); err != nil || count != 1000 {
		t.Fatalf("1000 boundary count=%d err=%v", count, err)
	}
	if count, err := s.BatchCreateLocations(context.Background(), &request); err != nil || count != 0 {
		t.Fatalf("replay count=%d err=%v", count, err)
	}
	request = dto.LocationBatchReq{WarehouseID: warehouseID, Zone: "END", RowFrom: 2147483647, RowTo: 2147483647, ColFrom: 2147483647, ColTo: 2147483647}
	if count, err := s.BatchCreateLocations(context.Background(), &request); err != nil || count != 1 {
		t.Fatalf("maximum endpoint count=%d err=%v", count, err)
	}
}
