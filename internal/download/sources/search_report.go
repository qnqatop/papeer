package sources

import (
	"context"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// SearchReport uses a pre-found PDF URL from the search phase.
type SearchReport struct{}

func NewSearchReport() *SearchReport { return &SearchReport{} }

func (s *SearchReport) Name() string { return "search_report" }

func (s *SearchReport) Resolve(_ context.Context, _ *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.PdfURL != "" {
		return download.ResolveResult{PdfURL: info.PdfURL}
	}
	return download.ResolveResult{Reason: "no pre-found PDF URL"}
}
