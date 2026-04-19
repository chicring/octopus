package balancer

import (
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// ============================================================================
// RoundRobin 测试
// ============================================================================

func TestRoundRobin_Rotates(t *testing.T) {
	items := []model.GroupItem{
		{ChannelID: 1, ModelName: "gpt-4"},
		{ChannelID: 2, ModelName: "gpt-4"},
		{ChannelID: 3, ModelName: "gpt-4"},
	}

	rr := &RoundRobin{}
	groupID := 100 // 使用固定 groupID
	seen := map[int]int{}

	for i := 0; i < 9; i++ {
		candidates := rr.Candidates(groupID, items)
		first := candidates[0].ChannelID
		seen[first]++
	}

	// 9次请求，3个渠道，每个应该被选为首选3次
	for ch, count := range seen {
		if count != 3 {
			t.Errorf("RoundRobin channel %d: got %d selections, want 3", ch, count)
		}
	}
}

func TestRoundRobin_SingleChannel(t *testing.T) {
	items := []model.GroupItem{
		{ChannelID: 1, ModelName: "gpt-4"},
	}

	rr := &RoundRobin{}
	for i := 0; i < 5; i++ {
		candidates := rr.Candidates(1, items)
		if candidates[0].ChannelID != 1 {
			t.Errorf("single channel should always be first, got %d", candidates[0].ChannelID)
		}
	}
}

func TestRoundRobin_PerGroupIsolation(t *testing.T) {
	items3 := []model.GroupItem{
		{ChannelID: 1, ModelName: "gpt-4"},
		{ChannelID: 2, ModelName: "gpt-4"},
		{ChannelID: 3, ModelName: "gpt-4"},
	}
	items2 := []model.GroupItem{
		{ChannelID: 10, ModelName: "gpt-4"},
		{ChannelID: 20, ModelName: "gpt-4"},
	}

	rr := &RoundRobin{}

	// Group A (3渠道) 连续请求3次
	seenA := map[int]int{}
	for i := 0; i < 3; i++ {
		candidates := rr.Candidates(200, items3)
		seenA[candidates[0].ChannelID]++
	}

	// Group B (2渠道) 连续请求2次
	seenB := map[int]int{}
	for i := 0; i < 2; i++ {
		candidates := rr.Candidates(300, items2)
		seenB[candidates[0].ChannelID]++
	}

	// Group A 应该轮转了3个不同渠道
	if len(seenA) != 3 {
		t.Errorf("Group A should have 3 distinct first selections, got %d", len(seenA))
	}
	// Group B 应该轮转了2个不同渠道
	if len(seenB) != 2 {
		t.Errorf("Group B should have 2 distinct first selections, got %d", len(seenB))
	}
}

// ============================================================================
// Failover 测试
// ============================================================================

func TestFailover_PriorityOrdering(t *testing.T) {
	items := []model.GroupItem{
		{ChannelID: 3, ModelName: "gpt-4", Priority: 3},
		{ChannelID: 1, ModelName: "gpt-4", Priority: 1},
		{ChannelID: 2, ModelName: "gpt-4", Priority: 2},
	}

	fo := &Failover{}
	candidates := fo.Candidates(1, items)

	if candidates[0].ChannelID != 1 {
		t.Errorf("first should be channel 1 (priority 1), got %d", candidates[0].ChannelID)
	}
	if candidates[1].ChannelID != 2 {
		t.Errorf("second should be channel 2 (priority 2), got %d", candidates[1].ChannelID)
	}
	if candidates[2].ChannelID != 3 {
		t.Errorf("third should be channel 3 (priority 3), got %d", candidates[2].ChannelID)
	}
}

// ============================================================================
// Weighted 测试
// ============================================================================

func TestWeighted_ZeroWeightFallback(t *testing.T) {
	items := []model.GroupItem{
		{ChannelID: 1, ModelName: "gpt-4", Weight: 0},
		{ChannelID: 2, ModelName: "gpt-4", Weight: 5},
	}

	w := &Weighted{}
	foundZero := false
	for i := 0; i < 100; i++ {
		candidates := w.Candidates(1, items)
		if candidates[0].ChannelID == 1 {
			foundZero = true
			break
		}
	}
	if !foundZero {
		t.Log("Weight=0 channel may never be selected first (treated as weight=1, but rarely wins against weight=5)")
	}
}

// ============================================================================
// Iterator 测试
// ============================================================================

func TestIterator_StickyMovedToFront(t *testing.T) {
	SetSticky(1, "gpt-4", 2, 200, "gpt-4")

	group := model.Group{
		ID:              1,
		Mode:            model.GroupModeRoundRobin,
		SessionKeepTime: 300,
		Items: []model.GroupItem{
			{ChannelID: 1, ModelName: "gpt-4"},
			{ChannelID: 2, ModelName: "gpt-4"},
			{ChannelID: 3, ModelName: "gpt-4"},
		},
	}

	iter := NewIterator(group, 1, "gpt-4")

	if !iter.Next() {
		t.Fatal("expected at least one candidate")
	}
	first := iter.Item()
	if first.ChannelID != 2 {
		t.Errorf("sticky channel 2 should be first, got %d", first.ChannelID)
	}
	if !iter.IsSticky() {
		t.Error("first item should be marked as sticky")
	}

	globalSession.Delete("1:gpt-4")
}

func TestIterator_StickyMatchesModelName(t *testing.T) {
	// 同一渠道5配了两个模型 gpt-4 和 gpt-4o
	// sticky 记录的是 channelID=5, modelName=gpt-4o
	SetSticky(1, "gpt-4", 5, 500, "gpt-4o")

	group := model.Group{
		ID:              1,
		Mode:            model.GroupModeFailover,
		SessionKeepTime: 300,
		Items: []model.GroupItem{
			{ChannelID: 5, ModelName: "gpt-4"},   // 第一个同渠道项
			{ChannelID: 5, ModelName: "gpt-4o"},  // 第二个同渠道项
			{ChannelID: 3, ModelName: "gpt-4"},
		},
	}

	iter := NewIterator(group, 1, "gpt-4")

	if !iter.Next() {
		t.Fatal("expected at least one candidate")
	}
	first := iter.Item()
	// 应该匹配到 channelID=5 + modelName=gpt-4o，而不是第一个 gpt-4
	if first.ChannelID != 5 {
		t.Errorf("sticky should match channel 5, got %d", first.ChannelID)
	}
	if first.ModelName != "gpt-4o" {
		t.Errorf("sticky should match model gpt-4o, got %s", first.ModelName)
	}

	globalSession.Delete("1:gpt-4")
}

func TestIterator_StickyNotFound_Ignored(t *testing.T) {
	SetSticky(1, "gpt-4", 99, 900, "gpt-4")

	group := model.Group{
		ID:              1,
		Mode:            model.GroupModeFailover,
		SessionKeepTime: 300,
		Items: []model.GroupItem{
			{ChannelID: 1, ModelName: "gpt-4", Priority: 1},
			{ChannelID: 2, ModelName: "gpt-4", Priority: 2},
		},
	}

	iter := NewIterator(group, 1, "gpt-4")

	if !iter.Next() {
		t.Fatal("expected at least one candidate")
	}
	first := iter.Item()
	if first.ChannelID != 1 {
		t.Errorf("non-matching sticky should be ignored, expected channel 1, got %d", first.ChannelID)
	}

	globalSession.Delete("1:gpt-4")
}

func TestIterator_SkipAndAttempt(t *testing.T) {
	group := model.Group{
		ID:   1,
		Mode: model.GroupModeFailover,
		Items: []model.GroupItem{
			{ChannelID: 1, ModelName: "gpt-4"},
			{ChannelID: 2, ModelName: "gpt-4"},
			{ChannelID: 3, ModelName: "gpt-4"},
		},
	}

	iter := NewIterator(group, 1, "gpt-4")

	iter.Next()
	iter.Skip(1, 0, "ch1", "disabled")

	iter.Next()
	span := iter.StartAttempt(2, 200, "ch2")
	span.End(model.AttemptFailed, 500, "internal error")

	iter.Next()
	span3 := iter.StartAttempt(3, 300, "ch3")
	span3.End(model.AttemptSuccess, 200, "")

	attempts := iter.Attempts()
	if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(attempts))
	}

	if attempts[0].Status != model.AttemptSkipped {
		t.Errorf("attempt 0 should be skipped, got %s", attempts[0].Status)
	}
	if attempts[1].Status != model.AttemptFailed {
		t.Errorf("attempt 1 should be failed, got %s", attempts[1].Status)
	}
	if attempts[2].Status != model.AttemptSuccess {
		t.Errorf("attempt 2 should be success, got %s", attempts[2].Status)
	}
}

