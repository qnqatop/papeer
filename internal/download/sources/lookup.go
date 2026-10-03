package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

const (
	s2DefaultBase       = "https://api.semanticscholar.org/graph/v1"
	openAlexDefaultBase = "https://api.openalex.org"

	// lookupRetries is the retry budget for metadata API lookups in the
	// download chain. Kept at 1: the later sources are the fallback, and a
	// long backoff on one API just stalls the whole paper.
	lookupRetries = 1

	// S2RateLimitedMarker is a stable substring of every reason produced when
	// Semantic Scholar rate-limits us. The frontend matches it to suggest
	// adding an API key (frontend/src/utils/errors.ts).
	S2RateLimitedMarker = "S2 rate limited"
)

// s2Paper is the S2 paper record shared by the arxiv and s2_doi sources. Both
// fields are requested in one call so the chain hits S2 once per DOI.
type s2Paper struct {
	// S2 returns externalIds with mixed value types — most keys are strings
	// (ArXiv, DOI, MAG) but some (PubMed, CorpusId) come back as numbers.
	ExternalIDs   map[string]any `json:"externalIds"`
	OpenAccessPdf *s2OAPdf       `json:"openAccessPdf"`
}

type s2OAPdf struct {
	URL    string `json:"url"`
	Status string `json:"status"`
}

// s2DOIKey is the Lookups key of the S2 paper-by-DOI record.
func s2DOIKey(doi string) string { return "s2:doi:" + strings.ToLower(doi) }

// fetchS2PaperByDOI returns the S2 record for info.DOI, memoized per paper.
func fetchS2PaperByDOI(ctx context.Context, client *httpclient.Client, info download.PaperInfo, base string) (*s2Paper, error) {
	if base == "" {
		base = s2DefaultBase
	}
	v, err := info.Lookups.Do(s2DOIKey(info.DOI), func() (any, error) {
		u := fmt.Sprintf("%s/paper/DOI:%s?fields=externalIds,openAccessPdf",
			base, url.PathEscape(info.DOI))
		var resp s2Paper
		if err := gatedGetJSON(ctx, client, info.Gate, u, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*s2Paper), nil
}

// gatedGetJSON performs a metadata GET through the rate gate: it fails fast
// with *download.RateLimitedError while the host is tripped, and trips the
// host when the request still ends in 429 after its retries.
func gatedGetJSON(ctx context.Context, client *httpclient.Client, gate *download.RateGate, rawURL string, dst any) error {
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Host
	}
	if until, blocked := gate.Blocked(host); blocked {
		return &download.RateLimitedError{Host: host, Until: until}
	}
	err := client.DoJSONWithRetry(ctx, rawURL, dst, lookupRetries)
	var se *httpclient.StatusError
	if errors.As(err, &se) && se.Code == http.StatusTooManyRequests {
		gate.Trip(host, se.RetryAfter)
	}
	return err
}

// isS2RateLimited reports whether err means S2 refused us for rate limiting,
// either live (429) or because the gate is tripped.
func isS2RateLimited(err error) bool {
	var rl *download.RateLimitedError
	if errors.As(err, &rl) {
		return true
	}
	var se *httpclient.StatusError
	return errors.As(err, &se) && se.Code == http.StatusTooManyRequests
}

// s2FailReason renders an S2 lookup error as a source Reason. Rate-limit
// outcomes always carry S2RateLimitedMarker.
func s2FailReason(prefix string, err error) string {
	var rl *download.RateLimitedError
	if errors.As(err, &rl) {
		return fmt.Sprintf("S2 skipped: %s (retry after %s)", S2RateLimitedMarker, rl.Until.Format("15:04:05"))
	}
	if isS2RateLimited(err) {
		return fmt.Sprintf("%s: %v", S2RateLimitedMarker, err)
	}
	return fmt.Sprintf("%s: %v", prefix, err)
}

// oaWork is the subset of an OpenAlex work used by the openalex and arxiv
// sources; the select list in fetchOpenAlexWork must cover these fields.
type oaWork struct {
	BestOALocation  *oaLoc  `json:"best_oa_location"`
	PrimaryLocation *oaLoc  `json:"primary_location"`
	Locations       []oaLoc `json:"locations"`
}

type oaLoc struct {
	PdfURL         string `json:"pdf_url"`
	LandingPageURL string `json:"landing_page_url"`
	IsOA           bool   `json:"is_oa"`
}

// fetchOpenAlexWork returns the OpenAlex work for info.DOI, memoized per paper.
func fetchOpenAlexWork(ctx context.Context, client *httpclient.Client, info download.PaperInfo, base string) (*oaWork, error) {
	if base == "" {
		base = openAlexDefaultBase
	}
	v, err := info.Lookups.Do("openalex:doi:"+strings.ToLower(info.DOI), func() (any, error) {
		u := fmt.Sprintf("%s/works/doi:%s?select=best_oa_location,primary_location,locations",
			base, url.PathEscape(info.DOI))
		var resp oaWork
		if err := client.DoJSON(ctx, u, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*oaWork), nil
}

// arxivIDPattern matches new-style (2101.00001) and old-style
// (hep-th/9901001, math.GT/0309136) arXiv identifiers with an optional
// version suffix, which is captured separately so it can be dropped.
const arxivIDPattern = `(\d{4}\.\d{4,5}|[a-z][a-z\-]*(?:\.[A-Z]{2})?/\d{7})(v\d+)?`

var (
	arxivURLRe = regexp.MustCompile(`(?i)^https?://(?:www\.|export\.)?arxiv\.org/(?:abs|pdf)/` + arxivIDPattern + `(?:\.pdf)?/?(?:[?#].*)?$`)
	arxivDOIRe = regexp.MustCompile(`(?i)^(?:https?://(?:dx\.)?doi\.org/)?10\.48550/arxiv\.` + arxivIDPattern + `$`)
)

// arxivIDFromURL extracts an arXiv ID from an arxiv.org abs/pdf URL or an
// arXiv DOI (10.48550/arXiv.ID). The version suffix is dropped so the PDF URL
// resolves to the latest version, matching what arXiv serves by default.
func arxivIDFromURL(s string) string {
	s = strings.TrimSpace(s)
	for _, re := range []*regexp.Regexp{arxivURLRe, arxivDOIRe} {
		if m := re.FindStringSubmatch(s); m != nil {
			return m[1]
		}
	}
	return ""
}

// arxivIDFromWork looks for an arXiv copy among the work's locations.
// OpenAlex lists the arXiv preprint as a location with source "arXiv
// (Cornell University)" whose landing page is arxiv.org/abs/IDvN or
// doi.org/10.48550/arXiv.ID; matching the URL alone is enough and does not
// depend on the source's display name.
func arxivIDFromWork(w *oaWork) string {
	if w == nil {
		return ""
	}
	locs := make([]oaLoc, 0, len(w.Locations)+2)
	for _, l := range []*oaLoc{w.BestOALocation, w.PrimaryLocation} {
		if l != nil {
			locs = append(locs, *l)
		}
	}
	locs = append(locs, w.Locations...)
	for _, l := range locs {
		for _, u := range []string{l.LandingPageURL, l.PdfURL} {
			if id := arxivIDFromURL(u); id != "" {
				return id
			}
		}
	}
	return ""
}
