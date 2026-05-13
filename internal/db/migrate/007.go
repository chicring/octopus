package migrate

import (
	"gorm.io/gorm"

	octopusModel "github.com/bestruirui/octopus/internal/model"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 7,
		Up: func(db *gorm.DB) error {
			if !db.Migrator().HasTable("relay_logs") {
				return nil
			}
			if !db.Migrator().HasColumn(&octopusModel.RelayLog{}, "CachedTokens") {
				if err := db.Migrator().AddColumn(&octopusModel.RelayLog{}, "CachedTokens"); err != nil {
					return err
				}
			}
			if !db.Migrator().HasColumn(&octopusModel.RelayLog{}, "CacheCreationTokens") {
				return db.Migrator().AddColumn(&octopusModel.RelayLog{}, "CacheCreationTokens")
			}
			return nil
		},
	})
}
