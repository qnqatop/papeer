package sources

import (
	"context"
	"strings"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// CyberLeninka resolves a PDF from the URL already found during search. Unlike
// DOI-based sources it makes no network call: the search provider stored the
// final .../pdf endpoint in PdfURL, so resolving is just a host check.
type CyberLeninka struct{}

func NewCyberLeninka() *CyberLeninka { return &CyberLeninka{} }

func (c *CyberLeninka) Name() string { return "cyberleninka" }

func (c *CyberLeninka) Resolve(_ context.Context, _ *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if strings.Contains(info.PdfURL, "cyberleninka.ru") {
		return download.ResolveResult{PdfURL: info.PdfURL}
	}
	return download.ResolveResult{Reason: "not a cyberleninka paper"}
}
