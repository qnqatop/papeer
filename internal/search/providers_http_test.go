package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

func jsonHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func textHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

func newTestHTTPClient() *httpclient.Client {
	return httpclient.New("test@univ.edu")
}

// ─── Semantic Scholar ────────────────────────────────────────────────────

func TestS2Search_HappyPath(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"data": [
			{
				"title": "Paper One",
				"abstract": "abs one",
				"year": 2024,
				"venue": "ICML",
				"authors": [{"name": "Alice"}, {"name": "Bob"}],
				"externalIds": {"DOI": "10.1/X", "ArXiv": "2401.01234"},
				"openAccessPdf": {"url": "https://oa/p1.pdf", "status": "GREEN"},
				"citationCount": 42
			}
		]
	}`))
	defer srv.Close()

	s := &SemanticScholar{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := s.Search(context.Background(), "test", 10, 2020)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d", len(papers))
	}
	p := papers[0]
	if p.Title != "Paper One" || p.Year == nil || *p.Year != 2024 {
		t.Errorf("unexpected paper: %+v", p)
	}
	if p.DOI != "10.1/x" {
		t.Errorf("DOI not lowercased: %q", p.DOI)
	}
	if p.PdfURL != "https://oa/p1.pdf" {
		t.Errorf("PdfURL = %q", p.PdfURL)
	}
	if p.CitationCount != 42 {
		t.Errorf("CitationCount = %d", p.CitationCount)
	}
	if p.Source != "semantic_scholar" {
		t.Errorf("Source = %q", p.Source)
	}
	if len(p.Authors) != 2 {
		t.Errorf("Authors = %v", p.Authors)
	}
}

func TestS2Search_SuppressesClosedPdf(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"data": [{"title": "X", "openAccessPdf": {"url": "https://x/p.pdf", "status": "CLOSED"}}]
	}`))
	defer srv.Close()

	s := &SemanticScholar{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, _ := s.Search(context.Background(), "q", 1, 2020)
	if len(papers) != 1 || papers[0].PdfURL != "" {
		t.Errorf("expected empty PdfURL for CLOSED status, got %+v", papers)
	}
}

func TestS2Search_EmptyAuthors(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"data": [{"title": "X", "authors": [{"name": ""}, {"name": "  "}, {"name": "Real"}]}]
	}`))
	defer srv.Close()

	s := &SemanticScholar{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, _ := s.Search(context.Background(), "q", 1, 2020)
	if len(papers[0].Authors) != 1 || papers[0].Authors[0] != "Real" {
		t.Errorf("Authors filtering broken: %v", papers[0].Authors)
	}
}

// ─── OpenAlex ────────────────────────────────────────────────────────────

func TestOpenAlexSearch_ReconstructsAbstract(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"results": [{
			"title": "OA Paper",
			"doi": "https://doi.org/10.5/Y",
			"publication_year": 2023,
			"cited_by_count": 7,
			"abstract_inverted_index": {"hello": [0], "world": [1]},
			"authorships": [{"author": {"display_name": "Carol"}}],
			"best_oa_location": {"pdf_url": "https://oa/p.pdf", "is_oa": true},
			"primary_location": {"source": {"display_name": "Nature"}}
		}]
	}`))
	defer srv.Close()

	o := &OpenAlex{client: newTestHTTPClient(), baseURL: srv.URL, email: "real@univ.edu"}
	papers, err := o.Search(context.Background(), "test", 10, 2020)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d", len(papers))
	}
	p := papers[0]
	if p.Abstract != "hello world" {
		t.Errorf("Abstract = %q", p.Abstract)
	}
	if p.DOI != "10.5/y" {
		t.Errorf("DOI = %q (want lowercased & stripped of host)", p.DOI)
	}
	if p.Venue != "Nature" {
		t.Errorf("Venue = %q", p.Venue)
	}
	if p.PdfURL != "https://oa/p.pdf" {
		t.Errorf("PdfURL = %q", p.PdfURL)
	}
	if p.CitationCount != 7 {
		t.Errorf("CitationCount = %d", p.CitationCount)
	}
}

func TestOpenAlexSearch_ClampsLimit(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()

	o := &OpenAlex{client: newTestHTTPClient(), baseURL: srv.URL}
	_, _ = o.Search(context.Background(), "test", 999, 2020)
	if !strings.Contains(capturedQuery, "per-page=50") {
		t.Errorf("limit not clamped to 50: %s", capturedQuery)
	}
}

// ─── Crossref ────────────────────────────────────────────────────────────

