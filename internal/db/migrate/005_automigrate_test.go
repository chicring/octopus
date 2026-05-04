package migrate

import (
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestAutoMigrateBreaksFixedPK 测试 AutoMigrate 是否会破坏已修复的复合主键
// 这是线上最可能的场景：迁移 004 修复了 PK → AutoMigrate 又破坏了 PK
func TestAutoMigrateBreaksFixedPK(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// 场景1: 迁移 004 修复后的表（无空格格式）
	db.Exec(`CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date,hour))`)

	pks := getSQLitePKColumns(t, db, "stats_hourlies")
	t.Logf("Scene1 Before AutoMigrate PK: %v", pks)

	if err := db.AutoMigrate(&model.StatsHourly{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	pks2 := getSQLitePKColumns(t, db, "stats_hourlies")
	t.Logf("Scene1 After AutoMigrate PK: %v", pks2)

	// 场景2: 迁移 004 修复后的表（有空格格式，和 004.go 的 SQL 一样）
	db2Path := filepath.Join(t.TempDir(), "test2.db")
	db2, err := gorm.Open(sqlite.Open(db2Path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db2.Exec(`CREATE TABLE stats_hourlies (
		date TEXT NOT NULL,
		hour INTEGER NOT NULL,
		input_token INTEGER,
		output_token INTEGER,
		input_cost REAL,
		output_cost REAL,
		wait_time INTEGER,
		output_time INTEGER,
		request_success INTEGER,
		request_failed INTEGER,
		PRIMARY KEY (date, hour)
	)`)

	pks3 := getSQLitePKColumns(t, db2, "stats_hourlies")
	t.Logf("Scene2 Before AutoMigrate PK: %v", pks3)

	if err := db2.AutoMigrate(&model.StatsHourly{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	pks4 := getSQLitePKColumns(t, db2, "stats_hourlies")
	t.Logf("Scene2 After AutoMigrate PK: %v", pks4)

	var ddl string
	db2.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='stats_hourlies'").Scan(&ddl)
	t.Logf("Scene2 After AutoMigrate DDL: %s", ddl)

	// 场景3: 旧版本创建的表（只有 date 单列 PK，bigint 类型）
	// 这是线上最可能的实际状态
	db3Path := filepath.Join(t.TempDir(), "test3.db")
	db3, err := gorm.Open(sqlite.Open(db3Path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db3.Exec(`CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint, PRIMARY KEY (date))`)

	pks5 := getSQLitePKColumns(t, db3, "stats_hourlies")
	t.Logf("Scene3 Before AutoMigrate PK: %v", pks5)

	// 先运行迁移 004（BeforeAutoMigrate）
	if err := fixStatsHourlyCompositePKSQLite(db3); err != nil {
		t.Fatalf("fixStatsHourlyCompositePKSQLite: %v", err)
	}

	pks6 := getSQLitePKColumns(t, db3, "stats_hourlies")
	t.Logf("Scene3 After migration 004 PK: %v", pks6)

	// 然后运行 AutoMigrate
	if err := db3.AutoMigrate(&model.StatsHourly{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	pks7 := getSQLitePKColumns(t, db3, "stats_hourlies")
	t.Logf("Scene3 After AutoMigrate PK: %v", pks7)

	db3.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='stats_hourlies'").Scan(&ddl)
	t.Logf("Scene3 After AutoMigrate DDL: %s", ddl)

	// 测试 OnConflict
	h1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 5}}
	err = db3.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h1).Error
	if err != nil {
		t.Errorf("OnConflict failed: %v", err)
	}
}

// TestAutoMigrateOnCorrectTable 测试 AutoMigrate 在正确 PK 的表上的行为（模拟重启）
func TestAutoMigrateOnCorrectTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// 先用 AutoMigrate 创建正确的表
	if err := db.AutoMigrate(&model.StatsHourly{}); err != nil {
		t.Fatal(err)
	}

	pks := getSQLitePKColumns(t, db, "stats_hourlies")
	t.Logf("First AutoMigrate PK: %v", pks)

	// 再运行一次 AutoMigrate（模拟重启）
	if err := db.AutoMigrate(&model.StatsHourly{}); err != nil {
		t.Fatal(err)
	}

	pks2 := getSQLitePKColumns(t, db, "stats_hourlies")
	t.Logf("Second AutoMigrate PK: %v", pks2)

	var ddl string
	db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='stats_hourlies'").Scan(&ddl)
	t.Logf("Second AutoMigrate DDL: %s", ddl)

	h1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 5}}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
		DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
	}).Create(&h1).Error
	if err != nil {
		t.Errorf("OnConflict failed: %v", err)
	}
}

// TestEnsureStatsCompositePKWithBrokenTable 测试 EnsureStatsCompositePK 在各种错误 PK 场景下的行为
func TestEnsureStatsCompositePKWithBrokenTable(t *testing.T) {
	tests := []struct {
		name      string
		createSQL string
	}{
		{
			name:      "single_column_PK_date",
			createSQL: `CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint, PRIMARY KEY (date))`,
		},
		{
			name:      "single_column_PK_hour",
			createSQL: `CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint, PRIMARY KEY (hour))`,
		},
		{
			name:      "no_PK",
			createSQL: `CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint)`,
		},
		{
			name:      "spaced_composite_PK",
			createSQL: `CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token bigint, output_token bigint, input_cost real, output_cost real, wait_time bigint, output_time bigint, request_success bigint, request_failed bigint, PRIMARY KEY (date, hour))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "test.db")
			db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}

			db.Exec("DROP TABLE IF EXISTS stats_hourlies")
			db.Exec(tt.createSQL)

			pks := getSQLitePKColumns(t, db, "stats_hourlies")
			t.Logf("Before EnsureStatsCompositePK PK: %v", pks)

			EnsureStatsCompositePK(db)

			pks2 := getSQLitePKColumns(t, db, "stats_hourlies")
			t.Logf("After EnsureStatsCompositePK PK: %v", pks2)

			// 检查 UNIQUE INDEX
			var idxCount int64
			db.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_stats_hourlies_pk'").Scan(&idxCount)
			t.Logf("UNIQUE INDEX exists: %v", idxCount > 0)

			// 测试 OnConflict
			h1 := model.StatsHourly{Date: "20260504", Hour: 10, StatsMetrics: model.StatsMetrics{RequestSuccess: 5}}
			err = db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
				DoUpdates: clause.AssignmentColumns([]string{"request_success"}),
			}).Create(&h1).Error
			if err != nil {
				t.Errorf("OnConflict failed: %v", err)
			}
		})
	}
}
