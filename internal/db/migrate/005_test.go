package migrate

import (
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestReproduceOnConflictBug 模拟线上 ON CONFLICT 错误
func TestReproduceOnConflictBug(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.AutoMigrate(&model.StatsHourly{})

	// 模拟线上 bug：重建表为只有单列主键
	simulateBrokenTable(t, db, "stats_hourlies",
		"CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date))")

	// 验证 OnConflict 失败
	hourly1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 5}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&hourly1).Error
	if err == nil {
		t.Fatal("OnConflict should fail with broken PK")
	}
	t.Logf("OnConflict failed as expected: %v", err)

	// 运行迁移修复
	if fixErr := fixStatsTablesCompositePK(db); fixErr != nil {
		t.Fatalf("migration failed: %v", fixErr)
	}

	// 验证修复后 upsert 成功
	hourly2 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 8}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&hourly2).Error
	if err != nil {
		t.Fatalf("OnConflict upsert still fails after migration: %v", err)
	}
	t.Log("OnConflict upsert works after migration!")
}

// TestUniqueIndexSavesOnConflict 测试 UNIQUE INDEX 作为兜底方案
// 即使 AutoMigrate 破坏了复合主键，UNIQUE INDEX 仍然能让 OnConflict 工作
func TestUniqueIndexSavesOnConflict(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// 创建只有单列主键的表（模拟 AutoMigrate 破坏后的状态）
	db.Exec("DROP TABLE IF EXISTS stats_hourlies")
	db.Exec(`CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date))`)

	// 没有 UNIQUE INDEX 时，OnConflict 应该失败
	h1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 5}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h1).Error
	if err == nil {
		t.Log("OnConflict unexpectedly succeeded without UNIQUE INDEX")
	} else {
		t.Logf("OnConflict failed without UNIQUE INDEX as expected: %v", err)
	}

	// 创建 UNIQUE INDEX
	if err := ensureUniqueIndexForCompositePK(db, "stats_hourlies", []string{"date", "hour"}); err != nil {
		t.Fatalf("create unique index: %v", err)
	}

	// 有 UNIQUE INDEX 后，OnConflict 应该成功
	h2 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 8}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h2).Error
	if err != nil {
		t.Fatalf("OnConflict failed even with UNIQUE INDEX: %v", err)
	}
	t.Log("OnConflict works with UNIQUE INDEX!")

	var got model.StatsHourly
	db.Where("date = ? AND hour = ?", "20260504", 10).First(&got)
	if got.RequestSuccess != 8 {
		t.Errorf("RequestSuccess = %d, want 8", got.RequestSuccess)
	}
}

// TestAutoMigrateBreaksPKThenEnsureFixes 验证完整流程：
// AutoMigrate 破坏 PK → EnsureStatsCompositePK 修复 → OnConflict 成功
func TestAutoMigrateBreaksPKThenEnsureFixes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// 模拟线上数据库：只有单列主键
	db.Exec("DROP TABLE IF EXISTS stats_hourlies")
	db.Exec(`CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date))`)

	// 模拟启动流程：AutoMigrate → EnsureStatsCompositePK
	db.AutoMigrate(&model.StatsHourly{})
	EnsureStatsCompositePK(db)

	// 验证 upsert 成功
	h := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 1}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h).Error
	if err != nil {
		t.Fatalf("upsert after EnsureStatsCompositePK failed: %v", err)
	}

	// 再模拟一次重启
	db.AutoMigrate(&model.StatsHourly{})
	EnsureStatsCompositePK(db)

	// upsert 仍然成功
	h2 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 3}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h2).Error
	if err != nil {
		t.Fatalf("upsert after 2nd restart failed: %v", err)
	}

	var got model.StatsHourly
	db.Where("date = ? AND hour = ?", "20260504", 10).First(&got)
	if got.RequestSuccess != 3 {
		t.Errorf("RequestSuccess = %d, want 3", got.RequestSuccess)
	}
	t.Log("Full flow works: AutoMigrate + EnsureStatsCompositePK on every restart!")
}

// TestEnsureStatsCompositePKAllTables 验证 EnsureStatsCompositePK 修复所有三个表
func TestEnsureStatsCompositePKAllTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.AutoMigrate(&model.StatsHourly{}, &model.StatsAPIKeyDaily{}, &model.StatsAPIKeyHourly{})

	// 模拟所有表 PK 被破坏
	simulateBrokenTable(t, db, "stats_hourlies",
		"CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date))")
	simulateBrokenTable(t, db, "stats_api_key_dailies",
		"CREATE TABLE stats_api_key_dailies (api_key_id INTEGER NOT NULL, date TEXT NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (api_key_id))")
	simulateBrokenTable(t, db, "stats_api_key_hourlies",
		"CREATE TABLE stats_api_key_hourlies (api_key_id INTEGER NOT NULL, date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (api_key_id))")

	EnsureStatsCompositePK(db)

	// 验证所有 upsert 成功
	h := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 1}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h).Error
	if err != nil {
		t.Fatalf("stats_hourlies: %v", err)
	}

	d := model.StatsAPIKeyDaily{APIKeyID: 1, Date: "20260504", StatsMetrics: model.StatsMetrics{RequestSuccess: 1}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "api_key_id"}, {Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&d).Error
	if err != nil {
		t.Fatalf("stats_api_key_dailies: %v", err)
	}

	ah := model.StatsAPIKeyHourly{APIKeyID: 1, Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 1}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "api_key_id"}, {Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&ah).Error
	if err != nil {
		t.Fatalf("stats_api_key_hourlies: %v", err)
	}

	t.Log("All tables fixed by EnsureStatsCompositePK!")
}

func simulateBrokenTable(t *testing.T, db *gorm.DB, args ...string) {
	t.Helper()
	if len(args) == 0 {
		args = []string{"stats_hourlies", "CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date))"}
	}
	table := args[0]
	createSQL := args[1]

	tmpTable := table + "_tmp_broken"
	db.Exec("DROP TABLE IF EXISTS " + tmpTable)
	if err := db.Exec("ALTER TABLE " + table + " RENAME TO " + tmpTable).Error; err != nil {
		t.Fatalf("rename %s: %v", table, err)
	}
	if err := db.Exec(createSQL).Error; err != nil {
		t.Fatalf("create broken %s: %v", table, err)
	}
	db.Exec("DROP TABLE IF EXISTS " + tmpTable)
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
