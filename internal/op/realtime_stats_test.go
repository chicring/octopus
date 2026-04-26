package op

import (
	"sync"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// resetRealtimeWindow 重置全局窗口（测试间隔离）
func resetRealtimeWindow() {
	globalRealtimeWindow.mu.Lock()
	defer globalRealtimeWindow.mu.Unlock()
	globalRealtimeWindow.buckets = [realtimeWindowSize]realtimeBucket{}
}

func TestRealtime_SameSecondAccumulate(t *testing.T) {
	resetRealtimeWindow()

	// 同一秒内多次记录，应累加
	StatsRealtimeRecord(100)
	StatsRealtimeRecord(200)
	StatsRealtimeRecord(300)

	got := StatsRealtimeGet()

	// RPS: 3 次请求在当前秒
	if got.RPS != 3 {
		t.Errorf("RPS: got %d, want 3", got.RPS)
	}
	// TPS: 600 tokens 在当前秒
	if got.TPS != 600 {
		t.Errorf("TPS: got %d, want 600", got.TPS)
	}
	// RPM: 同一秒内的请求也在 60 秒窗口内
	if got.RPM != 3 {
		t.Errorf("RPM: got %d, want 3", got.RPM)
	}
	if got.TPM != 600 {
		t.Errorf("TPM: got %d, want 600", got.TPM)
	}
}

func TestRealtime_EarlyFailureZeroOutputToken(t *testing.T) {
	resetRealtimeWindow()

	// 早期失败：outputTokens = 0
	StatsRealtimeRecord(0)

	got := StatsRealtimeGet()
	if got.RPS != 1 {
		t.Errorf("RPS: got %d, want 1", got.RPS)
	}
	if got.TPS != 0 {
		t.Errorf("TPS: got %d, want 0", got.TPS)
	}
}

func TestRealtime_CrossSecondOverwrite(t *testing.T) {
	resetRealtimeWindow()

	// 先在当前秒记录
	StatsRealtimeRecord(100)

	// 模拟 1 秒后，同一桶位被覆盖（直接操作内部结构）
	globalRealtimeWindow.mu.Lock()
	now := time.Now().Unix()
	idx := now % realtimeWindowSize
	// 写入一个"旧"秒的数据到同一桶位
	globalRealtimeWindow.buckets[idx] = realtimeBucket{
		Second:       now - 1,
		Requests:     5,
		OutputTokens: 500,
	}
	globalRealtimeWindow.mu.Unlock()

	// 再记录一次，应该覆盖旧桶
	StatsRealtimeRecord(50)

	got := StatsRealtimeGet()

	// 当前秒只有 1 次请求（StatsRealtimeRecord 覆盖了旧桶后 +1）
	if got.RPS != 1 {
		t.Errorf("RPS: got %d, want 1", got.RPS)
	}
	// 当前秒 TPS = 50（覆盖后写入的）
	if got.TPS != 50 {
		t.Errorf("TPS: got %d, want 50", got.TPS)
	}
}

func TestRealtime_WindowSize(t *testing.T) {
	resetRealtimeWindow()

	got := StatsRealtimeGet()
	if got.WindowSizeSec != 60 {
		t.Errorf("WindowSizeSec: got %d, want 60", got.WindowSizeSec)
	}
}

func TestRealtime_EmptyWindow(t *testing.T) {
	resetRealtimeWindow()

	got := StatsRealtimeGet()
	if got.RPS != 0 || got.RPM != 0 || got.TPS != 0 || got.TPM != 0 {
		t.Errorf("empty window should be all zeros, got: %+v", got)
	}
}

func TestRealtime_OldBucketsExpired(t *testing.T) {
	resetRealtimeWindow()

	// 手动写入一个 61 秒前的桶（应该被忽略）
	globalRealtimeWindow.mu.Lock()
	now := time.Now().Unix()
	idx := now % realtimeWindowSize
	globalRealtimeWindow.buckets[idx] = realtimeBucket{
		Second:       now - 61,
		Requests:     999,
		OutputTokens: 9999,
	}
	globalRealtimeWindow.mu.Unlock()

	got := StatsRealtimeGet()
	if got.RPM != 0 {
		t.Errorf("RPM: got %d, want 0 (expired bucket)", got.RPM)
	}
	if got.TPM != 0 {
		t.Errorf("TPM: got %d, want 0 (expired bucket)", got.TPM)
	}
}

func TestRealtime_MultipleSecondsWindow(t *testing.T) {
	resetRealtimeWindow()

	// 模拟在最近 3 秒内各有请求
	now := time.Now().Unix()
	globalRealtimeWindow.mu.Lock()
	for i := int64(0); i < 3; i++ {
		sec := now - i
		idx := sec % realtimeWindowSize
		globalRealtimeWindow.buckets[idx] = realtimeBucket{
			Second:       sec,
			Requests:     10,
			OutputTokens: 100,
		}
	}
	globalRealtimeWindow.mu.Unlock()

	got := StatsRealtimeGet()

	// RPM: 3 个桶 * 10 = 30
	if got.RPM != 30 {
		t.Errorf("RPM: got %d, want 30", got.RPM)
	}
	// TPM: 3 个桶 * 100 = 300
	if got.TPM != 300 {
		t.Errorf("TPM: got %d, want 300", got.TPM)
	}
	// RPS: 仅当前秒 = 10
	if got.RPS != 10 {
		t.Errorf("RPS: got %d, want 10", got.RPS)
	}
	// TPS: 仅当前秒 = 100
	if got.TPS != 100 {
		t.Errorf("TPS: got %d, want 100", got.TPS)
	}
}

func TestRealtime_ConcurrentRecord(t *testing.T) {
	resetRealtimeWindow()

	const goroutines = 100
	const recordsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < recordsPerGoroutine; i++ {
				StatsRealtimeRecord(1)
			}
		}()
	}
	wg.Wait()

	got := StatsRealtimeGet()

	totalExpected := int64(goroutines * recordsPerGoroutine)
	if got.RPM != totalExpected {
		t.Errorf("RPM: got %d, want %d", got.RPM, totalExpected)
	}
	if got.TPM != totalExpected { // 每次 record 1 token
		t.Errorf("TPM: got %d, want %d", got.TPM, totalExpected)
	}
}

func TestRealtime_ReturnType(t *testing.T) {
	resetRealtimeWindow()

	got := StatsRealtimeGet()
	// 验证返回类型正确
	var _ model.StatsRealtime = got
}
