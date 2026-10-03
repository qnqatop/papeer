package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qnqatop/papeer/internal/download"
)

// countingServer serves body (with status) for every request and counts hits.
func countingServer(t *testing.T, status int, header map[string]string, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// chain runs arxiv → s2_doi → openalex → s2_title against the given test
// servers with one per-paper memo, like Engine.DownloadOne does.
func chain(t *testing.T, s2URL, oaURL string, info download.PaperInfo) []download.ResolveResult {
	t.Helper()
	c, _ := testSetup(http.NotFoundHandler())
	info.Lookups = download.NewLookups()
	srcs := []download.Source{
		&ArXiv{s2BaseURL: s2URL, openAlexBaseURL: oaURL},
		&S2ByDOI{baseURL: s2URL},
		&OpenAlex{baseURL: oaURL},
		&S2ByTitle{baseURL: s2URL},
	}
	var out []download.ResolveResult
	for _, s := range srcs {
		out = append(out, s.Resolve(context.Background(), c, info))
	}
	return out
}

func TestChain_OneS2AndOneOpenAlexRequestPerDOI(t *testing.T) {
	s2, s2Hits := countingServer(t, 200, nil, `{"externalIds": {"DOI": "10.1/x"}, "openAccessPdf": null}`)
	oa, oaHits := countingServer(t, 200, nil, `{"best_oa_location": null, "primary_location": null, "locations": []}`)

	res := chain(t, s2.URL, oa.URL, download.PaperInfo{DOI: "10.1/x", Title: "A paper"})

	if n := s2Hits.Load(); n != 1 {
		t.Errorf("S2 requests = %d, want 1 (arxiv, s2_doi and s2_title share the DOI lookup)", n)
	}
	if n := oaHits.Load(); n != 1 {
		t.Errorf("OpenAlex requests = %d, want 1 (arxiv and openalex share the work)", n)
	}
	if !strings.Contains(res[3].Reason, "already checked by DOI") {
		t.Errorf("s2_title reason = %q, want skip", res[3].Reason)
	}
}

func TestChain_TitleSearchRunsWhenDOIUnknownToS2(t *testing.T) {
	var doiHits, searchHits atomic.Int32
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/paper/search") {
			searchHits.Add(1)
			_, _ = w.Write([]byte(`{"data": [{"openAccessPdf": {"url": "https://s2.example/p.pdf"}}]}`))
			return
		}
		doiHits.Add(1)
		w.WriteHeader(404)
	}))
	t.Cleanup(s2.Close)
	oa, _ := countingServer(t, 200, nil, `{}`)

	res := chain(t, s2.URL, oa.URL, download.PaperInfo{DOI: "10.1/x", Title: "A paper"})

	if doiHits.Load() != 1 || searchHits.Load() != 1 {
		t.Errorf("DOI hits = %d, search hits = %d; want 1 and 1", doiHits.Load(), searchHits.Load())
	}
	if res[3].PdfURL != "https://s2.example/p.pdf" {
		t.Errorf("s2_title = %+v, want title match", res[3])
	}
}

func TestChain_S2RateLimitTripsGate(t *testing.T) {
	// Retry-After: 1 keeps the single retry's wait short.
	s2, s2Hits := countingServer(t, 429, map[string]string{"Retry-After": "1"}, `{}`)
	oa, _ := countingServer(t, 200, nil, `{}`)
	gate := download.NewRateGate()

	start := time.Now()
	res := chain(t, s2.URL, oa.URL, download.PaperInfo{DOI: "10.1/x", Title: "A paper", Gate: gate})

	// One request + exactly one retry, then the gate short-circuits s2_title.
	if n := s2Hits.Load(); n != 2 {
		t.Errorf("S2 requests = %d, want 2 (1 retry)", n)
	}
	for i, r := range res {
		if i == 2 { // openalex
			continue
		}
		if !strings.Contains(r.Reason, S2RateLimitedMarker) {
			t.Errorf("source %d reason = %q, want %q marker", i, r.Reason, S2RateLimitedMarker)
		}
	}
	if !strings.Contains(res[3].Reason, "S2 skipped") {
		t.Errorf("s2_title reason = %q, want gate skip", res[3].Reason)
	}

	host := strings.TrimPrefix(s2.URL, "http://")
	until, blocked := gate.Blocked(host)
	if !blocked || until.Before(start.Add(download.MinRateTrip)) {
		t.Fatalf("gate = %v, %v; want blocked for at least MinRateTrip", until, blocked)
	}

	// The next paper (fresh memo, same app-scoped gate) never reaches S2.
	res = chain(t, s2.URL, oa.URL, download.PaperInfo{DOI: "10.2/y", Title: "Another", Gate: gate})
	if n := s2Hits.Load(); n != 2 {
		t.Errorf("S2 requests after trip = %d, want still 2", n)
	}
	if !strings.Contains(res[1].Reason, "S2 skipped: S2 rate limited (retry after ") {
		t.Errorf("s2_doi reason = %q", res[1].Reason)
	}
}

