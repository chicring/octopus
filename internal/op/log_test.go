package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func TestRelayLogList_apiKeyNamesFilter(t *testing.T) {
	ctx := context.Background()
	setupLogTestDB(t)

	// 填充缓存（ID 越大 = 时间越新，返回时按缓存倒序）
	relayLogCacheLock.Lock()
	relayLogCache = []model.RelayLog{
		{ID: 1, Time: 100, RequestAPIKeyName: "key-alpha", RequestModelName: "gpt-4"},
		{ID: 2, Time: 99, RequestAPIKeyName: "key-beta", RequestModelName: "gpt-4"},
		{ID: 3, Time: 98, RequestAPIKeyName: "key-alpha", RequestModelName: "claude-3"},
		{ID: 4, Time: 97, RequestAPIKeyName: "key-gamma", RequestModelName: "gpt-4", Error: "timeout"},
		{ID: 5, Time: 96, RequestAPIKeyName: "", RequestModelName: "gpt-4"},
	}
	relayLogCacheLock.Unlock()

	tests := []struct {
		name        string
		apiKeyNames []string
		modelNames  []string
		hasError    bool
		expectedIDs []int64
	}{
		{
			name:        "filter by single api key name",
			apiKeyNames: []string{"key-alpha"},
			expectedIDs: []int64{3, 1}, // 反转缓存顺序
		},
		{
			name:        "filter by multiple api key names",
			apiKeyNames: []string{"key-alpha", "key-gamma"},
			expectedIDs: []int64{4, 3, 1},
		},
		{
			name:        "filter by single model name",
			modelNames:  []string{"gpt-4"},
			expectedIDs: []int64{5, 4, 2, 1},
		},
		{
			name:        "filter by multiple model names",
			modelNames:  []string{"gpt-4", "claude-3"},
			expectedIDs: []int64{5, 4, 3, 2, 1},
		},
		{
			name:        "filter by api key name and model name",
			apiKeyNames: []string{"key-alpha"},
			modelNames:  []string{"gpt-4"},
			expectedIDs: []int64{1},
		},
		{
			name:        "filter by api key name and hasError",
			apiKeyNames: []string{"key-gamma"},
			hasError:    true,
			expectedIDs: []int64{4},
		},
		{
			name:        "no filter returns all",
			expectedIDs: []int64{5, 4, 3, 2, 1},
		},
		{
			name:        "filter by non-existent api key name",
			apiKeyNames: []string{"nonexistent"},
			expectedIDs: []int64{},
		},
		{
			name:        "empty api key name matches logs with empty key name",
			apiKeyNames: []string{""},
			expectedIDs: []int64{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, err := RelayLogList(ctx, nil, nil, 1, 100, tt.hasError, tt.apiKeyNames, tt.modelNames)
			if err != nil {
				t.Fatalf("RelayLogList() error = %v", err)
			}

			gotIDs := idsOf(logs)
			if len(gotIDs) != len(tt.expectedIDs) {
				t.Fatalf("expected IDs %v, got %v", tt.expectedIDs, gotIDs)
			}
			for i, id := range gotIDs {
				if id != tt.expectedIDs[i] {
					t.Errorf("index %d: expected ID %d, got %d", i, tt.expectedIDs[i], id)
				}
			}
		})
	}
}

func TestRelayLogList_modelNamesFilter(t *testing.T) {
	ctx := context.Background()
	setupLogTestDB(t)

	relayLogCacheLock.Lock()
	relayLogCache = []model.RelayLog{
		{ID: 10, Time: 100, RequestAPIKeyName: "key-a", RequestModelName: "gpt-4o"},
		{ID: 11, Time: 99, RequestAPIKeyName: "key-a", RequestModelName: "gpt-4"},
		{ID: 12, Time: 98, RequestAPIKeyName: "key-b", RequestModelName: "claude-3-opus"},
		{ID: 13, Time: 97, RequestAPIKeyName: "key-b", RequestModelName: "claude-3-sonnet"},
	}
	relayLogCacheLock.Unlock()

	tests := []struct {
		name        string
		modelNames  []string
		expectedIDs []int64
	}{
		{
			name:        "single model",
			modelNames:  []string{"gpt-4o"},
			expectedIDs: []int64{10},
		},
		{
			name:        "multiple models",
			modelNames:  []string{"gpt-4o", "claude-3-opus"},
			expectedIDs: []int64{12, 10}, // 反转缓存顺序
		},
		{
			name:        "non-existent model",
			modelNames:  []string{"nonexistent"},
			expectedIDs: []int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, err := RelayLogList(ctx, nil, nil, 1, 100, false, nil, tt.modelNames)
			if err != nil {
				t.Fatalf("RelayLogList() error = %v", err)
			}

			gotIDs := idsOf(logs)
			if len(gotIDs) != len(tt.expectedIDs) {
				t.Fatalf("expected IDs %v, got %v", tt.expectedIDs, gotIDs)
			}
			for i, id := range gotIDs {
				if id != tt.expectedIDs[i] {
					t.Errorf("index %d: expected ID %d, got %d", i, tt.expectedIDs[i], id)
				}
			}
		})
	}
}

