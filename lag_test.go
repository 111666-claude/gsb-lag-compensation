package lagcomp

import (
	"math"
	"math/rand"
	"testing"
)

func TestSnapshotAtExactMoment(t *testing.T) {
	history := NewHistory(Vec{X: 0, Y: 0})
	history.Record(0, map[string]Vec{"runner": {X: 10}})
	history.Record(100, map[string]Vec{"runner": {X: 20}})
	snapshot, err := history.SnapshotAt(100)
	if err != nil {
		t.Fatalf("取快照不该报错：%v", err)
	}
	if snapshot.AtMs != 100 || snapshot.Positions["runner"].X != 20 {
		t.Fatalf("取到的快照不对：%+v", snapshot)
	}
	if history.Retained() != 2 {
		t.Fatalf("短历史不该被裁：%d", history.Retained())
	}
}

func TestHitWithTwoHundredMillisecondRTT(t *testing.T) {
	history := NewHistory(Vec{X: 0, Y: 0})
	for at := int64(0); at <= 1000; at += 50 {
		history.Record(at, map[string]Vec{"runner": {X: float64(at) / 200}})
	}
	// RTT 200ms 时回滚 100ms，客户端 1000ms 开火拿到 900ms 的位置（x=4.5 米）。
	hit, err := history.Hit("runner", 1000, 200)
	if err != nil {
		t.Fatalf("判定不该报错：%v", err)
	}
	if !hit {
		t.Fatal("1000ms 开火应该打中 900ms 时距离 4.5 米的目标")
	}
}

func TestConstants(t *testing.T) {
	if MaxRewindMs != 500 || RetainMs != 2000 || Capacity != 512 || HitRadius != 8 {
		t.Fatal("延迟补偿常量被改了")
	}
}

func TestRewindHalfRTT(t *testing.T) {
	history := NewHistory(Vec{})
	cases := map[int64]int64{
		0:   0,
		1:   0,
		2:   1,
		300: 150,
		301: 150,
	}
	for rtt, want := range cases {
		if got := history.RewindMs(rtt); got != want {
			t.Fatalf("RTT %d 应回滚 %d，实际 %d", rtt, want, got)
		}
	}
}

func TestRewindClamp(t *testing.T) {
	history := NewHistory(Vec{})
	for rtt := int64(0); rtt <= 5000; rtt++ {
		rewind := history.RewindMs(rtt)
		if rewind < 0 || rewind > MaxRewindMs {
			t.Fatalf("RTT %d 的回滚 %d 越界", rtt, rewind)
		}
		if rtt > 0 && rewind < history.RewindMs(rtt-1) {
			t.Fatalf("回滚时长对 RTT 不单调：rtt=%d", rtt)
		}
	}
	if got := history.RewindMs(4000); got != MaxRewindMs {
		t.Fatalf("RTT 4000 应裁剪到 %d，实际 %d", MaxRewindMs, got)
	}
}

func TestSnapshotAtTooOld(t *testing.T) {
	history := NewHistory(Vec{})
	history.Record(600, map[string]Vec{"runner": {X: 1}})
	history.Record(700, map[string]Vec{"runner": {X: 2}})
	if _, err := history.SnapshotAt(599); err != ErrTooOld {
		t.Fatalf("早于最旧快照必须报 ErrTooOld，实际 %v", err)
	}
	if _, err := history.SnapshotAt(-1 << 40); err != ErrTooOld {
		t.Fatalf("远早于最旧快照也必须报 ErrTooOld，实际 %v", err)
	}
	// 恰好等于最旧一份不算越界。
	snapshot, err := history.SnapshotAt(600)
	if err != nil || snapshot.AtMs != 600 {
		t.Fatalf("AtMs 等于目标时刻应直接命中：snapshot=%+v err=%v", snapshot, err)
	}
}

