package op

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

// newTestChannelWithKeys 创建一个带多个 key 的渠道并写入 DB + 缓存。
func newTestChannelWithKeys(t *testing.T, name string, keys []model.ChannelKey) *model.Channel {
	t.Helper()
	ctx := context.Background()
	setupTestDBAndCache(t)

	ch := &model.Channel{
		Name:    name,
		Enabled: true,
		BaseUrls: []model.BaseUrl{{URL: "https://example.com", Type: outbound.OutboundTypeOpenAIChat}},
		Keys:    keys,
	}
	if err := ChannelCreate(ch, ctx); err != nil {
		t.Fatalf("ChannelCreate: %v", err)
	}
	return ch
}

// channelKeyPtr 生成 ChannelKey 的指针值字段，便于构造更新请求。
func channelKeyPtr(v string) *string { return &v }

// ============================================================================
// ChannelGetKeys — 渠道内多 key 穷举的核心
// ============================================================================

func TestChannelGetKeys_FiltersDisabledAndEmpty(t *testing.T) {
	ch := newTestChannelWithKeys(t, "filter-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "k1"},
		{Enabled: true, ChannelKey: "k2"}, // 创建后再禁用（GORM default:true 会把 false 当零值）
		{Enabled: true, ChannelKey: ""},   // 空 key 被过滤
		{Enabled: true, ChannelKey: "k4"},
	})

	// 禁用 k2
	disabled := false
	if _, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID: ch.ID,
		KeysToUpdate: []model.ChannelKeyUpdateRequest{
			{ID: ch.Keys[1].ID, Enabled: &disabled},
		},
	}, context.Background()); err != nil {
		t.Fatalf("disable k2: %v", err)
	}

	got := ChannelGetKeys(ch.ID)
	if len(got) != 2 {
		t.Fatalf("expected 2 eligible keys, got %d", len(got))
	}
	if got[0].ChannelKey != "k1" && got[1].ChannelKey != "k1" {
		t.Error("k1 should be present")
	}
	if got[0].ChannelKey != "k4" && got[1].ChannelKey != "k4" {
		t.Error("k4 should be present")
	}
}

func TestChannelGetKeys_Filters429Cooldown(t *testing.T) {
	ch := newTestChannelWithKeys(t, "cooldown-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "k1"},
		{Enabled: true, ChannelKey: "k2", StatusCode: 429, LastUseTimeStamp: time.Now().Unix()},
	})

	got := ChannelGetKeys(ch.ID)
	if len(got) != 1 {
		t.Fatalf("expected 1 key (429 cooldown filtered), got %d", len(got))
	}
	if got[0].ChannelKey != "k1" {
		t.Errorf("expected k1, got %s", got[0].ChannelKey)
	}
}

func TestChannelGetKeys_LowestCostFirst(t *testing.T) {
	ch := newTestChannelWithKeys(t, "cost-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "expensive", TotalCost: 10.0},
		{Enabled: true, ChannelKey: "cheap", TotalCost: 1.0},
		{Enabled: true, ChannelKey: "mid", TotalCost: 5.0},
	})

	got := ChannelGetKeys(ch.ID)
	if len(got) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(got))
	}
	if got[0].ChannelKey != "cheap" {
		t.Errorf("expected cheap first, got %s", got[0].ChannelKey)
	}
	if got[1].ChannelKey != "mid" {
		t.Errorf("expected mid second, got %s", got[1].ChannelKey)
	}
	if got[2].ChannelKey != "expensive" {
		t.Errorf("expected expensive last, got %s", got[2].ChannelKey)
	}
}

func TestChannelGetKeys_TiedCostRotation(t *testing.T) {
	// 3 个同成本 key，轮询应让起始位置变化
	ch := newTestChannelWithKeys(t, "rotation-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "a", TotalCost: 1.0},
		{Enabled: true, ChannelKey: "b", TotalCost: 1.0},
		{Enabled: true, ChannelKey: "c", TotalCost: 1.0},
	})

	channelKeyRotationCounters.Delete(ch.ID)
	seen := map[string]int{}
	for i := 0; i < 9; i++ {
		got := ChannelGetKeys(ch.ID)
		if len(got) != 3 {
			t.Fatalf("iter %d: expected 3 keys, got %d", i, len(got))
		}
		seen[got[0].ChannelKey]++
	}

	// 轮询应让每个 key 都作为首位出现过
	if len(seen) != 3 {
		t.Errorf("rotation should cycle through all 3 keys as first, got %d distinct", len(seen))
	}
	for k, c := range seen {
		if c != 3 {
			t.Errorf("key %s: expected 3 first-picks, got %d", k, c)
		}
	}
}

func TestChannelGetKeys_EmptyChannel(t *testing.T) {
	ch := newTestChannelWithKeys(t, "empty-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "only"},
	})

	got := ChannelGetKeys(ch.ID)
	if len(got) != 1 {
		t.Fatalf("expected 1 key, got %d", len(got))
	}
	// 未知渠道返回 nil
	if unknown := ChannelGetKeys(999999); unknown != nil {
		t.Errorf("unknown channel should return nil, got %v", unknown)
	}
}

// ============================================================================
// ChannelGetKey — 薄封装，应返回 ChannelGetKeys 的首个
// ============================================================================

