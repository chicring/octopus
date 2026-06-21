package migrate

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 9,
		Up:      migrateChannelTypeToBaseUrls,
	})
}

// 009:
// 将渠道级的 type / provider_id（旧列）下沉到每个 base_urls 条目。
// 迁移后 base_urls JSON 中每个对象都带上 type 和 provider_id。
// 幂等：通过 migration_records 表保证只执行一次；
// 同时内部对已带 type 字段的条目跳过，保证重跑安全。
//
// 注意：移除 Channel.Type/ProviderID 结构体字段后，旧 DB 列 type/provider_id
// 仍保留在表中（GORM AutoMigrate 不删列），本迁移读取这些列做回填。
func migrateChannelTypeToBaseUrls(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	hasColumn := func(table, column string) (bool, error) {
		switch dialect {
		case "sqlite":
			var name string
			if err := db.Raw("SELECT name FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column).
				Scan(&name).Error; err != nil {
				return false, fmt.Errorf("failed to check sqlite column %s.%s: %w", table, column, err)
			}
			return name == column, nil
		default:
			return db.Migrator().HasColumn(table, column), nil
		}
	}

	hasTypeCol, err := hasColumn("channels", "type")
	if err != nil {
		return err
	}
	hasProviderCol, err := hasColumn("channels", "provider_id")
	if err != nil {
		return err
	}

	if !hasTypeCol && !hasProviderCol {
		return nil
	}

	type row struct {
		ID         int    `gorm:"column:id"`
		Type       *int   `gorm:"column:type"`
		ProviderID *string `gorm:"column:provider_id"`
		BaseUrls   string `gorm:"column:base_urls"`
	}

	rows := make([]row, 0)
	if err := db.Raw(`
SELECT id, type, provider_id, base_urls
FROM channels
WHERE base_urls IS NOT NULL AND TRIM(base_urls) != '' AND TRIM(base_urls) != 'null' AND TRIM(base_urls) != '[]'
`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("failed to read channels for type migration: %w", err)
	}

	for _, r := range rows {
		var entries []map[string]any
		if err := json.Unmarshal([]byte(r.BaseUrls), &entries); err != nil {
			continue
		}

		changed := false
		for _, entry := range entries {
			// 幂等：已存在 type 字段的条目跳过
			if _, hasType := entry["type"]; hasType {
				continue
			}
			if r.Type != nil {
				entry["type"] = *r.Type
				changed = true
			}
			if r.ProviderID != nil && *r.ProviderID != "" {
				entry["provider_id"] = *r.ProviderID
				changed = true
			}
		}

		if !changed {
			continue
		}

		payload, err := json.Marshal(entries)
		if err != nil {
			return fmt.Errorf("failed to marshal base_urls for id=%d: %w", r.ID, err)
		}
		if err := db.Exec("UPDATE channels SET base_urls = ? WHERE id = ?", string(payload), r.ID).Error; err != nil {
			return fmt.Errorf("failed to update channels.base_urls for id=%d: %w", r.ID, err)
		}
	}

	return nil
}
