package op

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestStatsModelLegacySchemaMigratesBeforeSave(t *testing.T) {
	tests := []struct {
		name       string
		createSQL  string
		insertSQL  string
		wantError  string
		wantInput  int64
		wantReqOK  int64
		updateName string
	}{
		{
			name: "id_pk_without_name_unique",
			createSQL: `CREATE TABLE stats_models (
				id integer PRIMARY KEY AUTOINCREMENT,
				name text NOT NULL,
				channel_id integer NOT NULL,
				input_token bigint DEFAULT 0,
				output_token bigint DEFAULT 0,
				input_cost real DEFAULT 0,
				output_cost real DEFAULT 0,
				wait_time bigint DEFAULT 0,
				output_time bigint DEFAULT 0,
				request_success bigint DEFAULT 0,
				request_failed bigint DEFAULT 0
			)`,
			insertSQL: `INSERT INTO stats_models (name, channel_id, input_token, request_success) VALUES
				('gpt-4', 1, 10, 2),
				('gpt-4', 2, 20, 3)`,
			wantError:  "ON CONFLICT clause does not match",
			wantInput:  37,
			wantReqOK:  6,
			updateName: "gpt-4",
		},
		{
			name: "name_unique_with_channel_id_not_null",
			createSQL: `CREATE TABLE stats_models (
				id integer PRIMARY KEY AUTOINCREMENT,
				name text NOT NULL UNIQUE,
				channel_id integer NOT NULL,
				input_token bigint DEFAULT 0,
				output_token bigint DEFAULT 0,
				input_cost real DEFAULT 0,
				output_cost real DEFAULT 0,
				wait_time bigint DEFAULT 0,
				output_time bigint DEFAULT 0,
				request_success bigint DEFAULT 0,
				request_failed bigint DEFAULT 0
			)`,
			insertSQL:  `INSERT INTO stats_models (name, channel_id, input_token, request_success) VALUES ('claude', 1, 11, 4)`,
			wantError:  "NOT NULL constraint failed: stats_models.channel_id",
			wantInput:  18,
			wantReqOK:  5,
			updateName: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "legacy_stats_models.db")
			legacyDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
			if err != nil {
				t.Fatalf("open legacy db: %v", err)
			}
			if err := legacyDB.Exec(tt.createSQL).Error; err != nil {
				t.Fatalf("create legacy stats_models: %v", err)
			}
			if err := legacyDB.Exec(tt.insertSQL).Error; err != nil {
				t.Fatalf("insert legacy stats_models: %v", err)
			}

			legacyWrite := model.StatsModel{Name: tt.updateName, StatsMetrics: model.StatsMetrics{InputToken: 7, RequestSuccess: 1}}
			err = legacyDB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{"input_token", "request_success"}),
			}).Create(&legacyWrite).Error
			if err == nil {
				t.Fatal("legacy stats_models upsert should fail before migration")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("legacy error = %v, want substring %q", err, tt.wantError)
			}

			legacySQL, _ := legacyDB.DB()
			legacySQL.Close()

			resetStatsCachesForTest()
			if err := db.InitDB("sqlite", dbPath, false); err != nil {
				t.Fatalf("db init: %v", err)
			}

			gormDB := db.GetDB()
			assertSQLiteColumnAbsent(t, gormDB, "stats_models", "channel_id")
			assertSQLiteColumnAbsent(t, gormDB, "stats_models", "id")
			pks := getSQLitePKColumnsForOpTest(t, gormDB, "stats_models")
			if len(pks) != 1 || pks[0] != "name" {
				t.Fatalf("stats_models PK = %v, want [name]", pks)
			}

			if err := statsRefreshCache(context.Background()); err != nil {
				t.Fatalf("statsRefreshCache: %v", err)
			}
			if err := StatsModelUpdate(tt.updateName, model.StatsMetrics{InputToken: 7, RequestSuccess: 1}); err != nil {
				t.Fatalf("StatsModelUpdate: %v", err)
			}
			if err := StatsSaveDB(context.Background()); err != nil {
				t.Fatalf("StatsSaveDB: %v", err)
			}

			var got model.StatsModel
			if err := gormDB.Where("name = ?", tt.updateName).First(&got).Error; err != nil {
				t.Fatalf("query migrated stats model: %v", err)
			}
			if got.InputToken != tt.wantInput || got.RequestSuccess != tt.wantReqOK {
				t.Fatalf("merged stats = input_token:%d request_success:%d, want %d/%d", got.InputToken, got.RequestSuccess, tt.wantInput, tt.wantReqOK)
			}

			sqlDB, _ := gormDB.DB()
			sqlDB.Close()
		})
	}
}

func resetStatsCachesForTest() {
	statsEnsureOnce = sync.Once{}

	statsDailyCacheLock.Lock()
	statsDailyCache = model.StatsDaily{}
	statsDailyCacheLock.Unlock()

	statsTotalCacheLock.Lock()
	statsTotalCache = model.StatsTotal{}
	statsTotalCacheLock.Unlock()

	statsHourlyCache.Clear()
	statsHourlyCacheNeedUpdateLock.Lock()
	statsHourlyCacheNeedUpdate = make(map[hourlyKey]struct{})
	statsHourlyCacheNeedUpdateLock.Unlock()

	statsChannelCache.Clear()
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCache.Clear()
	statsModelCacheNeedUpdateLock.Lock()
	statsModelCacheNeedUpdate = make(map[string]struct{})
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCache.Clear()
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	statsAPIKeyDailyCacheLock.Lock()
	statsAPIKeyDailyCache.Clear()
	statsAPIKeyDailyCacheLock.Unlock()
	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	statsAPIKeyDailyCacheNeedUpdate = make(map[apiKeyDailyKey]struct{})
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	statsAPIKeyHourlyCacheLock.Lock()
	statsAPIKeyHourlyCache.Clear()
	statsAPIKeyHourlyCacheLock.Unlock()
	statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
	statsAPIKeyHourlyCacheNeedUpdate = make(map[apiKeyHourlyKey]struct{})
	statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()
}

func assertSQLiteColumnAbsent(t *testing.T, db *gorm.DB, table, column string) {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT count(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count).Error; err != nil {
		t.Fatalf("pragma_table_info %s.%s: %v", table, column, err)
	}
	if count != 0 {
		t.Fatalf("column %s.%s still exists", table, column)
	}
}

func getSQLitePKColumnsForOpTest(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", table).Rows()
	if err != nil {
		t.Fatalf("pragma_table_info for %s: %v", table, err)
	}
	defer rows.Close()

	var pks []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan pk column: %v", err)
		}
		pks = append(pks, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pk columns: %v", err)
	}
	return pks
}
