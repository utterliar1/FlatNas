package handlers

import (
	"sync"
	"testing"
	"time"
)

// resetUnlockAttempts 把限流状态恢复到干净值，避免用例间互相污染。
func resetUnlockAttempts() {
	unlockAttemptMu.Lock()
	defer unlockAttemptMu.Unlock()
	unlockAttempts = map[string][]time.Time{}
	unlockAttemptSweeps = 0
}

// TestUnlockRateLimitEnforcesLimit 验证单 IP 在窗口内超过上限后被限流。
func TestUnlockRateLimitEnforcesLimit(t *testing.T) {
	resetUnlockAttempts()
	const ip = "10.0.0.1"
	for i := 0; i < unlockAttemptLimit; i++ {
		if unlockRateLimited(ip) {
			t.Fatalf("第 %d 次尝试不应被限流", i+1)
		}
	}
	if !unlockRateLimited(ip) {
		t.Fatal("达到上限后应被限流")
	}
}

// TestUnlockRateLimitSweepsStaleEntries 是「map 只增不减」缺陷的回归测试：
// 陈旧条目（最后一次尝试已超出时间窗）必须在周期性扫描中被回收，
// 否则尝试过一次便不再出现的 IP 会永久占用键，导致内存缓慢增长。
func TestUnlockRateLimitSweepsStaleEntries(t *testing.T) {
	resetUnlockAttempts()

	// 造一个陈旧条目：直接写入一个远超时间窗的时间戳。
	unlockAttemptMu.Lock()
	unlockAttempts["10.0.0.2"] = []time.Time{time.Now().Add(-2 * unlockAttemptWindow)}
	unlockAttemptMu.Unlock()

	// 推进调用计数直到触发一次扫描。
	for i := 0; i < unlockSweepEvery; i++ {
		unlockRateLimited("10.0.0.3")
	}

	unlockAttemptMu.Lock()
	_, stillThere := unlockAttempts["10.0.0.2"]
	unlockAttemptMu.Unlock()

	if stillThere {
		t.Fatal("陈旧条目应在周期性扫描中被回收，否则 map 只增不减")
	}
}

// TestUnlockRateLimitKeepsFreshEntries 扫描不能误删仍在窗口内的活跃条目。
func TestUnlockRateLimitKeepsFreshEntries(t *testing.T) {
	resetUnlockAttempts()
	const ip = "10.0.0.4"
	unlockRateLimited(ip)

	for i := 0; i < unlockSweepEvery; i++ {
		unlockRateLimited("10.0.0.5")
	}

	unlockAttemptMu.Lock()
	ts, kept := unlockAttempts[ip]
	unlockAttemptMu.Unlock()

	if !kept || len(ts) == 0 {
		t.Fatal("窗口内的活跃条目不应被扫描误删")
	}
}

// TestUnlockRateLimitConcurrent 并发调用不应触发竞态（配合 go test -race 使用）。
func TestUnlockRateLimitConcurrent(t *testing.T) {
	resetUnlockAttempts()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 64; i++ {
				unlockRateLimited("10.1.0." + string(rune('0'+g)))
			}
		}(g)
	}
	wg.Wait()
}
