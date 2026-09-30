package lagcomp

import (
	"math"
	"math/rand"
	"testing"
)

func ceilLog2(n int) int {
	c := 0
	for 1<<c < n {
		c++
	}
	return c
}

func TestRewindHalfRTTAndClamp(t *testing.T) {
	h := NewHistory(Vec{})
	cases := []struct {
		rtt, want int64
	}{
		{0, 0}, {1, 0}, {2, 1}, {3, 1}, {300, 150},
		{200, 100}, {999, 499}, {1000, 500}, {1001, 500}, {4000, 500},
	}
	var prev int64
	checked := 0
	for rtt := int64(0); rtt <= 2000; rtt++ {
		got := h.RewindMs(rtt)
		if got < 0 || got > MaxRewindMs {
			t.Fatalf("RTT %d 回滚 %d 越界", rtt, got)
		}
		if got < prev {
			t.Fatalf("RewindMs 对 RTT 不单调：%d -> %d", prev, got)
		}
		if rtt >= 2 && got != rtt/2 && got != MaxRewindMs {
			t.Fatalf("RTT %d 回滚应为半 RTT 或上限，得到 %d", rtt, got)
		}
		prev = got
		checked++
	}
	for _, tc := range cases {
		if got := h.RewindMs(tc.rtt); got != tc.want {
			t.Fatalf("RTT %d 回滚应为 %d，得到 %d", tc.rtt, tc.want, got)
		}
	}
	if checked == 0 {
		t.Fatal("没有跑单调性检查")
	}
}

func TestSnapshotAtTooOld(t *testing.T) {
	h := NewHistory(Vec{})
	if _, err := h.SnapshotAt(0); err != ErrTooOld {
		t.Fatalf("空历史取快照应报 ErrTooOld，得到 %v", err)
	}
	h.Record(600, map[string]Vec{"runner": {X: 1}})
	h.Record(700, map[string]Vec{"runner": {X: 2}})
	if _, err := h.SnapshotAt(599); err != ErrTooOld {
		t.Fatalf("早于最旧快照应报 ErrTooOld，得到 %v", err)
	}
	s, err := h.SnapshotAt(600)
	if err != nil || s.AtMs != 600 {
		t.Fatalf("等于最旧快照时刻应取到 600，得到 %+v %v", s, err)
	}
	s, err = h.SnapshotAt(650)
	if err != nil || s.AtMs != 600 {
		t.Fatalf("650 应取到不大于它的最新一份 600，得到 %+v %v", s, err)
	}
}

func TestRetainMsPruning(t *testing.T) {
	h := NewHistory(Vec{})
	for at := int64(0); at <= 3000; at += 50 {
		h.Record(at, map[string]Vec{"runner": {X: float64(at)}})
	}
	if h.Retained() != 41 {
		t.Fatalf("最近 2000ms 内应保留 1000..3000 共 41 份，得到 %d", h.Retained())
	}
	if _, err := h.SnapshotAt(999); err != ErrTooOld {
		t.Fatalf("999 已过保留窗，应 ErrTooOld，得到 %v", err)
	}
	s, err := h.SnapshotAt(1000)
	if err != nil || s.AtMs != 1000 {
		t.Fatalf("窗边界 1000 的快照必须还在，得到 %+v %v", s, err)
	}
}

func TestCapacityPruning(t *testing.T) {
	h := NewHistory(Vec{})
	for at := int64(0); at < 600; at++ {
		h.Record(at, map[string]Vec{"runner": {X: float64(at)}})
	}
	if h.Retained() != Capacity {
		t.Fatalf("保留窗内数量超 Capacity 时应只留 %d 份，得到 %d", Capacity, h.Retained())
	}
	if _, err := h.SnapshotAt(87); err != ErrTooOld {
		t.Fatalf("最旧应为 88ms，87ms 应 ErrTooOld，得到 %v", err)
	}
	s, err := h.SnapshotAt(88)
	if err != nil || s.AtMs != 88 {
		t.Fatalf("最旧快照 88ms 应可取，得到 %+v %v", s, err)
	}
}

func TestHitReadOnlyAndRepeatable(t *testing.T) {
	h := NewHistory(Vec{})
	for at := int64(0); at <= 1000; at += 50 {
		h.Record(at, map[string]Vec{"runner": {X: float64(at) / 200}})
	}
	before := h.Retained()
	first, err := h.Hit("runner", 1000, 200)
	if err != nil || !first {
		t.Fatalf("首次判定应命中，得到 %v %v", first, err)
	}
	if h.Retained() != before {
		t.Fatal("判定改了历史数量")
	}
	oldest, err := h.SnapshotAt(0)
	if err != nil || oldest.AtMs != 0 {
		t.Fatalf("判定后最旧快照变了：%+v %v", oldest, err)
	}
	for i := 0; i < 5; i++ {
		got, err := h.Hit("runner", 1000, 200)
		if err != nil || got != first {
			t.Fatalf("第 %d 次重复判定结果不一致：%v %v", i, got, err)
		}
	}
	miss, err := h.Hit("ghost", 1000, 200)
	if err != nil || miss {
		t.Fatalf("不存在的目标应返回 false 且无错，得到 %v %v", miss, err)
	}
}

