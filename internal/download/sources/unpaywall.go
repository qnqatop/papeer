package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

// Unpaywall resolves PDF via Unpaywall API by DOI.
type Unpaywall struct {
	baseURL string // test hook; empty = production endpoint
}

func NewUnpaywall() *Unpaywall { return &Unpaywall{} }

func (u *Unpaywall) Name() string { return "unpaywall" }

func (u *Unpaywall) Resolve(ctx context.Context, client *httpclient.Client, info download.PaperInfo) download.ResolveResult {
	if info.DOI == "" {
		return download.ResolveResult{Reason: "no DOI"}
	}
	if info.Email == "" || strings.Contains(info.Email, "example") {
		return download.ResolveResult{Reason: "Unpaywall: no real email provided"}
	}

	base := u.baseURL
	if base == "" {
		base = "https://api.unpaywall.org/v2"
	}
	apiURL := fmt.Sprintf("%s/%s?email=%s",
		base, url.PathEscape(info.DOI), url.QueryEscape(info.Email))

	var resp struct {
		BestOALocation *upLoc  `json:"best_oa_location"`
		OALocations    []upLoc `json:"oa_locations"`
	}
	if err := client.DoJSON(ctx, apiURL, &resp); err != nil {
		return download.ResolveResult{Reason: fmt.Sprintf("Unpaywall lookup failed: %v", err)}
	}

	// Try best OA location first.
	if resp.BestOALocation != nil {
		if pdfURL := resp.BestOALocation.bestURL(); pdfURL != "" {
			return download.ResolveResult{PdfURL: pdfURL}
		}
	}

	// Fall back to any OA location.
	for _, loc := range resp.OALocations {
		if pdfURL := loc.bestURL(); pdfURL != "" {
			return download.ResolveResult{PdfURL: pdfURL}
		}
	}

	return download.ResolveResult{Reason: "Unpaywall: no OA"}
}

type upLoc struct {
	URLForPdf string `json:"url_for_pdf"`
	URL       string `json:"url"`
}

func (l upLoc) bestURL() string {
	if l.URLForPdf != "" {
		return l.URLForPdf
	}
	return l.URL
}
