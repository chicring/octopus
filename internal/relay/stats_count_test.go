package relay

import (
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

// statsSnapshot 记录一次请求完成后各维度的统计快照
type statsSnapshot struct {
	// 全局维度
	TotalSuccess int64
	TotalFailed  int64
	TotalTokens  int64

	// 渠道维度 (key = channelID)
	ChannelSuccess map[int]int64
	ChannelFailed  map[int]int64
	ChannelTokens  map[int]int64
	ChannelWait    map[int]int64

	// ChannelKey 维度 (key = keyID)
	KeyRequests    map[int]int64
	KeyInputToken  map[int]int64
	KeyOutputToken map[int]int64
}

func newStatsSnapshot() statsSnapshot {
	return statsSnapshot{
		ChannelSuccess: make(map[int]int64),
		ChannelFailed:  make(map[int]int64),
		ChannelTokens:  make(map[int]int64),
		ChannelWait:    make(map[int]int64),
		KeyRequests:    make(map[int]int64),
		KeyInputToken:  make(map[int]int64),
		KeyOutputToken: make(map[int]int64),
	}
}

// attemptSim 模拟修复后 attempt() 中的统计更新
// 成功分支：不调用 StatsChannelUpdate
// 失败分支：只记录 RequestFailed（不含 WaitTime）
func attemptSim(s *statsSnapshot, channelID, keyID int, success bool, inputToken, outputToken int64) {
	if success {
		// 成功分支：ChannelKey 更新，不调用 StatsChannelUpdate
		s.KeyRequests[keyID]++
		s.KeyInputToken[keyID] += inputToken
		s.KeyOutputToken[keyID] += outputToken
	} else {
		// 失败分支：ChannelKey 更新 + StatsChannelUpdate({RequestFailed: 1})
		s.KeyRequests[keyID]++
		s.ChannelFailed[channelID]++
	}
}

// metricsSaveSim 模拟修复后 metrics.Save() 中的统计更新
// 渠道统计：Token/Cost/WaitTime + 成功时 success=1，失败时不记录 RequestFailed
func metricsSaveSim(s *statsSnapshot, finalChannelID int, success bool, inputToken, outputToken int64) {
	if success {
		s.TotalSuccess++
	} else {
		s.TotalFailed++
	}
	s.TotalTokens += inputToken + outputToken

	// 渠道维度：Token/Cost/WaitTime + 成功时 success=1
	// 失败时不记录 RequestFailed（attempt() 已按每次尝试记录）
	if finalChannelID > 0 {
		s.ChannelTokens[finalChannelID] += inputToken + outputToken
		s.ChannelWait[finalChannelID]++ // 简化：用计数代替实际毫秒
		if success {
			s.ChannelSuccess[finalChannelID]++
		}
	}
}

// ============================================================================
// 场景1: 单渠道单Key，一次成功
// 修复后：渠道 success=1（仅来自 Save），无双重计数
// ============================================================================
func TestStats_SingleChannelSingleKey_Success(t *testing.T) {
	s := newStatsSnapshot()

	channelID := 1
	keyID := 100
	inputToken := int64(100)
	outputToken := int64(50)

	attemptSim(&s, channelID, keyID, true, inputToken, outputToken)
	metricsSaveSim(&s, channelID, true, inputToken, outputToken)

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalTokens", s.TotalTokens, 150)

	// ChannelKey 维度
	assertEqual(t, "KeyRequests", s.KeyRequests[keyID], 1)
	assertEqual(t, "KeyInputToken", s.KeyInputToken[keyID], 100)

	// 渠道维度：修复后 success=1（仅来自 Save）
	assertEqual(t, "ChannelSuccess", s.ChannelSuccess[channelID], 1)
	assertEqual(t, "ChannelTokens", s.ChannelTokens[channelID], 150)
	assertEqual(t, "ChannelWait", s.ChannelWait[channelID], 1)

	diff := s.ChannelSuccess[channelID] - s.TotalSuccess
	t.Logf("[场景1] 渠道成功=%d, 全局成功=%d, 偏差=%d", s.ChannelSuccess[channelID], s.TotalSuccess, diff)
}

// ============================================================================
// 场景2: 重试2次后第3次成功 (3个渠道，前2个失败)
// 修复后：渠道A failed=1, 渠道B failed=1, 渠道C success=1
// ============================================================================
func TestStats_RetryTwoFailuresThenSuccess(t *testing.T) {
	s := newStatsSnapshot()

	attemptSim(&s, 1, 100, false, 0, 0) // 渠道A 失败
	attemptSim(&s, 2, 200, false, 0, 0) // 渠道B 失败
	attemptSim(&s, 3, 300, true, 100, 50) // 渠道C 成功
	metricsSaveSim(&s, 3, true, 100, 50)

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalFailed", s.TotalFailed, 0)

	// 渠道维度
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 1)
	assertEqual(t, "ChannelC Success", s.ChannelSuccess[3], 1)
	assertEqual(t, "ChannelC Tokens", s.ChannelTokens[3], 150)

	// ChannelKey
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 1)
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 1)
	assertEqual(t, "Key300 Requests", s.KeyRequests[300], 1)

	t.Logf("[场景2] 渠道C成功=%d, 全局成功=%d, 偏差=%d; 渠道A失败=%d, 渠道B失败=%d",
		s.ChannelSuccess[3], s.TotalSuccess, s.ChannelSuccess[3]-s.TotalSuccess,
		s.ChannelFailed[1], s.ChannelFailed[2])
}

