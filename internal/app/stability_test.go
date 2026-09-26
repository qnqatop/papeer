package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/llm"
)

// ─── Radar: stopRadar must never block behind the scheduler ──────────────

func TestStopRadar_DoesNotDeadlockWithPeriodicRadar(t *testing.T) {
	old := radarStartDelay
	radarStartDelay = 0
	t.Cleanup(func() { radarStartDelay = old })

	a := newTestApp(t)
	if err := a.db.SetSetting("enable_radar", "true"); err != nil {
		t.Fatal(err)
	}
	if err := a.db.SetSetting("radar_frequency", "3h"); err != nil {
		t.Fatal(err)
	}

	exited := make(chan struct{})
	go func() {
		a.startRadarIfEnabled()
		close(exited)
	}()

	// Give the scheduler time to run once (no profiles) and enter its loop.
	time.Sleep(200 * time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		a.stopRadar()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stopRadar blocked: radar mutex held by the scheduler loop")
	}
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("radar scheduler did not exit after stopRadar")
	}
}

func TestStopRadar_BeforeStartPreventsStart(t *testing.T) {
	a := newTestApp(t)
	a.stopRadar()

	done := make(chan struct{})
	go func() {
		a.startRadarIfEnabled() // must return immediately, not sleep/run
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startRadarIfEnabled ran after stopRadar")
	}
}

func TestRadarLoop_StopsOnClose(t *testing.T) {
	stop := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	done := make(chan struct{})
	go func() {
		radarLoop(stop, 5*time.Millisecond, func() {
			mu.Lock()
			runs++
			mu.Unlock()
		})
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("radarLoop did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if runs == 0 {
		t.Error("radarLoop never ran")
	}
}

// ─── Cancel tokens ───────────────────────────────────────────────────────

func TestCancelTokens_IndependentOps(t *testing.T) {
	a := newTestApp(t)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	tok1 := a.setCancel(cancel1)
	tok2 := a.setCancel(cancel2)
	if tok1 == tok2 {
		t.Fatal("tokens must be unique")
	}

	// The first op finishing must not unregister the second one.
	a.clearCancel(tok1)
	a.CancelOperation()
	if ctx2.Err() == nil {
		t.Error("second op was not cancelled: first op's clearCancel wiped it")
	}
	if ctx1.Err() != nil {
		t.Error("finished op must not be cancelled after clearCancel")
	}
	a.clearCancel(tok2)

	// CancelOperation cancels all active ops.
	ctx3, cancel3 := context.WithCancel(context.Background())
	ctx4, cancel4 := context.WithCancel(context.Background())
	tok3 := a.setCancel(cancel3)
	tok4 := a.setCancel(cancel4)
	a.CancelOperation()
	if ctx3.Err() == nil || ctx4.Err() == nil {
		t.Error("CancelOperation must cancel every active op")
	}
	a.clearCancel(tok3)
	a.clearCancel(tok4)
	a.cancelMu.Lock()
	n := len(a.cancels)
	a.cancelMu.Unlock()
	if n != 0 {
		t.Errorf("cancels left registered: %d", n)
	}
}

// ─── safeGo ──────────────────────────────────────────────────────────────

func TestSafeGo_RecoversPanic(t *testing.T) {
	logged := make(chan string, 1)
	old := logPanic
	logPanic = func(_ context.Context, format string, args ...interface{}) {
		logged <- fmt.Sprintf(format, args...)
	}
	t.Cleanup(func() { logPanic = old })

	a := newTestApp(t)
	deferRan := make(chan struct{})
	a.safeGo("boom-op", func() {
		defer close(deferRan)
		panic("boom")
	})

	select {
	case msg := <-logged:
		if !strings.Contains(msg, "boom-op") {
			t.Errorf("panic log %q lacks goroutine name", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("panic was not recovered/logged")
	}
	select {
	case <-deferRan:
	default:
		t.Error("deferred calls inside fn must run on panic")
	}
}

// ─── DownloadPaper guard ─────────────────────────────────────────────────

func TestDownloadPaper_GuardReleasedOnEarlyError(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "") // invalid email → early error
	paper := &db.Paper{
		ProfileID: p.ID, Title: "T", TitleNormalized: "t", Status: "approved",
		Sources: db.JSONStringSlice{}, Authors: db.JSONStringSlice{}, ScoreReasons: db.JSONStringSlice{},
	}
	if err := a.db.UpsertPaper(paper); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := a.DownloadPaper(paper.ID); !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("attempt %d: err = %v, want ErrInvalidEmail (guard must not swallow retries)", i, err)
		}
	}
	// Unknown paper: GetPaper fails — the guard must be released too.
	_ = a.DownloadPaper(999999)

	a.dlPaperMu.Lock()
	defer a.dlPaperMu.Unlock()
	if len(a.dlPaperSet) != 0 {
		t.Errorf("guard set not cleared: %v", a.dlPaperSet)
	}
}

// ─── YAML import ─────────────────────────────────────────────────────────

