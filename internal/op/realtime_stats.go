package op

import (
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

const realtimeWindowSize = 60

// realtimeBucket 每秒一个桶
type realtimeBucket struct {
	Second       int64
	Requests     int64
	OutputTokens int64
}

// realtimeWindow 60 秒环形缓冲区
type realtimeWindow struct {
	mu      sync.RWMutex
	buckets [realtimeWindowSize]realtimeBucket
}

var globalRealtimeWindow realtimeWindow

// StatsRealtimeRecord 记录一次请求完成事件
func StatsRealtimeRecord(outputTokens int64) {
	now := time.Now().Unix()
	idx := now % realtimeWindowSize

	globalRealtimeWindow.mu.Lock()
	defer globalRealtimeWindow.mu.Unlock()

	if globalRealtimeWindow.buckets[idx].Second != now {
		globalRealtimeWindow.buckets[idx] = realtimeBucket{Second: now}
	}
	globalRealtimeWindow.buckets[idx].Requests++
	globalRealtimeWindow.buckets[idx].OutputTokens += outputTokens
}

// StatsRealtimeGet 返回当前实时指标快照
// RPS/TPS = RPM/TPM / windowSize（60秒滑动窗口的平均每秒速率）
func StatsRealtimeGet() model.StatsRealtime {
	now := time.Now().Unix()

	globalRealtimeWindow.mu.RLock()
	defer globalRealtimeWindow.mu.RUnlock()

	var rpm, tpm int64 // 近 60 秒

	for i := 0; i < realtimeWindowSize; i++ {
		b := &globalRealtimeWindow.buckets[i]
		age := now - b.Second
		if age < 0 || age >= realtimeWindowSize {
			continue
		}
		rpm += b.Requests
		tpm += b.OutputTokens
	}

	return model.StatsRealtime{
		WindowSizeSec: realtimeWindowSize,
		RPS:           rpm / int64(realtimeWindowSize),
		RPM:           rpm,
		TPS:           tpm / int64(realtimeWindowSize),
		TPM:           tpm,
	}
}