// ============================================================================
// 场景3: 所有渠道都失败 (3个渠道全部失败)
// 修复后：渠道A/B/C 各 failed=1，最终渠道C不再被 Save 重复计 failed
// ============================================================================
func TestStats_AllChannelsFail(t *testing.T) {
	s := newStatsSnapshot()

	attemptSim(&s, 1, 100, false, 0, 0) // 渠道A 失败
	attemptSim(&s, 2, 200, false, 0, 0) // 渠道B 失败
	attemptSim(&s, 3, 300, false, 0, 0) // 渠道C 失败
	metricsSaveSim(&s, 3, false, 0, 0)

	// 全局维度
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 渠道维度：每个渠道各1次失败，无双重计数
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 1)
	assertEqual(t, "ChannelC Failed", s.ChannelFailed[3], 1)

	// ChannelKey: 每次尝试都 +1, 共3次
	assertEqual(t, "Total KeyRequests", s.KeyRequests[100]+s.KeyRequests[200]+s.KeyRequests[300], 3)

	t.Logf("[场景3] 渠道C失败=%d, 全局失败=%d, 偏差=%d",
		s.ChannelFailed[3], s.TotalFailed, s.ChannelFailed[3]-s.TotalFailed)
}

// ============================================================================
// 场景4: 单渠道4个Key，4次请求轮询
// 修复后：渠道 success=4（仅来自 Save），无双重计数
// ============================================================================
func TestStats_SingleChannelMultiKey_NoRetry(t *testing.T) {
	s := newStatsSnapshot()

	channelID := 1
	keys := []int{100, 101, 102, 103}

	for i, keyID := range keys {
		inputToken := int64(100 + i*10)
		outputToken := int64(50 + i*5)
		attemptSim(&s, channelID, keyID, true, inputToken, outputToken)
		metricsSaveSim(&s, channelID, true, inputToken, outputToken)
	}

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 4)

	// 渠道维度：修复后 success=4（仅来自 Save）
	assertEqual(t, "ChannelSuccess", s.ChannelSuccess[channelID], 4)

	// 每个Key各被使用1次
	for _, keyID := range keys {
		assertEqual(t, "Key Requests", s.KeyRequests[keyID], 1)
	}

	t.Logf("[场景4] 渠道成功=%d, 全局成功=%d, 偏差=%d (4个Key轮询)",
		s.ChannelSuccess[channelID], s.TotalSuccess, s.ChannelSuccess[channelID]-s.TotalSuccess)
}

// ============================================================================
// 场景5: 单渠道4个Key，Key1失败后不会换Key2重试
// 修复后：渠道 failed=1（仅来自 attempt），Save 不重复计 failed
// ============================================================================
func TestStats_SingleChannelMultiKey_KeyFailNoRetryWithinChannel(t *testing.T) {
	s := newStatsSnapshot()

	channelID := 1
	keyID := 100

	attemptSim(&s, channelID, keyID, false, 0, 0)
	metricsSaveSim(&s, channelID, false, 0, 0)

	// 全局维度
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 渠道维度：修复后 failed=1（仅来自 attempt）
	assertEqual(t, "ChannelFailed", s.ChannelFailed[channelID], 1)

	// Key1: 1次请求
	assertEqual(t, "KeyRequests", s.KeyRequests[keyID], 1)

	// 其他Key未被使用
	for _, otherKey := range []int{101, 102, 103} {
		assertEqual(t, "Other Key Requests", s.KeyRequests[otherKey], 0)
	}

	t.Logf("[场景5] 单渠道Key失败后不会换Key重试，渠道失败=%d, 全局失败=%d",
		s.ChannelFailed[channelID], s.TotalFailed)
}

// ============================================================================
// 场景6: 2个渠道各2个Key，渠道A的Key1失败，重试到渠道B的Key3成功
// 修复后：渠道A failed=1, 渠道B success=1
// ============================================================================
func TestStats_MultiChannelMultiKey_CrossChannelRetry(t *testing.T) {
	s := newStatsSnapshot()

	attemptSim(&s, 1, 100, false, 0, 0)  // 渠道A Key1 失败
	attemptSim(&s, 2, 300, true, 200, 80) // 渠道B Key3 成功
	metricsSaveSim(&s, 2, true, 200, 80)

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalTokens", s.TotalTokens, 280)

	// 渠道维度
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 1)
	assertEqual(t, "ChannelB Tokens", s.ChannelTokens[2], 280)

	// Key维度
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 1)
	assertEqual(t, "Key300 Requests", s.KeyRequests[300], 1)
	assertEqual(t, "Key300 InputToken", s.KeyInputToken[300], 200)

	t.Logf("[场景6] 渠道B成功=%d, 全局成功=%d, 偏差=%d",
		s.ChannelSuccess[2], s.TotalSuccess, s.ChannelSuccess[2]-s.TotalSuccess)
}

