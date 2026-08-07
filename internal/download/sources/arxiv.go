package sources

import (
	"context"
	"fmt"
	"net/url"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// ArXiv resolves PDF by arXiv ID (either from paper metadata or by looking up DOI via S2).
type ArXiv struct{}

func NewArXiv() *ArXiv { return &ArXiv{} }

func (a *ArXiv) Name() string { return "arxiv" }

func (a *ArXiv) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	// If we already have an arXiv ID, use it directly.
	if info.ArxivID != "" {
		return download.ResolveResult{
			PdfURL: fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", info.ArxivID),
		}
	}

	// Try to get arXiv ID from Semantic Scholar by DOI.
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no arXiv ID or DOI"}
	}

	u := fmt.Sprintf("https://api.semanticscholar.org/graph/v1/paper/DOI:%s?fields=externalIds",
		url.PathEscape(info.DOI))

	// S2 returns externalIds with mixed value types — most keys are strings
	// (ArXiv, DOI, MAG) but some (PubMed, CorpusId) come back as numbers.
	// Decode into a generic map and coerce the value we need.
	var resp struct {
		ExternalIDs map[string]any `json:"externalIds"`
	}
	if err := client.DoJSONWithRetry(ctx, u, &resp, 3); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("S2 lookup failed: %v", err)}
	}

	arxivID := coerceString(resp.ExternalIDs["ArXiv"])
	if arxivID == "" {
		return download.ResolveResult{Reason: "no arXiv ID in S2 externalIds"}
	}

	return download.ResolveResult{
		PdfURL: fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", arxivID),
	}
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
