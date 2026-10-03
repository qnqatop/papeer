package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/llm"
)

// ─── Settings whitelist / secret filtering ───────────────────────────────

func TestSaveSetting_RejectsUnknownKey(t *testing.T) {
	a := newTestApp(t)
	for _, key := range []string{"semantic_scholar_api_key", "llm_deepseek_api_key", "whatever"} {
		if err := a.SaveSetting(key, "x"); err == nil {
			t.Errorf("SaveSetting(%q) = nil, want error", key)
		}
		if v, _ := a.db.GetSetting(key); v != "" {
			t.Errorf("rejected key %q was stored: %q", key, v)
		}
	}
	if err := a.SaveSetting("proxy_url", "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("SaveSetting(proxy_url): %v", err)
	}
	if v, _ := a.db.GetSetting("proxy_url"); v != "http://127.0.0.1:8080" {
		t.Errorf("proxy_url = %q", v)
	}
}

func TestGetSettings_HidesSecrets(t *testing.T) {
	a := newTestApp(t)
	_ = a.db.SetSetting("semantic_scholar_api_key", "secret-s2")
	_ = a.db.SetSetting("llm_deepseek_api_key", "secret-ds")
	_ = a.db.SetSetting("auto_update_check", "false")

	got, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range secretSettingKeys {
		if _, ok := got[k]; ok {
			t.Errorf("GetSettings leaked secret %q", k)
		}
	}
	if got["auto_update_check"] != "false" {
		t.Errorf("non-secret setting missing: %v", got)
	}
}

// ─── Semantic Scholar key in the keychain ────────────────────────────────

func TestMigrateS2Key_MovesToKeychain(t *testing.T) {
	keyring.MockInit()
	a := newTestApp(t)
	_ = a.db.SetSetting(s2KeySetting, "s2-legacy-key-1234")

	a.migrateS2Key()

	if v, _ := llm.GetSecret(s2KeySetting); v != "s2-legacy-key-1234" {
		t.Errorf("keychain = %q, want migrated key", v)
	}
	if v, _ := a.db.GetSetting(s2KeySetting); v != "" {
		t.Errorf("plaintext key left in DB: %q", v)
	}
	if got := a.semanticScholarKey(); got != "s2-legacy-key-1234" {
		t.Errorf("cached key = %q", got)
	}
}

func TestMigrateS2Key_KeychainUnavailableKeepsFallback(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(keyring.MockInit)
	a := newTestApp(t)
	_ = a.db.SetSetting(s2KeySetting, "s2-legacy-key-1234")

	a.migrateS2Key()

	if v, _ := a.db.GetSetting(s2KeySetting); v != "s2-legacy-key-1234" {
		t.Errorf("plaintext key must be kept when keychain is unavailable, got %q", v)
	}
	if got := a.semanticScholarKey(); got != "s2-legacy-key-1234" {
		t.Errorf("fallback key = %q, want DB value", got)
	}
}

func TestSetSemanticScholarKey_RoundTrip(t *testing.T) {
	keyring.MockInit()
	a := newTestApp(t)

	st, err := a.GetSemanticScholarKeyStatus()
	if err != nil || st.Set {
		t.Fatalf("initial status = %+v, %v; want unset", st, err)
	}

	if err := a.SetSemanticScholarKey("  abcdefgh12345678  "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	st, _ = a.GetSemanticScholarKeyStatus()
	if !st.Set || st.Masked != llm.MaskKey("abcdefgh12345678") {
		t.Errorf("status = %+v", st)
	}
	if got := a.semanticScholarKey(); got != "abcdefgh12345678" {
		t.Errorf("cached key = %q (must be trimmed)", got)
	}

	// A fresh App (e.g. next launch) loads it from the keychain.
	b := &App{db: a.db, ctx: a.ctx}
	if got := b.semanticScholarKey(); got != "abcdefgh12345678" {
		t.Errorf("reloaded key = %q", got)
	}

	if err := a.SetSemanticScholarKey(""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if st, _ := a.GetSemanticScholarKeyStatus(); st.Set {
		t.Errorf("status after clear = %+v", st)
	}
	if v, _ := llm.GetSecret(s2KeySetting); v != "" {
		t.Errorf("keychain still holds %q", v)
	}
}

func TestSetSemanticScholarKey_KeychainUnavailable(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(keyring.MockInit)
	a := newTestApp(t)

	if err := a.SetSemanticScholarKey("abcdefgh12345678"); err == nil {
		t.Fatal("expected an error when the keychain is unavailable")
	}
	if v, _ := a.db.GetSetting(s2KeySetting); v != "" {
		t.Errorf("key must not fall back to plaintext storage, got %q", v)
	}
}

// ─── /api/pdf handler wiring ─────────────────────────────────────────────

func TestNewPDFHandler(t *testing.T) {
	// Before Startup the DB is nil: 503, not a panic.
	rec := httptest.NewRecorder()
	NewPDFHandler(&App{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pdf/1", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("nil db: status = %d, want 503", rec.Code)
	}

	a := newTestApp(t)
	pdfDir := t.TempDir()
	prof, err := a.CreateProfile(db.Profile{Name: "P", Email: "real@univ.edu", PdfDir: pdfDir})
	if err != nil {
		t.Fatal(err)
	}
	addPaper := func(title, filename string, body []byte) int64 {
		p := &db.Paper{
			ProfileID: prof.ID, Title: title, TitleNormalized: title, Status: "downloaded",
			Sources: db.JSONStringSlice{}, Authors: db.JSONStringSlice{}, ScoreReasons: db.JSONStringSlice{},
		}
		if err := a.db.UpsertPaper(p); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pdfDir, filename), body, 0o644); err != nil {
			t.Fatal(err)
		}
		fn := filename
		if err := a.db.SaveDownload(&db.Download{PaperID: p.ID, Source: "test", Status: "ok", Filename: &fn}); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	good := addPaper("good", "good.pdf", []byte("%PDF-1.4 test"))
	notPDF := addPaper("bad", "evil.html", []byte("<script>alert(1)</script>"))

	h := NewPDFHandler(a)
	cases := []struct {
		path string
		want int
	}{
		{"/api/pdf/" + strconv.FormatInt(good, 10), http.StatusOK},
		{"/api/pdf/" + strconv.FormatInt(notPDF, 10), http.StatusNotFound}, // fails validatePDFPath
		{"/api/pdf/999999", http.StatusNotFound},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d", c.path, rec.Code, c.want)
		}
	}
}
