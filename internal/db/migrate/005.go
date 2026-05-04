package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 5,
		Up:      fixStatsTablesCompositePK,
	})
}

// fixStatsTablesCompositePK 检查并修复所有 stats 表的主键约束
// glebarez/sqlite 的 AutoMigrate 对复合主键处理有 bug，可能不会正确创建复合主键
// 导致 ON CONFLICT upsert 失败：SQL logic error: ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint
func fixStatsTablesCompositePK(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()
	if dialect != "sqlite" {
		return nil
	}

	tables := []struct {
		name       string
		pkColumns  []string
		createSQL  string
	}{
		{
			name:      "stats_hourlies",
			pkColumns: []string{"date", "hour"},
			createSQL: `CREATE TABLE stats_hourlies (
				date TEXT NOT NULL,
				hour INTEGER NOT NULL,
				input_token INTEGER, output_token INTEGER,
				input_cost REAL, output_cost REAL,
				wait_time INTEGER, output_time INTEGER,
				request_success INTEGER, request_failed INTEGER,
				PRIMARY KEY (date, hour)
			)`,
		},
		{
			name:      "stats_api_key_dailies",
			pkColumns: []string{"api_key_id", "date"},
			createSQL: `CREATE TABLE stats_api_key_dailies (
				api_key_id INTEGER NOT NULL,
				date TEXT NOT NULL,
				input_token INTEGER, output_token INTEGER,
				input_cost REAL, output_cost REAL,
				wait_time INTEGER, output_time INTEGER,
				request_success INTEGER, request_failed INTEGER,
				PRIMARY KEY (api_key_id, date)
			)`,
		},
		{
			name:      "stats_api_key_hourlies",
			pkColumns: []string{"api_key_id", "date", "hour"},
			createSQL: `CREATE TABLE stats_api_key_hourlies (
				api_key_id INTEGER NOT NULL,
				date TEXT NOT NULL,
				hour INTEGER NOT NULL,
				input_token INTEGER, output_token INTEGER,
				input_cost REAL, output_cost REAL,
				wait_time INTEGER, output_time INTEGER,
				request_success INTEGER, request_failed INTEGER,
				PRIMARY KEY (api_key_id, date, hour)
			)`,
		},
	}

	for _, t := range tables {
		if !db.Migrator().HasTable(t.name) {
			log.Infof("migration 005: table %s does not exist, skip", t.name)
			continue
		}
		ok, err := sqliteCheckCompositePK(db, t.name, t.pkColumns)
		if err != nil {
			return fmt.Errorf("check PK for %s: %w", t.name, err)
		}
		if ok {
			log.Infof("migration 005: table %s PK is correct, skip", t.name)
			continue
		}
		log.Infof("migration 005: table %s PK is incorrect, rebuilding...", t.name)
		if err := sqliteRebuildTable(db, t.name, t.createSQL); err != nil {
			return fmt.Errorf("rebuild %s: %w", t.name, err)
		}
		log.Infof("migration 005: table %s rebuilt successfully", t.name)
	}

	return nil
}

// sqliteCheckCompositePK 检查 SQLite 表的主键列是否匹配期望的列
func sqliteCheckCompositePK(db *gorm.DB, table string, expectedPKs []string) (bool, error) {
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", table).Rows()
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var pks []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		pks = append(pks, name)
	}

	if len(pks) != len(expectedPKs) {
		return false, nil
	}
	for i, pk := range pks {
		if pk != expectedPKs[i] {
			return false, nil
		}
	}
	return true, nil
}

// sqliteRebuildTable 用重建方式修复表的主键约束（SQLite 不支持 ALTER TABLE 修改主键）
func sqliteRebuildTable(db *gorm.DB, table, createSQL string) error {
	tmpTable := table + "_tmp"

	if err := db.Exec("DROP TABLE IF EXISTS " + tmpTable).Error; err != nil {
		return fmt.Errorf("drop tmp table: %w", err)
	}

	if err := db.Exec("ALTER TABLE " + table + " RENAME TO " + tmpTable).Error; err != nil {
		return fmt.Errorf("rename old table: %w", err)
	}

	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	// 获取旧表列名，动态构造 INSERT
	var cols []string
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?)", tmpTable).Rows()
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}
	for rows.Next() {
		var col string
		rows.Scan(&col)
		cols = append(cols, col)
	}
	rows.Close()

	colList := ""
	for i, c := range cols {
		if i > 0 {
			colList += ", "
		}
		colList += c
	}

	if err := db.Exec("INSERT OR IGNORE INTO "+table+" ("+colList+") SELECT "+colList+" FROM "+tmpTable).Error; err != nil {
		return fmt.Errorf("migrate data: %w", err)
	}

	if err := db.Exec("DROP TABLE " + tmpTable).Error; err != nil {
		return fmt.Errorf("drop tmp table: %w", err)
	}

	return nil
}
