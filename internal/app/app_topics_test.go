package app

import (
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

func TestGetTopics_EmptyWhenNoEligiblePapers(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	got, err := a.GetTopics(prof.ID, 0)
	if err != nil {
		t.Fatalf("GetTopics: %v", err)
	}
	if got != nil {
		t.Errorf("GetTopics(empty profile) = %v, want nil", got)
	}
}

func TestGetTopics_ClustersApprovedAndDownloadedPapers(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	quantum := []string{
		"quantum computing algorithms exploit entanglement between quantum bits",
		"quantum entanglement algorithms improve quantum computing bit fidelity",
		"entanglement based quantum computing algorithm research on quantum bits",
	}
	medieval := []string{
		"medieval castles and knights defended territory with armor and siege engines",
		"medieval knights wore armor and defended castles during siege warfare",
		"castles knights medieval armor and siege warfare history of fortification",
	}
	status := []string{"approved", "approved", "approved", "downloaded", "downloaded", "downloaded"}
	texts := append(append([]string{}, quantum...), medieval...)
	for i, text := range texts {
		p := &db.Paper{
			ProfileID:       prof.ID,
			Title:           text,
			TitleNormalized: text,
			Abstract:        "",
			Status:          status[i],
		}
		if err := a.db.UpsertPaper(p); err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
	}
	// Rejected / new papers must not be included in the clustered corpus.
	rejected := &db.Paper{ProfileID: prof.ID, Title: "irrelevant excluded paper text here", TitleNormalized: "excluded", Status: "rejected"}
	if err := a.db.UpsertPaper(rejected); err != nil {
		t.Fatal(err)
	}

	got, err := a.GetTopics(prof.ID, 2)
	if err != nil {
		t.Fatalf("GetTopics: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(topics) = %d, want 2", len(got))
	}
	total := 0
	for _, topic := range got {
		total += topic.Size
		for _, id := range topic.PaperIDs {
			if id == rejected.ID {
				t.Errorf("rejected paper leaked into topics: %+v", topic)
			}
		}
	}
	if total != 6 {
		t.Errorf("total clustered papers = %d, want 6 (approved+downloaded only)", total)
	}
}

func TestApprovedAndDownloadedFull_CombinesBothStatuses(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	approved := &db.Paper{ProfileID: prof.ID, Title: "A", TitleNormalized: "a", Status: "approved"}
	downloaded := &db.Paper{ProfileID: prof.ID, Title: "B", TitleNormalized: "b", Status: "downloaded"}
	newOne := &db.Paper{ProfileID: prof.ID, Title: "C", TitleNormalized: "c", Status: "new"}
	for _, p := range []*db.Paper{approved, downloaded, newOne} {
		if err := a.db.UpsertPaper(p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := a.approvedAndDownloadedFull(prof.ID)
	if err != nil {
		t.Fatalf("approvedAndDownloadedFull: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	ids := map[int64]bool{got[0].ID: true, got[1].ID: true}
	if !ids[approved.ID] || !ids[downloaded.ID] {
		t.Errorf("got = %+v, want approved+downloaded only", got)
	}
}
