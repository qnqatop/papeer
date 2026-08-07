package app

import (
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

// seedPapersWithYears creates one approved paper per (year, count) pair, so
// the profile's YearDistribution ends up exactly as specified.
func seedPapersWithYears(t *testing.T, a *App, profileID int64, byYear map[int]int) {
	t.Helper()
	n := 0
	for year, count := range byYear {
		for i := 0; i < count; i++ {
			n++
			p := &db.Paper{
				ProfileID:       profileID,
				Title:           "Paper", // duplicate titles are fine, normalized differs below
				TitleNormalized: "paper-unique-" + strings.Repeat("x", n),
				Year:            ptr.Ptr(year),
				Status:          "approved",
			}
			if err := a.db.UpsertPaper(p); err != nil {
				t.Fatalf("UpsertPaper: %v", err)
			}
		}
	}
}

func TestGenerateYearDistributionParagraph_NoDatedPapers(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	_, err := a.GenerateYearDistributionParagraph(prof.ID, nil)
	if err == nil {
		t.Fatal("expected error for empty year distribution")
	}
}

func TestGenerateYearDistributionParagraph_TrendUpward(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// Early years (2018-2020) have few papers, later years (2021-2023) have
	// many more — >20% swing → "trended upward".
	seedPapersWithYears(t, a, prof.ID, map[int]int{
		2018: 1, 2019: 1, 2020: 1,
		2021: 5, 2022: 5, 2023: 5,
	})

	got, err := a.GenerateYearDistributionParagraph(prof.ID, nil)
	if err != nil {
		t.Fatalf("GenerateYearDistributionParagraph: %v", err)
	}
	if !strings.Contains(got, "18 papers") {
		t.Errorf("paragraph = %q, want total of 18 papers mentioned", got)
	}
	if !strings.Contains(got, "between 2018 and 2023") {
		t.Errorf("paragraph = %q, want range 2018-2023", got)
	}
	if !strings.Contains(got, "median publication year: 2022") {
		t.Errorf("paragraph = %q, want median 2022", got)
	}
	if !strings.Contains(got, "trended upward") {
		t.Errorf("paragraph = %q, want 'trended upward'", got)
	}
}

func TestGenerateYearDistributionParagraph_TrendDownward(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	seedPapersWithYears(t, a, prof.ID, map[int]int{
		2018: 5, 2019: 5, 2020: 5,
		2021: 1, 2022: 1, 2023: 1,
	})

	got, err := a.GenerateYearDistributionParagraph(prof.ID, nil)
	if err != nil {
		t.Fatalf("GenerateYearDistributionParagraph: %v", err)
	}
	if !strings.Contains(got, "trended downward") {
		t.Errorf("paragraph = %q, want 'trended downward'", got)
	}
}

func TestGenerateYearDistributionParagraph_TrendSteady(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// Roughly equal counts on both halves → "held roughly steady".
	seedPapersWithYears(t, a, prof.ID, map[int]int{
		2018: 3, 2019: 3, 2020: 3, 2021: 3,
	})

	got, err := a.GenerateYearDistributionParagraph(prof.ID, nil)
	if err != nil {
		t.Fatalf("GenerateYearDistributionParagraph: %v", err)
	}
	if !strings.Contains(got, "held roughly steady") {
		t.Errorf("paragraph = %q, want 'held roughly steady'", got)
	}
}

func TestGenerateYearDistributionParagraph_SingleYear(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	seedPapersWithYears(t, a, prof.ID, map[int]int{2020: 4})

	got, err := a.GenerateYearDistributionParagraph(prof.ID, nil)
	if err != nil {
		t.Fatalf("GenerateYearDistributionParagraph: %v", err)
	}
	// minY == maxY: the "held roughly steady" branch is the only reachable
	// one (the maxY > minY trend logic is skipped entirely).
	if !strings.Contains(got, "between 2020 and 2020") {
		t.Errorf("paragraph = %q, want range 2020-2020", got)
	}
	if !strings.Contains(got, "held roughly steady") {
		t.Errorf("paragraph = %q, want steady trend for single-year corpus", got)
	}
}
