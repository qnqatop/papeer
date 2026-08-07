package search

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	"github.com/qnqatop/papeer/internal/httpclient"
)

type ArXiv struct {
	client  *httpclient.Client
	baseURL string // test hook; empty = production endpoint
}

func NewArXiv(c *httpclient.Client) *ArXiv {
	return &ArXiv{client: c}
}

func (a *ArXiv) Name() string { return "arxiv" }

func (a *ArXiv) Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error) {
	base := a.baseURL
	if base == "" {
		base = "http://export.arxiv.org/api/query"
	}
	u := fmt.Sprintf(
		"%s?search_query=%s&start=0&max_results=%d&sortBy=relevance",
		base, url.QueryEscape("all:"+query), limit,
	)

	text, err := a.client.DoText(ctx, u)
	if err != nil {
		return nil, err
	}

	var feed atomFeed
	if err := xml.Unmarshal([]byte(text), &feed); err != nil {
		return nil, fmt.Errorf("parse arXiv XML: %w", err)
	}

	out := make([]RawPaper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		year := extractArxivYear(e.Published)
		if year != nil && *year < yearMin {
			continue
		}

		arxivID := extractArxivID(e.ID)
		pdfURL := extractArxivPdfURL(e.Links, arxivID)
		doi := strings.ToLower(strings.TrimSpace(e.DOI))

		title := strings.TrimSpace(e.Title)
		title = strings.ReplaceAll(title, "\n", " ")
		title = collapseSpaces(title)

		abstract := strings.TrimSpace(e.Summary)
		abstract = strings.ReplaceAll(abstract, "\n", " ")
		abstract = collapseSpaces(abstract)

		out = append(out, RawPaper{
			Title:    title,
			Abstract: abstract,
			Year:     year,
			Venue:    "arXiv preprint",
			Authors:  extractArxivAuthors(e.Authors),
			DOI:      doi,
			ArxivID:  arxivID,
			PdfURL:   pdfURL,
			Source:   "arxiv",
		})
	}
	return out, nil
}

func extractArxivYear(published string) *int {
	if len(published) >= 4 {
		var y int
		if _, err := fmt.Sscanf(published[:4], "%d", &y); err == nil {
			return &y
		}
	}
	return nil
}

func extractArxivID(entryID string) string {
	// Format: http://arxiv.org/abs/2301.12345v1
	const prefix = "http://arxiv.org/abs/"
	if strings.HasPrefix(entryID, prefix) {
		id := entryID[len(prefix):]
		// Strip version suffix (v1, v2, etc.)
		if idx := strings.LastIndex(id, "v"); idx > 0 {
			id = id[:idx]
		}
		return id
	}
	return ""
}

func extractArxivPdfURL(links []atomLink, arxivID string) string {
	for _, l := range links {
		if l.Title == "pdf" && l.Href != "" {
			return l.Href
		}
	}
	if arxivID != "" {
		return "https://arxiv.org/pdf/" + arxivID + ".pdf"
	}
	return ""
}

func extractArxivAuthors(authors []atomAuthor) []string {
	out := make([]string, 0, len(authors))
	for _, a := range authors {
		if name := strings.TrimSpace(a.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func collapseSpaces(s string) string {
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

// Atom XML structures for arXiv API response.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	ID        string       `xml:"id"`
	Published string       `xml:"published"`
	Authors   []atomAuthor `xml:"author"`
	Links     []atomLink   `xml:"link"`
	DOI       string       `xml:"doi"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href  string `xml:"href,attr"`
	Title string `xml:"title,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
}
