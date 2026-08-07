package app

import (
	"testing"
	"time"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

// seedKeyPaper inserts an approved paper with the given citation count and
// (optional) publication year — the two inputs to the Most Cited / Rising
// rankers.
func seedKeyPaper(t *testing.T, a *App, profileID int64, title string, citations int, year *int) int64 {
	t.Helper()
	p := &db.Paper{
		ProfileID:       profileID,
		Title:           title,
		TitleNormalized: title,
		Year:            year,
		CitationCount:   citations,
		Status:          "approved",
	}
	if err := a.db.UpsertPaper(p); err != nil {
		t.Fatalf("UpsertPaper(%q): %v", title, err)
	}
	return p.ID
}

func TestGetKeyPapers_EmptyWhenNoPapers(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.MostCited) != 0 || len(got.Rising) != 0 || len(got.Bridge) != 0 {
		t.Errorf("got = %+v, want all empty", got)
	}
}

func TestGetKeyPapers_MostCitedOrdersDescending(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	low := seedKeyPaper(t, a, prof.ID, "Low", 5, nil)
	high := seedKeyPaper(t, a, prof.ID, "High", 100, nil)
	mid := seedKeyPaper(t, a, prof.ID, "Mid", 50, nil)

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.MostCited) != 3 {
		t.Fatalf("len(MostCited) = %d, want 3", len(got.MostCited))
	}
	wantOrder := []int64{high, mid, low}
	for i, id := range wantOrder {
		if got.MostCited[i].ID != id {
			t.Errorf("MostCited[%d].ID = %d, want %d", i, got.MostCited[i].ID, id)
		}
	}
	if got.MostCited[0].Score != 100 {
		t.Errorf("MostCited[0].Score = %v, want 100", got.MostCited[0].Score)
	}
}

func TestGetKeyPapers_MostCitedCapsAtLimit(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	for i := 0; i < keyPapersLimit+5; i++ {
		seedKeyPaper(t, a, prof.ID, string(rune('A'+i)), i, nil)
	}

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.MostCited) != keyPapersLimit {
		t.Errorf("len(MostCited) = %d, want %d (capped)", len(got.MostCited), keyPapersLimit)
	}
}

func TestGetKeyPapers_RisingUsesVelocityFormula(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	curYear := time.Now().Year()

	// oldHigh: 100 citations but old (age = curYear - (curYear-10) + 1 = 11) → score ≈ 9.09
	// newLow: 20 citations, published this year (age=1) → score = 20
	// newLow must rank above oldHigh in Rising despite far fewer raw citations.
	oldHigh := seedKeyPaper(t, a, prof.ID, "OldHigh", 100, ptr.Ptr(curYear-10))
	newLow := seedKeyPaper(t, a, prof.ID, "NewLow", 20, ptr.Ptr(curYear))
	noYear := seedKeyPaper(t, a, prof.ID, "NoYear", 9999, nil)
	_ = noYear

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.Rising) != 2 {
		t.Fatalf("len(Rising) = %d, want 2 (paper without year excluded)", len(got.Rising))
	}
	if got.Rising[0].ID != newLow {
		t.Errorf("Rising[0].ID = %d, want %d (higher citation velocity)", got.Rising[0].ID, newLow)
	}
	if got.Rising[0].Score != 20 {
		t.Errorf("Rising[0].Score = %v, want 20 (20 citations / age 1)", got.Rising[0].Score)
	}
	if got.Rising[1].ID != oldHigh {
		t.Errorf("Rising[1].ID = %d, want %d", got.Rising[1].ID, oldHigh)
	}
	wantOldScore := 100.0 / 11.0
	if diff := got.Rising[1].Score - wantOldScore; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Rising[1].Score = %v, want %v", got.Rising[1].Score, wantOldScore)
	}
	for _, kp := range got.Rising {
		if kp.ID == noYear {
			t.Errorf("paper without a year leaked into Rising: %+v", kp)
		}
	}
}

func TestGetKeyPapers_BridgeEmptyWithoutCitationLinks(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	seedKeyPaper(t, a, prof.ID, "Solo", 10, nil)

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.Bridge) != 0 {
		t.Errorf("Bridge = %+v, want empty (no citation_links fetched yet)", got.Bridge)
	}
}

func TestGetKeyPapers_BridgeRanksByBetweenness(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// Star topology: hub connects to 4 leaves. Betweenness of the hub is
	// C(4,2)=6, all leaves are 0 and therefore excluded (score<=0 filter).
	hub := seedKeyPaper(t, a, prof.ID, "Hub", 1, nil)
	leaves := make([]int64, 4)
	for i := range leaves {
		leaves[i] = seedKeyPaper(t, a, prof.ID, string(rune('a'+i)), 1, nil)
		if err := a.db.UpsertCitationLink(prof.ID, hub, leaves[i]); err != nil {
			t.Fatalf("UpsertCitationLink: %v", err)
		}
	}

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	if len(got.Bridge) != 1 {
		t.Fatalf("len(Bridge) = %d, want 1 (only the hub has positive betweenness)", len(got.Bridge))
	}
	if got.Bridge[0].ID != hub {
		t.Errorf("Bridge[0].ID = %d, want hub %d", got.Bridge[0].ID, hub)
	}
	if got.Bridge[0].Score != 6 {
		t.Errorf("Bridge[0].Score = %v, want 6", got.Bridge[0].Score)
	}
}

func TestGetKeyPapers_BridgeIgnoresLinksOutsideEligibleSet(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	a1 := seedKeyPaper(t, a, prof.ID, "A", 1, nil)
	a2 := seedKeyPaper(t, a, prof.ID, "B", 1, nil)
	if err := a.db.UpsertCitationLink(prof.ID, a1, a2); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	// Reject one of them — it drops out of approvedAndDownloadedFull, so its
	// betweenness score (if any) must not surface in Bridge.
	if err := a.db.UpdatePaperStatus(a2, "rejected"); err != nil {
		t.Fatalf("UpdatePaperStatus: %v", err)
	}

	got, err := a.GetKeyPapers(prof.ID)
	if err != nil {
		t.Fatalf("GetKeyPapers: %v", err)
	}
	for _, kp := range got.Bridge {
		if kp.ID == a2 {
			t.Errorf("rejected paper leaked into Bridge: %+v", kp)
		}
	}
}
