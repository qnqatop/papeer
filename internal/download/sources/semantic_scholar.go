package sources

import (
	"context"
	"fmt"
	"net/url"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

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

	// Shares the request (and its outcome) with the arxiv source.
	paper, err := fetchS2PaperByDOI(ctx, client, info, s.baseURL)
	if err != nil {
		return download.ResolveResult{Reason: s2FailReason("S2 lookup failed", err)}
	}
	return s2OAResult(paper.OpenAccessPdf, "S2: no openAccessPdf")
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

	// A title search only finds what the DOI lookup would have: skip it when
	// S2 already answered for this DOI. It still runs when there is no DOI,
	// S2 does not know the DOI (404), or the DOI lookup failed transiently.
	if info.DOI != "" {
		if r, ok := info.Lookups.Peek(s2DOIKey(info.DOI)); ok && r.Err == nil {
			return download.ResolveResult{Reason: "S2: already checked by DOI"}
		}
	}

	q := info.Title
	if len(q) > 200 {
		q = q[:200]
	}

	base := s.baseURL
	if base == "" {
		base = s2DefaultBase
	}
	u := fmt.Sprintf(
		"%s/paper/search?query=%s&limit=1&fields=openAccessPdf,title",
		base, url.QueryEscape(q))

	var resp struct {
		Data []s2Paper `json:"data"`
	}
	if err := gatedGetJSON(ctx, client, info.Gate, u, &resp); err != nil {
		return download.ResolveResult{Reason: s2FailReason("S2 search failed", err)}
	}

	if len(resp.Data) == 0 {
		return download.ResolveResult{Reason: "S2: not found by title"}
	}
	return s2OAResult(resp.Data[0].OpenAccessPdf, "S2: no openAccessPdf for title match")
}

// s2OAResult turns an S2 openAccessPdf entry into a ResolveResult.
func s2OAResult(oa *s2OAPdf, missingReason string) download.ResolveResult {
	if oa == nil {
		return download.ResolveResult{Reason: missingReason}
	}
	if oa.Status == "CLOSED" {
		return download.ResolveResult{Reason: "S2: CLOSED"}
	}
	if oa.URL != "" {
		return download.ResolveResult{PdfURL: oa.URL}
	}
	return download.ResolveResult{Reason: "S2: empty openAccessPdf URL"}
}