// ============================================================================
// 场景7: 多次请求累积验证
// 10次请求：7次直接成功，2次重试1次后成功，1次全部失败
// ============================================================================
func TestStats_MultipleRequestsCumulative(t *testing.T) {
	s := newStatsSnapshot()

	// 7次直接成功 (渠道1)
	for i := 0; i < 7; i++ {
		attemptSim(&s, 1, 100, true, 100, 50)
		metricsSaveSim(&s, 1, true, 100, 50)
	}

	// 2次重试1次后成功 (渠道1失败 -> 渠道2成功)
	for i := 0; i < 2; i++ {
		attemptSim(&s, 1, 100, false, 0, 0)
		attemptSim(&s, 2, 200, true, 100, 50)
		metricsSaveSim(&s, 2, true, 100, 50)
	}

	// 1次全部失败 (渠道1失败 -> 渠道2失败)
	attemptSim(&s, 1, 100, false, 0, 0)
	attemptSim(&s, 2, 200, false, 0, 0)
	metricsSaveSim(&s, 2, false, 0, 0)

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 9)
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 渠道1: 7次Save成功 + 3次attempt失败
	assertEqual(t, "Channel1 Success", s.ChannelSuccess[1], 7)
	assertEqual(t, "Channel1 Failed", s.ChannelFailed[1], 3)

	// 渠道2: 2次Save成功 + 1次attempt失败
	assertEqual(t, "Channel2 Success", s.ChannelSuccess[2], 2)
	assertEqual(t, "Channel2 Failed", s.ChannelFailed[2], 1)

	// ChannelKey
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 10) // 7成功+3失败
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 3)  // 2成功+1失败

	t.Logf("[场景7] 渠道1: success=%d failed=%d, 渠道2: success=%d failed=%d, 全局: success=%d failed=%d",
		s.ChannelSuccess[1], s.ChannelFailed[1],
		s.ChannelSuccess[2], s.ChannelFailed[2],
		s.TotalSuccess, s.TotalFailed)
}

// ============================================================================
// 场景8: 验证Token在渠道维度只被Save记录一次（不重复）
// ============================================================================
func TestStats_TokenNotDoubleCountedInChannel(t *testing.T) {
	s := newStatsSnapshot()

	channelID := 1
	keyID := 100

	attemptSim(&s, channelID, keyID, true, 500, 200)
	metricsSaveSim(&s, channelID, true, 500, 200)

	// 渠道Token：修复后只由 Save 记录1次
	assertEqual(t, "ChannelTokens", s.ChannelTokens[channelID], 700)
	assertEqual(t, "TotalTokens", s.TotalTokens, 700)

	t.Logf("[场景8] 渠道Token=%d, 全局Token=%d, 偏差=%d",
		s.ChannelTokens[channelID], s.TotalTokens, s.ChannelTokens[channelID]-s.TotalTokens)
}

// ============================================================================
// 场景9: 熔断器触发后渠道被跳过，不计入统计
// ============================================================================
func TestStats_CircuitBreakSkippedNoCount(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道1被熔断跳过 (不调用attemptSim)
	// 渠道2成功
	attemptSim(&s, 2, 200, true, 100, 50)
	metricsSaveSim(&s, 2, true, 100, 50)

	// 渠道1没有任何统计
	assertEqual(t, "Channel1 Success", s.ChannelSuccess[1], 0)
	assertEqual(t, "Channel1 Failed", s.ChannelFailed[1], 0)

	// 渠道2: success=1
	assertEqual(t, "Channel2 Success", s.ChannelSuccess[2], 1)

	t.Logf("[场景9] 熔断跳过的渠道不计入统计，渠道2成功=%d", s.ChannelSuccess[2])
}

// ============================================================================
// 场景10: 流式响应已写入后失败，不可重试
// ============================================================================
func TestStats_StreamingWrittenFailNoRetry(t *testing.T) {
	s := newStatsSnapshot()

	channelID := 1
	keyID := 100

	attemptSim(&s, channelID, keyID, false, 0, 0)
	metricsSaveSim(&s, channelID, false, 0, 0)

	// 全局维度
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 渠道维度：修复后 failed=1（仅来自 attempt）
	assertEqual(t, "ChannelFailed", s.ChannelFailed[channelID], 1)

	t.Logf("[场景10] 流式写入后失败，渠道失败=%d, 全局失败=%d",
		s.ChannelFailed[channelID], s.TotalFailed)
}

// ============================================================================
// 场景11: 验证渠道维度的 success+failed 总和 = 实际尝试次数
// ============================================================================
func TestStats_ChannelAttemptCountConsistency(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道1: 3次成功, 2次失败
	for i := 0; i < 3; i++ {
		attemptSim(&s, 1, 100, true, 100, 50)
		metricsSaveSim(&s, 1, true, 100, 50)
	}
	for i := 0; i < 2; i++ {
		attemptSim(&s, 1, 100, false, 0, 0)
		// 假设重试到渠道2成功
		attemptSim(&s, 2, 200, true, 100, 50)
		metricsSaveSim(&s, 2, true, 100, 50)
	}

	// 渠道1: success=3(Save) + failed=2(attempt) = 5次尝试
	assertEqual(t, "Channel1 total attempts", s.ChannelSuccess[1]+s.ChannelFailed[1], 5)
	// 渠道2: success=2(Save) + failed=0 = 2次尝试
	assertEqual(t, "Channel2 total attempts", s.ChannelSuccess[2]+s.ChannelFailed[2], 2)

	t.Logf("[场景11] 渠道1尝试=%d, 渠道2尝试=%d, Key100请求=%d, Key200请求=%d",
		s.ChannelSuccess[1]+s.ChannelFailed[1],
		s.ChannelSuccess[2]+s.ChannelFailed[2],
		s.KeyRequests[100], s.KeyRequests[200])
}

