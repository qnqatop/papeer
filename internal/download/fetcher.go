package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/qnqatop/papeer/internal/httpclient"
)

// Patterns for extracting PDF URLs from HTML pages.
//
// Attribute order on <meta> tags is not fixed — MDPI/Springer often put
// content="..." before name="citation_pdf_url", while other sites do the
// reverse. We match both orderings.
var (
	reCitationPDFNameFirst    = regexp.MustCompile(`(?is)<meta[^>]+name=["']citation_pdf_url["'][^>]+content=["']([^"']+)["']`)
	reCitationPDFContentFirst = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']citation_pdf_url["']`)
	reHrefPDF                 = regexp.MustCompile(`(?i)href=["']([^"']+\.pdf(?:\?[^"']*)?)["']`)
	reEmbedPDF                = regexp.MustCompile(`(?i)<embed[^>]+src=["']([^"']+\.pdf[^"']*)["']`)
	reIframePDF               = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+\.pdf[^"']*)["']`)
	reMetaRefresh             = regexp.MustCompile(`(?is)<meta[^>]+http-equiv=["']refresh["'][^>]+content=["'][^"']*url=([^"'>\s]+)["']`)
	reJSLocation              = regexp.MustCompile(`(?i)(?:window\.location(?:\.href|\.replace\()?\s*=\s*|location\.href\s*=\s*)["']([^"']+\.pdf[^"']*)["']`)

	// Hindawi was acquired by Wiley in 2024 and downloads.hindawi.com began
	// returning 404 in 2025. Old metadata APIs (Crossref, OpenAlex, search
	// reports) still hand out the legacy URL. DOI encoding is preserved in
	// the path: /journals/<short>/<year>/<id>.pdf → 10.1155/<year>/<id>.
	reHindawiLegacy = regexp.MustCompile(`(?i)^https?://downloads\.hindawi\.com/journals/[^/]+/(\d{4})/([^/.]+)\.pdf`)
)

// ExtractPDFFromHTML scans HTML content for a PDF URL using common patterns:
//  1. citation_pdf_url meta tag (Google Scholar standard, either attribute order)
//  2. <meta http-equiv="refresh" ...> with a url= target
//  3. JS window.location = "...pdf..." redirects (MDPI-style interstitials)
//  4. Direct .pdf links in href attributes
//  5. <embed> and <iframe> tags with .pdf src
//
// Returns the resolved absolute URL or an empty string if nothing found.
func ExtractPDFFromHTML(html string, baseURL string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	resolve := func(raw string) string {
		ref, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		return base.ResolveReference(ref).String()
	}

	// 1. citation_pdf_url meta tag (both attribute orders).
	for _, re := range []*regexp.Regexp{reCitationPDFNameFirst, reCitationPDFContentFirst} {
		if m := re.FindStringSubmatch(html); m != nil {
			return resolve(m[1])
		}
	}

	// 2. meta refresh redirect.
	if m := reMetaRefresh.FindStringSubmatch(html); m != nil {
		target := strings.Trim(m[1], `"' `)
		if target != "" {
			return resolve(target)
		}
	}

	// 3. JS window.location redirect to a .pdf URL.
	if m := reJSLocation.FindStringSubmatch(html); m != nil {
		return resolve(m[1])
	}

	// 4-5. href, embed, iframe patterns.
	for _, re := range []*regexp.Regexp{reHrefPDF, reEmbedPDF, reIframePDF} {
		if m := re.FindStringSubmatch(html); m != nil {
			return resolve(m[1])
		}
	}

	return ""
}

// rewriteDeadURL maps known-defunct PDF endpoints to their current home.
// Returns the input unchanged when no rule matches.
func rewriteDeadURL(rawURL string) string {
	if m := reHindawiLegacy.FindStringSubmatch(rawURL); m != nil {
		return fmt.Sprintf("https://onlinelibrary.wiley.com/doi/pdf/10.1155/%s/%s", m[1], m[2])
	}
	return rawURL
}

// looksLikeHTML returns true when data appears to be an HTML document
// rather than a binary file like PDF.
func looksLikeHTML(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	// Check for leading '<' (HTML/XML).
	if trimmed[0] == '<' {
		return true
	}
	// Check for common HTML markers in the first kilobyte.
	head := trimmed
	if len(head) > 1024 {
		head = head[:1024]
	}
	lower := strings.ToLower(string(head))
	return strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html")
}

// interstitialRetryDelays controls the wait-then-retry loop for publishers
// (notably MDPI) that serve a "logo + countdown" HTML interstitial on the
// initial PDF request and only return the binary on a subsequent re-fetch
// once the session cookie has settled. Total budget ≈ 13s.
var interstitialRetryDelays = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	7 * time.Second,
}

