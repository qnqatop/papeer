package app

import (
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

// ─── ExportGapsCSV (App layer) ────────────────────────────────────────────

func TestExportGapsCSV_NoGapsReturnsError(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	_, err := a.ExportGapsCSV(prof.ID)
	if err == nil {
		t.Fatal("expected error when there are no coverage gaps")
	}
}

func TestExportGapsCSV_ReturnsCSVContent(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// mention_count must reach 2 (GetCoverageGaps default minMentions).
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "e1", Title: "Missing Work"})
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "e1", Title: "Missing Work"})

	got, err := a.ExportGapsCSV(prof.ID)
	if err != nil {
		t.Fatalf("ExportGapsCSV: %v", err)
	}
	if !strings.Contains(got, "Missing Work") {
		t.Errorf("csv = %q, want it to contain the gap title", got)
	}
	if !strings.HasPrefix(got, "title,year,authors") {
		t.Errorf("csv = %q, want header first", got)
	}
}

// ─── ExportAnalysisMarkdown ────────────────────────────────────────────────

func TestExportAnalysisMarkdown_IncludesAllSections(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "Test Profile", "real@univ.edu")

	src := seedKeyPaper(t, a, prof.ID, "Quantum Computing Advances quantum bits algorithm", 10, ptr.Ptr(2021))
	tgt := seedKeyPaper(t, a, prof.ID, "Cold Start Recommenders cold start algorithm", 5, ptr.Ptr(2022))
	if err := a.db.UpsertCitationLink(prof.ID, src, tgt); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "gap1", Title: "Foundational Work"})
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "gap1", Title: "Foundational Work"})

	got, err := a.ExportAnalysisMarkdown(prof.ID)
	if err != nil {
		t.Fatalf("ExportAnalysisMarkdown: %v", err)
	}

	for _, want := range []string{
		"# Analysis Report — Test Profile",
		"## Overview",
		"Total papers by status:",
		"PDF availability:",
		"Citation graph:",
		"## Key Papers",
		"Most Cited",
		"## Coverage Gaps",
		"Foundational Work",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q\n--- full output ---\n%s", want, got)
		}
	}
}

func TestExportAnalysisMarkdown_HandlesEmptyProfileGracefully(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "Empty", "real@univ.edu")

	got, err := a.ExportAnalysisMarkdown(prof.ID)
	if err != nil {
		t.Fatalf("ExportAnalysisMarkdown (empty profile): %v", err)
	}
	if !strings.Contains(got, "# Analysis Report — Empty") {
		t.Errorf("markdown = %q, want the report header even for an empty profile", got)
	}
	// Sections that have nothing to show must be omitted, not printed empty.
	if strings.Contains(got, "## Topics") {
		t.Errorf("markdown should omit ## Topics section when there are no topics")
	}
	if strings.Contains(got, "## Coverage Gaps") {
		t.Errorf("markdown should omit ## Coverage Gaps section when there are none")
	}
}
