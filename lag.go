// Package lagcomp 是延迟补偿：固定容量的快照历史、按 RTT 回滚取快照、判定射击是否命中。
package lagcomp

import (
	"errors"
	"math"
)

// MaxRewindMs 是允许回滚的最大时长。
const MaxRewindMs int64 = 500

// RetainMs 是服务器保留的录像时长。
const RetainMs int64 = 2000

// Capacity 是快照历史的容量上限。
const Capacity = 512

// HitRadius 是命中半径，米。
const HitRadius = 8.0

// ErrTooOld 表示要回滚到的时刻已经不在历史里。
var ErrTooOld = errors.New("lagcomp: 该时刻的快照已经不在历史里")

// Vec 是二维坐标。
type Vec struct {
	X, Y float64
}

// Snapshot 是某一时刻的位置表。
type Snapshot struct {
	AtMs      int64
	Positions map[string]Vec
}

// History 是服务器快照历史。
type History struct {
	shooter Vec
	ring    [Capacity]Snapshot
	head    int
	size    int
	scanned int
}

// NewHistory 建一份历史，shooter 是射手所在位置。
func NewHistory(shooter Vec) *History {
	return &History{shooter: shooter}
}

// Record 记一份快照，按时间升序追加。
func (h *History) Record(atMs int64, positions map[string]Vec) {
	copied := make(map[string]Vec, len(positions))
	for name, position := range positions {
		copied[name] = position
	}
	if h.size == Capacity {
		h.head = (h.head + 1) % Capacity
	} else {
		h.size++
	}
	h.ring[(h.head+h.size-1)%Capacity] = Snapshot{AtMs: atMs, Positions: copied}
	// 只保留最近 RetainMs 内的快照（以最新一份为基准）。
	oldestAt := atMs - RetainMs
	for h.size > 0 && h.ring[h.head].AtMs < oldestAt {
		h.head = (h.head + 1) % Capacity
		h.size--
	}
}

// Retained 是当前保留的快照数。
func (h *History) Retained() int { return h.size }

// Scanned 是取快照累计看过的记录数（规模观测）。
func (h *History) Scanned() int { return h.scanned }

// RewindMs 是这次射击要回滚的时长。
// 等于 RTT 的一半（整数除法），并裁剪到 [0, MaxRewindMs]。
func (h *History) RewindMs(rttMs int64) int64 {
	rewind := rttMs / 2
	if rewind < 0 {
		rewind = 0
	}
	if rewind > MaxRewindMs {
		return MaxRewindMs
	}
	return rewind
}

// SnapshotAt 取 whenMs 时刻（含之前最近一份）的快照。
// 取 AtMs <= whenMs 的最新一份；whenMs 早于最旧快照时返回 ErrTooOld。
// 按时间二分，不许线性扫。
func (h *History) SnapshotAt(whenMs int64) (Snapshot, error) {
	if h.size == 0 {
		return Snapshot{}, nil
	}
	// 越界：目标早于最旧快照。先挡掉这条路，二分只需在其余记录间定位。
	if h.ring[h.head].AtMs > whenMs {
		return Snapshot{}, ErrTooOld
	}
	// 下界二分：找第一个 AtMs > whenMs 的下标，其前一份即为所求。
	lo, hi := 1, h.size
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		h.scanned++
		if h.at(mid).AtMs <= whenMs {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return h.at(lo - 1), nil
}

// at 取按时间升序的第 index 份快照（环形下标换算）。
func (h *History) at(index int) Snapshot {
	return h.ring[(h.head+index)%Capacity]
}

// Hit 判定 target 在客户端开火那一刻是否被击中。
func (h *History) Hit(target string, clientMs, rttMs int64) (bool, error) {
	snapshot, err := h.SnapshotAt(clientMs - h.RewindMs(rttMs))
	if err != nil {
		return false, err
	}
	position, ok := snapshot.Positions[target]
	if !ok {
		return false, nil
	}
	return math.Hypot(position.X-h.shooter.X, position.Y-h.shooter.Y) <= HitRadius, nil
}
