package lagcomp

import "testing"

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