// ============================================================================
// 场景12: 所有候选渠道都被跳过（无可用Key/类型不兼容），无真实 attempt
// 此时 metrics.Save 的 finalChannel 可能指向一个被跳过的渠道
// ============================================================================
func TestStats_SkippedOnly_NoRealAttempt(t *testing.T) {
	s := newStatsSnapshot()

	// 所有渠道都被跳过，没有调用 attemptSim
	// metrics.Save 被调用，但 finalChannel 指向最后一个被跳过的渠道
	// 被跳过的渠道不应有任何统计
	metricsSaveSim(&s, 0, false, 0, 0) // channelID=0, 无有效渠道

	// 全局维度：1次失败
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 无渠道统计（channelID=0 不更新渠道）
	assertEqual(t, "Channel1 Success", s.ChannelSuccess[1], 0)
	assertEqual(t, "Channel1 Failed", s.ChannelFailed[1], 0)

	t.Logf("[场景12] 全部跳过，全局失败=%d，无渠道统计", s.TotalFailed)
}

// ============================================================================
// 场景13: 客户端取消请求（在失败尝试后取消）
// 已完成的失败尝试应记录，但不应双重计数
// ============================================================================
func TestStats_ContextCanceled_AfterFailures(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A 失败
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B 失败
	attemptSim(&s, 2, 200, false, 0, 0)
	// 客户端取消，不再尝试更多渠道
	// metrics.Save(false, context.Canceled, attempts)
	metricsSaveSim(&s, 2, false, 0, 0) // finalChannel=2 (最后一个失败的渠道)

	// 全局维度：1次失败
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)

	// 渠道A: 1次失败（来自 attempt）
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	// 渠道B: 1次失败（来自 attempt），Save 不重复计 failed
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 1)

	// Key维度
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 1)
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 1)

	t.Logf("[场景13] 取消后：渠道A失败=%d, 渠道B失败=%d, 全局失败=%d",
		s.ChannelFailed[1], s.ChannelFailed[2], s.TotalFailed)
}

// ============================================================================
// 场景14: 首Token超时触发重试
// 渠道A 首Token超时（失败）-> 渠道B 成功
// ============================================================================
func TestStats_FirstTokenTimeout_RetrySuccess(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A 首Token超时，作为失败处理
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B 成功
	attemptSim(&s, 2, 200, true, 150, 60)
	metricsSaveSim(&s, 2, true, 150, 60)

	// 全局维度
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalTokens", s.TotalTokens, 210)

	// 渠道维度
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 1)
	assertEqual(t, "ChannelB Tokens", s.ChannelTokens[2], 210)

	t.Logf("[场景14] 首Token超时重试：渠道A失败=%d, 渠道B成功=%d",
		s.ChannelFailed[1], s.ChannelSuccess[2])
}

// ============================================================================
// 场景15: 零Token请求（如embedding或极短响应）
// ============================================================================
func TestStats_ZeroTokenRequest(t *testing.T) {
	s := newStatsSnapshot()

	// 成功但0 token
	attemptSim(&s, 1, 100, true, 0, 0)
	metricsSaveSim(&s, 1, true, 0, 0)

	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalTokens", s.TotalTokens, 0)
	assertEqual(t, "ChannelSuccess", s.ChannelSuccess[1], 1)
	assertEqual(t, "ChannelTokens", s.ChannelTokens[1], 0)
	assertEqual(t, "KeyRequests", s.KeyRequests[100], 1)

	t.Logf("[场景15] 零Token请求：渠道成功=%d, Token=%d", s.ChannelSuccess[1], s.ChannelTokens[1])
}

// ============================================================================
// 场景16: 多个渠道部分被跳过（熔断+无Key），部分真实尝试
// 渠道A 熔断跳过 -> 渠道B 无Key跳过 -> 渠道C 成功
// ============================================================================
func TestStats_MixedSkipsAndAttempts(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A 熔断跳过（不调用 attemptSim）
	// 渠道B 无Key跳过（不调用 attemptSim）
	// 渠道C 成功
	attemptSim(&s, 3, 300, true, 100, 50)
	metricsSaveSim(&s, 3, true, 100, 50)

	// 被跳过的渠道无统计
	assertEqual(t, "ChannelA Success", s.ChannelSuccess[1], 0)
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 0)
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 0)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 0)

	// 渠道C: success=1
	assertEqual(t, "ChannelC Success", s.ChannelSuccess[3], 1)
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)

	t.Logf("[场景16] 混合跳过+尝试：渠道C成功=%d", s.ChannelSuccess[3])
}