func TestImportYAMLFile_TransactionalAndPositioned(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	if _, err := a.SaveAxis(db.Axis{ProfileID: prof.ID, AxisKey: "existing", Position: 2}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	os.WriteFile(good, []byte(`topics:
  zeta:
    description: z
    keywords_exclude: [survey]
  alpha:
    description: a
    queries: [q1]
`), 0o644)

	n, err := a.importYAMLFile(prof.ID, good)
	if err != nil || n != 2 {
		t.Fatalf("import = %d, %v; want 2, nil", n, err)
	}
	axes, _ := a.ListAxes(prof.ID)
	if len(axes) != 3 || axes[1].AxisKey != "alpha" || axes[1].Position != 3 || axes[2].AxisKey != "zeta" || axes[2].Position != 4 {
		t.Fatalf("axes after import = %+v", axes)
	}
	if len(axes[2].Keywords) != 1 || axes[2].Keywords[0].Type != "exclude" {
		t.Errorf("exclude keyword lost: %+v", axes[2].Keywords)
	}

	// "existing" clashes with an axis already in the profile: nothing from
	// this file may be imported.
	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("topics:\n  aaa_new: {description: x}\n  existing: {description: dup}\n"), 0o644)
	if _, err := a.importYAMLFile(prof.ID, bad); err == nil {
		t.Fatal("expected duplicate axis_key error")
	}
	axes, _ = a.ListAxes(prof.ID)
	if len(axes) != 3 {
		t.Errorf("partial import: %d axes, want 3", len(axes))
	}
}

// ─── LLM profile base URL / key protection ───────────────────────────────