func TestBinarySearchScanBound(t *testing.T) {
	h := NewHistory(Vec{})
	for at := int64(0); at < 600; at++ {
		h.Record(at, map[string]Vec{"runner": {X: float64(at)}})
	}
	n := h.Retained()
	bound := ceilLog2(n)
	for when := int64(88); when < 599; when++ {
		before := h.Scanned()
		s, err := h.SnapshotAt(when)
		used := h.Scanned() - before
		if err != nil || s.AtMs != when {
			t.Fatalf("when=%d 应精确取到对应快照，得到 %+v %v", when, s, err)
		}
		if used > bound {
			t.Fatalf("when=%d 二分看了 %d 条，超过 ceil(log2(%d))=%d", when, used, n, bound)
		}
	}
}

// refHistory 是对照参考实现：半 RTT + 上限裁剪 + 二分取快照 + 双约束回收。
type refHistory struct {
	shooter   Vec
	snapshots []Snapshot
}

func (r *refHistory) record(atMs int64, positions map[string]Vec) {
	copied := make(map[string]Vec, len(positions))
	for name, p := range positions {
		copied[name] = p
	}
	r.snapshots = append(r.snapshots, Snapshot{AtMs: atMs, Positions: copied})
	if len(r.snapshots) > Capacity {
		r.snapshots = r.snapshots[len(r.snapshots)-Capacity:]
	}
	cutoff := atMs - RetainMs
	for len(r.snapshots) > 0 && r.snapshots[0].AtMs < cutoff {
		r.snapshots = r.snapshots[1:]
	}
}

func (r *refHistory) rewind(rttMs int64) int64 {
	if rttMs <= 0 {
		return 0
	}
	if rttMs/2 > MaxRewindMs {
		return MaxRewindMs
	}
	return rttMs / 2
}

func (r *refHistory) snapshotAt(whenMs int64) (Snapshot, bool) {
	if len(r.snapshots) == 0 || r.snapshots[0].AtMs > whenMs {
		return Snapshot{}, false
	}
	lo, hi := 1, len(r.snapshots)
	for lo < hi {
		mid := (lo + hi) / 2
		if r.snapshots[mid].AtMs <= whenMs {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return r.snapshots[lo-1], true
}

func (r *refHistory) hit(target string, clientMs, rttMs int64) (bool, bool) {
	s, ok := r.snapshotAt(clientMs - r.rewind(rttMs))
	if !ok {
		return false, false
	}
	p, ok := s.Positions[target]
	if !ok {
		return true, false
	}
	return true, math.Hypot(p.X-r.shooter.X, p.Y-r.shooter.Y) <= HitRadius
}

func TestMatchesReferenceImplementation(t *testing.T) {
	rng := rand.New(rand.NewSource(20260930))
	targets := []string{"a", "b", "c"}
	for trial := 0; trial < 200; trial++ {
		shooter := Vec{X: rng.Float64()*40 - 20, Y: rng.Float64()*40 - 20}
		h := NewHistory(shooter)
		ref := &refHistory{shooter: shooter}

		dense := rng.Intn(3) == 0
		num := 10 + rng.Intn(40)
		if dense {
			num = 500 + rng.Intn(150)
		}
		at := int64(0)
		for i := 0; i < num; i++ {
			if dense {
				at += 1
			} else {
				at += int64(1 + rng.Intn(300))
			}
			positions := map[string]Vec{}
			for _, name := range targets {
				positions[name] = Vec{X: rng.Float64()*60 - 30, Y: rng.Float64()*60 - 30}
			}
			h.Record(at, positions)
			ref.record(at, positions)
			if h.Retained() != len(ref.snapshots) {
				t.Fatalf("trial %d 第 %d 次记录后 Retained=%d 参考=%d", trial, i, h.Retained(), len(ref.snapshots))
			}
			if h.Retained() > 0 {
				got, err := h.SnapshotAt(ref.snapshots[0].AtMs)
				if err != nil || got.AtMs != ref.snapshots[0].AtMs {
					t.Fatalf("trial %d 最旧快照不一致：%+v %v", trial, got, err)
				}
			}
		}

		rttChoices := []int64{0, 1, 2, 3, 100, 200, 300, 999, 1000, 1001, 4000, rng.Int63n(5000)}
		for shot := 0; shot < 12; shot++ {
			clientMs := at - int64(rng.Intn(4500))
			rttMs := rttChoices[rng.Intn(len(rttChoices))]
			if got := h.RewindMs(rttMs); got != ref.rewind(rttMs) {
				t.Fatalf("trial %d RTT %d 回滚 %d != 参考 %d", trial, rttMs, got, ref.rewind(rttMs))
			}
			when := clientMs - h.RewindMs(rttMs)
			got, gotErr := h.SnapshotAt(when)
			want, wantOK := ref.snapshotAt(when)
			if (gotErr == nil) != wantOK {
				t.Fatalf("trial %d when=%d 越界判定不一致：%v vs %v", trial, when, gotErr, wantOK)
			}
			if gotErr == nil && got.AtMs != want.AtMs {
				t.Fatalf("trial %d when=%d 取到 %d 参考 %d", trial, when, got.AtMs, want.AtMs)
			}
			target := targets[rng.Intn(len(targets))]
			if rng.Intn(5) == 0 {
				target = "ghost"
			}
			hHit, hErr := h.Hit(target, clientMs, rttMs)
			rOK, rHit := ref.hit(target, clientMs, rttMs)
			if (hErr == nil) != rOK || hHit != rHit {
				t.Fatalf("trial %d target=%s client=%d rtt=%d 判定不一致：实现=(%v,%v) 参考=(%v,%v)",
					trial, target, clientMs, rttMs, hHit, hErr, rHit, rOK)
			}
		}
	}
}
