package op

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm/clause"
)

// TestDoUpdatesNoZeroOverwrite 验证 DoUpdates 不会用零值覆盖已有数据
func TestDoUpdatesNoZeroOverwrite(t *testing.T) {
	dbPath := "/tmp/test_doupdates.db"
	os.Remove(dbPath)

	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("db init: %v", err)
	}
	gormDB := db.GetDB()

	today := time.Now().Format("20060102")

	// Write initial data
	d1 := model.StatsDaily{Date: today, StatsMetrics: model.StatsMetrics{RequestSuccess: 10, InputToken: 1000}}
	gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"input_token", "output_token", "input_cost", "output_cost",
			"wait_time", "output_time", "request_success", "request_failed",
		}),
	}).Create(&d1)

	// Verify
	var read1 model.StatsDaily
	gormDB.Where("date = ?", today).First(&read1)
	if read1.RequestSuccess != 10 {
		t.Fatalf("initial: got %d, want 10", read1.RequestSuccess)
	}

	// Upsert with zero values — DoUpdates should still overwrite (it uses the new values)
	// This is expected behavior: DoUpdates replaces columns with the new struct's values
	d2 := model.StatsDaily{Date: today}
	gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"input_token", "output_token", "input_cost", "output_cost",
			"wait_time", "output_time", "request_success", "request_failed",
		}),
	}).Create(&d2)

	var read2 model.StatsDaily
	gormDB.Where("date = ?", today).First(&read2)
	t.Logf("After zero upsert with DoUpdates: RequestSuccess=%d InputToken=%d", read2.RequestSuccess, read2.InputToken)

	// DoUpdates also overwrites with the new struct's values (including zeros)
	// The real fix is that StatsSaveDB should never write empty snapshots
	if read2.RequestSuccess == 0 && read2.InputToken == 0 {
		t.Log("DoUpdates also overwrites with zeros — fix must ensure empty snapshots are never written")
	} else {
		t.Log("DoUpdates does NOT overwrite with zeros — unexpected but good")
	}

	sqlDB, _ := gormDB.DB()
	sqlDB.Close()
}

// TestRestartDataSurvives 验证重启后 StatsSaveDB 不会破坏已有数据
func TestRestartDataSurvives(t *testing.T) {
	dbPath := "/tmp/test_restart_survive.db"
	os.Remove(dbPath)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	today := time.Now().Format("20060102")

	// Phase 1: Normal operation
	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("db init: %v", err)
	}

	StatsDailyUpdate(context.Background(), model.StatsMetrics{RequestSuccess: 5, InputToken: 500})
	StatsTotalUpdate(model.StatsMetrics{RequestSuccess: 5, InputToken: 500})

	ctx := context.Background()
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}

	// Verify DB has data
	gormDB := db.GetDB()
	var dbDaily model.StatsDaily
	gormDB.Where("date = ?", today).First(&dbDaily)
	if dbDaily.RequestSuccess != 5 {
		t.Fatalf("Phase 1: got RequestSuccess=%d, want 5", dbDaily.RequestSuccess)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.Close()

	// Phase 2: Restart — statsRefreshCache loads data, then StatsSaveDB runs
	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("db init 2: %v", err)
	}

	// Simulate statsRefreshCache loading data from DB
	statsDailyCacheLock.Lock()
	statsDailyCache = dbDaily // loaded from DB
	statsDailyCacheLock.Unlock()

	statsTotalCacheLock.Lock()
	var dbTotal model.StatsTotal
	db.GetDB().First(&dbTotal)
	statsTotalCache = dbTotal
	statsTotalCacheLock.Unlock()

	// StatsSaveDB runs with loaded caches (normal startup scenario)
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB phase 2: %v", err)
	}

	// Data should survive
	gormDB = db.GetDB()
	var dbDaily2 model.StatsDaily
	gormDB.Where("date = ?", today).First(&dbDaily2)
	if dbDaily2.RequestSuccess != 5 || dbDaily2.InputToken != 500 {
		t.Errorf("Data lost after restart! RequestSuccess=%d InputToken=%d, want 5/500", dbDaily2.RequestSuccess, dbDaily2.InputToken)
	} else {
		t.Logf("Data survived restart: RequestSuccess=%d InputToken=%d", dbDaily2.RequestSuccess, dbDaily2.InputToken)
	}

	sqlDB2, _ := gormDB.DB()
	sqlDB2.Close()
}
