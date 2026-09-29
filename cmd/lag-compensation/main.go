// Command lag-compensation 跑延迟补偿样例。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/lagcomp"
)

// 样例世界：一个沿 x 轴远离射手的目标。
const (
	runnerStartX = 100.0
	runnerSpeed  = 0.1
	shooterX     = 195.0
)

func runnerPositions(atMs int64) map[string]lagcomp.Vec {
	return map[string]lagcomp.Vec{"runner": {X: runnerStartX + runnerSpeed*float64(atMs)}}
}

func feed(history *lagcomp.History, fromMs, untilMs, stepMs int64) {
	for at := fromMs; at <= untilMs; at += stepMs {
		history.Record(at, runnerPositions(at))
	}
}

// Run 执行一次命令行调用，返回退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lag-compensation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sample := flags.String("sample", "rtt", "rtt / clamp / stale")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	history := lagcomp.NewHistory(lagcomp.Vec{X: shooterX})
	var rttMs, clientMs int64
	switch *sample {
	case "rtt":
		feed(history, 0, 1000, 50)
		rttMs, clientMs = 300, 1000
	case "clamp":
		feed(history, 0, 1000, 50)
		rttMs, clientMs = 4000, 1000
	case "stale":
		// 历史只从 600ms 开始，客户端却要回滚到更早的位置。
		feed(history, 600, 1000, 50)
		rttMs, clientMs = 2000, 1000
	default:
		fmt.Fprintln(stderr, "需要 --sample rtt|clamp|stale")
		return 2
	}
	hit, err := history.Hit("runner", clientMs, rttMs)
	if err != nil {
		fmt.Fprintf(stdout, "rewindms=%d error=tooold\n", history.RewindMs(rttMs))
		return 0
	}
	fmt.Fprintf(stdout, "rewindms=%d hit=%v\n", history.RewindMs(rttMs), hit)
	return 0
}

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
