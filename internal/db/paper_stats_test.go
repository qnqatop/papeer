package db

import (
	"testing"
)

// ─── TopAuthors ───────────────────────────────────────────────────────────

func TestTopAuthors_CountsAndSortsDescending(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	papers := []Paper{
		{Title: "P1", TitleNormalized: "p1", Authors: JSONStringSlice{"Alice", "Bob"}, Status: "new"},
		{Title: "P2", TitleNormalized: "p2", Authors: JSONStringSlice{"Alice", "Carol"}, Status: "new"},
		{Title: "P3", TitleNormalized: "p3", Authors: JSONStringSlice{"Alice"}, Status: "new"},
		{Title: "P4", TitleNormalized: "p4", Authors: JSONStringSlice{"Bob"}, Status: "new"},
	}
	for i := range papers {
		papers[i].ProfileID = p.ID
		papers[i].Sources = JSONStringSlice{"test"}
		papers[i].ScoreReasons = JSONStringSlice{}
		if err := d.UpsertPaper(&papers[i]); err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
	}

	got, err := d.TopAuthors(p.ID, nil, 10)
	if err != nil {
		t.Fatalf("TopAuthors: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (Alice, Bob, Carol)", len(got))
	}
	if got[0].Name != "Alice" || got[0].Count != 3 {
		t.Errorf("top author = %+v, want Alice with 3", got[0])
	}
	if got[1].Name != "Bob" || got[1].Count != 2 {
		t.Errorf("2nd author = %+v, want Bob with 2", got[1])
	}
}

func TestTopAuthors_LimitTruncates(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	paper := &Paper{
		ProfileID: p.ID, Title: "P", TitleNormalized: "p",
		Authors: JSONStringSlice{"A", "B", "C", "D"},
		Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Status: "new",
	}
	if err := d.UpsertPaper(paper); err != nil {
		t.Fatal(err)
	}

	got, err := d.TopAuthors(p.ID, nil, 2)
	if err != nil {
		t.Fatalf("TopAuthors: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2 (limit)", len(got))
	}
}

func TestTopAuthors_ScopedByAxisViaM2M(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	axis1 := &Axis{ProfileID: p.ID, AxisKey: "axis1"}
	axis2 := &Axis{ProfileID: p.ID, AxisKey: "axis2"}
	if err := d.SaveAxis(axis1); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveAxis(axis2); err != nil {
		t.Fatal(err)
	}

	// Paper found via axis1 only.
	p1 := &Paper{
		ProfileID: p.ID, AxisID: &axis1.ID, Title: "P1", TitleNormalized: "p1",
		Authors: JSONStringSlice{"OnlyAxis1"}, Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Status: "new",
	}
	if err := d.UpsertPaper(p1); err != nil {
		t.Fatal(err)
	}

	// Paper found via both axis1 and axis2 (paper_axes M2M, not just the
	// legacy single-valued papers.axis_id column).
	p2 := &Paper{
		ProfileID: p.ID, AxisID: &axis1.ID, Title: "P2", TitleNormalized: "p2",
		Authors: JSONStringSlice{"BothAxes"}, Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Status: "new",
	}
	if err := d.UpsertPaper(p2); err != nil {
		t.Fatal(err)
	}
	if err := d.LinkPaperAxis(p2.ID, axis2.ID); err != nil {
		t.Fatalf("LinkPaperAxis: %v", err)
	}

	// axis2 should only see the paper explicitly linked to it via M2M, even
	// though papers.axis_id (legacy column) points p2 at axis1.
	gotAxis2, err := d.TopAuthors(p.ID, &axis2.ID, 10)
	if err != nil {
		t.Fatalf("TopAuthors(axis2): %v", err)
	}
	if len(gotAxis2) != 1 || gotAxis2[0].Name != "BothAxes" {
		t.Errorf("TopAuthors(axis2) = %+v, want only BothAxes", gotAxis2)
	}

	gotAxis1, err := d.TopAuthors(p.ID, &axis1.ID, 10)
	if err != nil {
		t.Fatalf("TopAuthors(axis1): %v", err)
	}
	if len(gotAxis1) != 2 {
		t.Errorf("TopAuthors(axis1) = %+v, want both papers", gotAxis1)
	}
}

// ─── CitationCountBuckets ─────────────────────────────────────────────────

func TestCitationCountBuckets(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	counts := []int{0, 1, 5, 6, 20, 21, 50, 51, 100, 101, 500}
	for i, c := range counts {
		paper := &Paper{
			ProfileID: p.ID, Title: string(rune('A' + i)), TitleNormalized: string(rune('a' + i)),
			CitationCount: c, Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}, Status: "new",
		}
		if err := d.UpsertPaper(paper); err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
	}

	buckets, err := d.CitationCountBuckets(p.ID, nil)
	if err != nil {
		t.Fatalf("CitationCountBuckets: %v", err)
	}

	want := map[string]int{
		"0": 1, "1-5": 2, "6-20": 2, "21-50": 2, "51-100": 2, "100+": 2,
	}
	if len(buckets) != len(want) {
		t.Fatalf("len(buckets) = %d, want %d", len(buckets), len(want))
	}
	// Order must be stable (chart-ready): 0, 1-5, 6-20, 21-50, 51-100, 100+.
	wantOrder := []string{"0", "1-5", "6-20", "21-50", "51-100", "100+"}
	for i, label := range wantOrder {
		if buckets[i].Label != label {
			t.Errorf("bucket[%d].Label = %q, want %q", i, buckets[i].Label, label)
		}
		if buckets[i].Count != want[label] {
			t.Errorf("bucket %q count = %d, want %d", label, buckets[i].Count, want[label])
		}
	}
}