func TestCrossrefSearch_HappyPath(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"message": {
			"items": [{
				"title": ["Crossref Paper"],
				"container-title": ["JMLR"],
				"abstract": "<p>abs</p>",
				"DOI": "10.7/Z",
				"author": [{"given": "Eve", "family": "Doe"}],
				"link": [{"URL": "https://x/y.pdf", "content-type": "application/pdf"}],
				"is-referenced-by-count": 3,
				"published-online": {"date-parts": [[2022, 5, 1]]}
			}]
		}
	}`))
	defer srv.Close()

	cr := &Crossref{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := cr.Search(context.Background(), "q", 5, 2020)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d", len(papers))
	}
	p := papers[0]
	if p.Title != "Crossref Paper" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Venue != "JMLR" {
		t.Errorf("Venue = %q", p.Venue)
	}
	if p.DOI != "10.7/z" {
		t.Errorf("DOI = %q", p.DOI)
	}
	if p.Year == nil || *p.Year != 2022 {
		t.Errorf("Year = %+v", p.Year)
	}
	if p.PdfURL != "https://x/y.pdf" {
		t.Errorf("PdfURL = %q", p.PdfURL)
	}
	if p.CitationCount != 3 {
		t.Errorf("CitationCount = %d", p.CitationCount)
	}
	if len(p.Authors) != 1 || p.Authors[0] != "Eve Doe" {
		t.Errorf("Authors = %v", p.Authors)
	}
}

func TestCrossrefSearch_PrefersPublishedPrintYear(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"message": {"items": [{
			"title": ["X"],
			"published-print":  {"date-parts": [[2019]]},
			"published-online": {"date-parts": [[2020]]},
			"issued":           {"date-parts": [[2018]]}
		}]}
	}`))
	defer srv.Close()

	cr := &Crossref{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, _ := cr.Search(context.Background(), "q", 1, 2010)
	if papers[0].Year == nil || *papers[0].Year != 2019 {
		t.Errorf("Year = %+v, want 2019 (published-print priority)", papers[0].Year)
	}
}

func TestCrossrefSearch_NoPDFLinkInLinks(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"message": {"items": [{
			"title": ["X"],
			"link": [{"URL": "https://x/y.xml", "content-type": "application/xml"}]
		}]}
	}`))
	defer srv.Close()

	cr := &Crossref{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, _ := cr.Search(context.Background(), "q", 1, 2020)
	if papers[0].PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty when no pdf content-type", papers[0].PdfURL)
	}
}

// ─── ArXiv ───────────────────────────────────────────────────────────────

func TestArXivSearch_HappyPath(t *testing.T) {
	atom := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2401.12345v2</id>
    <title>ArXiv Paper</title>
    <summary>abstract here</summary>
    <published>2024-01-15T12:00:00Z</published>
    <author><name>Frank</name></author>
    <link rel="alternate" type="text/html" href="http://arxiv.org/abs/2401.12345v2"/>
    <link title="pdf" rel="related" type="application/pdf" href="http://arxiv.org/pdf/2401.12345v2.pdf"/>
    <arxiv:doi xmlns:arxiv="http://arxiv.org/schemas/atom">10.99/AA</arxiv:doi>
  </entry>
</feed>`
	srv := httptest.NewServer(textHandler(atom))
	defer srv.Close()

	a := &ArXiv{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := a.Search(context.Background(), "test", 5, 2020)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d", len(papers))
	}
	p := papers[0]
	if p.Title != "ArXiv Paper" {
		t.Errorf("Title = %q", p.Title)
	}
	// extractArxivID strips the version suffix (v1, v2, ...).
	if p.ArxivID != "2401.12345" {
		t.Errorf("ArxivID = %q (version suffix should be stripped)", p.ArxivID)
	}
	if p.PdfURL != "http://arxiv.org/pdf/2401.12345v2.pdf" {
		t.Errorf("PdfURL = %q", p.PdfURL)
	}
	if p.Source != "arxiv" {
		t.Errorf("Source = %q", p.Source)
	}
	if p.Year == nil || *p.Year != 2024 {
		t.Errorf("Year = %+v", p.Year)
	}
}

