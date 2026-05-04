package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 5,
		Up:      fixStatsTablesCompositePK,
	})
}

// EnsureStatsCompositePK 每次 AutoMigrate 后检查并修复 stats 表的复合主键
// 这不是一次性迁移——因为 glebarez/sqlite 的 AutoMigrate 有 getAllColumns 正则 bug，
// 每次启动都可能破坏复合主键，所以需要每次都检查。
// 但由于迁移版本系统只运行一次，这里只修复一次。
// 如果 AutoMigrate 再次破坏 PK，需要用 UNIQUE INDEX 作为兜底。
func EnsureStatsCompositePK(db *gorm.DB) {
	if db == nil {
		log.Errorf("EnsureStatsCompositePK: db is nil")
		return
	}
	dialectName := db.Dialector.Name()
	log.Infof("EnsureStatsCompositePK: dialect=%s", dialectName)
	if dialectName != "sqlite" {
		log.Infof("EnsureStatsCompositePK: skipping non-sqlite dialect")
		return
	}
	for _, t := range statsTablesWithCompositePK {
		if !db.Migrator().HasTable(t.name) {
			log.Infof("EnsureStatsCompositePK: table %s does not exist, skip", t.name)
			continue
		}
		ok, err := sqliteCheckCompositePK(db, t.name, t.pkColumns)
		if err != nil {
			log.Errorf("EnsureStatsCompositePK: check PK for %s: %v", t.name, err)
			continue
		}
		if !ok {
			log.Infof("EnsureStatsCompositePK: fixing composite PK for table %s...", t.name)
			if err := sqliteRebuildTable(db, t.name, t.createSQL); err != nil {
				log.Errorf("EnsureStatsCompositePK: rebuild %s failed: %v", t.name, err)
			} else {
				log.Infof("EnsureStatsCompositePK: table %s composite PK fixed", t.name)
			}
		} else {
			log.Infof("EnsureStatsCompositePK: table %s PK is correct", t.name)
		}
		// 确保 UNIQUE INDEX 存在（兜底：即使 PK 被破坏，OnConflict 也能工作）
		if err := ensureUniqueIndexForCompositePK(db, t.name, t.pkColumns); err != nil {
			log.Errorf("EnsureStatsCompositePK: ensure unique index for %s: %v", t.name, err)
		} else {
			log.Infof("EnsureStatsCompositePK: unique index for %s OK", t.name)
		}
	}

	// 确保所有使用 OnConflict 的表都有 UNIQUE INDEX（包括单列 PK 表）
	ensureAllStatsOnConflictIndexes(db)
}

// ensureAllStatsOnConflictIndexes 为所有使用 OnConflict 的 stats 表确保 UNIQUE INDEX 存在
func ensureAllStatsOnConflictIndexes(db *gorm.DB) {
	for _, t := range statsOnConflictIndexes {
		if !db.Migrator().HasTable(t.table) {
			continue
		}
		if err := ensureUniqueIndexForCompositePK(db, t.table, t.columns); err != nil {
			log.Errorf("EnsureStatsCompositePK: ensure OnConflict index for %s: %v", t.table, err)
		}
	}
}

// statsTablesWithCompositePK 需要检查复合主键并修复的表
var statsTablesWithCompositePK = []struct {
	name      string
	pkColumns []string
	createSQL string
}{
	{
		name:      "stats_hourlies",
		pkColumns: []string{"date", "hour"},
		createSQL: `CREATE TABLE stats_hourlies (date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (date,hour))`,
	},
	{
		name:      "stats_api_key_dailies",
		pkColumns: []string{"api_key_id", "date"},
		createSQL: `CREATE TABLE stats_api_key_dailies (api_key_id INTEGER NOT NULL, date TEXT NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (api_key_id,date))`,
	},
	{
		name:      "stats_api_key_hourlies",
		pkColumns: []string{"api_key_id", "date", "hour"},
		createSQL: `CREATE TABLE stats_api_key_hourlies (api_key_id INTEGER NOT NULL, date TEXT NOT NULL, hour INTEGER NOT NULL, input_token INTEGER, output_token INTEGER, input_cost REAL, output_cost REAL, wait_time INTEGER, output_time INTEGER, request_success INTEGER, request_failed INTEGER, PRIMARY KEY (api_key_id,date,hour))`,
	},
}

