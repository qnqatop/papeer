package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

// DownloadEvent is emitted during download to report progress.
type DownloadEvent struct {
	Type     string `json:"type"` // "start", "resolving", "resolved", "downloading", "done", "fail", "complete"
	PaperID  int64  `json:"paper_id"`
	Title    string `json:"title"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Error    string `json:"error"`
	Current  int    `json:"current"`
	Total    int    `json:"total"`
	Filename string `json:"filename"`
}

// DownloadEventFunc receives download progress events.
type DownloadEventFunc func(DownloadEvent)

// Engine orchestrates parallel PDF downloads with fallback chain.
type Engine struct {
	client   *httpclient.Client
	database *db.DB
	sources  []Source
	pdfDir   string
	workers  int
	onEvent  DownloadEventFunc
}

// NewEngine creates a download engine.
func NewEngine(client *httpclient.Client, database *db.DB, sources []Source, pdfDir string, workers int, onEvent DownloadEventFunc) *Engine {
	if workers < 1 {
		workers = 4
	}
	if onEvent == nil {
		onEvent = func(DownloadEvent) {}
	}
	return &Engine{
		client:   client,
		database: database,
		sources:  sources,
		pdfDir:   pdfDir,
		workers:  workers,
		onEvent:  onEvent,
	}
}

// DownloadAll downloads PDFs for the given papers using worker pool.
func (e *Engine) DownloadAll(ctx context.Context, papers []db.Paper, email string) {
	if err := os.MkdirAll(e.pdfDir, 0o755); err != nil {
		e.onEvent(DownloadEvent{Type: "fail", Error: fmt.Sprintf("create pdf dir: %v", err)})
		return
	}

	total := len(papers)
	jobs := make(chan int, total)
	var wg sync.WaitGroup

	counter := &atomicCounter{}

	for i := 0; i < e.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
			if ctx.Err() != nil {
				// Drain remaining jobs so the sender doesn't block.
				for range jobs {
				}
				return
			}
			cur := counter.inc()
			e.DownloadOne(ctx, papers[idx], email, cur, total)
		}
		}()
	}

sendLoop:
	for i := range papers {
		select {
		case jobs <- i:
		case <-ctx.Done():
			// Context cancelled — stop sending new jobs.
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()

	e.onEvent(DownloadEvent{Type: "complete", Total: total})
}

// DownloadOne tries all sources in fallback order for a single paper.
func (e *Engine) DownloadOne(ctx context.Context, paper db.Paper, email string, current, total int) {
	info := PaperInfo{
		DOI:     ptr.Val(paper.DOI),
		ArxivID: ptr.Val(paper.ArxivID),
		Title:   paper.Title,
		PdfURL:  ptr.Val(paper.PdfURL),
		Email:   email,
	}

	// Reasons accumulated per source for the final fail event.
	var attempts []string
	appendAttempt := func(name, reason string) {
		if reason == "" {
			reason = "unknown"
		}
		attempts = append(attempts, fmt.Sprintf("%s: %s", name, reason))
	}

	e.onEvent(DownloadEvent{
		Type:    "start",
		PaperID: paper.ID,
		Title:   paper.Title,
		Current: current,
		Total:   total,
	})

	// Determine filename.
	year := 0
	if paper.Year != nil {
		year = *paper.Year
	}
	firstAuthor := ""
	if len(paper.Authors) > 0 {
		firstAuthor = paper.Authors[0]
	}
	filename := GenerateFilename(firstAuthor, year, paper.Title)
	dest := filepath.Join(e.pdfDir, filename)

	// Skip if already downloaded and valid.
	if _, err := os.Stat(dest); err == nil {
		if ValidatePDFFile(dest) == nil {
			e.onEvent(DownloadEvent{
				Type:     "done",
				PaperID:  paper.ID,
				Title:    paper.Title,
				Source:   "cached",
				Filename: filename,
				Current:  current,
				Total:    total,
			})
			_ = e.database.UpdatePaperStatus(paper.ID, "downloaded")
			return
		}
	}

	// Track URLs we have already fetched for this paper so two different
	// sources don't waste time hitting the same MDPI/Springer landing page
	// twice in a row.
	triedURLs := make(map[string]string) // url → reason from the first try

	// Try each source in fallback order.
	for _, src := range e.sources {
		if ctx.Err() != nil {
			return
		}

		e.onEvent(DownloadEvent{
			Type:    "resolving",
			PaperID: paper.ID,
			Title:   paper.Title,
			Source:  src.Name(),
			Current: current,
			Total:   total,
		})

		result := src.Resolve(ctx, e.client, info)

		// Log the attempt.
		dl := &db.Download{
			PaperID: paper.ID,
			Source:  src.Name(),
			URL:     ptr.Ptr(result.PdfURL),
		}

		if result.PdfURL == "" {
			dl.Status = "fail"
			dl.Reason = result.Reason
			_ = e.database.SaveDownload(dl)
			appendAttempt(src.Name(), result.Reason)
			continue
		}

		// Skip URLs already attempted (and failed) by an earlier source —
		// unless the first failure looks transient (timeout, network reset,
		// 5xx). Repositories like mdsoar.org sometimes flake on the first
		// hit and serve on the second; pre-Fix #1 the chain would die at
		// "duplicate URL" and never recover. After Fix #1 the first attempt
		// already exhausts Chrome, so a terminal failure stays terminal.
		if prevReason, seen := triedURLs[result.PdfURL]; seen {
			if !isTransientFailure(prevReason) {
				dl.Status = "fail"
				dl.Reason = "duplicate URL: " + prevReason
				_ = e.database.SaveDownload(dl)
				appendAttempt(src.Name(), "same URL as earlier source ("+prevReason+")")
				continue
			}
			// Fall through and retry the URL — fetcher will hit it fresh.
		}

		e.onEvent(DownloadEvent{
			Type:    "downloading",
			PaperID: paper.ID,
			Title:   paper.Title,
			Source:  src.Name(),
			URL:     result.PdfURL,
			Current: current,
			Total:   total,
		})

		// Try to fetch the PDF.
		err := FetchPDF(ctx, e.client, result.PdfURL, dest)
		if err != nil {
			triedURLs[result.PdfURL] = err.Error()
			dl.Status = "fail"
			dl.Reason = err.Error()
			_ = e.database.SaveDownload(dl)
			appendAttempt(src.Name(), err.Error())
			continue
		}

		// Success!
		fi, _ := os.Stat(dest)
		var fileSize int64
		if fi != nil {
			fileSize = fi.Size()
		}

		dl.Status = "ok"
		dl.Filename = ptr.Ptr(filename)
		dl.FileSize = ptr.Ptr(fileSize)
		_ = e.database.SaveDownload(dl)
		_ = e.database.UpdatePaperStatus(paper.ID, "downloaded")

		e.onEvent(DownloadEvent{
			Type:     "done",
			PaperID:  paper.ID,
			Title:    paper.Title,
			Source:   src.Name(),
			URL:      result.PdfURL,
			Filename: filename,
			Current:  current,
			Total:    total,
		})
		return
	}

	// All sources exhausted.
	errMsg := "all sources exhausted"
	if len(attempts) > 0 {
		errMsg = fmt.Sprintf("all sources exhausted: %s", strings.Join(attempts, "; "))
	}
	e.onEvent(DownloadEvent{
		Type:    "fail",
		PaperID: paper.ID,
		Title:   paper.Title,
		Error:   errMsg,
		Current: current,
		Total:   total,
	})
}

// isTransientFailure decides whether a previous download error is worth
// retrying when a later source returns the same URL. Transient = timeouts,
// connection resets, 5xx — things that often clear on a fresh request. UA
// blocks, 4xx responses, and PDF validation failures stay terminal because
// Chrome (already tried in Fix #1) would have caught any recoverable case.
func isTransientFailure(reason string) bool {
	r := strings.ToLower(reason)
	transientMarkers := []string{
		"timeout", "timed out", "deadline exceeded",
		"connection reset", "connection refused", "broken pipe", "eof",
		"no such host", "i/o timeout", "tls handshake",
		"http 500", "http 502", "http 503", "http 504", "http 408", "http 429",
	}
	for _, m := range transientMarkers {
		if strings.Contains(r, m) {
			return true
		}
	}
	return false
}

// atomicCounter is a simple mutex-based counter for tracking progress.
type atomicCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCounter) inc() int {
	c.mu.Lock()
	c.n++
	n := c.n
	c.mu.Unlock()
	return n
}
