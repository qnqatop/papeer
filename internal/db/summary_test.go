package db

import (
	"testing"
)

func TestSummary_CRUD(t *testing.T) {
	d := testDB(t)
	// Setup
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test Paper', 'test paper')`)

	s := &Summary{
		PaperID:  1,
		Provider: "deepseek",
		Model:    "deepseek-v4-pro",
		Status:   "generating",
	}

	// Create
	if err := d.CreateSummary(s); err != nil {
		t.Fatalf("CreateSummary: %v", err)
	}
	if s.ID == 0 {
		t.Error("expected non-zero ID after create")
	}

	// Get
	got, err := d.GetSummary(1, "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	if got.Status != "generating" {
		t.Errorf("expected status 'generating', got %q", got.Status)
	}

	// Update
	got.Content = "Updated content"
	got.TokensIn = 100
	got.TokensOut = 50
	got.Status = "done"
	if err := d.UpdateSummary(got); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}

	got2, _ := d.GetSummary(1, "deepseek-v4-pro")
	if got2.Content != "Updated content" {
		t.Error("content not updated")
	}

	// Delete
	if err := d.DeleteSummary(got.ID); err != nil {
		t.Fatalf("DeleteSummary: %v", err)
	}
	got3, _ := d.GetSummary(1, "deepseek-v4-pro")
	if got3 != nil {
		t.Error("expected nil after delete")
	}
}

func TestSummary_Duplicate(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test', 'test')`)

	s1 := &Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"}
	if err := d.CreateSummary(s1); err != nil {
		t.Fatal(err)
	}

	s2 := &Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"}
	if err := d.CreateSummary(s2); err == nil {
		t.Error("expected UNIQUE constraint error")
	}
}

func TestSummary_Upsert(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test', 'test')`)

	s := &Summary{PaperID: 1, Provider: "gemini", Model: "gemini-3-flash-preview", Content: "v1", Status: "done"}
	if err := d.UpsertSummary(s); err != nil {
		t.Fatal(err)
	}

	s.Content = "v2"
	if err := d.UpsertSummary(s); err != nil {
		t.Fatal(err)
	}

	got, _ := d.GetSummary(1, "gemini-3-flash-preview")
	if got.Content != "v2" {
		t.Errorf("expected 'v2', got %q", got.Content)
	}
}

func TestSummary_HasActiveSummary(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test', 'test')`)

	status, err := d.HasActiveSummary(1, "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	if status != "" {
		t.Errorf("expected empty status, got %q", status)
	}

	d.CreateSummary(&Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "generating"})
	status, _ = d.HasActiveSummary(1, "deepseek-v4-pro")
	if status != "generating" {
		t.Errorf("expected 'generating', got %q", status)
	}
}

func TestSummary_GetSummariesByPaper(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test', 'test')`)

	d.CreateSummary(&Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"})
	d.CreateSummary(&Summary{PaperID: 1, Provider: "gemini", Model: "gemini-3-flash-preview", Status: "done"})

	list, err := d.GetSummariesByPaper(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 summaries, got %d", len(list))
	}
}

func TestSummary_ListSummariesByProfile(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Paper A', 'paper a')`)

	d.CreateSummary(&Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"})

	list, err := d.ListSummariesByProfile(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].PaperTitle != "Paper A" {
		t.Errorf("expected 'Paper A', got %q", list[0].PaperTitle)
	}
}

func TestSummary_ListSummariesByProfile_CrossProfile(t *testing.T) {
	d := testDB(t)
	// Profile 1
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'profile1')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'axis1')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Paper A', 'paper a')`)

	// Profile 2
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (2, 'profile2')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (2, 2, 'axis2')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (2, 2, 2, 'Paper B', 'paper b')`)

	d.CreateSummary(&Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"})
	d.CreateSummary(&Summary{PaperID: 2, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"})

	// Profile 1 should only see Paper A's summary
	list, err := d.ListSummariesByProfile(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 summary for profile 1, got %d", len(list))
	}
	if list[0].PaperTitle != "Paper A" {
		t.Errorf("expected 'Paper A', got %q", list[0].PaperTitle)
	}
	if list[0].AxisKey != "axis1" {
		t.Errorf("expected axis_key 'axis1', got %q", list[0].AxisKey)
	}

	// Profile 2 should only see Paper B's summary
	list2, err := d.ListSummariesByProfile(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Fatalf("expected 1 summary for profile 2, got %d", len(list2))
	}
	if list2[0].PaperTitle != "Paper B" {
		t.Errorf("expected 'Paper B', got %q", list2[0].PaperTitle)
	}
}

func TestSummary_CascadeDelete(t *testing.T) {
	d := testDB(t)
	d.Exec(`INSERT INTO search_profiles (id, name) VALUES (1, 'test')`)
	d.Exec(`INSERT INTO axes (id, profile_id, axis_key) VALUES (1, 1, 'test')`)
	d.Exec(`INSERT INTO papers (id, profile_id, axis_id, title, title_normalized) VALUES (1, 1, 1, 'Test', 'test')`)

	d.CreateSummary(&Summary{PaperID: 1, Provider: "deepseek", Model: "deepseek-v4-pro", Status: "done"})

	// Delete paper — summary should be cascade-deleted
	d.Exec(`DELETE FROM papers WHERE id=1`)

	got, _ := d.GetSummary(1, "deepseek-v4-pro")
	if got != nil {
		t.Error("summary should be deleted on CASCADE")
	}
}
