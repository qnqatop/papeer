package sources

import (
	"context"
	"fmt"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// OpenAlex resolves PDF via OpenAlex best_oa_location by DOI.
type OpenAlex struct {
	// baseURL overrides the OpenAlex API host. Used by tests; empty means the
	// real public endpoint.
	baseURL string
}

func NewOpenAlex() *OpenAlex { return &OpenAlex{} }

func (o *OpenAlex) Name() string { return "openalex" }

func (o *OpenAlex) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no DOI"}
	}

	// Usually already fetched (and memoized) by the arxiv source.
	work, err := fetchOpenAlexWork(ctx, client, info, o.baseURL)
	if err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("OpenAlex lookup failed: %v", err)}
	}

	for _, loc := range []*oaLoc{work.BestOALocation, work.PrimaryLocation} {
		if loc == nil {
			continue
		}
		if loc.PdfURL != "" {
			return download.ResolveResult{PdfURL: loc.PdfURL}
		}
		if loc.IsOA && loc.LandingPageURL != "" {
			return download.ResolveResult{PdfURL: loc.LandingPageURL}
		}
	}

	return download.ResolveResult{Reason: "OpenAlex: no OA location"}
}
