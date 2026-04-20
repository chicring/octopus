package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 3,
		Up:      populateProviderID,
	})
}

// 003: 从 legacy type 回填 provider_id（幂等，仅当 provider_id 为空时执行）
func populateProviderID(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	hasColumn := func(table, column string) bool {
		switch dialect {
		case "sqlite":
			var name string
			db.Raw("SELECT name FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column).Scan(&name)
			return name == column
		case "mysql":
			var count int64
			db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&count)
			return count > 0
		case "postgres":
			var count int64
			db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?", table, column).Scan(&count)
			return count > 0
		default:
			return db.Migrator().HasColumn(table, column)
		}
	}

	if !hasColumn("channels", "provider_id") {
		return nil
	}

	// legacy type → provider_id 映射
	mapping := map[int]string{
		0: "openai-chat",
		1: "openai-response",
		2: "anthropic",
		3: "gemini",
		4: "volcengine",
		5: "openai-embedding",
	}

	for legacyType, providerID := range mapping {
		if err := db.Exec(
			"UPDATE channels SET provider_id = ? WHERE type = ? AND (provider_id IS NULL OR provider_id = '')",
			providerID, legacyType,
		).Error; err != nil {
			return fmt.Errorf("failed to backfill provider_id for type=%d: %w", legacyType, err)
		}
	}

	return nil
}