func TestCitationCountBuckets_AxisFilter(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	axis := &Axis{ProfileID: p.ID, AxisKey: "a"}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	inAxis := &Paper{ProfileID: p.ID, AxisID: &axis.ID, Title: "In", TitleNormalized: "in",
		CitationCount: 10, Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}, Status: "new"}
	outAxis := &Paper{ProfileID: p.ID, Title: "Out", TitleNormalized: "out",
		CitationCount: 10, Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}, Status: "new"}
	if err := d.UpsertPaper(inAxis); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertPaper(outAxis); err != nil {
		t.Fatal(err)
	}

	buckets, err := d.CitationCountBuckets(p.ID, &axis.ID)
	if err != nil {
		t.Fatalf("CitationCountBuckets: %v", err)
	}
	total := 0
	for _, b := range buckets {
		total += b.Count
	}
	if total != 1 {
		t.Errorf("total count scoped to axis = %d, want 1", total)
	}
}

// ─── GetPDFAvailability ───────────────────────────────────────────────────

func TestGetPDFAvailability(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	withPDF := &Paper{ProfileID: p.ID, Title: "WithPDF", TitleNormalized: "withpdf", Status: "downloaded",
		Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}}
	withoutPDF := &Paper{ProfileID: p.ID, Title: "WithoutPDF", TitleNormalized: "withoutpdf", Status: "approved",
		Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}}
	notEligible := &Paper{ProfileID: p.ID, Title: "New", TitleNormalized: "new", Status: "new",
		Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{}}
	if err := d.UpsertPaper(withPDF); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertPaper(withoutPDF); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertPaper(notEligible); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Exec(`INSERT INTO downloads (paper_id, source, status) VALUES (?, 'arxiv', 'ok')`, withPDF.ID); err != nil {
		t.Fatalf("insert download: %v", err)
	}
	// A failed download attempt must not count as "with PDF".
	if _, err := d.Exec(`INSERT INTO downloads (paper_id, source, status) VALUES (?, 'arxiv', 'fail')`, withoutPDF.ID); err != nil {
		t.Fatalf("insert download: %v", err)
	}

	got, err := d.GetPDFAvailability(p.ID)
	if err != nil {
		t.Fatalf("GetPDFAvailability: %v", err)
	}
	if got.Approved != 2 {
		t.Errorf("Approved = %d, want 2 (approved+downloaded, excludes 'new')", got.Approved)
	}
	if got.WithPDF != 1 {
		t.Errorf("WithPDF = %d, want 1", got.WithPDF)
	}
	if got.Percent != 50 {
		t.Errorf("Percent = %v, want 50", got.Percent)
	}
}

func TestGetPDFAvailability_ZeroApprovedNoDivideByZero(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	got, err := d.GetPDFAvailability(p.ID)
	if err != nil {
		t.Fatalf("GetPDFAvailability: %v", err)
	}
	if got.Approved != 0 || got.WithPDF != 0 || got.Percent != 0 {
		t.Errorf("got = %+v, want all zero", got)
	}
}

// ─── PapersByStatus / YearDistribution axis M2M scoping ──────────────────

func TestPapersByStatus_ScopedByAxisViaM2M(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	axis1 := &Axis{ProfileID: p.ID, AxisKey: "axis1"}
	axis2 := &Axis{ProfileID: p.ID, AxisKey: "axis2"}
	if err := d.SaveAxis(axis1); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveAxis(axis2); err != nil {
		t.Fatal(err)
	}

	paper := &Paper{
		ProfileID: p.ID, AxisID: &axis1.ID, Title: "Shared", TitleNormalized: "shared", Status: "approved",
		Sources: JSONStringSlice{"test"}, ScoreReasons: JSONStringSlice{}, Authors: JSONStringSlice{},
	}
	if err := d.UpsertPaper(paper); err != nil {
		t.Fatal(err)
	}
	// Link the same paper to a second axis via the M2M table — a paper found
	// through multiple axes must be counted under both.
	if err := d.LinkPaperAxis(paper.ID, axis2.ID); err != nil {
		t.Fatalf("LinkPaperAxis: %v", err)
	}

	for _, axis := range []*Axis{axis1, axis2} {
		byStatus, err := d.PapersByStatus(p.ID, &axis.ID)
		if err != nil {
			t.Fatalf("PapersByStatus(%s): %v", axis.AxisKey, err)
		}
		if byStatus["approved"] != 1 {
			t.Errorf("PapersByStatus(%s)['approved'] = %d, want 1", axis.AxisKey, byStatus["approved"])
		}
	}
}
