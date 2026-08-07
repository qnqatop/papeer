package search

import "context"

// Provider searches a single academic API and returns raw papers.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error)
}

// RawPaper is the unified format returned by all providers before dedup/scoring.
type RawPaper struct {
	Title         string   `json:"title"`
	Abstract      string   `json:"abstract"`
	Year          *int     `json:"year"`
	Venue         string   `json:"venue"`
	Authors       []string `json:"authors"`
	DOI           string   `json:"doi"`
	ArxivID       string   `json:"arxiv_id"`
	PdfURL        string   `json:"pdf_url"`
	CitationCount int      `json:"citation_count"`
	Source        string   `json:"source"` // provider name
}
