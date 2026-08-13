package db

import (
	"os"
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	d, err := NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNewDB_CreatesTables(t *testing.T) {
	d := testDB(t)

	tables := []string{"search_profiles", "axes", "queries", "keywords", "papers", "downloads", "summaries"}
	for _, tbl := range tables {
		var name string
		err := d.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", tbl, err)
		}
	}
}

func TestNewDB_WALMode(t *testing.T) {
	d := testDB(t)

	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("expected WAL mode, got %q", mode)
	}
}

func TestNewDB_ForeignKeys(t *testing.T) {
	d := testDB(t)

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys=1, got %d", fk)
	}
}

func TestNewDB_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	d1, err := NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	d1.Close()

	// Opening same DB again should not fail.
	d2, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	d2.Close()

	_ = os.RemoveAll(dir)
}

// TestNewDB_Idempotent_NewColumnsAndTables opens a DB three times in a row
// (simulating the app being restarted repeatedly against an existing file)
// and checks that the Analysis v2 migrations — the ALTER TABLE ADD COLUMN
// for last_citation_fetch_at and the citation_mentions table — land exactly
// once and survive every subsequent open without erroring (the migrate()
// step swallows "duplicate column" errors, which this guards against
// regressing into a hard failure).
func TestNewDB_Idempotent_NewColumnsAndTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	for i := 0; i < 3; i++ {
		d, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}

		var colCount int
		if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('papers') WHERE name='last_citation_fetch_at'`).Scan(&colCount); err != nil {
			t.Fatalf("open #%d: pragma_table_info: %v", i, err)
		}
		if colCount != 1 {
			t.Errorf("open #%d: last_citation_fetch_at column count = %d, want 1", i, colCount)
		}

		var tblName string
		if err := d.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='citation_mentions'`).Scan(&tblName); err != nil {
			t.Errorf("open #%d: citation_mentions table not found: %v", i, err)
		}

		d.Close()
	}
}

// TestAxis_LangScope_DefaultsToEn checks that a row inserted without an
// explicit lang_scope (as legacy databases would have it after the migration
// adds the column) reads back as the English default via GetAxis.
func TestAxis_LangScope_DefaultsToEn(t *testing.T) {
	d := testDB(t)

	p := &Profile{Name: "test", Email: "t@t.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	// Insert bypassing SaveAxis so lang_scope is left to the column DEFAULT.
	res, err := d.Exec(`INSERT INTO axes (profile_id, axis_key, description, position) VALUES (?,?,?,?)`,
		p.ID, "legacy", "", 0)
	if err != nil {
		t.Fatalf("insert axis: %v", err)
	}
	id, _ := res.LastInsertId()

	axis, err := d.GetAxis(id)
	if err != nil {
		t.Fatalf("GetAxis: %v", err)
	}
	if axis.LangScope != "en" {
		t.Errorf("LangScope = %q, want %q", axis.LangScope, "en")
	}
}

func TestSummaries_TableConstraints(t *testing.T) {
	d := testDB(t)

	// Setup
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test Paper', 'test paper')`)

	// UNIQUE(paper_id, model) — second insert with same pair fails.
	d.Exec(`INSERT INTO summaries (paper_id, provider, model, status) VALUES (1, 'deepseek', 'deepseek-v4-pro', 'done')`)
	_, err := d.Exec(`INSERT INTO summaries (paper_id, provider, model, status) VALUES (1, 'deepseek', 'deepseek-v4-pro', 'done')`)
	if err == nil {
		t.Error("expected UNIQUE constraint error for duplicate (paper_id, model)")
	}

	// CHECK constraint on status — invalid value fails.
	_, err = d.Exec(`INSERT INTO summaries (paper_id, provider, model, status) VALUES (1, 'gemini', 'gemini-3-flash-preview', 'invalid_status')`)
	if err == nil {
		t.Error("expected CHECK constraint error for invalid status")
	}

	// ON DELETE CASCADE — deleting paper deletes summaries.
	d.Exec(`DELETE FROM papers WHERE id=1`)
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM summaries WHERE paper_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 summaries after paper delete, got %d", count)
	}

	// Indexes exist.
	for _, idx := range []string{"idx_summaries_paper", "idx_summaries_status"} {
		var name string
		err := d.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", idx, err)
		}
	}
}
