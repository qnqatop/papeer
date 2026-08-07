package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// Crossref resolves PDF via Crossref direct links by DOI.
type Crossref struct {
	baseURL string // test hook; empty = production endpoint
}

func NewCrossref() *Crossref { return &Crossref{} }

func (c *Crossref) Name() string { return "crossref" }

func (c *Crossref) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no DOI"}
	}

	base := c.baseURL
	if base == "" {
		base = "https://api.crossref.org"
	}
	u := fmt.Sprintf("%s/works/%s", base, url.PathEscape(info.DOI))

	var resp struct {
		Message struct {
			Link []struct {
				URL         string `json:"URL"`
				ContentType string `json:"content-type"`
			} `json:"link"`
		} `json:"message"`
	}
	if err := client.DoJSON(ctx, u, &resp); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("Crossref lookup failed: %v", err)}
	}

	for _, link := range resp.Message.Link {
		if strings.Contains(link.ContentType, "pdf") && link.URL != "" {
			return download.ResolveResult{PdfURL: link.URL}
		}
	}

	return download.ResolveResult{Reason: "Crossref: no PDF link"}
}