func TestChannelGetKey_ReturnsFirstFromGetKeys(t *testing.T) {
	ch := newTestChannelWithKeys(t, "getkey-test", []model.ChannelKey{
		{Enabled: true, ChannelKey: "first", TotalCost: 1.0},
		{Enabled: true, ChannelKey: "second", TotalCost: 5.0},
	})

	k := ChannelGetKey(ch.ID)
	if k.ChannelKey != "first" {
		t.Errorf("expected first (lowest cost), got %s", k.ChannelKey)
	}

	// 空渠道
	k2 := ChannelGetKey(999999)
	if k2.ChannelKey != "" {
		t.Errorf("unknown channel should return empty, got %s", k2.ChannelKey)
	}
}

// ============================================================================
// 问题 2 回归测试：remark 在 ChannelKeySaveDB 时不被覆盖
// ============================================================================

func TestChannelKeySaveDB_DoesNotOverwriteRemark(t *testing.T) {
	ctx := context.Background()
	ch := newTestChannelWithKeys(t, "remark-save", []model.ChannelKey{
		{Enabled: true, ChannelKey: "k1", Remark: "original"},
	})

	// 1. 用 ChannelUpdate 把 remark 改成 "updated"
	newRemark := "updated"
	if _, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID: ch.ID,
		KeysToUpdate: []model.ChannelKeyUpdateRequest{
			{ID: ch.Keys[0].ID, Remark: &newRemark},
		},
	}, ctx); err != nil {
		t.Fatalf("ChannelUpdate remark: %v", err)
	}

	// 2. 模拟转发请求更新统计，把 key 标记为 dirty（内存里 remark 仍是旧值）
	got, _ := ChannelGet(ch.ID, ctx)
	dirtyKey := got.Keys[0]
	dirtyKey.Remark = "original" // 内存里是旧 remark
	dirtyKey.StatusCode = 500
	dirtyKey.TotalRequests = 1
	if err := ChannelKeyUpdate(dirtyKey); err != nil {
		t.Fatalf("ChannelKeyUpdate: %v", err)
	}

	// 3. ChannelKeySaveDB 应只更新统计字段，不覆盖 remark
	if err := ChannelKeySaveDB(ctx); err != nil {
		t.Fatalf("ChannelKeySaveDB: %v", err)
	}

	// 4. 直接从 DB 读取（不走缓存），remark 应仍是 "updated"
	var dbKey model.ChannelKey
	if err := db.GetDB().Where("channel_id = ?", ch.ID).First(&dbKey).Error; err != nil {
		t.Fatalf("read key from DB: %v", err)
	}
	if dbKey.Remark != "updated" {
		t.Errorf("remark was overwritten by ChannelKeySaveDB: got %q, want %q", dbKey.Remark, "updated")
	}
	if dbKey.StatusCode != 500 {
		t.Errorf("status_code not persisted: got %d, want 500", dbKey.StatusCode)
	}
	if dbKey.TotalRequests != 1 {
		t.Errorf("total_requests not persisted: got %d, want 1", dbKey.TotalRequests)
	}
}

// ============================================================================
// 问题 2 回归测试：remark 在 channelRefreshCacheByID 时不被覆盖
// ============================================================================

func TestChannelRefreshCacheByID_DoesNotOverwriteRemark(t *testing.T) {
	ctx := context.Background()
	ch := newTestChannelWithKeys(t, "remark-refresh", []model.ChannelKey{
		{Enabled: true, ChannelKey: "k1", Remark: "original"},
	})

	// 1. ChannelUpdate 改 remark
	newRemark := "via-update"
	if _, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID: ch.ID,
		KeysToUpdate: []model.ChannelKeyUpdateRequest{
			{ID: ch.Keys[0].ID, Remark: &newRemark},
		},
	}, ctx); err != nil {
		t.Fatalf("ChannelUpdate remark: %v", err)
	}

	// 2. 把 key 标 dirty（内存 remark 仍是旧值）
	got, _ := ChannelGet(ch.ID, ctx)
	dirtyKey := got.Keys[0]
	dirtyKey.Remark = "original"
	dirtyKey.StatusCode = 502
	if err := ChannelKeyUpdate(dirtyKey); err != nil {
		t.Fatalf("ChannelKeyUpdate: %v", err)
	}

	// 3. 刷新缓存（会先落库 dirty key 再 reload）
	if err := channelRefreshCacheByID(ch.ID, ctx); err != nil {
		t.Fatalf("channelRefreshCacheByID: %v", err)
	}

	// 4. 缓存里 remark 应为 "via-update"
	ch2, _ := ChannelGet(ch.ID, ctx)
	if ch2.Keys[0].Remark != "via-update" {
		t.Errorf("remark overwritten during refresh: got %q, want %q", ch2.Keys[0].Remark, "via-update")
	}
	if ch2.Keys[0].StatusCode != 502 {
		t.Errorf("status_code not persisted during refresh: got %d, want 502", ch2.Keys[0].StatusCode)
	}
}

// 防止编译器警告（如果未来移除 setupTestDBAndCache）
var _ = filepath.Join
var _ = atomic.AddUint64
var _ = channelKeyPtr
