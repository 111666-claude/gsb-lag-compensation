// Package lagcomp 是延迟补偿：按 RTT 回滚服务器快照，再判定射击是否命中。
// 缺陷：回滚时长写死 100ms、不看 RTT，也从不裁剪到上限；历史不够旧时不报错，
// 直接拿最旧的一帧顶上。
package lagcomp

import (
	"errors"
	"math"
)

// MaxRewindMs 是允许回滚的最大时长，毫秒。
const MaxRewindMs int64 = 500

// HitRadius 是命中半径，米。
const HitRadius = 8.0

// fixedRewindMs 是写死的回滚时长。
const fixedRewindMs int64 = 100

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
	shooter   Vec
	snapshots []Snapshot
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
	h.snapshots = append(h.snapshots, Snapshot{AtMs: atMs, Positions: copied})
}

// RewindMs 是这次射击要回滚的时长。
// 缺陷：写死成 fixedRewindMs，与 RTT 无关，也没有上限裁剪。
func (h *History) RewindMs(rttMs int64) int64 {
	return fixedRewindMs
}

// SnapshotAt 取 whenMs 时刻（含之前最近一份）的快照。
// 缺陷：历史不够旧的时候返回最旧的一帧而不是 ErrTooOld。
func (h *History) SnapshotAt(whenMs int64) (Snapshot, error) {
	if len(h.snapshots) == 0 {
		return Snapshot{}, nil
	}
	found := h.snapshots[0]
	for _, snapshot := range h.snapshots {
		if snapshot.AtMs <= whenMs {
			found = snapshot
		}
	}
	return found, nil
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
