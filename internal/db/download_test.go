package db

import (
	"github.com/qnqatop/papeer/internal/ptr"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadCRUD(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)

	// Save downloads.
	dl1 := &Download{PaperID: paper.ID, Source: "arxiv", Status: "fail", Reason: "no arXiv id"}
	dl2 := &Download{PaperID: paper.ID, Source: "semantic_scholar", URL: ptr.Ptr("https://example.com/paper.pdf"), Status: "ok", Filename: ptr.Ptr("test.pdf"), FileSize: ptr.Ptr(int64(50000))}

	if err := d.SaveDownload(dl1); err != nil {
		t.Fatalf("SaveDownload: %v", err)
	}
	if err := d.SaveDownload(dl2); err != nil {
		t.Fatalf("SaveDownload: %v", err)
	}

	// List.
	downloads, err := d.ListDownloads(paper.ID)
	if err != nil {
		t.Fatalf("ListDownloads: %v", err)
	}
	if len(downloads) != 2 {
		t.Fatalf("len = %d, want 2", len(downloads))
	}

	// Stats.
	stats, err := d.GetDownloadStats(p.ID)
	if err != nil {
		t.Fatalf("GetDownloadStats: %v", err)
	}
	if stats.TotalAttempts != 2 {
		t.Errorf("total = %d, want 2", stats.TotalAttempts)
	}
	if stats.ByStatus["ok"] != 1 {
		t.Errorf("ok = %d, want 1", stats.ByStatus["ok"])
	}
	if stats.ByStatus["fail"] != 1 {
		t.Errorf("fail = %d, want 1", stats.ByStatus["fail"])
	}
	if stats.BySource["arxiv"] != 1 {
		t.Errorf("arxiv = %d, want 1", stats.BySource["arxiv"])
	}

	// Top fail reasons.
	reasons, err := d.TopFailReasons(p.ID, 10)
	if err != nil {
		t.Fatalf("TopFailReasons: %v", err)
	}
	if len(reasons) != 1 || reasons[0].Count != 1 {
		t.Errorf("reasons = %v", reasons)
	}
}

func TestGetPaperPDFPath_Success(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	pdfDir := t.TempDir()
	d.Exec(`UPDATE search_profiles SET pdf_dir=? WHERE id=?`, pdfDir, p.ID)

	// Create actual file on disk
	os.WriteFile(filepath.Join(pdfDir, "test.pdf"), []byte("%PDF-1.4 test"), 0o644)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)

	d.SaveDownload(&Download{PaperID: paper.ID, Source: "arxiv", Status: "ok", Filename: ptr.Ptr("test.pdf")})

	path, err := d.GetPaperPDFPath(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPath: %v", err)
	}
	expected := filepath.Join(pdfDir, "test.pdf")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestGetPaperPDFPath_NoDownloads(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	d.Exec(`UPDATE search_profiles SET pdf_dir=? WHERE id=?`, t.TempDir(), p.ID)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)

	_, err := d.GetPaperPDFPath(paper.ID)
	if err == nil {
		t.Error("expected error for paper with no downloads")
	}
}

func TestGetPaperPDFPath_FailThenOk(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	pdfDir := t.TempDir()
	d.Exec(`UPDATE search_profiles SET pdf_dir=? WHERE id=?`, pdfDir, p.ID)
	os.WriteFile(filepath.Join(pdfDir, "paper.pdf"), []byte("%PDF-1.4 test"), 0o644)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)

	// First download fails
	d.SaveDownload(&Download{PaperID: paper.ID, Source: "arxiv", Status: "fail", Reason: "no pdf"})
	// Second succeeds
	d.SaveDownload(&Download{PaperID: paper.ID, Source: "semantic_scholar", Status: "ok", Filename: ptr.Ptr("paper.pdf")})

	path, err := d.GetPaperPDFPath(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPath: %v", err)
	}
	expected := filepath.Join(pdfDir, "paper.pdf")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestGetPaperPDFPath_LastOkWins(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	pdfDir := t.TempDir()
	d.Exec(`UPDATE search_profiles SET pdf_dir=? WHERE id=?`, pdfDir, p.ID)
	os.WriteFile(filepath.Join(pdfDir, "first.pdf"), []byte("%PDF-1.4"), 0o644)
	os.WriteFile(filepath.Join(pdfDir, "second.pdf"), []byte("%PDF-1.4"), 0o644)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)

	d.SaveDownload(&Download{PaperID: paper.ID, Source: "arxiv", Status: "ok", Filename: ptr.Ptr("first.pdf")})
	d.SaveDownload(&Download{PaperID: paper.ID, Source: "semantic_scholar", Status: "ok", Filename: ptr.Ptr("second.pdf")})

	path, err := d.GetPaperPDFPath(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPath: %v", err)
	}
	expected := filepath.Join(pdfDir, "second.pdf")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestGetPaperPDFPath_FileNotOnDisk(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	pdfDir := t.TempDir()
	d.Exec(`UPDATE search_profiles SET pdf_dir=? WHERE id=?`, pdfDir, p.ID)
	// Do NOT create the file on disk

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper",
		TitleNormalized: "test paper",
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	d.UpsertPaper(paper)
	d.SaveDownload(&Download{PaperID: paper.ID, Source: "arxiv", Status: "ok", Filename: ptr.Ptr("missing.pdf")})

	_, err := d.GetPaperPDFPath(paper.ID)
	if err == nil {
		t.Error("expected error for file not on disk")
	}
}

