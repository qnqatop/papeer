package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/httpclient"
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

	// Request must be a POST carrying the documented JSON body.
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotBody.Mode != "articles" || gotBody.Q != "рекомендательная система абитуриент" || gotBody.Size != 10 || gotBody.From != 0 {
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
			{"name": "Old", "year": "2010", "link": "/a/old"},
			{"name": "New", "year": 2022, "link": "/a/new"},
			{"name": "NoYear", "link": "/a/noyear"}
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
