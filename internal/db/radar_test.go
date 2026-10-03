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
	if err := d.SetAxisRadarRun(axis.ID, time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)); err != nil {
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