func TestRelayLogList_combinedFilter(t *testing.T) {
	ctx := context.Background()
	setupLogTestDB(t)

	relayLogCacheLock.Lock()
	relayLogCache = []model.RelayLog{
		{ID: 20, Time: 100, RequestAPIKeyName: "admin", RequestModelName: "gpt-4"},
		{ID: 21, Time: 99, RequestAPIKeyName: "admin", RequestModelName: "gpt-4", Error: "rate limit"},
		{ID: 22, Time: 98, RequestAPIKeyName: "user", RequestModelName: "gpt-4"},
		{ID: 23, Time: 97, RequestAPIKeyName: "admin", RequestModelName: "claude-3"},
		{ID: 24, Time: 96, RequestAPIKeyName: "user", RequestModelName: "claude-3", Error: "timeout"},
	}
	relayLogCacheLock.Unlock()

	// admin key + error only → ID 21
	logs, err := RelayLogList(ctx, nil, nil, 1, 100, true, []string{"admin"}, nil)
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 1 || logs[0].ID != 21 {
		t.Errorf("expected [21], got IDs %v", idsOf(logs))
	}

	// admin key + gpt-4 model → ID 21, 20 (reversed cache order)
	logs, err = RelayLogList(ctx, nil, nil, 1, 100, false, []string{"admin"}, []string{"gpt-4"})
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}

	// user key + claude-3 model + error → ID 24
	logs, err = RelayLogList(ctx, nil, nil, 1, 100, true, []string{"user"}, []string{"claude-3"})
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 1 || logs[0].ID != 24 {
		t.Errorf("expected [24], got IDs %v", idsOf(logs))
	}
}

func TestRelayLogList_pagination(t *testing.T) {
	ctx := context.Background()
	setupLogTestDB(t)

	relayLogCacheLock.Lock()
	relayLogCache = make([]model.RelayLog, 5)
	for i := 0; i < 5; i++ {
		relayLogCache[i] = model.RelayLog{
			ID:                int64(i + 1),
			Time:              int64(100 - i),
			RequestAPIKeyName: "key-a",
			RequestModelName:  "gpt-4",
		}
	}
	relayLogCacheLock.Unlock()

	// 不筛选，page=1, pageSize=2
	logs, err := RelayLogList(ctx, nil, nil, 1, 2, false, nil, nil)
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("page 1: expected 2 logs, got %d", len(logs))
	}

	// 筛选 api key，page=1, pageSize=2
	logs, err = RelayLogList(ctx, nil, nil, 1, 2, false, []string{"key-a"}, nil)
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("filtered page 1: expected 2 logs, got %d", len(logs))
	}

	// 筛选不匹配的 api key，应返回 0 条
	logs, err = RelayLogList(ctx, nil, nil, 1, 2, false, []string{"nonexistent"}, nil)
	if err != nil {
		t.Fatalf("RelayLogList() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("non-matching filter: expected 0 logs, got %d", len(logs))
	}
}

func idsOf(logs []model.RelayLog) []int64 {
	ids := make([]int64, len(logs))
	for i, l := range logs {
		ids[i] = l.ID
	}
	return ids
}

func setupLogTestDB(t *testing.T) {
	t.Helper()

	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "log-test.db"), false); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		relayLogCacheLock.Lock()
		relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
		relayLogCacheLock.Unlock()
	})

	if err := InitCache(); err != nil {
		t.Fatalf("InitCache() error = %v", err)
	}
}
