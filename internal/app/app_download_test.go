package app

import (
	"errors"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

func TestDownloadPaper_RejectsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "NoEmail", "")

	paper := &db.Paper{
		ProfileID:       p.ID,
		Title:           "Test",
		TitleNormalized: "test",
		Status:          "approved",
		Sources:         db.JSONStringSlice{"test"},
		Authors:         db.JSONStringSlice{},
		ScoreReasons:    db.JSONStringSlice{},
	}
	if err := a.db.UpsertPaper(paper); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	err := a.DownloadPaper(paper.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("err = %v, want ErrInvalidEmail", err)
	}
}

func TestDownloadPaper_RejectsNoPdfDir(t *testing.T) {
	a := newTestApp(t)
	p := createValidProfile(t, a, "P", "real@univ.edu")
	// PdfDir is empty by default
	paper := &db.Paper{
		ProfileID:       p.ID,
		Title:           "Test",
		TitleNormalized: "test",
		Status:          "approved",
		Sources:         db.JSONStringSlice{"test"},
		Authors:         db.JSONStringSlice{},
		ScoreReasons:    db.JSONStringSlice{},
	}
	if err := a.db.UpsertPaper(paper); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	err := a.DownloadPaper(paper.ID)
	if err == nil {
		t.Error("expected error for empty pdf_dir")
	}
}

func TestDownloadPaper_GuardPreventsDuplicate(t *testing.T) {
	a := newTestApp(t)
	p := createValidProfile(t, a, "P", "real@univ.edu")
	p.PdfDir = t.TempDir()
	a.db.UpdateProfile(p)

	paper := &db.Paper{
		ProfileID:       p.ID,
		Title:           "Test",
		TitleNormalized: "test",
		Status:          "approved",
		Sources:         db.JSONStringSlice{"test"},
		Authors:         db.JSONStringSlice{},
		ScoreReasons:    db.JSONStringSlice{},
	}
	if err := a.db.UpsertPaper(paper); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	// Pre-populate the guard set — simulate that a download is in progress.
	a.dlPaperMu.Lock()
	if a.dlPaperSet == nil {
		a.dlPaperSet = make(map[int64]bool)
	}
	a.dlPaperSet[paper.ID] = true
	a.dlPaperMu.Unlock()

	// DownloadPaper should see the guard and return nil without spawning a goroutine.
	err := a.DownloadPaper(paper.ID)
	if err != nil {
		t.Errorf("DownloadPaper should return nil when guard is active, got: %v", err)
	}
}
