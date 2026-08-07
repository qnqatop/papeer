package db

import "testing"

func TestProfileCRUD(t *testing.T) {
	d := testDB(t)

	// Create
	p := &Profile{
		Name:            "Test Profile",
		Email:           "test@example.com",
		PdfDir:          "/tmp/pdfs",
		YearMin:         2018,
		VintageYear:     2010,
		MaxPerQuery:     25,
		DownloadSources: JSONStringSlice{"arxiv", "semantic_scholar"},
	}
	if err := d.CreateProfile(p); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Get
	got, err := d.GetProfile(p.ID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "Test Profile" {
		t.Errorf("name = %q, want %q", got.Name, "Test Profile")
	}
	if got.Email != "test@example.com" {
		t.Errorf("email = %q", got.Email)
	}
	if len(got.DownloadSources) != 2 || got.DownloadSources[0] != "arxiv" {
		t.Errorf("download_sources = %v", got.DownloadSources)
	}

	// List
	list, err := d.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}

	// Update
	got.Name = "Updated"
	got.YearMin = 2020
	if err := d.UpdateProfile(got); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got2, _ := d.GetProfile(p.ID)
	if got2.Name != "Updated" || got2.YearMin != 2020 {
		t.Errorf("after update: name=%q year_min=%d", got2.Name, got2.YearMin)
	}

	// Delete
	if err := d.DeleteProfile(p.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	list, _ = d.ListProfiles()
	if len(list) != 0 {
		t.Errorf("after delete: len=%d", len(list))
	}
}
