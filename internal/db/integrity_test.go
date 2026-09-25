package db

import (
	"path/filepath"
	"testing"
)

func insertTestPaper(t *testing.T, d *DB, profileID int64, axisID *int64, title string) *Paper {
	t.Helper()
	p := &Paper{
		ProfileID:       profileID,
		AxisID:          axisID,
		Title:           title,
		TitleNormalized: title,
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "new",
	}
	if err := d.UpsertPaper(p); err != nil {
		t.Fatalf("UpsertPaper(%q): %v", title, err)
	}
	return p
}

func TestDeleteAxis_WithPapers(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	a := &Axis{ProfileID: prof.ID, AxisKey: "with_papers"}
	if err := d.SaveAxis(a); err != nil {
		t.Fatal(err)
	}
	paper := insertTestPaper(t, d, prof.ID, &a.ID, "paper on axis")

	if err := d.DeleteAxis(a.ID); err != nil {
		t.Fatalf("DeleteAxis with papers: %v", err)
	}
	if _, err := d.GetAxis(a.ID); err == nil {
		t.Error("axis still exists after delete")
	}
	got, err := d.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("paper must survive axis deletion: %v", err)
	}
	if got.AxisID != nil {
		t.Errorf("paper.axis_id = %v, want NULL", *got.AxisID)
	}
}

func TestAppendAxes_PositionsAfterExistingAndAtomic(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	existing := &Axis{ProfileID: prof.ID, AxisKey: "existing", Position: 4}
	if err := d.SaveAxis(existing); err != nil {
		t.Fatal(err)
	}

	axes := []Axis{
		{AxisKey: "a", Keywords: []Keyword{{Word: "x", Type: "exclude"}}},
		{AxisKey: "b"},
	}
	if err := d.AppendAxes(prof.ID, axes); err != nil {
		t.Fatalf("AppendAxes: %v", err)
	}
	if axes[0].Position != 5 || axes[1].Position != 6 {
		t.Errorf("positions = %d,%d, want 5,6", axes[0].Position, axes[1].Position)
	}

	// A duplicate axis_key in the batch must roll back the whole batch.
	bad := []Axis{{AxisKey: "c"}, {AxisKey: "existing"}}
	if err := d.AppendAxes(prof.ID, bad); err == nil {
		t.Fatal("expected error for duplicate axis_key")
	}
	list, err := d.ListAxes(prof.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("axes after failed import = %d, want 3 (no partial import)", len(list))
	}
}

func TestSetPaperTags_DuplicateIDsAndAtomic(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	paper := insertTestPaper(t, d, prof.ID, nil, "tagged")
	t1 := &Tag{ProfileID: prof.ID, Name: "one", Color: "#000"}
	t2 := &Tag{ProfileID: prof.ID, Name: "two", Color: "#000"}
	if err := d.CreateTag(t1); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateTag(t2); err != nil {
		t.Fatal(err)
	}

	if err := d.SetPaperTags(paper.ID, []int64{t1.ID, t2.ID, t1.ID}); err != nil {
		t.Fatalf("SetPaperTags with duplicates: %v", err)
	}
	tags, _ := d.ListPaperTags(paper.ID)
	if len(tags) != 2 {
		t.Fatalf("tags = %d, want 2", len(tags))
	}

	// A bad tag id fails the FK check; previous tags must be kept.
	if err := d.SetPaperTags(paper.ID, []int64{t1.ID, 999999}); err == nil {
		t.Fatal("expected error for unknown tag id")
	}
	tags, _ = d.ListPaperTags(paper.ID)
	if len(tags) != 2 {
		t.Errorf("tags after failed set = %d, want 2 (rolled back)", len(tags))
	}
}

func TestUpsertPaper_MergeSetsID(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	first := insertTestPaper(t, d, prof.ID, nil, "same title")
	second := insertTestPaper(t, d, prof.ID, nil, "same title")
	if second.ID != first.ID {
		t.Errorf("merged paper ID = %d, want %d", second.ID, first.ID)
	}
}

func TestListPapers_SearchEscapesWildcards(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	insertTestPaper(t, d, prof.ID, nil, "100% accuracy")
	insertTestPaper(t, d, prof.ID, nil, "1000 samples")
	insertTestPaper(t, d, prof.ID, nil, "snake_case names")
	insertTestPaper(t, d, prof.ID, nil, "snakeXcase names")

	for _, tc := range []struct {
		q    string
		want int
	}{
		{"100%", 1},
		{"snake_case", 1},
		{`\`, 0},
	} {
		got, total, err := d.ListPapers(PaperFilter{ProfileID: prof.ID, Search: tc.q})
		if err != nil {
			t.Fatalf("ListPapers(%q): %v", tc.q, err)
		}
		if len(got) != tc.want || total != tc.want {
			t.Errorf("search %q: got %d/%d, want %d", tc.q, len(got), total, tc.want)
		}
	}
}

func TestNewDB_ResetsInterruptedSummaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	prof := createTestProfile(t, d)
	paper := insertTestPaper(t, d, prof.ID, nil, "summarized")
	if err := d.CreateSummary(&Summary{PaperID: paper.ID, Provider: "p", Model: "m", Status: "generating"}); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d, err = NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	status, err := d.HasActiveSummary(paper.ID, "m")
	if err != nil {
		t.Fatal(err)
	}
	if status != "error" {
		t.Errorf("status after reopen = %q, want error", status)
	}
}

func TestUpsertSummary_ReturnsExistingID(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	p1 := insertTestPaper(t, d, prof.ID, nil, "one")
	p2 := insertTestPaper(t, d, prof.ID, nil, "two")

	s1 := &Summary{PaperID: p1.ID, Provider: "p", Model: "m", Status: "error"}
	if err := d.CreateSummary(s1); err != nil {
		t.Fatal(err)
	}
	// A later insert on the same connection makes LastInsertId point elsewhere.
	if err := d.CreateSummary(&Summary{PaperID: p2.ID, Provider: "p", Model: "m", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	up := &Summary{PaperID: p1.ID, Provider: "p", Model: "m", Status: "generating"}
	if err := d.UpsertSummary(up); err != nil {
		t.Fatal(err)
	}
	if up.ID != s1.ID {
		t.Errorf("upsert ID = %d, want existing %d", up.ID, s1.ID)
	}
}
