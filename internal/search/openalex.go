package search

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/qnqatop/papeer/internal/httpclient"
)

type OpenAlex struct {
	client  *httpclient.Client
	email   string
	baseURL string // test hook; empty = production endpoint
}

func NewOpenAlex(c *httpclient.Client, email string) *OpenAlex {
	return &OpenAlex{client: c, email: email}
}

func (o *OpenAlex) Name() string { return "openalex" }

func (o *OpenAlex) Language() string { return "en" }

func (o *OpenAlex) Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error) {
	if limit > 50 {
		limit = 50
	}
	base := o.baseURL
	if base == "" {
		base = "https://api.openalex.org"
	}
	u := fmt.Sprintf(
		"%s/works?search=%s&per-page=%d&filter=from_publication_date:%d-01-01",
		base, url.QueryEscape(query), limit, yearMin,
	)
	if o.email != "" && !strings.Contains(o.email, "example") {
		u += "&mailto=" + url.QueryEscape(o.email)
	}

	var resp oaResponse
	if err := o.client.DoJSONWithRetry(ctx, u, &resp, 2); err != nil {
		return nil, err
	}

	out := make([]RawPaper, 0, len(resp.Results))
	for _, w := range resp.Results {
		pdfURL := extractOAPdfURL(w)
		abstract := reconstructAbstract(w.AbstractInvertedIndex)

		doi := w.DOI
		if strings.HasPrefix(doi, "https://doi.org/") {
			doi = strings.ToLower(doi[len("https://doi.org/"):])
		}

		var venue string
		if loc := w.PrimaryLocation; loc != nil && loc.Source != nil {
			venue = loc.Source.DisplayName
		}

		out = append(out, RawPaper{
			Title:         strings.TrimSpace(w.Title),
			Abstract:      strings.TrimSpace(abstract),
			Year:          w.PublicationYear,
			Venue:         venue,
			Authors:       extractOAAuthors(w.Authorships),
			DOI:           doi,
			PdfURL:        pdfURL,
			CitationCount: w.CitedByCount,
			Source:        "openalex",
		})
	}
	return out, nil
}

// reconstructAbstract rebuilds text from OpenAlex inverted index.
func reconstructAbstract(inv map[string][]int) string {
	if len(inv) == 0 {
		return ""
	}
	words := make(map[int]string)
	maxPos := 0
	for word, positions := range inv {
		for _, p := range positions {
			words[p] = word
			if p > maxPos {
				maxPos = p
			}
		}
	}
	sorted := make([]int, 0, len(words))
	for p := range words {
		sorted = append(sorted, p)
	}
	sort.Ints(sorted)

	parts := make([]string, 0, len(sorted))
	for _, p := range sorted {
		parts = append(parts, words[p])
	}
	return strings.Join(parts, " ")
}

func extractOAPdfURL(w oaWork) string {
	for _, loc := range []*oaLocation{w.BestOALocation, w.PrimaryLocation} {
		if loc == nil {
			continue
		}
		if loc.PdfURL != "" {
			return loc.PdfURL
		}
		if loc.IsOA && loc.LandingPageURL != "" {
			return loc.LandingPageURL
		}
	}
	return ""
}

func extractOAAuthors(authorships []oaAuthorship) []string {
	out := make([]string, 0, len(authorships))
	for _, a := range authorships {
		if name := strings.TrimSpace(a.Author.DisplayName); name != "" {
			out = append(out, name)
		}
	}
	return out
}

type oaResponse struct {
	Results []oaWork `json:"results"`
}

type oaWork struct {
	Title                 string           `json:"title"`
	DOI                   string           `json:"doi"`
	PublicationYear       *int             `json:"publication_year"`
	CitedByCount          int              `json:"cited_by_count"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	Authorships           []oaAuthorship   `json:"authorships"`
	PrimaryLocation       *oaLocation      `json:"primary_location"`
	BestOALocation        *oaLocation      `json:"best_oa_location"`
}

type oaAuthorship struct {
	Author oaAuthor `json:"author"`
}

type oaAuthor struct {
	DisplayName string `json:"display_name"`
}

type oaLocation struct {
	PdfURL         string    `json:"pdf_url"`
	LandingPageURL string    `json:"landing_page_url"`
	IsOA           bool      `json:"is_oa"`
	Source         *oaSource `json:"source"`
}

type oaSource struct {
	DisplayName string `json:"display_name"`
}