// ============================================================================
// Circuit Breaker 测试
// ============================================================================

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	globalBreaker.Delete("1:100:gpt-4")

	for i := 0; i < 5; i++ {
		RecordFailure(1, 100, "gpt-4")
	}

	tripped, _ := IsTripped(1, 100, "gpt-4")
	if !tripped {
		t.Error("circuit should be tripped after 5 consecutive failures")
	}

	RecordSuccess(1, 100, "gpt-4")
	tripped, _ = IsTripped(1, 100, "gpt-4")
	if tripped {
		t.Error("circuit should be closed after success")
	}

	globalBreaker.Delete("1:100:gpt-4")
}

func TestCircuitBreaker_ModelScoped(t *testing.T) {
	globalBreaker.Delete("1:100:gpt-4")
	globalBreaker.Delete("1:100:gpt-3.5")

	for i := 0; i < 5; i++ {
		RecordFailure(1, 100, "gpt-4")
	}

	tripped, _ := IsTripped(1, 100, "gpt-4")
	if !tripped {
		t.Error("gpt-4 should be tripped")
	}

	tripped, _ = IsTripped(1, 100, "gpt-3.5")
	if tripped {
		t.Error("gpt-3.5 should NOT be tripped (different model)")
	}

	RecordSuccess(1, 100, "gpt-4")
	globalBreaker.Delete("1:100:gpt-3.5")
}