// FetchPDF downloads a PDF from url into dest, handling HTML landing pages,
// Akamai Bot Manager challenges, and MDPI-style "wait N seconds" interstitials.
//
// Flow:
//
//  1. GET the URL.
//  2. If the body is an Akamai interstitial, POST the verify endpoint and
//     re-GET. Cookies are reused across attempts via the client's jar.
//  3. If the body is HTML, try in order:
//     a) Scan for an embedded PDF link (citation_pdf_url, meta refresh,
//     JS window.location, href, embed, iframe) and GET it.
//     b) If still HTML and the URL contains /pdf, re-GET the same URL
//     after a delay — MDPI shows a logo for ~5s before the PDF stream
//     becomes available with the session cookie set by the first hit.
//  4. Validate the final bytes look like a PDF.
func FetchPDF(ctx context.Context, client *httpclient.Client, rawURL string, dest string) error {
	// Step 0: rewrite known-dead URLs (e.g., Hindawi → Wiley after 2024
	// acquisition). Old upstream APIs still hand out the legacy host.
	rawURL = rewriteDeadURL(rawURL)

	// Step 1: download the URL.
	data, _, err := client.DownloadFile(ctx, rawURL)
	if err != nil {
		// HTTP-level block (403 from ACM/IEEE/Elsevier after all UA strategies,
		// 4xx/5xx with no body, network reset). DownloadFile gives us nothing
		// to validate, so the regular HTML→Chrome path never triggers. Try
		// Chrome here as a last resort — it carries cookies and a real JS
		// runtime and frequently slips past UA-based bot walls.
		if ChromeAvailable() {
			if chromeErr := FetchPDFWithChrome(ctx, rawURL, dest); chromeErr == nil {
				return nil
			} else if !errors.Is(chromeErr, ErrChromeNotFound) {
				return fmt.Errorf("downloading %s (chrome fallback: %v): %w", rawURL, chromeErr, err)
			}
		}
		return fmt.Errorf("downloading %s: %w", rawURL, err)
	}

	// Step 2: handle Akamai Bot Manager interstitial. Detected before the
	// generic HTML-landing flow because the body looks like HTML but the
	// solution is a verify-POST, not an embedded PDF link.
	//
	// Akamai may serve several rounds of interstitials (new token + new pow
	// each round). Loop up to maxAkamaiRounds and break out when the response
	// stops being an interstitial.
	const maxAkamaiRounds = 4
	for round := 0; round < maxAkamaiRounds; round++ {
		if !looksLikeHTML(data) || !isAkamaiInterstitial(data) {
			break
		}
		ok, solveErr := solveAkamaiInterstitial(ctx, client, rawURL, data)
		if solveErr != nil {
			return fmt.Errorf("Akamai bypass for %s failed (round %d): %w", rawURL, round+1, solveErr)
		}
		if !ok {
			return fmt.Errorf("Akamai bypass for %s rejected by server (round %d)", rawURL, round+1)
		}
		// Brief pause so the new ak_bmsc cookie registers on Akamai edge
		// before we re-fetch — the JS interstitial does the same.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		// Re-fetch with the fresh ak_bmsc cookie in the jar.
		data, _, err = client.DownloadFile(ctx, rawURL)
		if err != nil {
			return fmt.Errorf("downloading after Akamai bypass %s: %w", rawURL, err)
		}
	}

	// Step 3a: regular HTML landing page → scan for embedded PDF link.
	if looksLikeHTML(data) {
		pdfURL := ExtractPDFFromHTML(string(data), rawURL)
		if pdfURL != "" && pdfURL != rawURL {
			data2, _, err := client.DownloadFile(ctx, pdfURL)
			if err == nil {
				data = data2
			}
		}
	}

	// Step 3b: still HTML at a /pdf URL → wait for the JS countdown and
	// re-fetch the same URL. The first hit set the cookies the publisher
	// needs to stream the PDF binary on the next request.
	if looksLikeHTML(data) && isPDFEndpoint(rawURL) {
		for _, wait := range interstitialRetryDelays {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			next, _, err := client.DownloadFile(ctx, rawURL)
			if err != nil {
				break
			}
			data = next
			if !looksLikeHTML(data) {
				break
			}
		}
	}

	// Step 4: validate. If validation fails and the body is HTML, fall back
	// to headless Chrome. Originally limited to Akamai interstitials, but
	// publishers like ScienceDirect (signed pdfft URLs gated by session
	// cookies + Referer) and mid-tier journals (mazums.ac.ir,
	// jhs.mazums.ac.ir) hand us a regular landing page whose
	// citation_pdf_url either isn't there or points at another HTML stop —
	// only a real browser navigation actually yields the binary. Akamai
	// detection is left earlier in the flow so we still skip the cheap
	// HTML-scanning round when we can prove it's a bot wall.
	if err := ValidatePDF(data); err != nil {
		if looksLikeHTML(data) && ChromeAvailable() {
			if chromeErr := FetchPDFWithChrome(ctx, rawURL, dest); chromeErr == nil {
				return nil
			} else if !errors.Is(chromeErr, ErrChromeNotFound) {
				return fmt.Errorf("PDF validation failed for %s (chrome fallback: %v): %w", rawURL, chromeErr, err)
			}
		}
		return fmt.Errorf("PDF validation failed for %s: %w", rawURL, err)
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}

	return nil
}

// isPDFEndpoint reports whether the URL is most likely the publisher's PDF
// stream (ends in .pdf or has a /pdf segment), as opposed to an article
// landing page. Used to gate the wait-and-retry interstitial bypass — we
// don't want to retry article landings, only PDF endpoints that should
// eventually serve a binary.
func isPDFEndpoint(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	if strings.HasSuffix(p, ".pdf") {
		return true
	}
	if strings.HasSuffix(p, "/pdf") {
		return true
	}
	if strings.Contains(p, "/pdf/") {
		return true
	}
	return false
}
