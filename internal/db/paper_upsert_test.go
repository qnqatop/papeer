package db

import (
	"testing"

	"github.com/qnqatop/papeer/internal/ptr"
)

func batchPaper(profileID int64, axisID *int64, title string) *Paper {
	return &Paper{
		ProfileID:       profileID,
		AxisID:          axisID,
		Title:           title,
		TitleNormalized: title,
		Sources:         JSONStringSlice{"openalex"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "new",
	}
}

func countPapers(t *testing.T, d *DB, profileID int64) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM papers WHERE profile_id=?`, profileID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUpsertPapers_BatchInsertAndLink(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	ax := &Axis{ProfileID: prof.ID, AxisKey: "a"}
	if err := d.SaveAxis(ax); err != nil {
		t.Fatal(err)
	}

	papers := []*Paper{
		batchPaper(prof.ID, &ax.ID, "first"),
		batchPaper(prof.ID, &ax.ID, "second"),
		batchPaper(prof.ID, &ax.ID, "third"),
	}
	res, err := d.UpsertPapers(papers)
	if err != nil {
		t.Fatalf("UpsertPapers: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results", len(res))
	}
	for i, r := range res {
		if r.Err != nil || !r.Inserted || r.ID == 0 || r.ID != papers[i].ID {
			t.Errorf("result %d = %+v (paper id %d)", i, r, papers[i].ID)
		}
	}
	if n := countPapers(t, d, prof.ID); n != 3 {
		t.Errorf("papers = %d, want 3", n)
	}
	counts, err := d.PaperAxisCounts([]int64{res[0].ID, res[1].ID, res[2].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if counts[r.ID] != 1 {
			t.Errorf("paper %d linked to %d axes, want 1", r.ID, counts[r.ID])
		}
	}
}

func TestUpsertPapers_MergeReportsNotInserted(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)

	existing := batchPaper(prof.ID, nil, "same title")
	existing.DOI = ptr.Ptr("10.1/x")
	existing.CitationCount = 3
	if err := d.UpsertPaper(existing); err != nil {
		t.Fatal(err)
	}

	dup := batchPaper(prof.ID, nil, "different title")
	dup.DOI = ptr.Ptr("10.1/x")
	dup.CitationCount = 9
	dup.Sources = JSONStringSlice{"crossref"}
	fresh := batchPaper(prof.ID, nil, "fresh")

	// The same new paper twice in one batch: the second merges into the
	// row the first inserted (visible inside the transaction).
	again := batchPaper(prof.ID, nil, "fresh")

	res, err := d.UpsertPapers([]*Paper{dup, fresh, again})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Inserted || res[0].ID != existing.ID || dup.ID != existing.ID {
		t.Errorf("dup result = %+v, want merge into %d", res[0], existing.ID)
	}
	if !res[1].Inserted {
		t.Errorf("fresh result = %+v, want inserted", res[1])
	}
	if res[2].Inserted || res[2].ID != res[1].ID {
		t.Errorf("repeat result = %+v, want merge into %d", res[2], res[1].ID)
	}

	got, err := d.GetPaper(existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CitationCount != 9 || len(got.Sources) != 2 {
		t.Errorf("merged paper = citations %d, sources %v", got.CitationCount, got.Sources)
	}
	if n := countPapers(t, d, prof.ID); n != 2 {
		t.Errorf("papers = %d, want 2", n)
	}
}

func TestUpsertPapers_FailureIsolatedToOnePaper(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)

	bad := batchPaper(prof.ID, nil, "bad status")
	bad.Status = "bogus"                                                   // violates the status CHECK constraint
	badAxis := batchPaper(prof.ID, ptr.Ptr(int64(987654)), "missing axis") // FK violation

	papers := []*Paper{
		batchPaper(prof.ID, nil, "ok one"),
		bad,
		badAxis,
		batchPaper(prof.ID, nil, "ok two"),
	}
	res, err := d.UpsertPapers(papers)
	if err != nil {
		t.Fatalf("a per-paper failure must not fail the batch: %v", err)
	}
	if res[0].Err != nil || res[3].Err != nil {
		t.Errorf("good papers failed: %v / %v", res[0].Err, res[3].Err)
	}
	for _, i := range []int{1, 2} {
		if res[i].Err == nil {
			t.Errorf("paper %d: expected an error", i)
		}
		if res[i].ID != 0 || papers[i].ID != 0 {
			t.Errorf("paper %d: failed paper got id %d / %d", i, res[i].ID, papers[i].ID)
		}
	}
	if n := countPapers(t, d, prof.ID); n != 2 {
		t.Errorf("papers = %d, want 2 (failed ones rolled back)", n)
	}
	for _, i := range []int{0, 3} {
		if _, err := d.GetPaper(res[i].ID); err != nil {
			t.Errorf("paper %d not committed: %v", i, err)
		}
	}
}

func TestUpsertPapers_Empty(t *testing.T) {
	d := testDB(t)
	res, err := d.UpsertPapers(nil)
	if err != nil || len(res) != 0 {
		t.Errorf("UpsertPapers(nil) = %v, %v", res, err)
	}
}

func TestUpsertPaper_ReturnsPerPaperError(t *testing.T) {
	d := testDB(t)
	prof := createTestProfile(t, d)
	p := batchPaper(prof.ID, nil, "bad")
	p.Status = "bogus"
	if err := d.UpsertPaper(p); err == nil {
		t.Fatal("expected error")
	}
	if p.ID != 0 {
		t.Errorf("failed paper got id %d", p.ID)
	}
}
