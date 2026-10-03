package db

import (
	"testing"

	"github.com/qnqatop/papeer/internal/ptr"
)

func TestCountPapersByStatus_MatchesListPapers(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	other := &Profile{Name: "Other", Email: "other@example.com", DownloadSources: JSONStringSlice{}}
	if err := d.CreateProfile(other); err != nil {
		t.Fatal(err)
	}

	papers := []Paper{
		{ProfileID: p.ID, Title: "Graph neural nets", PreScore: 8, Status: "approved", Year: ptr.Ptr(2023)},
		{ProfileID: p.ID, Title: "Graph sampling", PreScore: 6, Status: "new", Year: ptr.Ptr(2022)},
		{ProfileID: p.ID, Title: "Graph theory basics", PreScore: 3, Status: "new", Year: ptr.Ptr(2019)},
		{ProfileID: p.ID, Title: "Unrelated chemistry", PreScore: 7, Status: "rejected", Year: ptr.Ptr(2023)},
		{ProfileID: p.ID, Title: "Graph downloads", PreScore: 9, Status: "downloaded", Year: ptr.Ptr(2024)},
		{ProfileID: other.ID, Title: "Graph elsewhere", PreScore: 9, Status: "new", Year: ptr.Ptr(2024)},
	}
	for i := range papers {
		papers[i].TitleNormalized = papers[i].Title
		papers[i].Sources = JSONStringSlice{"test"}
		papers[i].Authors = JSONStringSlice{}
		papers[i].ScoreReasons = JSONStringSlice{}
		if err := d.UpsertPaper(&papers[i]); err != nil {
			t.Fatalf("insert paper %d: %v", i, err)
		}
	}
	// user_score is not set by UpsertPaper.
	if err := d.SetUserScore(papers[0].ID, 5); err != nil {
		t.Fatal(err)
	}
	if err := d.SetUserScore(papers[1].ID, 2); err != nil {
		t.Fatal(err)
	}

	filters := map[string]PaperFilter{
		"all":            {ProfileID: p.ID},
		"search":         {ProfileID: p.ID, Search: "Graph"},
		"min_score":      {ProfileID: p.ID, MinScore: 6},
		"year_range":     {ProfileID: p.ID, YearFrom: 2020, YearTo: 2023},
		"min_user_score": {ProfileID: p.ID, MinUserScore: 2},
		// Status, sort and paging must not affect the counts.
		"ignores_status_and_paging": {ProfileID: p.ID, Status: "rejected", SortBy: "year", Limit: 1, Offset: 3},
	}
	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			got, err := d.CountPapersByStatus(f)
			if err != nil {
				t.Fatalf("CountPapersByStatus: %v", err)
			}
			for _, status := range []string{"new", "approved", "rejected", "downloaded"} {
				lf := f
				lf.Status = status
				lf.Limit, lf.Offset, lf.SortBy = 1, 0, ""
				_, want, err := d.ListPapers(lf)
				if err != nil {
					t.Fatalf("ListPapers: %v", err)
				}
				if got[status] != want {
					t.Errorf("status %q: count=%d, ListPapers total=%d", status, got[status], want)
				}
			}
		})
	}

	got, _ := d.CountPapersByStatus(PaperFilter{ProfileID: p.ID, MinUserScore: 2})
	if got["approved"] != 1 || got["new"] != 1 || got["rejected"] != 0 {
		t.Errorf("min_user_score=2: got %v, want approved=1 new=1", got)
	}
}