// statsOnConflictIndexes 所有使用 OnConflict upsert 的表和对应的列
// 确保 UNIQUE INDEX 存在，这样即使 PK 丢失，OnConflict 也能工作
var statsOnConflictIndexes = []struct {
	table   string
	columns []string
}{
	{table: "stats_totals", columns: []string{"id"}},
	{table: "stats_dailies", columns: []string{"date"}},
	{table: "stats_hourlies", columns: []string{"date", "hour"}},
	{table: "stats_channels", columns: []string{"channel_id"}},
	{table: "stats_models", columns: []string{"name"}},
	{table: "stats_api_keys", columns: []string{"api_key_id"}},
	{table: "stats_api_key_dailies", columns: []string{"api_key_id", "date"}},
	{table: "stats_api_key_hourlies", columns: []string{"api_key_id", "date", "hour"}},
}

func fixStatsTablesCompositePK(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if db.Dialector.Name() != "sqlite" {
		return nil
	}

	for _, t := range statsTablesWithCompositePK {
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

	// 创建 UNIQUE INDEX 作为兜底
	for _, t := range statsTablesWithCompositePK {
		if !db.Migrator().HasTable(t.name) {
			continue
		}
		if err := ensureUniqueIndexForCompositePK(db, t.name, t.pkColumns); err != nil {
			return fmt.Errorf("ensure unique index for %s: %w", t.name, err)
		}
	}

	return nil
}

// ensureUniqueIndexForCompositePK 确保 UNIQUE INDEX 存在
// 如果创建失败（因为数据重复），先去重再重试
func ensureUniqueIndexForCompositePK(db *gorm.DB, table string, columns []string) error {
	indexName := "idx_" + table + "_pk"

	// 检查索引是否已存在
	var count int64
	db.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&count)
	if count > 0 {
		return nil
	}

	colList := ""
	for i, c := range columns {
		if i > 0 {
			colList += ","
		}
		colList += c
	}

	log.Infof("creating unique index %s on %s(%s)", indexName, table, colList)
	err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS "+indexName+" ON "+table+" ("+colList+")").Error
	if err == nil {
		log.Infof("unique index %s created successfully", indexName)
		return nil
	}

	// 创建失败，可能是因为数据重复。去重后重试。
	log.Infof("unique index %s creation failed (likely duplicate data): %v, deduplicating...", indexName, err)
	if dedupErr := sqliteDeduplicateTable(db, table, columns); dedupErr != nil {
		return fmt.Errorf("deduplicate %s failed: %w (original index error: %v)", table, dedupErr, err)
	}

	log.Infof("deduplication done, retrying unique index %s on %s(%s)", indexName, table, colList)
	return db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS "+indexName+" ON "+table+" ("+colList+")").Error
}

// sqliteDeduplicateTable 去重：保留每组重复数据中 rowid 最小的那条，删除其余
func sqliteDeduplicateTable(db *gorm.DB, table string, keyColumns []string) error {
	colList := ""
	for i, c := range keyColumns {
		if i > 0 {
			colList += ", "
		}
		colList += c
	}

	// 获取所有列名
	var allCols []string
	rows, err := db.Raw("SELECT name FROM pragma_table_info(?)", table).Rows()
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}
	for rows.Next() {
		var col string
		rows.Scan(&col)
		allCols = append(allCols, col)
	}
	rows.Close()

	// 检查是否有 rowid（SQLite 所有表都有隐式 rowid，除非是 WITHOUT ROWID 表）
	// 用子查询删除重复行：保留 rowid 最小的，删除其余
	result := db.Exec(
		"DELETE FROM "+table+" WHERE rowid NOT IN ("+
			"SELECT MIN(rowid) FROM "+table+" GROUP BY "+colList+")")
	if result.Error != nil {
		return fmt.Errorf("deduplicate: %w", result.Error)
	}
	log.Infof("deduplicated %s: deleted %d duplicate rows", table, result.RowsAffected)
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
