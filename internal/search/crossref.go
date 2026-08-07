package search

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qnqatop/papeer/internal/httpclient"
)

type Crossref struct {
	client  *httpclient.Client
	baseURL string // test hook; empty = production endpoint
}

func NewCrossref(c *httpclient.Client) *Crossref {
	return &Crossref{client: c}
}

func (cr *Crossref) Name() string { return "crossref" }

func (cr *Crossref) Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error) {
	base := cr.baseURL
	if base == "" {
		base = "https://api.crossref.org"
	}
	u := fmt.Sprintf(
		"%s/works?query=%s&rows=%d&filter=from-pub-date:%d-01-01,type:journal-article",
		base, url.QueryEscape(query), limit, yearMin,
	)

	var resp crResponse
	if err := cr.client.DoJSONWithRetry(ctx, u, &resp, 2); err != nil {
		return nil, err
	}

	out := make([]RawPaper, 0, len(resp.Message.Items))
	for _, w := range resp.Message.Items {
		title := ""
		if len(w.Title) > 0 {
			title = strings.TrimSpace(w.Title[0])
		}
		venue := ""
		if len(w.ContainerTitle) > 0 {
			venue = w.ContainerTitle[0]
		}

		year := extractCRYear(w)
		pdfURL := extractCRPdfURL(w.Link)

		out = append(out, RawPaper{
			Title:         title,
			Abstract:      strings.TrimSpace(w.Abstract),
			Year:          year,
			Venue:         venue,
			Authors:       extractCRAuthors(w.Author),
			DOI:           strings.ToLower(w.DOI),
			PdfURL:        pdfURL,
			CitationCount: w.IsReferencedByCount,
			Source:        "crossref",
		})
	}
	return out, nil
}

func extractCRYear(w crWork) *int {
	for _, key := range []string{"published-print", "published-online", "issued"} {
		var d *crDate
		switch key {
		case "published-print":
			d = w.PublishedPrint
		case "published-online":
			d = w.PublishedOnline
		case "issued":
			d = w.Issued
		}
		if d != nil && len(d.DateParts) > 0 && len(d.DateParts[0]) > 0 {
			y := d.DateParts[0][0]
			return &y
		}
	}
	return nil
}

func extractCRPdfURL(links []crLink) string {
	for _, l := range links {
		if strings.Contains(l.ContentType, "pdf") && l.URL != "" {
			return l.URL
		}
	}
	return ""
}

func extractCRAuthors(authors []crAuthor) []string {
	out := make([]string, 0, len(authors))
	for _, a := range authors {
		name := strings.TrimSpace(a.Given + " " + a.Family)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

type crResponse struct {
	Message crMessage `json:"message"`
}

type crMessage struct {
	Items []crWork `json:"items"`
}

type crWork struct {
	Title               []string   `json:"title"`
	ContainerTitle      []string   `json:"container-title"`
	Abstract            string     `json:"abstract"`
	DOI                 string     `json:"DOI"`
	Author              []crAuthor `json:"author"`
	Link                []crLink   `json:"link"`
	IsReferencedByCount int        `json:"is-referenced-by-count"`
	PublishedPrint      *crDate    `json:"published-print"`
	PublishedOnline     *crDate    `json:"published-online"`
	Issued              *crDate    `json:"issued"`
}

type crAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

type crLink struct {
	URL         string `json:"URL"`
	ContentType string `json:"content-type"`
}

type crDate struct {
	DateParts [][]int `json:"date-parts"`
}
