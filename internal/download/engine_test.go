package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

// mockSource resolves to a fixed URL or returns an error reason.
type mockSource struct {
	name   string
	pdfURL string
	reason string
}

func (m *mockSource) Name() string { return m.name }
func (m *mockSource) Resolve(_ context.Context, _ *httpclient.Client, _ PaperInfo) ResolveResult {
	if m.pdfURL != "" {
		return ResolveResult{PdfURL: m.pdfURL}
	}
	return ResolveResult{Reason: m.reason}
}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func createTestPaper(t *testing.T, d *db.DB) db.Paper {
	t.Helper()
	p := &db.Profile{Name: "test", Email: "test@test.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	year := 2023
	paper := &db.Paper{
		ProfileID:       p.ID,
		Title:           "Test Paper About Cold Start",
		TitleNormalized: "test paper about cold start",
		Year:            &year,
		DOI:             ptr.Ptr("10.1234/test"),
		Authors:         db.JSONStringSlice{"John Smith"},
		Status:          "approved",
	}
	if err := d.UpsertPaper(paper); err != nil {
		t.Fatal(err)
	}
	return *paper
}

// validPDFBytes creates a minimal valid PDF for testing.
func validPDFBytes() []byte {
	header := []byte("%PDF-1.4 test content\n")
	padding := make([]byte, 25000-len(header)-6) // ensure >20KB
	trailer := []byte("\n%%EOF\n")
	result := make([]byte, 0, len(header)+len(padding)+len(trailer))
	result = append(result, header...)
	result = append(result, padding...)
	result = append(result, trailer...)
	return result
}

func TestEngine_FallbackChain(t *testing.T) {
	// Set up a server that serves valid PDF.
	pdfData := validPDFBytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()

	d := testDB(t)
	paper := createTestPaper(t, d)
	pdfDir := filepath.Join(t.TempDir(), "pdfs")
	client := httpclient.New("test@test.com")

	sources := []Source{
		&mockSource{name: "failing_source", reason: "not found"},
		&mockSource{name: "working_source", pdfURL: srv.URL + "/paper.pdf"},
	}

	var events []DownloadEvent
	var mu sync.Mutex
	onEvent := func(e DownloadEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	engine := NewEngine(client, d, sources, pdfDir, 1, onEvent)
	engine.DownloadAll(context.Background(), []db.Paper{paper}, "test@test.com")

	// Verify PDF was downloaded.
	files, _ := filepath.Glob(filepath.Join(pdfDir, "*.pdf"))
	if len(files) != 1 {
		t.Fatalf("expected 1 PDF file, got %d", len(files))
	}

	// Verify download records in DB.
	downloads, err := d.ListDownloads(paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Should have 2 attempts: 1 fail + 1 success.
	if len(downloads) != 2 {
		t.Errorf("download attempts = %d, want 2", len(downloads))
	}

	// Verify events.
	mu.Lock()
	defer mu.Unlock()

	hasStart := false
	hasDone := false
	hasComplete := false
	for _, e := range events {
		if e.Type == "start" {
			hasStart = true
		}
		if e.Type == "done" && e.Source == "working_source" {
			hasDone = true
		}
		if e.Type == "complete" {
			hasComplete = true
		}
	}
	if !hasStart {
		t.Error("missing start event")
	}
	if !hasDone {
		t.Error("missing done event from working_source")
	}
	if !hasComplete {
		t.Error("missing complete event")
	}
}

func TestEngine_AllSourcesFail(t *testing.T) {
	d := testDB(t)
	paper := createTestPaper(t, d)
	pdfDir := filepath.Join(t.TempDir(), "pdfs")
	client := httpclient.New("test@test.com")

	sources := []Source{
		&mockSource{name: "src1", reason: "not found"},
		&mockSource{name: "src2", reason: "no DOI"},
	}

	var failEvent *DownloadEvent
	var mu sync.Mutex
	engine := NewEngine(client, d, sources, pdfDir, 1, func(e DownloadEvent) {
		mu.Lock()
		if e.Type == "fail" && strings.HasPrefix(e.Error, "all sources exhausted") {
			failEvent = &e
		}
		mu.Unlock()
	})

	engine.DownloadAll(context.Background(), []db.Paper{paper}, "test@test.com")

	mu.Lock()
	defer mu.Unlock()
	if failEvent == nil {
		t.Error("expected 'all sources exhausted' fail event")
	}

	// No PDF should be downloaded.
	files, _ := filepath.Glob(filepath.Join(pdfDir, "*.pdf"))
	if len(files) != 0 {
		t.Errorf("expected 0 PDF files, got %d", len(files))
	}

	// Fail event must carry per-source reasons so the tester can act on them.
	if !strings.Contains(failEvent.Error, "src1: not found") {
		t.Errorf("missing src1 reason in error: %q", failEvent.Error)
	}
	if !strings.Contains(failEvent.Error, "src2: no DOI") {
		t.Errorf("missing src2 reason in error: %q", failEvent.Error)
	}
}

func TestEngine_SkipCached(t *testing.T) {
	d := testDB(t)
	paper := createTestPaper(t, d)
	pdfDir := filepath.Join(t.TempDir(), "pdfs")
	os.MkdirAll(pdfDir, 0o755)
	client := httpclient.New("test@test.com")

	// Pre-create a valid PDF file.
	year := 2023
	filename := GenerateFilename("John Smith", year, paper.Title)
	dest := filepath.Join(pdfDir, filename)
	os.WriteFile(dest, validPDFBytes(), 0o644)

	var doneSource string
	var mu sync.Mutex
	engine := NewEngine(client, d, nil, pdfDir, 1, func(e DownloadEvent) {
		mu.Lock()
		if e.Type == "done" {
			doneSource = e.Source
		}
		mu.Unlock()
	})

	engine.DownloadAll(context.Background(), []db.Paper{paper}, "")

	mu.Lock()
	defer mu.Unlock()
	if doneSource != "cached" {
		t.Errorf("source = %q, want 'cached'", doneSource)
	}

	// The cached hit must leave an "ok" download row with the filename so
	// the PDF viewer (GetPaperPDFPath) can locate the file.
	dls, err := d.ListDownloads(paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, dl := range dls {
		if dl.Status == "ok" && dl.Filename != nil && *dl.Filename == filename {
			found = true
		}
	}
	if !found {
		t.Errorf("no ok download row with filename %q: %+v", filename, dls)
	}
}

func TestIsTransientFailure_TypedOnly(t *testing.T) {
	// Text that merely contains "503"/"eof" must not be treated as transient.
	notTransient := []error{
		fmt.Errorf("PDF validation failed for https://doi.org/10.1503/cmaj.1: not a PDF"),
		fmt.Errorf("PDF validation failed for https://x.org/geoffrey.pdf: PDF truncated (no %%%%EOF marker)"),
		fmt.Errorf("downloading x: %w", &httpclient.StatusError{Code: 404, Host: "a"}),
	}
	for _, err := range notTransient {
		if isTransientFailure(err) {
			t.Errorf("isTransientFailure(%v) = true, want false", err)
		}
	}
	if !isTransientFailure(fmt.Errorf("downloading x: %w", &httpclient.StatusError{Code: 503, Host: "a"})) {
		t.Error("wrapped 503 StatusError should be transient")
	}
	if isTransientFailure(context.Canceled) {
		t.Error("context.Canceled must not be transient")
	}
}

func TestEngine_ContextCancel(t *testing.T) {
	d := testDB(t)
	paper := createTestPaper(t, d)
	pdfDir := filepath.Join(t.TempDir(), "pdfs")
	client := httpclient.New("test@test.com")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	engine := NewEngine(client, d, []Source{
		&mockSource{name: "src", pdfURL: "https://example.com/paper.pdf"},
	}, pdfDir, 1, nil)

	// Should not panic or hang.
	engine.DownloadAll(ctx, []db.Paper{paper}, "")

	// No download should have happened.
	downloads, _ := d.ListDownloads(paper.ID)
	for _, dl := range downloads {
		if dl.Status == "ok" {
			t.Error("unexpected successful download after cancel")
		}
	}
	_ = fmt.Sprintf("downloads: %d", len(downloads))
}

// slowMockSource adds a delay before resolving.
type slowMockSource struct {
	name   string
	pdfURL string
}

func (m *slowMockSource) Name() string { return m.name }
func (m *slowMockSource) Resolve(ctx context.Context, _ *httpclient.Client, _ PaperInfo) ResolveResult {
	// Simulate slow resolution.
	select {
	case <-ctx.Done():
		return ResolveResult{Reason: "cancelled"}
	default:
	}
	return ResolveResult{PdfURL: m.pdfURL}
}

func TestEngine_CancelStopsAfterFewFiles(t *testing.T) {
	pdfData := validPDFBytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfData)
	}))
	defer srv.Close()

	d := testDB(t)
	p := &db.Profile{Name: "test", Email: "t@t.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	// Create 20 papers.
	var papers []db.Paper
	for i := 0; i < 20; i++ {
		year := 2023
		paper := &db.Paper{
			ProfileID:       p.ID,
			Title:           fmt.Sprintf("Paper %d", i),
			TitleNormalized: fmt.Sprintf("paper %d", i),
			Year:            &year,
			DOI:             ptr.Ptr(fmt.Sprintf("10.1234/test-%d", i)),
			Authors:         db.JSONStringSlice{"Author"},
			Status:          "approved",
		}
		if err := d.UpsertPaper(paper); err != nil {
			t.Fatal(err)
		}
		papers = append(papers, *paper)
	}

	pdfDir := filepath.Join(t.TempDir(), "pdfs")
	client := httpclient.New("t@t.com")

	ctx, cancel := context.WithCancel(context.Background())

	var events []DownloadEvent
	var mu sync.Mutex
	doneCount := 0

	engine := NewEngine(client, d, []Source{
		&slowMockSource{name: "src", pdfURL: srv.URL + "/paper.pdf"},
	}, pdfDir, 1, func(e DownloadEvent) {
		mu.Lock()
		events = append(events, e)
		if e.Type == "done" {
			doneCount++
			if doneCount >= 2 {
				cancel() // Cancel after 2 downloads.
			}
		}
		mu.Unlock()
	})

	engine.DownloadAll(ctx, papers, "t@t.com")

	mu.Lock()
	defer mu.Unlock()

	// Should have downloaded at most a few papers, not all 20.
	if doneCount >= 10 {
		t.Errorf("expected cancel to stop early, but downloaded %d/20 papers", doneCount)
	}
}

func TestApproveByScore_OnlyNewPapers(t *testing.T) {
	d := testDB(t)
	p := &db.Profile{Name: "test", Email: "t@t.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	// Create papers with different statuses and scores.
	makePaper := func(title, status string, score int) {
		year := 2023
		paper := &db.Paper{
			ProfileID:       p.ID,
			Title:           title,
			TitleNormalized: title,
			Year:            &year,
			PreScore:        score,
			Status:          status,
			Authors:         db.JSONStringSlice{"A"},
			Sources:         db.JSONStringSlice{"test"},
			ScoreReasons:    db.JSONStringSlice{},
		}
		if err := d.UpsertPaper(paper); err != nil {
			t.Fatal(err)
		}
	}

	makePaper("new-high", "new", 9)           // should be approved
	makePaper("new-low", "new", 3)            // should NOT be approved
	makePaper("rejected-high", "rejected", 9) // should NOT change
	makePaper("approved-low", "approved", 2)  // should NOT change

	count, err := d.ApproveByScore(p.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("ApproveByScore: approved %d, want 1", count)
	}

	// Verify statuses.
	check := func(title, wantStatus string) {
		papers, _, _ := d.ListPapers(db.PaperFilter{ProfileID: p.ID, Search: title, Limit: 10})
		for _, pp := range papers {
			if pp.Title == title && pp.Status != wantStatus {
				t.Errorf("%s: status=%q, want %q", title, pp.Status, wantStatus)
			}
		}
	}

	check("new-high", "approved")
	check("new-low", "new")
	check("rejected-high", "rejected")
	check("approved-low", "approved")
}

func TestDownloadApproved_OnlyApprovedPapers(t *testing.T) {
	d := testDB(t)
	p := &db.Profile{Name: "test", Email: "t@t.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}

	year := 2023
	makeP := func(title, status string) {
		paper := &db.Paper{
			ProfileID:       p.ID,
			Title:           title,
			TitleNormalized: title,
			Year:            &year,
			Status:          status,
			Authors:         db.JSONStringSlice{"A"},
			Sources:         db.JSONStringSlice{"test"},
			ScoreReasons:    db.JSONStringSlice{},
		}
		if err := d.UpsertPaper(paper); err != nil {
			t.Fatal(err)
		}
	}

	makeP("paper-new", "new")
	makeP("paper-approved", "approved")
	makeP("paper-rejected", "rejected")
	makeP("paper-downloaded", "downloaded")

	papers, err := d.GetApprovedPapers(p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(papers) != 1 {
		t.Fatalf("GetApprovedPapers: got %d, want 1", len(papers))
	}
	if papers[0].Title != "paper-approved" {
		t.Errorf("expected 'paper-approved', got %q", papers[0].Title)
	}
}