func TestValidateLLMBaseURL(t *testing.T) {
	ok := []string{"", "https://api.deepseek.com/v1", "http://localhost:11434/v1",
		"http://127.0.0.1:1234/v1", "http://[::1]:8080/v1", "HTTPS://Example.org"}
	for _, u := range ok {
		if err := validateLLMBaseURL(u); err != nil {
			t.Errorf("validateLLMBaseURL(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{"http://api.example.org/v1", "http://192.168.1.5:11434", "ftp://host", "api.deepseek.com/v1", "://x"}
	for _, u := range bad {
		if err := validateLLMBaseURL(u); err == nil {
			t.Errorf("validateLLMBaseURL(%q) = nil, want error", u)
		}
	}
}

func TestLLMOrigin(t *testing.T) {
	if llmOrigin("https://API.x.com/v1") != llmOrigin("https://api.x.com:443/other") {
		t.Error("same host with default port and different path must be the same origin")
	}
	if llmOrigin("https://api.x.com/v1") == llmOrigin("https://api.x.com:8443/v1") {
		t.Error("different port must be a different origin")
	}
	if llmOrigin("") != llmOrigin("https://api.openai.com/v1") {
		t.Error("empty base URL is the SDK default (api.openai.com)")
	}
}

func TestUpdateLLMProfile_HostChangeRequiresKey(t *testing.T) {
	keyring.MockInit()
	a := newTestApp(t)

	created, err := a.CreateLLMProfile(db.LLMProfile{
		Name: "P", BaseURL: "https://api.good.com/v1", Models: db.JSONStringSlice{"m"}, DefaultModel: "m",
	}, "sk-secret-key-1234")
	if err != nil {
		t.Fatalf("CreateLLMProfile: %v", err)
	}

	moved := created.LLMProfile
	moved.BaseURL = "https://evil.example.net/v1"
	if _, err := a.UpdateLLMProfile(moved, ""); !errors.Is(err, ErrLLMKeyReentry) {
		t.Fatalf("host change without key: err = %v, want ErrLLMKeyReentry", err)
	}
	stored, _ := a.db.GetLLMProfile(created.ID)
	if stored.BaseURL != "https://api.good.com/v1" {
		t.Errorf("base URL changed despite refusal: %q", stored.BaseURL)
	}

	// Same origin, different path: allowed without re-entering the key.
	samehost := created.LLMProfile
	samehost.BaseURL = "https://api.good.com/v2"
	if _, err := a.UpdateLLMProfile(samehost, ""); err != nil {
		t.Errorf("path-only change: %v", err)
	}

	// Host change with a new key: allowed, key replaced.
	if _, err := a.UpdateLLMProfile(moved, "sk-new-key-5678"); err != nil {
		t.Fatalf("host change with new key: %v", err)
	}
	if k, _ := llm.GetKey(created.ID); k != "sk-new-key-5678" {
		t.Errorf("key = %q, want the new key", k)
	}

	// Plain http to a remote host is refused.
	insecure := created.LLMProfile
	insecure.BaseURL = "http://evil.example.net/v1"
	if _, err := a.UpdateLLMProfile(insecure, "sk-other-key-0000"); !errors.Is(err, ErrInsecureLLMBaseURL) {
		t.Errorf("http update: err = %v, want ErrInsecureLLMBaseURL", err)
	}
}

func TestCreateLLMProfile_RejectsRemoteHTTP(t *testing.T) {
	keyring.MockInit()
	a := newTestApp(t)
	_, err := a.CreateLLMProfile(db.LLMProfile{Name: "X", BaseURL: "http://api.example.org/v1", DefaultModel: "m"}, "k")
	if !errors.Is(err, ErrInsecureLLMBaseURL) {
		t.Errorf("err = %v, want ErrInsecureLLMBaseURL", err)
	}
	if list, _ := a.db.ListLLMProfiles(); len(list) != 0 {
		t.Errorf("profile created despite invalid base URL")
	}
	if _, err := a.CreateLLMProfile(db.LLMProfile{Name: "Local", BaseURL: "http://localhost:11434/v1", DefaultModel: "m"}, ""); err != nil {
		t.Errorf("local http profile: %v", err)
	}
	if err := a.TestLLMProfileDraft("http://api.example.org/v1", "k", "m"); !errors.Is(err, ErrInsecureLLMBaseURL) {
		t.Errorf("draft test over http: err = %v, want ErrInsecureLLMBaseURL", err)
	}
}

// ─── Summaries ───────────────────────────────────────────────────────────

func TestGenerateSummary_RetryAfterErrorRow(t *testing.T) {
	keyring.MockInit()
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	prof.PdfDir = t.TempDir()
	if err := a.db.UpdateProfile(prof); err != nil {
		t.Fatal(err)
	}
	paperID := seedKeyPaper(t, a, prof.ID, "Paper", 1, nil)

	// A "downloaded" PDF that can't be parsed: generation fails in
	// ExtractText, before any network call.
	fname := "broken.pdf"
	os.WriteFile(filepath.Join(prof.PdfDir, fname), []byte("not a pdf"), 0o644)
	if err := a.db.SaveDownload(&db.Download{PaperID: paperID, Source: "test", Status: "ok", Filename: &fname}); err != nil {
		t.Fatal(err)
	}

	if _, err := a.CreateLLMProfile(db.LLMProfile{
		Name: "P", BaseURL: "https://api.good.com/v1", Models: db.JSONStringSlice{"m"}, DefaultModel: "m", IsActive: true,
	}, "sk-secret-key-1234"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		_, err := a.GenerateSummary(paperID, "")
		if err == nil {
			t.Fatalf("attempt %d: expected extraction error", i)
		}
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "create summary") {
			t.Fatalf("attempt %d: retry after an error row hit the unique constraint: %v", i, err)
		}
		status, _ := a.db.HasActiveSummary(paperID, "m")
		if status != "error" {
			t.Errorf("attempt %d: status = %q, want error", i, status)
		}
	}
}

// ─── ResolveAndAddExternal status on existing paper ─────────────────────

func TestApplyRequestedStatus(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	newID := seedKeyPaper(t, a, prof.ID, "Existing new", 1, nil)
	a.db.UpdatePaperStatus(newID, "new")
	if err := a.applyRequestedStatus(newID, "approved"); err != nil {
		t.Fatal(err)
	}
	if p, _ := a.db.GetPaper(newID); p.Status != "approved" {
		t.Errorf("status = %q, want approved", p.Status)
	}

	dlID := seedKeyPaper(t, a, prof.ID, "Existing downloaded", 1, nil)
	a.db.UpdatePaperStatus(dlID, "downloaded")
	if err := a.applyRequestedStatus(dlID, "approved"); err != nil {
		t.Fatal(err)
	}
	if p, _ := a.db.GetPaper(dlID); p.Status != "downloaded" {
		t.Errorf("status = %q, want downloaded kept", p.Status)
	}
}

func TestUpsertExisting_ReturnsLoadableID(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	firstID := seedKeyPaper(t, a, prof.ID, "Same Title", 1, nil)

	// What ResolveAndAddExternal does for a paper already in the library.
	dup := &db.Paper{ProfileID: prof.ID, Title: "Same Title", TitleNormalized: "Same Title", Status: "approved"}
	if err := a.db.UpsertPaper(dup); err != nil {
		t.Fatal(err)
	}
	if dup.ID != firstID {
		t.Fatalf("merged ID = %d, want %d", dup.ID, firstID)
	}
	if _, err := a.db.GetPaper(dup.ID); err != nil {
		t.Errorf("GetPaper(merged id): %v", err)
	}
}

// ─── OpenSystemPDF path validation ───────────────────────────────────────

func TestValidatePDFPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "paper.PDF")
	os.WriteFile(good, []byte("%PDF"), 0o644)
	notPDF := filepath.Join(dir, "script.sh")
	os.WriteFile(notPDF, []byte("x"), 0o644)
	dirPDF := filepath.Join(dir, "folder.pdf")
	os.Mkdir(dirPDF, 0o755)

	if err := validatePDFPath(good); err != nil {
		t.Errorf("valid pdf: %v", err)
	}
	for _, p := range []string{"relative/paper.pdf", notPDF, filepath.Join(dir, "missing.pdf"), dirPDF} {
		if err := validatePDFPath(p); err == nil {
			t.Errorf("validatePDFPath(%q) = nil, want error", p)
		}
	}
}
