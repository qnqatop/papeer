package sources

import (
	"context"
	"fmt"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// ArXiv resolves a PDF by arXiv ID: from paper metadata, an arXiv DOI, the
// paper's OpenAlex locations, or (last) Semantic Scholar externalIds.
type ArXiv struct {
	// Test hooks; empty = production endpoints.
	s2BaseURL       string
	openAlexBaseURL string
}

func NewArXiv() *ArXiv { return &ArXiv{} }

func (a *ArXiv) Name() string { return "arxiv" }

func (a *ArXiv) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	// If we already have an arXiv ID, use it directly.
	if info.ArxivID != "" {
		return arxivResult(info.ArxivID)
	}
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no arXiv ID or DOI"}
	}
	// arXiv-minted DOIs embed the ID.
	if id := arxivIDFromURL(info.DOI); id != "" {
		return arxivResult(id)
	}

	// OpenAlex first: it is not rate-limited like S2, and the work record is
	// memoized for the openalex source later in the chain.
	oaNote := "no arXiv location in OpenAlex"
	work, err := fetchOpenAlexWork(ctx, client, info, a.openAlexBaseURL)
	if err != nil {
		oaNote = fmt.Sprintf("OpenAlex lookup failed: %v", err)
	} else if id := arxivIDFromWork(work); id != "" {
		return arxivResult(id)
	}

	// Fall back to S2 externalIds (memoized for s2_doi). Reasons are joined
	// with "," — the engine and the frontend use ";" between sources.
	paper, err := fetchS2PaperByDOI(ctx, client, info, a.s2BaseURL)
	if err != nil {
		return download.ResolveResult{Reason: oaNote + ", " + s2FailReason("S2 lookup failed", err)}
	}
	arxivID := coerceString(paper.ExternalIDs["ArXiv"])
	if arxivID == "" {
		return download.ResolveResult{Reason: oaNote + ", no arXiv ID in S2 externalIds"}
	}
	return arxivResult(arxivID)
}

func arxivResult(id string) download.ResolveResult {
	return download.ResolveResult{PdfURL: fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", id)}
}

// coerceString turns whatever S2 returned (string, number, nil) into a string.
// Numeric IDs are formatted without trailing ".0" to keep arxiv URLs clean.
func coerceString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		// JSON numbers always decode to float64; trim trailing zero if integer.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
