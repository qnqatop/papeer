package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/download"
)

// jsonServer spins up an httptest.Server that responds with the given body
// for any request. Failed-handler tests can override the handler.
func jsonServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ─── OpenAlex ────────────────────────────────────────────────────────────

func TestOpenAlex_BestOA_PdfURL(t *testing.T) {
	srv := jsonServer(t, `{
		"best_oa_location": {"pdf_url": "https://oa.example/paper.pdf", "is_oa": true}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://oa.example/paper.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestOpenAlex_BestOA_LandingPageFallback(t *testing.T) {
	// pdf_url empty, but is_oa=true and landing_page_url present → return landing.
	srv := jsonServer(t, `{
		"best_oa_location": {"landing_page_url": "https://oa.example/landing", "is_oa": true}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://oa.example/landing" {
		t.Errorf("PdfURL = %q, want landing fallback", r.PdfURL)
	}
}

func TestOpenAlex_FallsBackToPrimaryLocation(t *testing.T) {
	srv := jsonServer(t, `{
		"best_oa_location": null,
		"primary_location": {"pdf_url": "https://publisher.example/p.pdf"}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://publisher.example/p.pdf" {
		t.Errorf("PdfURL = %q, want primary_location.pdf_url", r.PdfURL)
	}
}

func TestOpenAlex_NoOALocation(t *testing.T) {
	srv := jsonServer(t, `{"best_oa_location": null, "primary_location": null}`)
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "no OA location") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestOpenAlex_NoLandingWhenNotOA(t *testing.T) {
	// is_oa=false → must NOT return landing_page_url even though present.
	// Documents current behavior; if we relax it, this test should change.
	srv := jsonServer(t, `{
		"best_oa_location": {"landing_page_url": "https://publisher.example/", "is_oa": false}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty when is_oa=false and only landing present", r.PdfURL)
	}
}

func TestOpenAlex_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c, _ := testSetup(http.NotFoundHandler())
	o := &OpenAlex{baseURL: srv.URL}

	r := o.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty on 500", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "OpenAlex lookup failed") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

// ─── Crossref ────────────────────────────────────────────────────────────

func TestCrossref_PicksPDFContentType(t *testing.T) {
	srv := jsonServer(t, `{
		"message": {
			"link": [
				{"URL": "https://publisher.example/xml", "content-type": "application/xml"},
				{"URL": "https://publisher.example/pdf", "content-type": "application/pdf"}
			]
		}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	cr := &Crossref{baseURL: srv.URL}

	r := cr.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://publisher.example/pdf" {
		t.Errorf("PdfURL = %q, want pdf link", r.PdfURL)
	}
}

func TestCrossref_NoPDFLink(t *testing.T) {
	srv := jsonServer(t, `{
		"message": {"link": [{"URL": "https://x/y.xml", "content-type": "application/xml"}]}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	cr := &Crossref{baseURL: srv.URL}

	r := cr.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "no PDF link") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestCrossref_EmptyMessage(t *testing.T) {
	srv := jsonServer(t, `{"message": {}}`)
	c, _ := testSetup(http.NotFoundHandler())
	cr := &Crossref{baseURL: srv.URL}

	r := cr.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

// ─── Unpaywall ───────────────────────────────────────────────────────────

func TestUnpaywall_BestOA_URLForPdf(t *testing.T) {
	srv := jsonServer(t, `{
		"best_oa_location": {"url_for_pdf": "https://oa.example/p.pdf", "url": "https://oa.example/landing"}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	u := &Unpaywall{baseURL: srv.URL}

	r := u.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x", Email: "real@univ.edu"})
	if r.PdfURL != "https://oa.example/p.pdf" {
		t.Errorf("PdfURL = %q, want url_for_pdf", r.PdfURL)
	}
}

func TestUnpaywall_BestOA_URLFallback(t *testing.T) {
	// url_for_pdf empty → fall through to url.
	srv := jsonServer(t, `{
		"best_oa_location": {"url": "https://oa.example/landing"}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	u := &Unpaywall{baseURL: srv.URL}

	r := u.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x", Email: "real@univ.edu"})
	if r.PdfURL != "https://oa.example/landing" {
		t.Errorf("PdfURL = %q, want url fallback", r.PdfURL)
	}
}

func TestUnpaywall_AltOALocationsFallback(t *testing.T) {
	srv := jsonServer(t, `{
		"best_oa_location": {"url_for_pdf": "", "url": ""},
		"oa_locations": [
			{"url_for_pdf": "", "url": ""},
			{"url_for_pdf": "https://mirror.example/p.pdf"}
		]
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	u := &Unpaywall{baseURL: srv.URL}

	r := u.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x", Email: "real@univ.edu"})
	if r.PdfURL != "https://mirror.example/p.pdf" {
		t.Errorf("PdfURL = %q, want second oa_locations entry", r.PdfURL)
	}
}

func TestUnpaywall_NoOA(t *testing.T) {
	srv := jsonServer(t, `{"best_oa_location": null, "oa_locations": []}`)
	c, _ := testSetup(http.NotFoundHandler())
	u := &Unpaywall{baseURL: srv.URL}

	r := u.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x", Email: "real@univ.edu"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "no OA") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

// ─── Semantic Scholar by DOI ─────────────────────────────────────────────

func TestS2ByDOI_OpenAccess(t *testing.T) {
	srv := jsonServer(t, `{
		"openAccessPdf": {"url": "https://s2.example/p.pdf", "status": "GREEN"}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByDOI{baseURL: srv.URL}

	r := s.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "https://s2.example/p.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestS2ByDOI_ClosedStatusRejected(t *testing.T) {
	srv := jsonServer(t, `{
		"openAccessPdf": {"url": "https://x/p.pdf", "status": "CLOSED"}
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByDOI{baseURL: srv.URL}

	r := s.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty for CLOSED status", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "CLOSED") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestS2ByDOI_NullPdf(t *testing.T) {
	srv := jsonServer(t, `{"openAccessPdf": null}`)
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByDOI{baseURL: srv.URL}

	r := s.Resolve(context.Background(), c, download.PaperInfo{DOI: "10.1/x"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

// ─── Semantic Scholar by title ───────────────────────────────────────────

func TestS2ByTitle_Match(t *testing.T) {
	srv := jsonServer(t, `{
		"data": [{"openAccessPdf": {"url": "https://s2.example/p.pdf"}}]
	}`)
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByTitle{baseURL: srv.URL}

	r := s.Resolve(context.Background(), c, download.PaperInfo{Title: "Some Paper Title"})
	if r.PdfURL != "https://s2.example/p.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestS2ByTitle_EmptyData(t *testing.T) {
	srv := jsonServer(t, `{"data": []}`)
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByTitle{baseURL: srv.URL}

	r := s.Resolve(context.Background(), c, download.PaperInfo{Title: "Some Paper"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
	if !strings.Contains(r.Reason, "not found by title") {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestS2ByTitle_TruncatesLongTitle(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	c, _ := testSetup(http.NotFoundHandler())
	s := &S2ByTitle{baseURL: srv.URL}

	longTitle := strings.Repeat("a", 500)
	s.Resolve(context.Background(), c, download.PaperInfo{Title: longTitle})

	// Query is URL-encoded; expect at most 200 'a' chars (each becomes single byte "a").
	count := strings.Count(got, "a")
	if count > 200 {
		t.Errorf("title not truncated: got %d 'a' chars in query", count)
	}
}