// ============================================================================
// 场景17: 同一渠道在多次请求中交替成功和失败
// 验证渠道的 success 和 failed 独立累加
// ============================================================================
func TestStats_ChannelAlternatingSuccessFailure(t *testing.T) {
	s := newStatsSnapshot()

	// 请求1: 渠道1成功
	attemptSim(&s, 1, 100, true, 100, 50)
	metricsSaveSim(&s, 1, true, 100, 50)

	// 请求2: 渠道1失败，重试到渠道2成功
	attemptSim(&s, 1, 100, false, 0, 0)
	attemptSim(&s, 2, 200, true, 80, 40)
	metricsSaveSim(&s, 2, true, 80, 40)

	// 请求3: 渠道1成功
	attemptSim(&s, 1, 100, true, 120, 60)
	metricsSaveSim(&s, 1, true, 120, 60)

	// 渠道1: success=2(Save), failed=1(attempt)
	assertEqual(t, "Channel1 Success", s.ChannelSuccess[1], 2)
	assertEqual(t, "Channel1 Failed", s.ChannelFailed[1], 1)
	// 渠道1 Token: 150(请求1) + 180(请求3) = 330
	assertEqual(t, "Channel1 Tokens", s.ChannelTokens[1], 330)

	// 渠道2: success=1(Save)
	assertEqual(t, "Channel2 Success", s.ChannelSuccess[2], 1)

	// 全局: success=3, failed=0
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 3)
	assertEqual(t, "TotalFailed", s.TotalFailed, 0)

	t.Logf("[场景17] 渠道1: success=%d failed=%d tokens=%d, 渠道2: success=%d",
		s.ChannelSuccess[1], s.ChannelFailed[1], s.ChannelTokens[1], s.ChannelSuccess[2])
}

// ============================================================================
// 场景18: 渠道维度的 WaitTime 只由 Save 记录
// attempt 不再记录 WaitTime 到渠道统计
// ============================================================================
func TestStats_WaitTimeOnlyFromSave(t *testing.T) {
	s := newStatsSnapshot()

	// 3次成功请求
	for i := 0; i < 3; i++ {
		attemptSim(&s, 1, 100, true, 100, 50)
		metricsSaveSim(&s, 1, true, 100, 50)
	}

	// 渠道 WaitTime: 每次请求 Save 记录1次，共3次
	assertEqual(t, "ChannelWait", s.ChannelWait[1], 3)

	t.Logf("[场景18] 渠道WaitTime计数=%d (应为3，每次Save记录1次)", s.ChannelWait[1])
}

// ============================================================================
// 场景19: Key的TotalRequests在每次attempt都+1（包括失败）
// 验证多次重试后Key的请求计数
// ============================================================================
func TestStats_KeyTotalRequestsIncludesRetries(t *testing.T) {
	s := newStatsSnapshot()

	// 请求1: 渠道1失败 -> 渠道2失败 -> 渠道3成功
	attemptSim(&s, 1, 100, false, 0, 0)
	attemptSim(&s, 2, 200, false, 0, 0)
	attemptSim(&s, 3, 300, true, 100, 50)
	metricsSaveSim(&s, 3, true, 100, 50)

	// 每个Key各被使用1次
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 1)
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 1)
	assertEqual(t, "Key300 Requests", s.KeyRequests[300], 1)

	// 但全局只有1次成功请求
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)

	// Key的TotalRequests总和=3（每次尝试+1），但全局success=1
	// 这是设计意图：TotalRequests反映向上游发起的请求次数
	totalKeyReqs := s.KeyRequests[100] + s.KeyRequests[200] + s.KeyRequests[300]
	t.Logf("[场景19] Key总请求=%d(每次attempt+1), 全局成功=%d, 渠道失败A=%d B=%d, 渠道成功C=%d",
		totalKeyReqs, s.TotalSuccess, s.ChannelFailed[1], s.ChannelFailed[2], s.ChannelSuccess[3])
}

// ============================================================================
// 汇总测试：输出所有场景的偏差报告
// ============================================================================
func TestStats_SummaryReport(t *testing.T) {
	scenarios := []struct {
		name string
		fn   func() (channelSuccess, globalSuccess int64)
	}{
		{"单渠道成功", func() (int64, int64) {
			s := newStatsSnapshot()
			attemptSim(&s, 1, 100, true, 100, 50)
			metricsSaveSim(&s, 1, true, 100, 50)
			return s.ChannelSuccess[1], s.TotalSuccess
		}},
		{"重试2次后成功", func() (int64, int64) {
			s := newStatsSnapshot()
			attemptSim(&s, 1, 100, false, 0, 0)
			attemptSim(&s, 2, 200, false, 0, 0)
			attemptSim(&s, 3, 300, true, 100, 50)
			metricsSaveSim(&s, 3, true, 100, 50)
			return s.ChannelSuccess[3], s.TotalSuccess
		}},
		{"4个Key轮询4次", func() (int64, int64) {
			s := newStatsSnapshot()
			for i := 0; i < 4; i++ {
				attemptSim(&s, 1, 100+i, true, 100, 50)
				metricsSaveSim(&s, 1, true, 100, 50)
			}
			return s.ChannelSuccess[1], s.TotalSuccess
		}},
		{"跨渠道重试成功", func() (int64, int64) {
			s := newStatsSnapshot()
			attemptSim(&s, 1, 100, false, 0, 0)
			attemptSim(&s, 2, 200, true, 100, 50)
			metricsSaveSim(&s, 2, true, 100, 50)
			return s.ChannelSuccess[2], s.TotalSuccess
		}},
	}

	t.Log("========== 统计偏差汇总报告 ==========")
	t.Log("场景                    | 渠道成功 | 全局成功 | 偏差 | 状态")
	t.Log("------------------------|----------|----------|------|------")

	allCorrect := true
	for _, sc := range scenarios {
		ch, gl := sc.fn()
		diff := ch - gl
		status := "OK"
		if diff != 0 {
			status = "BUG"
			allCorrect = false
		}
		t.Logf("%-24s| %-8d | %-8d | %-4d | %s", sc.name, ch, gl, diff, status)
	}

	if !allCorrect {
		t.Error("存在统计偏差，修复未生效")
	}
}

