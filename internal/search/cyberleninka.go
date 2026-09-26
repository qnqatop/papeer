package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qnqatop/papeer/internal/httpclient"
)

// CyberLeninka searches the largest Russian-language open-access article base
// via its undocumented but public POST endpoint /api/search. There is no DOI
// in CyberLeninka records; papers are identified by a relative article slug,
// from which the PDF URL is built as https://cyberleninka.ru<link>/pdf.
type CyberLeninka struct {
	client  *httpclient.Client
	email   string
	baseURL string // test hook; empty = production endpoint
}

func NewCyberLeninka(c *httpclient.Client, email string) *CyberLeninka {
	return &CyberLeninka{client: c, email: email}
}

func (cl *CyberLeninka) Name() string { return "cyberleninka" }

// Language reports the article language served by this provider, so the engine
// can restrict a Russian-scoped axis to Russian sources.
func (cl *CyberLeninka) Language() string { return "ru" }

// clRetryBackoff holds the pauses between attempts on temporary API failures
// (delays from the dissertation harvester, fetch_ru_refs.py): up to
// len(clRetryBackoff)+1 attempts in total.
var clRetryBackoff = []time.Duration{3 * time.Second, 8 * time.Second}

// clYearFilterSize is the page size requested when a year filter is set. The
// API cannot filter by year, so filtering happens client-side; asking for only
// `limit` records would leave few or none after the filter (e.g. the radar asks
// for 3 recent papers — the 3 most relevant ones are rarely from last year).
const clYearFilterSize = 100

func (cl *CyberLeninka) Search(ctx context.Context, query string, limit int, yearMin int) ([]RawPaper, error) {
	if limit > 100 {
		limit = 100
	}
	if limit < 1 {
		limit = 1
	}
	base := cl.baseURL
	if base == "" {
		base = "https://cyberleninka.ru"
	}
	u := base + "/api/search"

	size := limit
	if yearMin > 0 {
		size = clYearFilterSize
	}
	body, err := json.Marshal(clRequest{Mode: "articles", Q: query, Size: size, From: 0})
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"User-Agent": httpclient.PoliteUA(cl.email)}

	raw, err := cl.postWithRetry(ctx, u, body, headers)
	if err != nil {
		return nil, err
	}

	var resp clResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("cyberleninka: decode response: %w", err)
	}

	out := make([]RawPaper, 0, min(limit, len(resp.Articles)))
	for _, a := range resp.Articles {
		if len(out) >= limit {
			break
		}
		// API has no year filter; drop older articles on our side.
		if yearMin > 0 && a.Year.v != nil && *a.Year.v < yearMin {
			continue
		}
		// The PDF URL is built by appending the relative link to our host, so
		// only accept article slugs — anything else (empty, absolute, or a
		// ".evil.com/..." suffix) would point the download somewhere else.
		if !strings.HasPrefix(a.Link, "/article/") {
			continue
		}
		out = append(out, RawPaper{
			Title:    stripTags(a.Name),
			Abstract: stripTags(a.Annotation),
			Year:     a.Year.v,
			Venue:    stripTags(a.Journal),
			Authors:  a.Authors,
			DOI:      "", // CyberLeninka has no DOI
			PdfURL:   "https://cyberleninka.ru" + a.Link + "/pdf",
			Source:   "cyberleninka",
		})
	}
	return out, nil
}

// postWithRetry POSTs the search request, retrying on 429/502/503/504 with the
// clRetryBackoff pauses. PostJSON has no retry variant, so the loop lives here.
//
// A 200 response whose body is not JSON is CyberLeninka's captcha/anti-bot
// page. It is reported as HTTP 429 without retrying, so the engine's circuit
// breaker skips the provider for the remaining queries instead of hitting the
// captcha again on every query.
func (cl *CyberLeninka) postWithRetry(ctx context.Context, url string, body []byte, headers map[string]string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		raw, status, err := cl.client.PostJSON(ctx, url, body, headers)
		if err == nil && status < 400 {
			if !looksLikeJSON(raw) {
				return nil, &httpclient.StatusError{Code: 429, Host: "cyberleninka.ru"}
			}
			return raw, nil
		}
		if err == nil {
			err = &httpclient.StatusError{Code: status, Host: "cyberleninka.ru"}
		}
		if attempt >= len(clRetryBackoff) || !isCLRetryable(status) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(clRetryBackoff[attempt]):
		}
	}
}

// looksLikeJSON reports whether body starts (after whitespace) like a JSON
// object — the search API always answers with one.
func looksLikeJSON(body []byte) bool {
	b := bytes.TrimSpace(body)
	return len(b) > 0 && b[0] == '{'
}

// isCLRetryable reports whether a status warrants a retry. status == 0 means the
// transport failed before a response; those are not retried (network errors are
// usually not transient here and the circuit breaker handles the rest).
func isCLRetryable(status int) bool {
	switch status {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

// reHighlightTag matches only the inline highlight/formatting tags CyberLeninka
// puts into name/annotation/journal fields (query matches are wrapped in <b>).
// A generic `<[^>]+>` would also eat text such as "p<0.05 при n>30".
var reHighlightTag = regexp.MustCompile(`(?i)</?(?:b|i|em|strong|mark|sup|sub)\s*>`)

// stripTags removes the highlight markup and decodes HTML entities (&quot;,
// &laquo;, &amp; ...) so titles read and deduplicate as plain text.
func stripTags(s string) string {
	s = reHighlightTag.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

type clRequest struct {
	Mode string `json:"mode"`
	Q    string `json:"q"`
	Size int    `json:"size"`
	From int    `json:"from"`
}

type clResponse struct {
	Found    int         `json:"found"`
	Articles []clArticle `json:"articles"`
}

type clArticle struct {
	Name       string   `json:"name"`
	Annotation string   `json:"annotation"`
	Journal    string   `json:"journal"`
	Year       clYear   `json:"year"`
	Authors    []string `json:"authors"`
	Link       string   `json:"link"`
}

// clYear accepts both numeric (2019) and string ("2019") year encodings —
// CyberLeninka is not consistent — and yields a *int or nil when absent/unparseable.
type clYear struct{ v *int }

func (y *clYear) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil // ignore unparseable year rather than failing the whole decode
	}
	y.v = &n
	return nil
}
