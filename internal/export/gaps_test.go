package export

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

func testGaps() []db.ExternalCitationWithMentions {
	return []db.ExternalCitationWithMentions{
		{
			ExternalCitation: db.ExternalCitation{
				ID:            1,
				S2PaperID:     "s2-1",
				Title:         "A Paper, With A Comma",
				Year:          ptr.Ptr(2019),
				CitationCount: 42,
				Authors:       db.JSONStringSlice{"Alice Smith", "Bob Jones"},
				MentionCount:  3,
			},
			MentionedBy: []int64{10, 20, 30},
		},
		{
			ExternalCitation: db.ExternalCitation{
				ID:            2,
				S2PaperID:     "s2-2",
				Title:         "No Year Paper",
				Year:          nil,
				CitationCount: 0,
				Authors:       db.JSONStringSlice{},
				MentionCount:  2,
			},
			MentionedBy: nil,
		},
	}
}

func TestExportGapsCSV_HeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportGapsCSV(testGaps(), &buf); err != nil {
		t.Fatalf("ExportGapsCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("len(records) = %d, want 3", len(records))
	}

	wantHeader := []string{"title", "year", "authors", "citation_count", "mention_count", "mentioned_by_paper_ids", "s2_paper_id"}
	for i, col := range wantHeader {
		if records[0][i] != col {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}

	row1 := records[1]
	// The comma inside the title must round-trip correctly through CSV
	// quoting (encoding/csv handles this — this asserts the writer doesn't
	// bypass it, e.g. by hand-joining fields with commas).
	if row1[0] != "A Paper, With A Comma" {
		t.Errorf("row1 title = %q, want the comma preserved", row1[0])
	}
	if row1[1] != "2019" {
		t.Errorf("row1 year = %q, want 2019", row1[1])
	}
	if row1[2] != "Alice Smith; Bob Jones" {
		t.Errorf("row1 authors = %q, want semicolon-joined", row1[2])
	}
	if row1[3] != "42" {
		t.Errorf("row1 citation_count = %q, want 42", row1[3])
	}
	if row1[4] != "3" {
		t.Errorf("row1 mention_count = %q, want 3", row1[4])
	}
	if row1[5] != "10; 20; 30" {
		t.Errorf("row1 mentioned_by_paper_ids = %q, want '10; 20; 30'", row1[5])
	}
	if row1[6] != "s2-1" {
		t.Errorf("row1 s2_paper_id = %q, want s2-1", row1[6])
	}

	row2 := records[2]
	if row2[1] != "" {
		t.Errorf("row2 year = %q, want empty for nil year", row2[1])
	}
	if row2[5] != "" {
		t.Errorf("row2 mentioned_by_paper_ids = %q, want empty for no mentions", row2[5])
	}
}

func TestExportGapsCSV_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportGapsCSV(nil, &buf); err != nil {
		t.Fatalf("ExportGapsCSV(nil): %v", err)
	}
	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1 (header only)", len(records))
	}
}

func TestExportGapsCSV_Deterministic(t *testing.T) {
	gaps := testGaps()
	var buf1, buf2 bytes.Buffer
	if err := ExportGapsCSV(gaps, &buf1); err != nil {
		t.Fatal(err)
	}
	if err := ExportGapsCSV(gaps, &buf2); err != nil {
		t.Fatal(err)
	}
	if buf1.String() != buf2.String() {
		t.Errorf("ExportGapsCSV output differs between identical calls")
	}
}
