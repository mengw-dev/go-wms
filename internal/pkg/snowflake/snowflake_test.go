package snowflake

import (
	"sync"
	"testing"
)

func TestClockRollbackDoesNotReuseIDs(t *testing.T) {
	g := generator{nodeID: 7}
	var previous int64
	for _, timestamp := range []int64{epoch + 100, epoch + 101, epoch + 90, epoch + 100, epoch + 102} {
		id := g.nextAt(timestamp)
		if id <= previous {
			t.Fatalf("ID did not increase: previous=%d current=%d", previous, id)
		}
		if node := (id >> nodeShift) & maxNode; node != 7 {
			t.Fatalf("node=%d", node)
		}
		previous = id
	}
}

func TestConcurrentGenerationAtSameMillisecond(t *testing.T) {
	var g generator // 节点 0 合法。
	const workers, perWorker = 12, 1000
	ids := make(chan int64, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				ids <- g.nextAt(epoch + 100)
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[int64]struct{}, workers*perWorker)
	for id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID: %d", id)
		}
		seen[id] = struct{}{}
	}
	// 12000 个 ID 用完三个毫秒的序列空间，不需要依赖系统时钟前进。
	if g.lastTS != epoch+102 {
		t.Fatalf("logical timestamp=%d", g.lastTS)
	}
}

func TestDifferentNodesAndInitialization(t *testing.T) {
	a, b := generator{nodeID: 0}, generator{nodeID: maxNode}
	if a.nextAt(epoch+100) == b.nextAt(epoch+100) {
		t.Fatal("different nodes generated same ID")
	}
	for _, node := range []int64{-1, maxNode + 1} {
		if err := Init(node); err == nil {
			t.Fatalf("accepted node %d", node)
		}
	}
	if err := Init(0); err != nil {
		t.Fatal(err)
	}
	first := Next()
	if (first>>nodeShift)&maxNode != 0 {
		t.Fatal("node zero was ignored")
	}
	if err := Init(0); err != nil {
		t.Fatal(err)
	}
	if next := Next(); next <= first {
		t.Fatalf("initialization reset ID state: %d <= %d", next, first)
	}
}