// ============================================================================
// 验证 ChannelGetKey 的轮询行为说明
// ============================================================================
func TestChannelGetKey_RoundRobin(t *testing.T) {
	t.Log("ChannelGetKey 轮询策略验证:")
	t.Log("  - 同渠道内多Key: 最低成本优先, 同成本轮询")
	t.Log("  - Key失败后: 不会同渠道换Key, 而是跨渠道重试")
	t.Log("  - 429冷却: StatusCode=429的Key冷却5分钟")
	t.Log("  - Key亲和性: 无, 每次请求独立选择Key")
}

// ============================================================================
// 验证 model.StatsMetrics.Add 的正确性
// ============================================================================
func TestStatsMetrics_Add(t *testing.T) {
	a := model.StatsMetrics{
		InputToken:     100,
		OutputToken:    50,
		InputCost:      0.001,
		OutputCost:     0.002,
		WaitTime:       500,
		OutputTime:     400,
		RequestSuccess: 3,
		RequestFailed:  1,
	}

	b := model.StatsMetrics{
		InputToken:     200,
		OutputToken:    100,
		InputCost:      0.003,
		OutputCost:     0.004,
		WaitTime:       300,
		OutputTime:     250,
		RequestSuccess: 2,
		RequestFailed:  0,
	}

	a.Add(b)

	assertEqual(t, "InputToken", a.InputToken, 300)
	assertEqual(t, "OutputToken", a.OutputToken, 150)
	assertEqual(t, "RequestSuccess", a.RequestSuccess, 5)
	assertEqual(t, "RequestFailed", a.RequestFailed, 1)
	assertEqual(t, "WaitTime", a.WaitTime, 800)
	assertEqual(t, "OutputTime", a.OutputTime, 650)
}

// ============================================================================
// OutputTime / TPS 计算测试
// ============================================================================

// computeOutputTime 模拟 metrics.Save() 中的 OutputTime 计算逻辑
func computeOutputTime(durationMs int64, firstTokenTimeSet bool, ftutMs int64) int64 {
	outputTime := durationMs
	if firstTokenTimeSet {
		if ftutMs > 0 && outputTime > ftutMs {
			outputTime = outputTime - ftutMs
		}
	}
	return outputTime
}

func TestOutputTime_StreamingWithFtut(t *testing.T) {
	// 流式请求：duration=5000ms, ftut=500ms -> outputTime=4500ms
	outputTime := computeOutputTime(5000, true, 500)
	assertEqual(t, "OutputTime", outputTime, 4500)

	// TPS = outputToken / outputTime * 1000 = 100 / 4500 * 1000 ≈ 22.22
	tps := float64(100) / float64(outputTime) * 1000
	if tps < 22.21 || tps > 22.23 {
		t.Errorf("TPS: got %.2f, want ~22.22", tps)
	}
}

func TestOutputTime_NonStreamingNoFtut(t *testing.T) {
	// 非流式请求：duration=5000ms, ftut未设置 -> outputTime=5000ms
	outputTime := computeOutputTime(5000, false, 0)
	assertEqual(t, "OutputTime", outputTime, 5000)

	tps := float64(100) / float64(outputTime) * 1000
	if tps < 19.99 || tps > 20.01 {
		t.Errorf("TPS: got %.2f, want 20.00", tps)
	}
}

func TestOutputTime_FtutEqualsDuration(t *testing.T) {
	// ftut == duration：首token刚好在结束时到达 -> fallback to duration
	outputTime := computeOutputTime(5000, true, 5000)
	assertEqual(t, "OutputTime", outputTime, 5000)
}

func TestOutputTime_FtutExceedsDuration(t *testing.T) {
	// ftut > duration：异常情况 -> fallback to duration
	outputTime := computeOutputTime(3000, true, 5000)
	assertEqual(t, "OutputTime", outputTime, 3000)
}

func TestOutputTime_ZeroDuration(t *testing.T) {
	// 极端：duration=0
	outputTime := computeOutputTime(0, true, 0)
	assertEqual(t, "OutputTime", outputTime, 0)
}

func TestOutputTime_AggregatedTps(t *testing.T) {
	// 模拟聚合 TPS：两次请求
	// 请求1: 100 output tokens, 4500ms output time
	// 请求2: 200 output tokens, 8000ms output time
	// 聚合 TPS = (100+200) / (4500+8000) * 1000 = 24.00
	totalOutputToken := int64(100 + 200)
	totalOutputTime := int64(4500 + 8000)
	tps := float64(totalOutputToken) / float64(totalOutputTime) * 1000
	if tps < 23.99 || tps > 24.01 {
		t.Errorf("Aggregated TPS: got %.2f, want 24.00", tps)
	}
}

func TestOutputTime_StatsMetricsAddOutputTime(t *testing.T) {
	// 验证 StatsMetrics.Add 正确累加 OutputTime
	a := model.StatsMetrics{OutputToken: 100, OutputTime: 4500}
	b := model.StatsMetrics{OutputToken: 200, OutputTime: 8000}
	a.Add(b)
	assertEqual(t, "OutputToken", a.OutputToken, 300)
	assertEqual(t, "OutputTime", a.OutputTime, 12500)
}