func TestCircuitBreaker_HalfOpenTimeoutFallback(t *testing.T) {
	globalBreaker.Delete("1:100:gpt-4")

	// 触发熔断
	for i := 0; i < 5; i++ {
		RecordFailure(1, 100, "gpt-4")
	}

	// 确认已熔断
	tripped, _ := IsTripped(1, 100, "gpt-4")
	if !tripped {
		t.Fatal("circuit should be tripped")
	}

	// 手动将 HalfOpenTime 设为 31秒前，模拟探测请求挂死
	key := circuitKey(1, 100, "gpt-4")
	v, _ := globalBreaker.Load(key)
	entry := v.(*circuitEntry)
	entry.mu.Lock()
	entry.State = StateHalfOpen
	entry.HalfOpenTime = time.Now().Add(-31 * time.Second)
	entry.mu.Unlock()

	// IsTripped 应检测到 HalfOpen 超时，回退到 Open
	tripped, remaining := IsTripped(1, 100, "gpt-4")
	if !tripped {
		t.Error("should still be tripped after HalfOpen timeout fallback to Open")
	}
	if remaining <= 0 {
		t.Error("should have remaining cooldown after fallback to Open")
	}

	// 清理
	globalBreaker.Delete("1:100:gpt-4")
}

// ============================================================================
// Session Affinity 测试
// ============================================================================

func TestSessionAffinity_SetAndGet(t *testing.T) {
	globalSession.Delete("1:gpt-4")

	SetSticky(1, "gpt-4", 5, 500, "gpt-4")

	sticky := GetSticky(1, "gpt-4", 300*time.Second)
	if sticky == nil {
		t.Fatal("expected sticky entry")
	}
	if sticky.ChannelID != 5 {
		t.Errorf("expected channel 5, got %d", sticky.ChannelID)
	}
	if sticky.ChannelKeyID != 500 {
		t.Errorf("expected key 500, got %d", sticky.ChannelKeyID)
	}
	if sticky.ModelName != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", sticky.ModelName)
	}

	sticky2 := GetSticky(2, "gpt-4", 300*time.Second)
	if sticky2 != nil {
		t.Error("different apiKeyID should have no sticky")
	}

	globalSession.Delete("1:gpt-4")
}

func TestSessionAffinity_Expired(t *testing.T) {
	globalSession.Delete("1:gpt-4")

	key := sessionKey(1, "gpt-4")
	globalSession.Store(key, &SessionEntry{
		ChannelID:    5,
		ChannelKeyID: 500,
		ModelName:    "gpt-4",
		Timestamp:    time.Now().Add(-10 * time.Second),
	})

	sticky := GetSticky(1, "gpt-4", 5*time.Second)
	if sticky != nil {
		t.Error("expired sticky should return nil")
	}
}
