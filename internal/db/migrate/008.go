package migrate

import (
	"gorm.io/gorm"

	octopusModel "github.com/bestruirui/octopus/internal/model"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 8,
		Up: func(db *gorm.DB) error {
			if !db.Migrator().HasTable("relay_logs") {
				return nil
			}
			if !db.Migrator().HasColumn(&octopusModel.RelayLog{}, "DebugContent") {
				return db.Migrator().AddColumn(&octopusModel.RelayLog{}, "DebugContent")
			}
			return nil
		},
	})
}