func TestHistoryRetainMs(t *testing.T) {
	history := NewHistory(Vec{})
	for at := int64(0); at <= 5000; at += 100 {
		history.Record(at, map[string]Vec{"runner": {X: float64(at)}})
		// 只保留最近 RetainMs，且单调升序意味着最旧一份必须在窗口内。
		if history.Retained() > Capacity {
			t.Fatalf("快照数超过 Capacity：%d", history.Retained())
		}
	}
	if history.Retained() != 21 {
		t.Fatalf("100ms 步长、窗口 %dms 应保留 21 份，实际 %d", RetainMs, history.Retained())
	}
	if _, err := history.SnapshotAt(2999); err != ErrTooOld {
		t.Fatalf("窗口外时刻应报 ErrTooOld，实际 %v", err)
	}
	// 30 分钟 144fps 规模：内存只跟 Capacity 有关。
	big := NewHistory(Vec{})
	for frame := int64(0); frame < 30*60*144; frame++ {
		at := frame * 1000 / 144
		big.Record(at, map[string]Vec{"runner": {X: float64(frame)}})
	}
	if big.Retained() > Capacity {
		t.Fatalf("30 分钟对局后快照数不得超过 Capacity：%d", big.Retained())
	}
	if _, err := big.SnapshotAt(big.ring[big.head].AtMs - 1); err != ErrTooOld {
		t.Fatalf("回收后的旧时刻必须报 ErrTooOld，实际 %v", err)
	}
}

func TestHistoryCapacityEviction(t *testing.T) {
	history := NewHistory(Vec{})
	// 密集采样让 Capacity 先生效（整段都在 RetainMs 窗口内）。
	for at := int64(0); at < 1000; at++ {
		history.Record(at, map[string]Vec{"runner": {X: float64(at)}})
	}
	if history.Retained() != Capacity {
		t.Fatalf("窗口内应顶到 Capacity=%d，实际 %d", Capacity, history.Retained())
	}
	if _, err := history.SnapshotAt(487); err != ErrTooOld {
		t.Fatalf("被容量顶掉的最旧快照时刻应报 ErrTooOld，实际 %v", err)
	}
	snapshot, err := history.SnapshotAt(488)
	if err != nil || snapshot.AtMs != 488 {
		t.Fatalf("容量淘汰后最旧一份应为 488ms：snapshot=%+v err=%v", snapshot, err)
	}
}

func TestHitReadOnlyAndRepeatable(t *testing.T) {
	history := NewHistory(Vec{X: 0})
	for at := int64(0); at <= 2000; at += 25 {
		history.Record(at, map[string]Vec{"runner": {X: float64(at) / 250}})
	}
	before := history.Retained()
	want, err := history.Hit("runner", 1500, 300)
	if err != nil {
		t.Fatalf("判定不该报错：%v", err)
	}
	if history.Retained() != before {
		t.Fatal("判定不得改动历史")
	}
	for i := 0; i < 5; i++ {
		got, err := history.Hit("runner", 1500, 300)
		if err != nil || got != want {
			t.Fatalf("重复判定结果不一致：got=%v err=%v want=%v", got, err, want)
		}
	}
}

func TestScannedIsBinarySearch(t *testing.T) {
	// 任意历史规模、任意目标时刻，单次二分看过的记录数都不得超过 ceil(log2(Retained))。
	for n := 1; n <= Capacity; n++ {
		history := NewHistory(Vec{})
		for i := 0; i < n; i++ {
			history.Record(int64(i)*2, map[string]Vec{"runner": {X: float64(i)}})
		}
		limit := ceilLog2(n)
		for when := int64(-2); when <= int64(n)*2+1; when++ {
			before := history.Scanned()
			_, _ = history.SnapshotAt(when)
			if delta := history.Scanned() - before; delta > limit {
				t.Fatalf("n=%d when=%d 看了 %d 条，超过 ceil(log2(n))=%d", n, when, delta, limit)
			}
		}
	}
}

func ceilLog2(n int) int {
	if n <= 1 {
		return 0
	}
	return int(math.Ceil(math.Log2(float64(n))))
}

// referenceHistory 是独立的参考实现：切片按 RTT 回滚 + 上限裁剪 + 二分取快照。
type referenceHistory struct {
	shooter   Vec
	snapshots []Snapshot
}

