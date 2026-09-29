# lag-compensation

延迟补偿：按 RTT 回滚服务器快照，再判定射击是否命中，只用 Go 标准库。

```
go test ./...
go run ./cmd/lag-compensation --sample rtt
go run ./cmd/lag-compensation --sample clamp
go run ./cmd/lag-compensation --sample stale
```

## 口径（README 为准）

- **回滚时长按 RTT 算**：`rewind = rttMs / 2`（整数除法），再裁剪到 `MaxRewindMs` 以内。
  `--sample rtt`（RTT 300ms、1000ms 开火）按口径是 `rewindms=150`，现在是 `rewindms=100`。
  样例世界里的目标以每毫秒 0.1 米远离站在 x=195 的射手：150ms 的回滚把目标算到 x=185
  （距射手 10 米，超过 8 米命中半径，不命中），现在的 100ms 只回滚到 x=190（距 5 米），
  给出 `hit=true` 的假命中。
- **上限要裁剪**：RTT 再大也只回滚 `MaxRewindMs`。`--sample clamp`（RTT 4000ms）
  按口径是 `rewindms=500`（目标在 x=150，不命中），现在是 `rewindms=100`。
- **历史不够旧要报错**：回滚目标时刻早于历史里最旧的一份快照时返回 `ErrTooOld`，
  不许拿最旧的一帧顶替。`--sample stale`（历史从 600ms 起、按口径要回滚到 500ms）
  按口径是 `rewindms=500 error=tooold`，现在是 `rewindms=100 hit=true`。
- 不变量：`RewindMs` 对 RTT 单调不减、且永远不超过 `MaxRewindMs`；RTT=0 时回滚 0；
  同一份历史重复判定结果相同；判定只读快照，不改历史。
- 规模：单次判定 O(log 快照数)（历史按时间升序，二分即可），内存只跟快照窗口的时长有关；
  不许把整场录像留下再线性扫。

## 现在的行为

- `RewindMs` 直接返回 `fixedRewindMs = 100`，完全不看 RTT，也没有上限裁剪。
- `SnapshotAt` 找不到足够旧的快照时返回最旧的一帧和 `nil`，`ErrTooOld` 从来没被用过。

## 输出

```
rewindms=150 hit=false
rewindms=500 hit=false
rewindms=500 error=tooold
```

## 目录

```
lag.go                  延迟补偿
cmd/lag-compensation    命令行入口
lag_test.go             go test 用例
```
