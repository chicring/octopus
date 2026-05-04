package migrate

import (
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 6,
		Up:      fixStatsModelSchema,
	})
}

// EnsureStatsModelSchema 修复旧版 stats_models 表：旧表以 id 为主键并残留 channel_id NOT NULL，
// 当前写入按 name upsert，会导致 ON CONFLICT 或 channel_id NOT NULL 失败。
func EnsureStatsModelSchema(db *gorm.DB) {
	if err := fixStatsModelSchema(db); err != nil {
		log.Errorf("EnsureStatsModelSchema: %v", err)
	}
}

func fixStatsModelSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	if !db.Migrator().HasTable("stats_models") {
		return nil
	}

	ok, err := sqliteStatsModelSchemaOK(db)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	cols, err := sqliteTableColumns(db, "stats_models")
	if err != nil {
		return err
	}
	if _, ok := cols["name"]; !ok {
		return fmt.Errorf("stats_models missing name column")
	}

	log.Infof("migration 006: rebuilding legacy stats_models schema")
	if err := sqliteRebuildStatsModels(db, cols); err != nil {
		return err
	}
	log.Infof("migration 006: stats_models rebuilt successfully")
	return nil
}

func sqliteStatsModelSchemaOK(db *gorm.DB) (bool, error) {
	cols, err := sqliteTableColumns(db, "stats_models")
	if err != nil {
		return false, err
	}
	for _, legacy := range []string{"id", "channel_id", "request_count"} {
		if _, ok := cols[legacy]; ok {
			return false, nil
		}
	}
	return sqliteCheckCompositePK(db, "stats_models", []string{"name"})
}

func sqliteTableColumns(db *gorm.DB, table string) (map[string]struct{}, error) {
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?)", table).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[name] = struct{}{}
	}
	return cols, rows.Err()
}

func sqliteRebuildStatsModels(db *gorm.DB, oldCols map[string]struct{}) error {
	const table = "stats_models"
	const tmpTable = "stats_models_tmp"
	const createSQL = `CREATE TABLE stats_models (name TEXT NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (name))`

	metricCols := []string{
		"input_token",
		"output_token",
		"input_cost",
		"output_cost",
		"wait_time",
		"output_time",
		"request_success",
		"request_failed",
	}
	exprs := make([]string, 0, len(metricCols))
	for _, col := range metricCols {
		if col == "request_success" {
			exprs = append(exprs, sqliteStatsModelSumExpr(oldCols, col, "request_count"))
			continue
		}
		exprs = append(exprs, sqliteStatsModelSumExpr(oldCols, col))
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DROP TABLE IF EXISTS " + tmpTable).Error; err != nil {
			return fmt.Errorf("drop temp stats_models: %w", err)
		}
		if err := tx.Exec("ALTER TABLE " + table + " RENAME TO " + tmpTable).Error; err != nil {
			return fmt.Errorf("rename stats_models: %w", err)
		}
		if err := tx.Exec(createSQL).Error; err != nil {
			return fmt.Errorf("create stats_models: %w", err)
		}

		insertSQL := fmt.Sprintf(
			"INSERT INTO %s (name, %s) SELECT name, %s FROM %s WHERE name IS NOT NULL AND name <> '' GROUP BY name",
			table,
			strings.Join(metricCols, ", "),
			strings.Join(exprs, ", "),
			tmpTable,
		)
		if err := tx.Exec(insertSQL).Error; err != nil {
			return fmt.Errorf("migrate stats_models data: %w", err)
		}
		if err := tx.Exec("DROP TABLE " + tmpTable).Error; err != nil {
			return fmt.Errorf("drop old stats_models: %w", err)
		}
		return nil
	})
}

func sqliteStatsModelSumExpr(cols map[string]struct{}, col string, fallback ...string) string {
	_, hasCol := cols[col]
	if len(fallback) > 0 {
		fb := fallback[0]
		_, hasFallback := cols[fb]
		if hasCol && hasFallback {
			return fmt.Sprintf("SUM(CASE WHEN COALESCE(%s, 0) <> 0 THEN COALESCE(%s, 0) ELSE COALESCE(%s, 0) END)", col, col, fb)
		}
		if hasFallback {
			return fmt.Sprintf("SUM(COALESCE(%s, 0))", fb)
		}
	}
	if hasCol {
		return fmt.Sprintf("SUM(COALESCE(%s, 0))", col)
	}
	return "0"
}