func TestGetFailedDownloadPaperIDs(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	paper1 := &Paper{ProfileID: p.ID, Title: "Has fail + ok", TitleNormalized: "has fail + ok", Sources: JSONStringSlice{"test"}, Authors: JSONStringSlice{}, ScoreReasons: JSONStringSlice{}, Status: "approved"}
	paper2 := &Paper{ProfileID: p.ID, Title: "Only fail", TitleNormalized: "only fail", Sources: JSONStringSlice{"test"}, Authors: JSONStringSlice{}, ScoreReasons: JSONStringSlice{}, Status: "approved"}
	paper3 := &Paper{ProfileID: p.ID, Title: "Only ok", TitleNormalized: "only ok", Sources: JSONStringSlice{"test"}, Authors: JSONStringSlice{}, ScoreReasons: JSONStringSlice{}, Status: "approved"}
	paper4 := &Paper{ProfileID: p.ID, Title: "No downloads", TitleNormalized: "no downloads", Sources: JSONStringSlice{"test"}, Authors: JSONStringSlice{}, ScoreReasons: JSONStringSlice{}, Status: "approved"}
	d.UpsertPaper(paper1)
	d.UpsertPaper(paper2)
	d.UpsertPaper(paper3)
	d.UpsertPaper(paper4)

	// Paper 1: both fail and ok → NOT in failed set
	d.SaveDownload(&Download{PaperID: paper1.ID, Source: "arxiv", Status: "fail", Reason: "no pdf"})
	d.SaveDownload(&Download{PaperID: paper1.ID, Source: "semantic_scholar", Status: "ok", Filename: ptr.Ptr("paper1.pdf")})

	// Paper 2: only fail → SHOULD be in failed set
	d.SaveDownload(&Download{PaperID: paper2.ID, Source: "arxiv", Status: "fail", Reason: "timeout"})

	// Paper 3: only ok → NOT in failed set
	d.SaveDownload(&Download{PaperID: paper3.ID, Source: "semantic_scholar", Status: "ok", Filename: ptr.Ptr("paper3.pdf")})

	// Paper 4: no downloads → NOT in failed set

	ids, err := d.GetFailedDownloadPaperIDs(p.ID)
	if err != nil {
		t.Fatalf("GetFailedDownloadPaperIDs: %v", err)
	}

	if ids[paper1.ID] {
		t.Error("paper1 (fail+ok) should NOT be in failed set")
	}
	if !ids[paper2.ID] {
		t.Error("paper2 (only fail) SHOULD be in failed set")
	}
	if ids[paper3.ID] {
		t.Error("paper3 (only ok) should NOT be in failed set")
	}
	if ids[paper4.ID] {
		t.Error("paper4 (no downloads) should NOT be in failed set")
	}
	if len(ids) != 1 {
		t.Errorf("len(ids) = %d, want 1", len(ids))
	}
}

func TestGetFailedDownloadPaperIDs_EmptyProfile(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	ids, err := d.GetFailedDownloadPaperIDs(p.ID)
	if err != nil {
		t.Fatalf("GetFailedDownloadPaperIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty set, got %d entries", len(ids))
	}
}
