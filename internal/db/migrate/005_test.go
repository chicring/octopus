package migrate

import (
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestFullProductionFlow 模拟线上完整启动流程：
// 旧版本创建错误表 → 新版本 InitDB → AutoMigrate → AfterAutoMigrate → EnsureStatsCompositePK → StatsSaveDB
func TestFullProductionFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "octopus.db")

	// ===== 第一次启动：旧版本创建的数据库（只有单列主键） =====
	oldDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// 模拟线上 bug：只有单列主键
	oldDB.Exec("DROP TABLE IF EXISTS stats_hourlies")
	oldDB.Exec(`CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint, PRIMARY KEY (date))`)
	oldDB.Exec("INSERT INTO stats_hourlies (date, hour, request_success) VALUES ('20260504', 10, 5)")

	// 确认 OnConflict 失败
	h1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 8}}
	err = oldDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h1).Error
	if err == nil {
		t.Fatal("OnConflict should fail with broken PK")
	}
	t.Logf("Old DB: OnConflict failed: %v", err)

	pks := getSQLitePKColumns(t, oldDB, "stats_hourlies")
	t.Logf("Old DB PK: %v", pks)

	sqlDB, _ := oldDB.DB()
	sqlDB.Close()

	// ===== 第二次启动：新版本 v1.2.6 =====
	newDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// 模拟 InitDB 完整流程
	t.Log("--- Simulating InitDB ---")

	// 1. BeforeAutoMigrate
	if err := BeforeAutoMigrate(newDB); err != nil {
		t.Fatalf("BeforeAutoMigrate: %v", err)
	}

	// 2. AutoMigrate
	if err := newDB.AutoMigrate(
		&model.StatsTotal{},
		&model.StatsDaily{},
		&model.StatsHourly{},
		&model.StatsModel{},
		&model.StatsChannel{},
		&model.StatsAPIKey{},
		&model.StatsAPIKeyDaily{},
		&model.StatsAPIKeyHourly{},
		&MigrationRecord{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	pks2 := getSQLitePKColumns(t, newDB, "stats_hourlies")
	t.Logf("After AutoMigrate PK: %v", pks2)

	var ddl string
	newDB.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='stats_hourlies'").Scan(&ddl)
	t.Logf("After AutoMigrate DDL: %s", ddl)

	// 3. AfterAutoMigrate
	if err := AfterAutoMigrate(newDB); err != nil {
		t.Fatalf("AfterAutoMigrate: %v", err)
	}

	pks3 := getSQLitePKColumns(t, newDB, "stats_hourlies")
	t.Logf("After AfterAutoMigrate PK: %v", pks3)

	// 4. EnsureStatsCompositePK
	EnsureStatsCompositePK(newDB)

	pks4 := getSQLitePKColumns(t, newDB, "stats_hourlies")
	t.Logf("After EnsureStatsCompositePK PK: %v", pks4)

	// 检查 UNIQUE INDEX
	var idxCount int64
	newDB.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_stats_hourlies_pk'").Scan(&idxCount)
	t.Logf("UNIQUE INDEX exists: %v", idxCount > 0)

	// 5. StatsSaveDB：OnConflict upsert
	h2 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 8}}
	err = newDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h2).Error
	if err != nil {
		t.Fatalf("OnConflict upsert FAILED after full fix: %v", err)
	}
	t.Log("OnConflict upsert works!")

	// ===== 第三次启动：模拟重启 =====
	sqlDB2, _ := newDB.DB()
	sqlDB2.Close()

	restartDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	BeforeAutoMigrate(restartDB)
	restartDB.AutoMigrate(
		&model.StatsTotal{},
		&model.StatsDaily{},
		&model.StatsHourly{},
		&model.StatsModel{},
		&model.StatsChannel{},
		&model.StatsAPIKey{},
		&model.StatsAPIKeyDaily{},
		&model.StatsAPIKeyHourly{},
		&MigrationRecord{},
	)
	AfterAutoMigrate(restartDB)
	EnsureStatsCompositePK(restartDB)

	pks5 := getSQLitePKColumns(t, restartDB, "stats_hourlies")
	t.Logf("After restart PK: %v", pks5)

	restartDB.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_stats_hourlies_pk'").Scan(&idxCount)
	t.Logf("After restart UNIQUE INDEX exists: %v", idxCount > 0)

	h3 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 12}}
	err = restartDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h3).Error
	if err != nil {
		t.Fatalf("OnConflict upsert FAILED after restart: %v", err)
	}
	t.Log("OnConflict upsert works after restart!")

	sqlDB3, _ := restartDB.DB()
	sqlDB3.Close()
}

func getSQLitePKColumns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", table).Rows()
	if err != nil {
		t.Fatalf("pragma_table_info for %s: %v", table, err)
	}
	defer rows.Close()
	var pks []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		pks = append(pks, name)
	}
	return pks
}
