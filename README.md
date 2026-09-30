# lag-compensation

延迟补偿：固定容量的快照历史、按 RTT 回滚取快照、判定射击是否命中，只用 Go 标准库。

```
go test ./...
go run ./cmd/lag-compensation --sample rtt
go run ./cmd/lag-compensation --sample clamp
go run ./cmd/lag-compensation --sample stale
go run ./cmd/lag-compensation --sample growth
go run ./cmd/lag-compensation --sample scan
```

## 口径（README 为准）

- **回滚时长按 RTT 算**：`rewind = rttMs / 2`（整数除法），再裁剪到 `MaxRewindMs` 以内。
- **历史有界**：只保留最近 `RetainMs` 内的快照，并且不超过 `Capacity` 份；
  `Retained()` 必须反映这两条约束。
- **越界要报错**：`SnapshotAt(when)` 取 `AtMs <= when` 的最新一份；`when` 早于最旧快照时
  返回 `ErrTooOld`，不许拿最旧一帧顶替。
- **查询代价**：`Scanned()` 是取快照累计看过的记录数，按时间二分后每次不超过
  `ceil(log2(Retained))`，不许线性扫。
- **命中判定**：用回滚后的快照位置算距离，`<= HitRadius` 才算命中。
- **规模**：30 分钟 144fps（约 25.9 万份快照）、每秒 5 万次判定，内存只跟 `Capacity` 有关。

## 输出契约（不改格式）

```
rewindms=150 hit=false
rewindms=500 hit=false
rewindms=500 error=tooold
retained<=41
retained=41 scanned<=65000
```

## 常量

```
MaxRewindMs = 500
RetainMs    = 2000
Capacity    = 512
HitRadius   = 8.0
```

## 目录

```
lag.go                  延迟补偿
cmd/lag-compensation    命令行入口
lag_test.go             go test 用例
```
