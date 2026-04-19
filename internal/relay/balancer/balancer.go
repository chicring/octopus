package balancer

import (
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/bestruirui/octopus/internal/model"
)

// Balancer 负载均衡策略接口
type Balancer interface {
	// Candidates 返回按策略排序的候选列表
	// groupID 用于 per-group 计数器隔离（RoundRobin 需要）
	Candidates(groupID int, items []model.GroupItem) []model.GroupItem
}

// GetBalancer 根据模式返回对应的负载均衡器
func GetBalancer(mode model.GroupMode) Balancer {
	switch mode {
	case model.GroupModeRoundRobin:
		return &RoundRobin{}
	case model.GroupModeRandom:
		return &Random{}
	case model.GroupModeFailover:
		return &Failover{}
	case model.GroupModeWeighted:
		return &Weighted{}
	default:
		return &RoundRobin{}
	}
}

// RoundRobin 轮询：per-group 计数器，从上次位置开始轮转排列
type RoundRobin struct{}

var groupRoundRobinCounters sync.Map // map[int]*uint64

func getGroupRoundRobinCounter(groupID int) *uint64 {
	if v, ok := groupRoundRobinCounters.Load(groupID); ok {
		return v.(*uint64)
	}
	newCounter := new(uint64)
	actual, _ := groupRoundRobinCounters.LoadOrStore(groupID, newCounter)
	return actual.(*uint64)
}

func (b *RoundRobin) Candidates(groupID int, items []model.GroupItem) []model.GroupItem {
	n := len(items)
	if n == 0 {
		return nil
	}
	counter := getGroupRoundRobinCounter(groupID)
	idx := int(atomic.AddUint64(counter, 1)%uint64(n) - 1)
	if idx < 0 {
		idx = n - 1
	}
	result := make([]model.GroupItem, n)
	for i := 0; i < n; i++ {
		result[i] = items[(idx+i)%n]
	}
	return result
}

// Random 随机：随机打乱所有 items
type Random struct{}

func (b *Random) Candidates(_ int, items []model.GroupItem) []model.GroupItem {
	n := len(items)
	if n == 0 {
		return nil
	}
	result := make([]model.GroupItem, n)
	copy(result, items)
	rand.Shuffle(n, func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})
	return result
}

// Failover 故障转移：按优先级排序
type Failover struct{}

func (b *Failover) Candidates(_ int, items []model.GroupItem) []model.GroupItem {
	if len(items) == 0 {
		return nil
	}
	return sortByPriority(items)
}

// Weighted 加权分配：按权重概率排序
type Weighted struct{}

func (b *Weighted) Candidates(_ int, items []model.GroupItem) []model.GroupItem {
	n := len(items)
	if n == 0 {
		return nil
	}

	// 构建加权随机排序
	type weightedItem struct {
		item  model.GroupItem
		score float64
	}

	totalWeight := 0
	for _, item := range items {
		w := item.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	scored := make([]weightedItem, n)
	for i, item := range items {
		w := item.Weight
		if w <= 0 {
			w = 1
		}
		// 给每个 item 一个加权随机分数：weight/totalWeight 作为概率基础，加上随机扰动
		scored[i] = weightedItem{
			item:  item,
			score: rand.Float64() * float64(w) / float64(totalWeight),
		}
	}

	// 按分数降序排列（分数越高优先级越高）
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]model.GroupItem, n)
	for i := range scored {
		result[i] = scored[i].item
	}
	return result
}

func sortByPriority(items []model.GroupItem) []model.GroupItem {
	sorted := make([]model.GroupItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted
}
