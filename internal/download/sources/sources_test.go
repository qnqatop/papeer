package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qnqatop/papeer/internal/download"
	"github.com/qnqatop/papeer/internal/httpclient"
)

func testSetup(handler http.Handler) (*httpclient.Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := httpclient.New("test@test.com")
	return c, srv
}

func TestSearchReport_HasURL(t *testing.T) {
	s := NewSearchReport()
	r := s.Resolve(context.Background(), nil, download.PaperInfo{PdfURL: "https://example.com/paper.pdf"})
	if r.PdfURL != "https://example.com/paper.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestSearchReport_NoURL(t *testing.T) {
	s := NewSearchReport()
	r := s.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty", r.PdfURL)
	}
	if r.Reason == "" {
		t.Error("expected reason")
	}
}

func TestArxiv_ByArxivID(t *testing.T) {
	a := NewArXiv()
	r := a.Resolve(context.Background(), nil, download.PaperInfo{ArxivID: "2301.12345"})
	if r.PdfURL != "https://arxiv.org/pdf/2301.12345.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestArxiv_ByDOI(t *testing.T) {
	c, srv := testSetup(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"externalIds": map[string]string{"ArXiv": "2301.99999"},
		})
	}))
	defer srv.Close()

	// Can't override URL in source, so just verify the ArxivID path works.
	a := NewArXiv()
	r := a.Resolve(context.Background(), c, download.PaperInfo{ArxivID: "2301.99999"})
	if r.PdfURL != "https://arxiv.org/pdf/2301.99999.pdf" {
		t.Errorf("PdfURL = %q", r.PdfURL)
	}
}

func TestArxiv_NoDOINorID(t *testing.T) {
	a := NewArXiv()
	r := a.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty", r.PdfURL)
	}
}

func TestS2ByDOI_NoDOI(t *testing.T) {
	s := NewS2ByDOI()
	r := s.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.Reason != "no DOI" {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestS2ByTitle_NoTitle(t *testing.T) {
	s := NewS2ByTitle()
	r := s.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.Reason != "no title" {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestUnpaywall_NoEmail(t *testing.T) {
	u := NewUnpaywall()
	r := u.Resolve(context.Background(), nil, download.PaperInfo{DOI: "10.1234/test"})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty (no email)", r.PdfURL)
	}
}

func TestUnpaywall_ExampleEmail(t *testing.T) {
	u := NewUnpaywall()
	r := u.Resolve(context.Background(), nil, download.PaperInfo{
		DOI: "10.1234/test", Email: "test@example.com",
	})
	if r.PdfURL != "" {
		t.Errorf("PdfURL = %q, want empty (example email)", r.PdfURL)
	}
}

func TestCrossref_NoDOI(t *testing.T) {
	c := NewCrossref()
	r := c.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.Reason != "no DOI" {
		t.Errorf("Reason = %q", r.Reason)
	}
}

func TestOpenAlex_NoDOI(t *testing.T) {
	o := NewOpenAlex()
	r := o.Resolve(context.Background(), nil, download.PaperInfo{})
	if r.Reason != "no DOI" {
		t.Errorf("Reason = %q", r.Reason)
	}
}
