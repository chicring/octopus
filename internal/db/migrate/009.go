package migrate

import (
	"gorm.io/gorm"

	octopusModel "github.com/bestruirui/octopus/internal/model"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 9,
		Up: func(db *gorm.DB) error {
			if db.Migrator().HasTable(&octopusModel.Channel{}) {
				if !db.Migrator().HasColumn(&octopusModel.Channel{}, "OfficialURL") {
					if err := db.Migrator().AddColumn(&octopusModel.Channel{}, "OfficialURL"); err != nil {
						return err
					}
				}
				if !db.Migrator().HasColumn(&octopusModel.Channel{}, "UsageQuery") {
					if err := db.Migrator().AddColumn(&octopusModel.Channel{}, "UsageQuery"); err != nil {
						return err
					}
				}
			}

			if db.Migrator().HasTable(&octopusModel.ChannelKey{}) {
				if !db.Migrator().HasColumn(&octopusModel.ChannelKey{}, "IsCLI") {
					if err := db.Migrator().AddColumn(&octopusModel.ChannelKey{}, "IsCLI"); err != nil {
						return err
					}
				}
				if !db.Migrator().HasColumn(&octopusModel.ChannelKey{}, "Multiplier") {
					if err := db.Migrator().AddColumn(&octopusModel.ChannelKey{}, "Multiplier"); err != nil {
						return err
					}
				}
				if !db.Migrator().HasColumn(&octopusModel.ChannelKey{}, "Models") {
					if err := db.Migrator().AddColumn(&octopusModel.ChannelKey{}, "Models"); err != nil {
						return err
					}
				}
			}
			return nil
		},
	})
}