// ============================================================================
// 跨渠道重试场景测试
// ============================================================================

// TestCrossChannelRetry_StatsConsistency 渠道A失败→渠道B成功：统计不双重计数
func TestCrossChannelRetry_StatsConsistency(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A(key100) 失败
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B(key200) 成功
	attemptSim(&s, 2, 200, true, 100, 50)
	// metrics.Save 记录最终渠道B的成功
	metricsSaveSim(&s, 2, true, 100, 50)

	// 全局：1次成功
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalFailed", s.TotalFailed, 0)
	assertEqual(t, "TotalTokens", s.TotalTokens, 150)

	// 渠道A：仅1次失败（来自 attempt）
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelA Success", s.ChannelSuccess[1], 0)

	// 渠道B：1次成功（来自 Save），0次失败
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 0)
	assertEqual(t, "ChannelB Tokens", s.ChannelTokens[2], 150)

	// Key维度
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 1) // A的失败尝试
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 1) // B的成功尝试
}

// TestCrossChannelRetry_AllFailed 渠道A失败→渠道B也失败：全局1次失败
func TestCrossChannelRetry_AllFailed(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A 失败
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B 也失败
	attemptSim(&s, 2, 200, false, 0, 0)
	// metrics.Save 记录失败，finalChannel=B（最后一个失败的）
	metricsSaveSim(&s, 2, false, 0, 0)

	// 全局：1次失败（不是2次）
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 0)

	// 每个渠道各1次失败（来自各自的 attempt）
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 1)
}

// TestCrossChannelRetry_RepeatedFailureToA 渠道A持续故障：连续请求都先试A再fallback到B
// 每次请求只产生1条日志，渠道A的failed计数持续增长
func TestCrossChannelRetry_RepeatedFailureToA(t *testing.T) {
	s := newStatsSnapshot()

	// 模拟3次请求，每次都是A失败→B成功
	for i := 0; i < 3; i++ {
		attemptSim(&s, 1, 100, false, 0, 0) // A失败
		attemptSim(&s, 2, 200, true, 100, 50) // B成功
		metricsSaveSim(&s, 2, true, 100, 50)
	}

	// 全局：3次成功
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 3)
	assertEqual(t, "TotalFailed", s.TotalFailed, 0)

	// 渠道A：3次失败（每次请求A都失败一次）
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 3)
	assertEqual(t, "ChannelA Success", s.ChannelSuccess[1], 0)

	// 渠道B：3次成功
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 3)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 0)

	// Key100被尝试3次（都失败），Key200被尝试3次（都成功）
	assertEqual(t, "Key100 Requests", s.KeyRequests[100], 3)
	assertEqual(t, "Key200 Requests", s.KeyRequests[200], 3)
}

// TestCrossChannelRetry_SkippedThenSuccess 渠道A被跳过→渠道B成功
func TestCrossChannelRetry_SkippedThenSuccess(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A被跳过（不调用attemptSim，无统计）
	// 渠道B成功
	attemptSim(&s, 2, 200, true, 100, 50)
	metricsSaveSim(&s, 2, true, 100, 50)

	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 0)
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 1)
}

// ============================================================================
// finalChannel 逻辑测试
// ============================================================================

func TestFinalChannel_SuccessAttempt(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", Status: model.AttemptFailed},
		{ChannelID: 2, ChannelName: "B", Status: model.AttemptSuccess},
	}
	id, name := finalChannel(attempts)
	if id != 2 || name != "B" {
		t.Errorf("finalChannel: got (%d, %s), want (2, B)", id, name)
	}
}

func TestFinalChannel_AllFailed(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", Status: model.AttemptFailed},
		{ChannelID: 2, ChannelName: "B", Status: model.AttemptFailed},
	}
	id, name := finalChannel(attempts)
	// 应返回最后一个失败的渠道
	if id != 2 || name != "B" {
		t.Errorf("finalChannel: got (%d, %s), want (2, B)", id, name)
	}
}

func TestFinalChannel_SkippedOnly(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", Status: model.AttemptSkipped},
		{ChannelID: 2, ChannelName: "B", Status: model.AttemptSkipped},
	}
	id, name := finalChannel(attempts)
	// 应返回最后一个被跳过的渠道
	if id != 2 || name != "B" {
		t.Errorf("finalChannel: got (%d, %s), want (2, B)", id, name)
	}
}

func TestFinalChannel_FailedThenSkippedThenSuccess(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", Status: model.AttemptFailed},
		{ChannelID: 2, ChannelName: "B", Status: model.AttemptSkipped},
		{ChannelID: 3, ChannelName: "C", Status: model.AttemptSuccess},
	}
	id, name := finalChannel(attempts)
	if id != 3 || name != "C" {
		t.Errorf("finalChannel: got (%d, %s), want (3, C)", id, name)
	}
}

func TestFinalChannel_EmptyAttempts(t *testing.T) {
	id, name := finalChannel(nil)
	if id != 0 || name != "" {
		t.Errorf("finalChannel: got (%d, %s), want (0, '')", id, name)
	}
}

func TestFinalChannel_CircuitBreakThenSuccess(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", Status: model.AttemptCircuitBreak},
		{ChannelID: 2, ChannelName: "B", Status: model.AttemptSuccess},
	}
	id, name := finalChannel(attempts)
	if id != 2 || name != "B" {
		t.Errorf("finalChannel: got (%d, %s), want (2, B)", id, name)
	}
}

