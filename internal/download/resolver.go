package download

import (
	"context"

	"github.com/qnqatop/papeer/internal/httpclient"
)

// PaperInfo contains the metadata needed by download sources to resolve a PDF URL.
type PaperInfo struct {
	DOI     string
	ArxivID string
	Title   string
	PdfURL  string // pre-found URL from search phase
	Email   string // for Unpaywall
}

// ResolveResult is the outcome of a source trying to find a PDF URL.
type ResolveResult struct {
	PdfURL string
	Reason string // human-readable explanation if not found
}

// Source resolves a PDF URL from paper metadata.
// Each source tries one strategy (e.g., arXiv by ID, S2 by DOI, Unpaywall).
type Source interface {
	// Name returns a short identifier for this source (e.g., "arxiv", "s2_doi").
	Name() string

	// Resolve tries to find a PDF URL for the given paper.
	// Returns a URL on success, or empty URL with a reason on failure.
	Resolve(ctx context.Context, client *httpclient.Client, info PaperInfo) ResolveResult
}

// DefaultSourceOrder defines the fallback chain order across open sources.
// search_report → arxiv → s2_doi → openalex → unpaywall → crossref → s2_title
var DefaultSourceOrder = []string{
	"search_report", "cyberleninka", "arxiv", "s2_doi", "openalex",
	"unpaywall", "crossref", "s2_title",
}
