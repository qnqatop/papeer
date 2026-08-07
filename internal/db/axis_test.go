package db

import "testing"

func createTestProfile(t *testing.T, d *DB) *Profile {
	t.Helper()
	p := &Profile{
		Name:            "Test",
		Email:           "test@example.com",
		YearMin:         2018,
		VintageYear:     2010,
		MaxPerQuery:     25,
		DownloadSources: JSONStringSlice{"arxiv"},
	}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAxisCRUD(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	// Save new axis with queries and keywords.
	a := &Axis{
		ProfileID:   p.ID,
		AxisKey:     "cold_start",
		Description: "Cold start problem",
		Queries: []Query{
			{Text: "cold start recommender"},
			{Text: "zero shot recommendation"},
		},
		Keywords: []Keyword{
			{Word: "cold", Type: "must"},
			{Word: "recommend", Type: "must"},
			{Word: "zero-shot", Type: "boost"},
		},
	}
	if err := d.SaveAxis(a); err != nil {
		t.Fatalf("SaveAxis (create): %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Get and verify.
	got, err := d.GetAxis(a.ID)
	if err != nil {
		t.Fatalf("GetAxis: %v", err)
	}
	if got.AxisKey != "cold_start" {
		t.Errorf("axis_key = %q", got.AxisKey)
	}
	if len(got.Queries) != 2 {
		t.Errorf("queries len = %d, want 2", len(got.Queries))
	}
	if len(got.Keywords) != 3 {
		t.Errorf("keywords len = %d, want 3", len(got.Keywords))
	}
	if got.Queries[0].Text != "cold start recommender" {
		t.Errorf("query[0] = %q", got.Queries[0].Text)
	}

	// Update: change queries.
	got.Description = "Updated description"
	got.Queries = []Query{
		{Text: "new query only"},
	}
	got.Keywords = []Keyword{
		{Word: "updated", Type: "boost"},
	}
	if err := d.SaveAxis(got); err != nil {
		t.Fatalf("SaveAxis (update): %v", err)
	}
	got2, _ := d.GetAxis(a.ID)
	if got2.Description != "Updated description" {
		t.Errorf("description = %q", got2.Description)
	}
	if len(got2.Queries) != 1 {
		t.Errorf("queries len = %d, want 1", len(got2.Queries))
	}
	if len(got2.Keywords) != 1 {
		t.Errorf("keywords len = %d, want 1", len(got2.Keywords))
	}

	// List.
	list, err := d.ListAxes(p.ID)
	if err != nil {
		t.Fatalf("ListAxes: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}

	// Reorder.
	a2 := &Axis{ProfileID: p.ID, AxisKey: "neural", Description: "Neural"}
	if err := d.SaveAxis(a2); err != nil {
		t.Fatal(err)
	}
	if err := d.ReorderAxes(p.ID, []int64{a2.ID, a.ID}); err != nil {
		t.Fatalf("ReorderAxes: %v", err)
	}
	list, _ = d.ListAxes(p.ID)
	if list[0].ID != a2.ID || list[1].ID != a.ID {
		t.Errorf("order wrong: %d, %d", list[0].ID, list[1].ID)
	}

	// Delete.
	if err := d.DeleteAxis(a.ID); err != nil {
		t.Fatalf("DeleteAxis: %v", err)
	}
	list, _ = d.ListAxes(p.ID)
	if len(list) != 1 {
		t.Errorf("after delete: len=%d", len(list))
	}
}

func TestAxisCascadeDelete(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	a := &Axis{
		ProfileID: p.ID,
		AxisKey:   "test",
		Queries:   []Query{{Text: "q1"}},
		Keywords:  []Keyword{{Word: "k1", Type: "must"}},
	}
	d.SaveAxis(a)

	// Deleting profile should cascade to axes, queries, keywords.
	d.DeleteProfile(p.ID)

	var count int
	d.QueryRow("SELECT COUNT(*) FROM axes").Scan(&count)
	if count != 0 {
		t.Errorf("axes not cascaded: %d", count)
	}
	d.QueryRow("SELECT COUNT(*) FROM queries").Scan(&count)
	if count != 0 {
		t.Errorf("queries not cascaded: %d", count)
	}
	d.QueryRow("SELECT COUNT(*) FROM keywords").Scan(&count)
	if count != 0 {
		t.Errorf("keywords not cascaded: %d", count)
	}
}