func TestArXivSearch_FiltersOldYears(t *testing.T) {
	atom := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/1801.00001</id>
    <title>Old</title>
    <published>2018-01-01T00:00:00Z</published>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2401.00002</id>
    <title>Recent</title>
    <published>2024-01-01T00:00:00Z</published>
  </entry>
</feed>`
	srv := httptest.NewServer(textHandler(atom))
	defer srv.Close()

	a := &ArXiv{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, _ := a.Search(context.Background(), "q", 10, 2020)
	if len(papers) != 1 || papers[0].Title != "Recent" {
		t.Errorf("year filter broken: %+v", papers)
	}
}

func TestArXivSearch_MalformedXMLReturnsError(t *testing.T) {
	srv := httptest.NewServer(textHandler(`<not-xml`))
	defer srv.Close()

	a := &ArXiv{client: newTestHTTPClient(), baseURL: srv.URL}
	_, err := a.Search(context.Background(), "q", 1, 2020)
	if err == nil {
		t.Error("expected XML parse error")
	}
}

// ─── CyberLeninka ─────────────────────────────────────────────────────────

func TestCyberLeninkaSearch_HappyPath(t *testing.T) {
	var gotMethod string
	var gotBody clRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"found": 1,
			"articles": [{
				"name": "Рекомендательная <b>система</b> для абитуриентов",
				"annotation": "Аннотация со <b>ссылкой</b>",
				"journal": "Вестник <b>вуза</b>",
				"year": "2019",
				"authors": ["Иванов И.И.", "Петров П.П."],
				"link": "/article/n/rekomendatelnaya-sistema"
			}]
		}`))
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL, email: "real@univ.edu"}
	papers, err := cl.Search(context.Background(), "рекомендательная система абитуриент", 10, 2015)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Request must be a POST carrying the documented JSON body. With a year
	// filter the page is enlarged to clYearFilterSize (filtering is
	// client-side), and the result is cut back to the requested limit.
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotBody.Mode != "articles" || gotBody.Q != "рекомендательная система абитуриент" || gotBody.Size != clYearFilterSize || gotBody.From != 0 {
		t.Errorf("request body = %+v", gotBody)
	}

	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d", len(papers))
	}
	p := papers[0]
	if p.Title != "Рекомендательная система для абитуриентов" {
		t.Errorf("Title not stripped of <b>: %q", p.Title)
	}
	if p.Abstract != "Аннотация со ссылкой" {
		t.Errorf("Abstract = %q", p.Abstract)
	}
	if p.Venue != "Вестник вуза" {
		t.Errorf("Venue = %q", p.Venue)
	}
	if p.Year == nil || *p.Year != 2019 {
		t.Errorf("Year = %v (want 2019 from string)", p.Year)
	}
	if len(p.Authors) != 2 {
		t.Errorf("Authors = %v", p.Authors)
	}
	if p.DOI != "" {
		t.Errorf("DOI = %q, want empty (CyberLeninka has no DOI)", p.DOI)
	}
	if p.PdfURL != "https://cyberleninka.ru/article/n/rekomendatelnaya-sistema/pdf" {
		t.Errorf("PdfURL = %q", p.PdfURL)
	}
	if p.Source != "cyberleninka" {
		t.Errorf("Source = %q", p.Source)
	}
}

func TestCyberLeninkaSearch_CapsSizeAt100(t *testing.T) {
	var gotSize int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body clRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotSize = body.Size
		_, _ = w.Write([]byte(`{"found": 0, "articles": []}`))
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	if _, err := cl.Search(context.Background(), "q", 500, 0); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotSize != 100 {
		t.Errorf("size = %d, want capped at 100", gotSize)
	}
}

