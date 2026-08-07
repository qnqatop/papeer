package sources

import (
	"context"
	"fmt"
	"net/url"

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

	base := o.baseURL
	if base == "" {
		base = "https://api.openalex.org"
	}
	u := fmt.Sprintf("%s/works/doi:%s", base, url.PathEscape(info.DOI))

	var resp struct {
		BestOALocation  *oaLoc `json:"best_oa_location"`
		PrimaryLocation *oaLoc `json:"primary_location"`
	}
	if err := client.DoJSON(ctx, u, &resp); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("OpenAlex lookup failed: %v", err)}
	}

	for _, loc := range []*oaLoc{resp.BestOALocation, resp.PrimaryLocation} {
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

type oaLoc struct {
	PdfURL         string `json:"pdf_url"`
	LandingPageURL string `json:"landing_page_url"`
	IsOA           bool   `json:"is_oa"`
}