func (r *referenceHistory) record(atMs int64, positions map[string]Vec) {
	copied := make(map[string]Vec, len(positions))
	for name, position := range positions {
		copied[name] = position
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

func (r *referenceHistory) rewindMs(rttMs int64) int64 {
	rewind := rttMs / 2
	if rewind < 0 {
		rewind = 0
	}
	if rewind > MaxRewindMs {
		return MaxRewindMs
	}
	return rewind
}

func (r *referenceHistory) snapshotAt(whenMs int64) (Snapshot, bool) {
	lo, hi := 0, len(r.snapshots)
	for lo < hi {
		mid := (lo + hi) / 2
		if r.snapshots[mid].AtMs <= whenMs {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return Snapshot{}, false
	}
	return r.snapshots[lo-1], true
}

func (r *referenceHistory) hit(target string, clientMs, rttMs int64) (bool, bool) {
	snapshot, ok := r.snapshotAt(clientMs - r.rewindMs(rttMs))
	if !ok {
		return false, false
	}
	position, ok := snapshot.Positions[target]
	if !ok {
		return false, true
	}
	return math.Hypot(position.X-r.shooter.X, position.Y-r.shooter.Y) <= HitRadius, true
}

func TestReferenceDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(20260930))
	for caseNo := 0; caseNo < 200; caseNo++ {
		shooter := Vec{X: rng.Float64()*40 - 20, Y: rng.Float64()*40 - 20}
		history := NewHistory(shooter)
		ref := &referenceHistory{shooter: shooter}

		count := rng.Intn(900)
		start := rng.Int63n(5000)
		step := int64(1 + rng.Intn(6))
		for i := 0; i < count; i++ {
			at := start + int64(i)*step
			positions := map[string]Vec{
				"runner": {X: rng.Float64()*60 - 30, Y: rng.Float64()*60 - 30},
			}
			if rng.Intn(3) == 0 {
				positions["crawler"] = Vec{X: rng.Float64()*20 - 10, Y: rng.Float64()*20 - 10}
			}
			history.Record(at, positions)
			ref.record(at, positions)
		}

		if history.Retained() != len(ref.snapshots) {
			t.Fatalf("case %d：Retained=%d 参考=%d", caseNo, history.Retained(), len(ref.snapshots))
		}
		for i := 0; i < history.Retained(); i++ {
			if got := history.at(i).AtMs; got != ref.snapshots[i].AtMs {
				t.Fatalf("case %d：第 %d 份快照时间 %d != %d", caseNo, i, got, ref.snapshots[i].AtMs)
			}
		}

		var clientMs int64
		if count > 0 {
			clientMs = start + rng.Int63n(int64(count)*step+3000) - 1500
		}
		rttMs := int64(rng.Intn(6000))
		if history.RewindMs(rttMs) != ref.rewindMs(rttMs) {
			t.Fatalf("case %d：rewind 不一致 rtt=%d", caseNo, rttMs)
		}

		target := "runner"
		if rng.Intn(2) == 0 {
			target = "crawler"
		}
		gotHit, gotErr := history.Hit(target, clientMs, rttMs)
		refHit, refOK := ref.hit(target, clientMs, rttMs)
		if (gotErr == ErrTooOld) != (!refOK) {
			t.Fatalf("case %d：越界判定不一致 gotErr=%v refOK=%v", caseNo, gotErr, refOK)
		}
		if gotErr == nil && gotHit != refHit {
			t.Fatalf("case %d：命中不一致 got=%v ref=%v（rtt=%d client=%d）", caseNo, gotHit, refHit, rttMs, clientMs)
		}

		// 重复判定必须结果相同。
		again, err := history.Hit(target, clientMs, rttMs)
		if err != gotErr || again != gotHit {
			t.Fatalf("case %d：重复判定结果漂移", caseNo)
		}

		if n := history.Retained(); n > 0 {
			limit := ceilLog2(n)
			before := history.Scanned()
			_, err := history.SnapshotAt(clientMs)
			if err == nil && history.Scanned()-before > limit {
				t.Fatalf("case %d：二分扫描 %d 条超过 ceil(log2(%d))=%d", caseNo, history.Scanned()-before, n, limit)
			}
		}
	}
}