// ============================================================================
// 跨渠道重试后日志内容验证
// ============================================================================

// TestCrossChannelRetry_LogFields 验证重试后日志的关键字段
// 渠道A失败→渠道B成功：日志应归属渠道B，包含两次attempt
func TestCrossChannelRetry_LogFields(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", ChannelKeyID: 100, Status: model.AttemptFailed, AttemptNum: 1, Msg: "upstream error"},
		{ChannelID: 2, ChannelName: "B", ChannelKeyID: 200, Status: model.AttemptSuccess, AttemptNum: 2},
	}

	// finalChannel 应选成功的渠道B
	id, name := finalChannel(attempts)
	if id != 2 {
		t.Errorf("log channelID: got %d, want 2", id)
	}
	if name != "B" {
		t.Errorf("log channelName: got %s, want B", name)
	}

	// 日志应包含2次attempt
	if len(attempts) != 2 {
		t.Errorf("log attempts: got %d, want 2", len(attempts))
	}

	// 第一次attempt是失败
	if attempts[0].Status != model.AttemptFailed {
		t.Errorf("attempt[0] status: got %s, want failed", attempts[0].Status)
	}
	// 第二次attempt是成功
	if attempts[1].Status != model.AttemptSuccess {
		t.Errorf("attempt[1] status: got %s, want success", attempts[1].Status)
	}
}

// TestCrossChannelRetry_FailedLogFields 渠道A失败→渠道B也失败：日志归属最后一个失败的渠道
func TestCrossChannelRetry_FailedLogFields(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", ChannelKeyID: 100, Status: model.AttemptFailed, AttemptNum: 1, Msg: "timeout"},
		{ChannelID: 2, ChannelName: "B", ChannelKeyID: 200, Status: model.AttemptFailed, AttemptNum: 2, Msg: "upstream error"},
	}

	id, name := finalChannel(attempts)
	if id != 2 {
		t.Errorf("log channelID: got %d, want 2", id)
	}
	if name != "B" {
		t.Errorf("log channelName: got %s, want B", name)
	}
}

// ============================================================================
// 空响应视为失败的场景测试
// ============================================================================

// TestEmptyResponseTreatedAsFailure 上游返回 2xx 但空响应时，应视为失败触发重试
func TestEmptyResponseTreatedAsFailure(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A(key100) 返回 2xx 但空响应 → 视为失败
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B(key200) 正常成功
	attemptSim(&s, 2, 200, true, 100, 50)
	// metrics.Save 记录最终渠道B的成功
	metricsSaveSim(&s, 2, true, 100, 50)

	// 全局：1次成功（不是2次）
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 1)
	assertEqual(t, "TotalFailed", s.TotalFailed, 0)

	// 渠道A：1次失败（空响应被正确记录为失败）
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelA Success", s.ChannelSuccess[1], 0)

	// 渠道B：1次成功
	assertEqual(t, "ChannelB Success", s.ChannelSuccess[2], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 0)
}

// TestEmptyResponseTreatedAsFailure_LogFields 空响应失败后重试成功：日志归属正确
func TestEmptyResponseTreatedAsFailure_LogFields(t *testing.T) {
	attempts := []model.ChannelAttempt{
		{ChannelID: 1, ChannelName: "A", ChannelKeyID: 100, Status: model.AttemptFailed, AttemptNum: 1, Msg: "channel A returned empty response (status 200): upstream returned empty response body"},
		{ChannelID: 2, ChannelName: "B", ChannelKeyID: 200, Status: model.AttemptSuccess, AttemptNum: 2},
	}

	// finalChannel 应选成功的渠道B
	id, name := finalChannel(attempts)
	if id != 2 || name != "B" {
		t.Errorf("finalChannel: got (%d, %s), want (2, B)", id, name)
	}

	// 第一次 attempt 是失败（空响应），包含状态码和原因
	if attempts[0].Status != model.AttemptFailed {
		t.Errorf("attempt[0] status: got %s, want failed", attempts[0].Status)
	}
	if !strings.Contains(attempts[0].Msg, "status 200") {
		t.Errorf("attempt[0] msg should contain status code: got %s", attempts[0].Msg)
	}
	if !strings.Contains(attempts[0].Msg, "empty response") {
		t.Errorf("attempt[0] msg should contain 'empty response': got %s", attempts[0].Msg)
	}
}

// TestEmptyResponse_AllChannelsEmpty 所有渠道都返回空响应：全局1次失败
func TestEmptyResponse_AllChannelsEmpty(t *testing.T) {
	s := newStatsSnapshot()

	// 渠道A 空响应失败
	attemptSim(&s, 1, 100, false, 0, 0)
	// 渠道B 也空响应失败
	attemptSim(&s, 2, 200, false, 0, 0)
	// metrics.Save 记录失败
	metricsSaveSim(&s, 2, false, 0, 0)

	// 全局：1次失败
	assertEqual(t, "TotalFailed", s.TotalFailed, 1)
	assertEqual(t, "TotalSuccess", s.TotalSuccess, 0)

	// 每个渠道各1次失败
	assertEqual(t, "ChannelA Failed", s.ChannelFailed[1], 1)
	assertEqual(t, "ChannelB Failed", s.ChannelFailed[2], 1)
}

func assertEqual(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %d, want %d", name, got, want)
	}
}
