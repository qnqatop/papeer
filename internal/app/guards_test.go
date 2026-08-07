package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

// newTestApp builds an App backed by a fresh on-disk SQLite database in a
// temp directory. The DB is closed automatically when the test ends.
//
// We deliberately set a.ctx to context.Background() — the email guards we
// exercise here return ErrInvalidEmail *before* any wails-runtime call, so
// the placeholder context is never used in those paths.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	database, err := db.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	return &App{
		db:  database,
		ctx: context.Background(),
	}
}

// createValidProfile inserts a profile with a real-looking email so callers
// can exercise the path beyond the email guard.
func createValidProfile(t *testing.T, a *App, name, email string) *db.Profile {
	t.Helper()
	p, err := a.CreateProfile(db.Profile{Name: name, Email: email, YearMin: 2020})
	if err != nil {
		t.Fatalf("CreateProfile(%q, %q): %v", name, email, err)
	}
	return p
}

// insertProfileRaw bypasses CreateProfile to seed legacy data with invalid
// email — simulates an existing DB from before the email-required change.
func insertProfileRaw(t *testing.T, a *App, name, email string) *db.Profile {
	t.Helper()
	p := &db.Profile{Name: name, Email: email, YearMin: 2020}
	if err := a.db.CreateProfile(p); err != nil {
		t.Fatalf("raw CreateProfile: %v", err)
	}
	return p
}

func TestCreateProfile_RejectsInvalidEmail(t *testing.T) {
	a := newTestApp(t)

	cases := []struct {
		name  string
		email string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"malformed", "not-an-email"},
		{"placeholder example.com", "u@example.com"},
		{"placeholder test.com", "u@test.com"},
		{"placeholder localhost", "u@localhost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.CreateProfile(db.Profile{Name: "X", Email: tc.email})
			if !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("CreateProfile(%q) err=%v, want ErrInvalidEmail", tc.email, err)
			}
		})
	}
}

func TestCreateProfile_AcceptsValidEmail(t *testing.T) {
	a := newTestApp(t)

	p, err := a.CreateProfile(db.Profile{Name: "Real", Email: "real@univ.edu"})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p == nil || p.ID == 0 {
		t.Fatalf("expected stored profile with id, got %+v", p)
	}
}

func TestUpdateProfile_RejectsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := createValidProfile(t, a, "P1", "real@anthropic.com")

	p.Email = "u@example.com"
	if err := a.UpdateProfile(*p); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("UpdateProfile err=%v, want ErrInvalidEmail", err)
	}
}

func TestUpdateProfile_AcceptsValidEmail(t *testing.T) {
	a := newTestApp(t)
	p := createValidProfile(t, a, "P1", "real@anthropic.com")

	p.Email = "real@univ.edu"
	if err := a.UpdateProfile(*p); err != nil {
		t.Errorf("UpdateProfile err=%v, want nil", err)
	}

	got, err := a.GetProfile(p.ID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Email != "real@univ.edu" {
		t.Errorf("Email = %q, want %q", got.Email, "real@univ.edu")
	}
}

func TestSearchAxis_GuardsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "")

	_, err := a.SearchAxis(p.ID, 1)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("SearchAxis err=%v, want ErrInvalidEmail", err)
	}
}

func TestSearchAllAxes_GuardsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "u@example.com")

	_, err := a.SearchAllAxes(p.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("SearchAllAxes err=%v, want ErrInvalidEmail", err)
	}
}

func TestDownloadApproved_GuardsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "")

	err := a.DownloadApproved(p.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("DownloadApproved err=%v, want ErrInvalidEmail", err)
	}
}

func TestRunRadar_GuardsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "")

	_, err := a.RunRadar(p.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("RunRadar err=%v, want ErrInvalidEmail", err)
	}
}

func TestFetchCitations_GuardsInvalidEmail(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "")

	err := a.FetchCitations(p.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("FetchCitations err=%v, want ErrInvalidEmail", err)
	}
}

func TestProfilesNeedingEmail_FiltersByValidity(t *testing.T) {
	a := newTestApp(t)
	insertProfileRaw(t, a, "Legacy1", "")
	insertProfileRaw(t, a, "Legacy2", "u@example.com")
	insertProfileRaw(t, a, "Legacy3", "garbage")
	createValidProfile(t, a, "Real", "real@univ.edu")

	bad, err := a.ProfilesNeedingEmail()
	if err != nil {
		t.Fatalf("ProfilesNeedingEmail: %v", err)
	}
	if len(bad) != 3 {
		t.Fatalf("len(bad) = %d, want 3", len(bad))
	}
	gotNames := map[string]bool{}
	for _, p := range bad {
		gotNames[p.Name] = true
	}
	for _, want := range []string{"Legacy1", "Legacy2", "Legacy3"} {
		if !gotNames[want] {
			t.Errorf("expected %q in result, got names %v", want, gotNames)
		}
	}
	if gotNames["Real"] {
		t.Errorf("valid profile leaked into ProfilesNeedingEmail result")
	}
}

func TestProfilesNeedingEmail_EmptyWhenAllValid(t *testing.T) {
	a := newTestApp(t)
	createValidProfile(t, a, "P1", "real@univ.edu")
	createValidProfile(t, a, "P2", "u@uni-bonn.de")

	bad, err := a.ProfilesNeedingEmail()
	if err != nil {
		t.Fatalf("ProfilesNeedingEmail: %v", err)
	}
	if len(bad) != 0 {
		t.Errorf("expected empty result, got %d profiles", len(bad))
	}
}
