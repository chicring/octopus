package migrate

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPopulateProviderID_BackfillsKnownLegacyTypesOnlyWhenEmpty(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	_ = err
	_ = db

	db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrate-003-backfill.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE channels (
			id INTEGER PRIMARY KEY,
			type INTEGER,
			provider_id TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct {
		ID         int
		Type       int
		ProviderID *string
	}{
		{ID: 1, Type: 0, ProviderID: nil},
		{ID: 2, Type: 1, ProviderID: strPtr("")},
		{ID: 3, Type: 2, ProviderID: strPtr("custom-provider")},
		{ID: 4, Type: 5, ProviderID: nil},
		{ID: 5, Type: 99, ProviderID: nil},
	}
	for _, row := range rows {
		if err := db.Exec("INSERT INTO channels(id, type, provider_id) VALUES (?, ?, ?)", row.ID, row.Type, row.ProviderID).Error; err != nil {
			t.Fatalf("insert row %d: %v", row.ID, err)
		}
	}

	if err := populateProviderID(db); err != nil {
		t.Fatalf("populateProviderID() error = %v", err)
	}

	type result struct {
		ID         int
		ProviderID string
	}
	var got []result
	if err := db.Raw("SELECT id, COALESCE(provider_id, '') AS provider_id FROM channels ORDER BY id").Scan(&got).Error; err != nil {
		t.Fatalf("query rows: %v", err)
	}

	want := map[int]string{
		1: "openai-chat",
		2: "openai-response",
		3: "custom-provider",
		4: "openai-embedding",
		5: "",
	}
	for _, row := range got {
		if row.ProviderID != want[row.ID] {
			t.Fatalf("row %d provider_id = %q, want %q", row.ID, row.ProviderID, want[row.ID])
		}
	}
}

func TestPopulateProviderID_NoProviderIDColumnIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrate-003-no-column.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Exec(`CREATE TABLE channels (id INTEGER PRIMARY KEY, type INTEGER)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := populateProviderID(db); err != nil {
		t.Fatalf("populateProviderID() should no-op, got %v", err)
	}
}

func strPtr(s string) *string { return &s }
