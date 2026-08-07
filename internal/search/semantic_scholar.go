package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

type SemanticScholar struct {
	client  *httpclient.Client
	baseURL string // test hook; empty = production endpoint
}

func NewSemanticScholar(c *httpclient.Client) *SemanticScholar {
	return &SemanticScholar{client: c}
}

func (s *SemanticScholar) Name() string { return "semantic_scholar" }

func (s *SemanticScholar) Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error) {
	fields := "title,abstract,authors,year,venue,externalIds,openAccessPdf,citationCount"
	base := s.baseURL
	if base == "" {
		base = "https://api.semanticscholar.org/graph/v1"
	}
	u := fmt.Sprintf(
		"%s/paper/search?query=%s&limit=%d&fields=%s&year=%d-",
		base, url.QueryEscape(query), limit, fields, yearMin,
	)

	var resp s2Response
	if err := s.client.DoJSONWithRetry(ctx, u, &resp, 1); err != nil {
		return nil, err
	}

	out := make([]RawPaper, 0, len(resp.Data))
	for _, p := range resp.Data {
		doi := strings.ToLower(p.ExternalIDs.DOI)
		var pdfURL string
		if p.OpenAccessPdf != nil && p.OpenAccessPdf.Status != "CLOSED" {
			pdfURL = p.OpenAccessPdf.URL
		}

		out = append(out, RawPaper{
			Title:         strings.TrimSpace(p.Title),
			Abstract:      strings.TrimSpace(p.Abstract),
			Year:          p.Year,
			Venue:         strings.TrimSpace(p.Venue),
			Authors:       extractS2Authors(p.Authors),
			DOI:           doi,
			ArxivID:       p.ExternalIDs.ArXiv,
			PdfURL:        pdfURL,
			CitationCount: ptr.Val(p.CitationCount),
			Source:        "semantic_scholar",
		})
	}
	return out, nil
}

func extractS2Authors(authors []s2Author) []string {
	out := make([]string, 0, len(authors))
	for _, a := range authors {
		if name := strings.TrimSpace(a.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

type s2Response struct {
	Data []s2Paper `json:"data"`
}

type s2Paper struct {
	Title         string     `json:"title"`
	Abstract      string     `json:"abstract"`
	Year          *int       `json:"year"`
	Venue         string     `json:"venue"`
	Authors       []s2Author `json:"authors"`
	ExternalIDs   s2ExtIDs   `json:"externalIds"`
	OpenAccessPdf *s2OAPdf   `json:"openAccessPdf"`
	CitationCount *int       `json:"citationCount"`
}

type s2Author struct {
	Name string `json:"name"`
}

type s2ExtIDs struct {
	DOI   string `json:"DOI"`
	ArXiv string `json:"ArXiv"`
}

type s2OAPdf struct {
	URL    string `json:"url"`
	Status string `json:"status"`
}
