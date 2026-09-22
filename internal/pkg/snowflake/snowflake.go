// Package snowflake 提供进程内并发安全的雪花 ID 生成器。
package snowflake

import (
	"fmt"
	"sync"
	"time"
)

// 简化雪花算法：41 位毫秒时间戳 + 10 位节点 + 12 位序列。
const (
	epoch     int64 = 1704038400000 // 北京时间 2024-01-01 00:00:00；保持既有 ID 的纪元不变
	nodeBits  uint  = 10
	seqBits   uint  = 12
	maxNode   int64 = -1 ^ (-1 << nodeBits)
	maxSeq    int64 = -1 ^ (-1 << seqBits)
	nodeShift       = seqBits
	tsShift         = nodeBits + seqBits
)

type generator struct {
	mu     sync.Mutex
	nodeID int64
	lastTS int64
	seq    int64
}

var defaultGenerator = generator{nodeID: 1}

// Init 在进程启动时设置节点号，多实例必须使用不同节点号。
// 不清空序列和逻辑时间，避免重复初始化导致 ID 重用。
func Init(node int64) error {
	if node < 0 || node > maxNode {
		return fmt.Errorf("snowflake node must be between 0 and %d", maxNode)
	}
	defaultGenerator.mu.Lock()
	defer defaultGenerator.mu.Unlock()
	defaultGenerator.nodeID = node
	return nil
}

// Next 在当前进程生命周期内生成递增 ID。跨实例依赖节点号唯一；
// 重启后的时钟不能早于上次使用的逻辑时间，数据库主键仍需兜底。
func Next() int64 {
	return defaultGenerator.nextAt(time.Now().UnixMilli())
}

func (g *generator) nextAt(now int64) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	// 时钟回拨时继续使用上次逻辑时间，不能重置序列生成已用过的 ID。
	now = max(now, g.lastTS)
	if now == g.lastTS {
		g.seq = (g.seq + 1) & maxSeq
		if g.seq == 0 {
			// 序列耗尽时推进一毫秒，避免在持锁状态下忙等系统时钟。
			now++
		}
	} else {
		g.seq = 0
	}
	g.lastTS = now
	return ((now - epoch) << tsShift) | (g.nodeID << nodeShift) | g.seq
}