func TestCyberLeninkaSearch_YearMinFilter(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"found": 3,
		"articles": [
			{"name": "Old", "year": "2010", "link": "/article/n/old"},
			{"name": "New", "year": 2022, "link": "/article/n/new"},
			{"name": "NoYear", "link": "/article/n/noyear"}
		]
	}`))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := cl.Search(context.Background(), "q", 10, 2015)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Old (2010) dropped; New (2022) kept; NoYear kept (unknown year not filtered).
	if len(papers) != 2 {
		t.Fatalf("len(papers) = %d, want 2 (Old filtered out)", len(papers))
	}
	for _, p := range papers {
		if p.Title == "Old" {
			t.Errorf("2010 paper should have been filtered by yearMin=2015")
		}
	}
}

func TestCyberLeninkaSearch_SizeIsLimitWithoutYearFilter(t *testing.T) {
	var gotSize int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body clRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotSize = body.Size
		_, _ = w.Write([]byte(`{"found": 0, "articles": []}`))
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	if _, err := cl.Search(context.Background(), "q", 10, 0); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotSize != 10 {
		t.Errorf("size = %d, want 10 (no year filter → no over-fetch)", gotSize)
	}
}

// Regression: the radar asks for 3 recent papers. The API has no year filter,
// so asking for only 3 records returned the 3 most relevant — all old — and the
// client-side filter left nothing.
func TestCyberLeninkaSearch_YearFilterOverFetchesAndTruncates(t *testing.T) {
	var articles []string
	for i := 0; i < 20; i++ {
		articles = append(articles, fmt.Sprintf(`{"name": "Old %d", "year": 2001, "link": "/article/n/old-%d"}`, i, i))
	}
	for i := 0; i < 5; i++ {
		articles = append(articles, fmt.Sprintf(`{"name": "New %d", "year": 2025, "link": "/article/n/new-%d"}`, i, i))
	}
	srv := httptest.NewServer(jsonHandler(`{"found": 25, "articles": [` + strings.Join(articles, ",") + `]}`))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := cl.Search(context.Background(), "q", 3, 2024)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 3 {
		t.Fatalf("len(papers) = %d, want 3 (limit)", len(papers))
	}
	for _, p := range papers {
		if !strings.HasPrefix(p.Title, "New") {
			t.Errorf("unexpected paper %q, want only recent ones", p.Title)
		}
	}
}

func TestCyberLeninkaSearch_SkipsNonArticleLinks(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(`{
		"found": 4,
		"articles": [
			{"name": "Empty", "link": ""},
			{"name": "HostSuffix", "link": ".evil.com/x"},
			{"name": "Absolute", "link": "https://evil.com/article/n/x"},
			{"name": "Good", "link": "/article/n/good"}
		]
	}`))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := cl.Search(context.Background(), "q", 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 || papers[0].Title != "Good" {
		t.Fatalf("papers = %+v, want only the /article/ one", papers)
	}
	if papers[0].PdfURL != "https://cyberleninka.ru/article/n/good/pdf" {
		t.Errorf("PdfURL = %q", papers[0].PdfURL)
	}
}

// A captcha page served with 200 must surface as HTTP 429 (so the engine's
// circuit breaker skips the provider) and must not be retried.
func TestCyberLeninkaSearch_CaptchaPageIsRateLimit(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>captcha</body></html>"))
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	_, err := cl.Search(context.Background(), "q", 10, 0)
	if got := httpStatusCode(err); got != 429 {
		t.Fatalf("status = %d (err %v), want 429", got, err)
	}
	if providerUnavailableReason(err) == "" {
		t.Error("captcha error should trip the circuit breaker")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("calls = %d, want 1 (no retry on captcha)", n)
	}
}

func withFastCLBackoff(t *testing.T) {
	t.Helper()
	saved := clRetryBackoff
	clRetryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { clRetryBackoff = saved })
}

func TestCyberLeninkaSearch_RetriesTransientThenSucceeds(t *testing.T) {
	withFastCLBackoff(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"found": 1, "articles": [{"name": "Ok", "link": "/article/n/ok"}]}`))
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	papers, err := cl.Search(context.Background(), "q", 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d, want 1", len(papers))
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("calls = %d, want 3", n)
	}
}

func TestCyberLeninkaSearch_GivesUpAfterAllAttempts(t *testing.T) {
	withFastCLBackoff(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	_, err := cl.Search(context.Background(), "q", 10, 0)
	if got := httpStatusCode(err); got != 429 {
		t.Fatalf("status = %d (err %v), want 429", got, err)
	}
	if n, want := atomic.LoadInt32(&calls), int32(len(clRetryBackoff)+1); n != want {
		t.Errorf("calls = %d, want %d", n, want)
	}
}

func TestCyberLeninkaSearch_DoesNotRetryClientErrors(t *testing.T) {
	withFastCLBackoff(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	cl := &CyberLeninka{client: newTestHTTPClient(), baseURL: srv.URL}
	if _, err := cl.Search(context.Background(), "q", 10, 0); httpStatusCode(err) != 400 {
		t.Fatalf("err = %v, want HTTP 400", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("calls = %d, want 1", n)
	}
}

func TestStripTags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Рекомендательная <b>система</b>", "Рекомендательная система"},
		{"<B>Bold</B> and <em>em</em>", "Bold and em"},
		// Comparison signs in text must survive (a generic <[^>]+> ate them).
		{"различия значимы (p<0.05) при n>30", "различия значимы (p<0.05) при n>30"},
		{"&laquo;Цифровая&raquo; школа &amp; вуз", "«Цифровая» школа & вуз"},
		{"&quot;Quoted&quot; &lt;b&gt; stays literal", `"Quoted" <b> stays literal`},
		{"  padded  ", "padded"},
	}
	for _, c := range cases {
		if got := stripTags(c.in); got != c.want {
			t.Errorf("stripTags(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCLYear_Unmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want *int
	}{
		{`2019`, ptr.Ptr(2019)},
		{`"2019"`, ptr.Ptr(2019)},
		{`""`, nil},
		{`null`, nil},
		{`"n/a"`, nil},
	}
	for _, c := range cases {
		var y clYear
		if err := json.Unmarshal([]byte(c.in), &y); err != nil {
			t.Fatalf("Unmarshal(%s): %v", c.in, err)
		}
		switch {
		case c.want == nil && y.v != nil:
			t.Errorf("clYear(%s) = %d, want nil", c.in, *y.v)
		case c.want != nil && (y.v == nil || *y.v != *c.want):
			t.Errorf("clYear(%s) = %v, want %d", c.in, y.v, *c.want)
		}
	}
}
