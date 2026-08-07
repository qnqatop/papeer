package db

import (
	"strings"
	"testing"
	"time"
)

func TestEnsureTag_CreatesNew(t *testing.T) {
	d := testDB(t)

	// Create a profile first.
	p := &Profile{Name: "test", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	id, err := d.EnsureTag(p.ID, "radar", "#f97316")
	if err != nil {
		t.Fatalf("EnsureTag failed: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero tag id")
	}

	// Verify it exists.
	tags, err := d.ListTags(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != "radar" {
		t.Fatalf("expected tag name 'radar', got %q", tags[0].Name)
	}
	if tags[0].Color != "#f97316" {
		t.Fatalf("expected tag color '#f97316', got %q", tags[0].Color)
	}
}

func TestEnsureTag_ReturnsExisting(t *testing.T) {
	d := testDB(t)

	p := &Profile{Name: "test", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	id1, err := d.EnsureTag(p.ID, "monitoring", "#f97316")
	if err != nil {
		t.Fatal(err)
	}

	// Second call should return the same ID.
	id2, err := d.EnsureTag(p.ID, "monitoring", "#f97316")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("expected same tag id (%d), got %d", id1, id2)
	}

	// Still only one tag.
	tags, _ := d.ListTags(p.ID)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
}

func TestGetPaperIDsAfterTime_FindsNewPapers(t *testing.T) {
	d := testDB(t)

	p := &Profile{Name: "test", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	// Insert a paper and record the time.
	before := time.Now().UTC()
	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "New Radar Paper",
		TitleNormalized: "new radar paper",
		Status:          "new",
	}
	d.insertPaper(paper)

	// Should find it.
	ids, err := d.GetPaperIDsAfterTime(p.ID, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 new paper, got %d", len(ids))
	}
	if ids[0] != paper.ID {
		t.Fatalf("expected paper id %d, got %d", paper.ID, ids[0])
	}

	// Should not find papers inserted before the cutoff.
	after := time.Now().UTC().Add(time.Second)
	ids, err = d.GetPaperIDsAfterTime(p.ID, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 papers after cutoff, got %d", len(ids))
	}
}

func TestGetPaperIDsAfterTime_ScopedToProfile(t *testing.T) {
	d := testDB(t)

	p1 := &Profile{Name: "p1", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p1); err != nil {
		t.Fatal(err)
	}
	p2 := &Profile{Name: "p2", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p2); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	d.insertPaper(&Paper{ProfileID: p1.ID, Title: "P1 Paper", TitleNormalized: "p1 paper", Status: "new"})
	d.insertPaper(&Paper{ProfileID: p2.ID, Title: "P2 Paper", TitleNormalized: "p2 paper", Status: "new"})

	ids, err := d.GetPaperIDsAfterTime(p1.ID, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 paper for p1, got %d", len(ids))
	}
}

func TestSetAxisRadarRun_UpdatesTimestamp(t *testing.T) {
	d := testDB(t)

	p := &Profile{Name: "test", YearMin: 2020, MaxPerQuery: 10}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	axis := &Axis{
		ProfileID: p.ID,
		AxisKey:   "test_axis",
	}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	// Verify it starts NULL.
	var lastRun *string
	err := d.QueryRow(`SELECT last_radar_run FROM axes WHERE id=?`, axis.ID).Scan(&lastRun)
	if err != nil {
		t.Fatal(err)
	}
	if lastRun != nil {
		t.Fatal("expected NULL last_radar_run for new axis")
	}

	// Set the timestamp.
	ts := "2026-05-06 12:00:00"
	if _, err := d.Exec(`UPDATE axes SET last_radar_run=? WHERE id=?`, ts, axis.ID); err != nil {
		t.Fatal(err)
	}

	// Verify it was set.
	err = d.QueryRow(`SELECT last_radar_run FROM axes WHERE id=?`, axis.ID).Scan(&lastRun)
	if err != nil {
		t.Fatal(err)
	}
	if lastRun == nil || !strings.HasPrefix(*lastRun, "2026-05-06") {
		got := "<nil>"
		if lastRun != nil {
			got = *lastRun
		}
		t.Fatalf("expected timestamp starting with %q, got %q", "2026-05-06", got)
	}
}
