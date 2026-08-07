package sources

import (
	"context"
	"fmt"
	"net/url"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

type s2OAResponse struct {
	OpenAccessPdf *struct {
		URL    string `json:"url"`
		Status string `json:"status"`
	} `json:"openAccessPdf"`
}

// S2ByDOI resolves PDF via Semantic Scholar openAccessPdf using DOI.
type S2ByDOI struct {
	baseURL string // test hook; empty = production endpoint
}

func NewS2ByDOI() *S2ByDOI { return &S2ByDOI{} }

func (s *S2ByDOI) Name() string { return "s2_doi" }

func (s *S2ByDOI) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no DOI"}
	}

	base := s.baseURL
	if base == "" {
		base = "https://api.semanticscholar.org/graph/v1"
	}
	u := fmt.Sprintf("%s/paper/DOI:%s?fields=openAccessPdf",
		base, url.PathEscape(info.DOI))

	var resp s2OAResponse
	if err := client.DoJSONWithRetry(ctx, u, &resp, 3); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("S2 lookup failed: %v", err)}
	}

	if resp.OpenAccessPdf == nil {
		return download.ResolveResult{Reason: "S2: no openAccessPdf"}
	}
	if resp.OpenAccessPdf.Status == "CLOSED" {
		return download.ResolveResult{Reason: "S2: CLOSED"}
	}
	if resp.OpenAccessPdf.URL != "" {
		return download.ResolveResult{PdfURL: resp.OpenAccessPdf.URL}
	}
	return download.ResolveResult{Reason: "S2: empty openAccessPdf URL"}
}

// S2ByTitle resolves PDF via Semantic Scholar search by title.
type S2ByTitle struct {
	baseURL string // test hook; empty = production endpoint
}

func NewS2ByTitle() *S2ByTitle { return &S2ByTitle{} }

func (s *S2ByTitle) Name() string { return "s2_title" }

func (s *S2ByTitle) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.Title == "" {
		return download.ResolveResult{Reason: "no title"}
	}

	q := info.Title
	if len(q) > 200 {
		q = q[:200]
	}

	base := s.baseURL
	if base == "" {
		base = "https://api.semanticscholar.org/graph/v1"
	}
	u := fmt.Sprintf(
		"%s/paper/search?query=%s&limit=1&fields=openAccessPdf,title",
		base, url.QueryEscape(q))

	var resp struct {
		Data []s2OAResponse `json:"data"`
	}
	if err := client.DoJSONWithRetry(ctx, u, &resp, 3); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("S2 search failed: %v", err)}
	}

	if len(resp.Data) == 0 {
		return download.ResolveResult{Reason: "S2: not found by title"}
	}

	oa := resp.Data[0].OpenAccessPdf
	if oa == nil {
		return download.ResolveResult{Reason: "S2: no openAccessPdf for title match"}
	}
	if oa.Status == "CLOSED" {
		return download.ResolveResult{Reason: "S2: CLOSED"}
	}
	if oa.URL != "" {
		return download.ResolveResult{PdfURL: oa.URL}
	}
	return download.ResolveResult{Reason: "S2: empty openAccessPdf URL"}
}
