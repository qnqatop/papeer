package db

import (
	"github.com/qnqatop/papeer/internal/ptr"
	"testing"
)

func TestPaperUpsert_NewPaper(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "Cold Start Recommender",
		TitleNormalized: "cold start recommender",
		Abstract:        "A paper about cold start.",
		Year:            ptr.Ptr(2022),
		DOI:             ptr.Ptr("10.1234/test"),
		Sources:         JSONStringSlice{"semantic_scholar"},
		Authors:         JSONStringSlice{"Author One"},
		ScoreReasons:    JSONStringSlice{},
		Status:          "new",
	}
	if err := d.UpsertPaper(paper); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if paper.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
}

func TestPaperUpsert_MergeByDOI(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	doi := "10.1234/merge-test"

	// First insert.
	p1 := &Paper{
		ProfileID:       p.ID,
		Title:           "Paper One",
		TitleNormalized: "paper one",
		Abstract:        "Short abstract.",
		Year:            ptr.Ptr(2022),
		DOI:             &doi,
		CitationCount:   10,
		Sources:         JSONStringSlice{"semantic_scholar"},
		Authors:         JSONStringSlice{"Author A"},
		ScoreReasons:    JSONStringSlice{},
		PdfURL:          ptr.Ptr("https://example.com/1.pdf"),
		PdfSource:       ptr.Ptr("openalex"),
		Status:          "new",
	}
	d.UpsertPaper(p1)

	// Second insert with same DOI — should merge.
	p2 := &Paper{
		ProfileID:       p.ID,
		Title:           "Paper One Variant",
		TitleNormalized: "paper one variant",
		Abstract:        "A much longer abstract that provides more detail about the paper.",
		Year:            ptr.Ptr(2022),
		DOI:             &doi,
		CitationCount:   50,
		Sources:         JSONStringSlice{"openalex"},
		Authors:         JSONStringSlice{"Author B"},
		ScoreReasons:    JSONStringSlice{},
		PdfURL:          ptr.Ptr("https://arxiv.org/pdf/123.pdf"),
		PdfSource:       ptr.Ptr("arxiv"),
		Status:          "new",
	}
	d.UpsertPaper(p2)

	// Verify merge result.
	got, err := d.GetPaper(p1.ID)
	if err != nil {
		t.Fatalf("GetPaper: %v", err)
	}

	// Sources merged.
	if len(got.Sources) != 2 {
		t.Errorf("sources = %v, want 2 items", got.Sources)
	}

	// Max citation count.
	if got.CitationCount != 50 {
		t.Errorf("citation_count = %d, want 50", got.CitationCount)
	}

	// Longer abstract.
	if got.Abstract != p2.Abstract {
		t.Errorf("abstract not updated to longer version")
	}

	// Better PDF source (arxiv has priority 0, openalex has priority 2).
	if got.PdfSource == nil || *got.PdfSource != "arxiv" {
		t.Errorf("pdf_source = %v, want arxiv", got.PdfSource)
	}

	// Only 1 paper in DB.
	papers, total, _ := d.ListPapers(PaperFilter{ProfileID: p.ID})
	if total != 1 || len(papers) != 1 {
		t.Errorf("total = %d, len = %d, want 1", total, len(papers))
	}
}

func TestPaperUpsert_MergeByTitle(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	// First insert without DOI.
	p1 := &Paper{
		ProfileID:       p.ID,
		Title:           "Neural Collaborative Filtering",
		TitleNormalized: "neural collaborative filtering",
		Sources:         JSONStringSlice{"arxiv"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "new",
	}
	d.UpsertPaper(p1)

	// Second with same normalized title, no DOI.
	p2 := &Paper{
		ProfileID:       p.ID,
		Title:           "Neural Collaborative Filtering!",
		TitleNormalized: "neural collaborative filtering",
		DOI:             ptr.Ptr("10.9999/ncf"),
		Sources:         JSONStringSlice{"crossref"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "new",
	}
	d.UpsertPaper(p2)

	got, _ := d.GetPaper(p1.ID)
	// DOI should be filled from second insert.
	if got.DOI == nil || *got.DOI != "10.9999/ncf" {
		t.Errorf("doi = %v, want 10.9999/ncf", got.DOI)
	}
}

func TestListPapers_Filters(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	for i, paper := range []Paper{
		{Title: "High Score Paper", TitleNormalized: "high score paper", PreScore: 8, Status: "approved", Year: ptr.Ptr(2023)},
		{Title: "Low Score Paper", TitleNormalized: "low score paper", PreScore: 2, Status: "new", Year: ptr.Ptr(2020)},
		{Title: "Medium Paper", TitleNormalized: "medium paper", PreScore: 5, Status: "new", Year: ptr.Ptr(2021)},
	} {
		paper.ProfileID = p.ID
		paper.Sources = JSONStringSlice{"test"}
		paper.Authors = JSONStringSlice{}
		paper.ScoreReasons = JSONStringSlice{}
		if err := d.UpsertPaper(&paper); err != nil {
			t.Fatalf("insert paper %d: %v", i, err)
		}
	}

	// Filter by status.
	papers, total, _ := d.ListPapers(PaperFilter{ProfileID: p.ID, Status: "new"})
	if total != 2 {
		t.Errorf("status=new: total=%d, want 2", total)
	}
	_ = papers

	// Filter by min score.
	_, total, _ = d.ListPapers(PaperFilter{ProfileID: p.ID, MinScore: 5})
	if total != 2 {
		t.Errorf("min_score=5: total=%d, want 2", total)
	}

	// Search by title.
	_, total, _ = d.ListPapers(PaperFilter{ProfileID: p.ID, Search: "High"})
	if total != 1 {
		t.Errorf("search=High: total=%d, want 1", total)
	}

	// Sort by year.
	papers, _, _ = d.ListPapers(PaperFilter{ProfileID: p.ID, SortBy: "year"})
	if papers[0].Title != "High Score Paper" {
		t.Errorf("sort by year: first = %q", papers[0].Title)
	}
}

func TestBulkUpdateStatus(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	var ids []int64
	for i := 0; i < 3; i++ {
		paper := &Paper{
			ProfileID:       p.ID,
			Title:           "Paper " + string(rune('A'+i)),
			TitleNormalized: "paper " + string(rune('a'+i)),
			Sources:         JSONStringSlice{"test"},
			Authors:         JSONStringSlice{},
			ScoreReasons:    JSONStringSlice{},
			Status:          "new",
		}
		d.UpsertPaper(paper)
		ids = append(ids, paper.ID)
	}

	n, err := d.BulkUpdateStatus(ids[:2], "approved")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("affected = %d, want 2", n)
	}

	got, _ := d.GetPaper(ids[0])
	if got.Status != "approved" {
		t.Errorf("status = %q", got.Status)
	}
	got, _ = d.GetPaper(ids[2])
	if got.Status != "new" {
		t.Errorf("status = %q, want new", got.Status)
	}
}
