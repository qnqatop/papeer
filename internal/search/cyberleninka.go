package search

import (
	"context"
	"encoding/json"
	"fmt"
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

// clRetryBackoff mirrors the battle-tested delays from the dissertation
// harvester (fetch_ru_refs.py): retry temporary API failures three times.
var clRetryBackoff = []time.Duration{3 * time.Second, 8 * time.Second, 20 * time.Second}

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

	body, err := json.Marshal(clRequest{Mode: "articles", Q: query, Size: limit, From: 0})
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

	out := make([]RawPaper, 0, len(resp.Articles))
	for _, a := range resp.Articles {
		// API has no year filter; drop older articles on our side.
		if yearMin > 0 && a.Year.v != nil && *a.Year.v < yearMin {
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
// harvester's 3/8/20 s backoff (up to 3 attempts). PostJSON has no retry
// variant, so the loop lives here.
func (cl *CyberLeninka) postWithRetry(ctx context.Context, url string, body []byte, headers map[string]string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < len(clRetryBackoff); attempt++ {
		raw, status, err := cl.client.PostJSON(ctx, url, body, headers)
		if err == nil && status < 400 {
			return raw, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = &httpclient.StatusError{Code: status, Host: "cyberleninka.ru"}
		}

		if attempt < len(clRetryBackoff)-1 && isCLRetryable(status) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(clRetryBackoff[attempt]):
			}
			continue
		}
		break
	}
	return nil, lastErr
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

var reHTMLTag = regexp.MustCompile(`<[^>]+>`)

// stripTags removes HTML markup (CyberLeninka highlights query matches with
// <b> tags in name/annotation/journal fields).
func stripTags(s string) string {
	return strings.TrimSpace(reHTMLTag.ReplaceAllString(s, ""))
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
