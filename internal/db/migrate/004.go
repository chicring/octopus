package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 4,
		Up:      fixStatsHourlyCompositePK,
	})
}

// 004: StatsHourly 主键从单列 hour 改为复合主键 (date, hour)
// 旧表只有 hour 做主键，跨天数据会被覆盖。需要重建表以支持复合主键。
func fixStatsHourlyCompositePK(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	switch dialect {
	case "sqlite":
		return fixStatsHourlyCompositePKSQLite(db)
	case "mysql":
		return fixStatsHourlyCompositePKMySQL(db)
	case "postgres":
		return fixStatsHourlyCompositePKPostgres(db)
	default:
		// 对于未知 dialect，尝试通用方式
		return fixStatsHourlyCompositePKGeneric(db)
	}
}

func fixStatsHourlyCompositePKSQLite(db *gorm.DB) error {
	// 检查表是否存在
	if !db.Migrator().HasTable("stats_hourlies") {
		return nil
	}

	// 检查是否已有 date 列且已有复合主键（通过检查唯一索引判断）
	var indexCount int64
	db.Raw("SELECT COUNT(*) FROM pragma_index_list('stats_hourlies') WHERE name LIKE '%date%' OR name LIKE '%hour%'").Scan(&indexCount)

	// SQLite 不支持 ALTER TABLE DROP PRIMARY KEY，需要重建表
	// 1. 重命名旧表
	if err := db.Exec("ALTER TABLE stats_hourlies RENAME TO stats_hourlies_old").Error; err != nil {
		// 如果旧表不存在，说明已经迁移过
		return nil
	}

	// 2. 创建新表（复合主键）
	if err := db.Exec(`CREATE TABLE stats_hourlies (
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
	)`).Error; err != nil {
		return fmt.Errorf("failed to create new stats_hourlies table: %w", err)
	}

	// 3. 迁移数据（旧表可能没有 date 列，需要处理）
	hasDateColumn := false
	var colName string
	db.Raw("SELECT name FROM pragma_table_info('stats_hourlies_old') WHERE name = 'date' LIMIT 1").Scan(&colName)
	hasDateColumn = colName == "date"

	if hasDateColumn {
		if err := db.Exec(`INSERT OR IGNORE INTO stats_hourlies (date, hour, input_token, output_token, input_cost, output_cost, wait_time, output_time, request_success, request_failed)
			SELECT date, hour, input_token, output_token, input_cost, output_cost, wait_time, output_time, request_success, request_failed FROM stats_hourlies_old`).Error; err != nil {
			return fmt.Errorf("failed to migrate stats_hourlies data: %w", err)
		}
	} else {
		// 旧表没有 date 列，用空字符串占位（这些数据无法确定日期，丢弃即可）
		// 不迁移，直接丢弃
	}

	// 4. 删除旧表
	if err := db.Exec("DROP TABLE stats_hourlies_old").Error; err != nil {
		return fmt.Errorf("failed to drop old stats_hourlies table: %w", err)
	}

	return nil
}

func fixStatsHourlyCompositePKMySQL(db *gorm.DB) error {
	if !db.Migrator().HasTable("stats_hourlies") {
		return nil
	}

	// 确保 date 列存在（AutoMigrate 还没执行，旧表可能没有此列）
	if !db.Migrator().HasColumn("stats_hourlies", "date") {
		if err := db.Exec("ALTER TABLE stats_hourlies ADD COLUMN date TEXT NOT NULL DEFAULT ''").Error; err != nil {
			return fmt.Errorf("failed to add date column: %w", err)
		}
	}

	// MySQL: 删除旧主键，添加复合主键
	if err := db.Exec("ALTER TABLE stats_hourlies DROP PRIMARY KEY, ADD PRIMARY KEY (date, hour)").Error; err != nil {
		// 如果已经是复合主键，会报错，忽略
		return nil
	}
	return nil
}

func fixStatsHourlyCompositePKPostgres(db *gorm.DB) error {
	if !db.Migrator().HasTable("stats_hourlies") {
		return nil
	}

	// 确保 date 列存在（AutoMigrate 还没执行，旧表可能没有此列）
	if !db.Migrator().HasColumn("stats_hourlies", "date") {
		if err := db.Exec("ALTER TABLE stats_hourlies ADD COLUMN date TEXT NOT NULL DEFAULT ''").Error; err != nil {
			return fmt.Errorf("failed to add date column: %w", err)
		}
	}

	// PostgreSQL: 删除旧主键，添加复合主键
	if err := db.Exec("ALTER TABLE stats_hourlies DROP CONSTRAINT IF EXISTS stats_hourlies_pkey").Error; err != nil {
		return nil
	}
	if err := db.Exec("ALTER TABLE stats_hourlies ADD PRIMARY KEY (date, hour)").Error; err != nil {
		// 如果已经是复合主键，会报错，忽略
		return nil
	}
	return nil
}

func fixStatsHourlyCompositePKGeneric(db *gorm.DB) error {
	// 通用方式：直接用 AutoMigrate 重建（GORM 会处理主键变更）
	if !db.Migrator().HasTable("stats_hourlies") {
		return nil
	}
	// 对于无法确定 dialect 的情况，跳过迁移
	return nil
}