func TestArxiv_FromOpenAlexLocationsWithoutS2(t *testing.T) {
	s2, s2Hits := countingServer(t, 200, nil, `{"externalIds": {"ArXiv": "9999.99999"}}`)
	oa, _ := countingServer(t, 200, nil, `{
		"best_oa_location": {"landing_page_url": "https://publisher.example/a", "is_oa": true},
		"locations": [
			{"landing_page_url": "https://doi.org/10.1/x"},
			{"landing_page_url": "https://arxiv.org/abs/2101.00001v2", "pdf_url": "https://arxiv.org/pdf/2101.00001v2",
			 "source": {"display_name": "arXiv (Cornell University)"}}
		]
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	a := &ArXiv{s2BaseURL: s2.URL, openAlexBaseURL: oa.URL}

	r := a.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://arxiv.org/pdf/2101.00001.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
	if s2Hits.Load() != 0 {
		t.Errorf("S2 hit %d times, want 0 when OpenAlex has the arXiv ID", s2Hits.Load())
	}
}

func TestArxiv_FallsBackToS2ExternalIDs(t *testing.T) {
	s2, _ := countingServer(t, 200, nil, `{"externalIds": {"ArXiv": "2301.99999", "CorpusId": 123}}`)
	oa, _ := countingServer(t, 200, nil, `{"locations": [{"landing_page_url": "https://publisher.example/a"}]}`)
	c, _ := testSetup(http.NotFoundHandler())
	a := &ArXiv{s2BaseURL: s2.URL, openAlexBaseURL: oa.URL}

	r := a.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://arxiv.org/pdf/2301.99999.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestArxiv_ArxivDOINeedsNoRequest(t *testing.T) {
	a := NewArXiv()
	r := a.Resolve(context.Background(), nil, download.PaperInfo{DOI: "10.48550/arXiv.2301.12345"})
	if r.PdfURL != "https://arxiv.org/pdf/2301.12345.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestArxivIDFromURL(t *testing.T) {
	cases := map[string]string{
		"https://arxiv.org/abs/2101.00001v2":           "2101.00001",
		"http://arxiv.org/abs/2101.00001":              "2101.00001",
		"https://arxiv.org/pdf/2101.00001v3":           "2101.00001",
		"https://arxiv.org/pdf/2101.00001v3.pdf":       "2101.00001",
		"https://export.arxiv.org/abs/1501.0001":       "1501.0001",
		"https://arxiv.org/abs/hep-th/9901001v1":       "hep-th/9901001",
		"https://arxiv.org/abs/math.GT/0309136":        "math.GT/0309136",
		"https://doi.org/10.48550/arXiv.2101.00001":    "2101.00001",
		"10.48550/arxiv.2101.00001":                    "2101.00001",
		"https://arxiv.org/list/cs.AI/recent":          "",
		"https://publisher.example/abs/2101.00001":     "",
		"https://arxiv.org.evil.example/abs/2101.0001": "",
		"": "",
	}
	for in, want := range cases {
		if got := arxivIDFromURL(in); got != want {
			t.Errorf("arxivIDFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
