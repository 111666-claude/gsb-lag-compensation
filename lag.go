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

// History 是服务器快照历史，底层是定长环形缓冲，内存只跟 Capacity 有关。
type History struct {
	shooter Vec
	ring    []Snapshot
	head    int
	count   int
	scanned int
}

// NewHistory 建一份历史，shooter 是射手所在位置。
func NewHistory(shooter Vec) *History {
	return &History{
		shooter: shooter,
		ring:    make([]Snapshot, Capacity),
	}
}

// at 取逻辑下标 index（0 是最旧一份）对应的快照。
func (h *History) at(index int) Snapshot {
	return h.ring[(h.head+index)%Capacity]
}

// Record 记一份快照，按时间升序追加。
// 只保留最近 RetainMs 内、且总数不超过 Capacity 的快照，超出的最旧一份先丢。
func (h *History) Record(atMs int64, positions map[string]Vec) {
	copied := make(map[string]Vec, len(positions))
	for name, position := range positions {
		copied[name] = position
	}
	if h.count == Capacity {
		h.ring[h.head] = Snapshot{AtMs: atMs, Positions: copied}
		h.head = (h.head + 1) % Capacity
	} else {
		h.ring[(h.head+h.count)%Capacity] = Snapshot{AtMs: atMs, Positions: copied}
		h.count++
	}
	cutoff := atMs - RetainMs
	for h.count > 0 && h.at(0).AtMs < cutoff {
		h.ring[h.head] = Snapshot{}
		h.head = (h.head + 1) % Capacity
		h.count--
	}
}

// Retained 是当前保留的快照数（同时受 RetainMs 与 Capacity 约束）。
func (h *History) Retained() int { return h.count }

// Scanned 是取快照累计看过的记录数（规模观测，只统计二分比较）。
func (h *History) Scanned() int { return h.scanned }

// RewindMs 是这次射击要回滚的时长：rttMs 的一半（整数除法），再裁剪到 MaxRewindMs。
func (h *History) RewindMs(rttMs int64) int64 {
	if rttMs <= 0 {
		return 0
	}
	rewind := rttMs / 2
	if rewind > MaxRewindMs {
		return MaxRewindMs
	}
	return rewind
}

// SnapshotAt 取 AtMs 不大于 whenMs 的最新一份快照。
// whenMs 早于最旧快照（或历史为空）时返回 ErrTooOld。按时间二分，不许线性扫。
func (h *History) SnapshotAt(whenMs int64) (Snapshot, error) {
	if h.count == 0 || h.at(0).AtMs > whenMs {
		return Snapshot{}, ErrTooOld
	}
	lo, hi := 1, h.count
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

// Hit 判定 target 在客户端开火那一刻是否被击中，只读快照，不改历史。
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
