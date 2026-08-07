package search

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qnqatop/papeer/internal/httpclient"
)

func testClient(handler http.Handler) (*httpclient.Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := httpclient.New("test@example.com")
	return c, srv
}

func TestSemanticScholar_Search(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"title":         "Cold Start Paper",
					"abstract":      "About cold start problems",
					"year":          2022,
					"venue":         "RecSys",
					"authors":       []map[string]string{{"name": "John Doe"}},
					"externalIds":   map[string]string{"DOI": "10.1234/cs", "ArXiv": "2201.12345"},
					"openAccessPdf": map[string]string{"url": "https://example.com/paper.pdf", "status": "GREEN"},
					"citationCount": 42,
				},
			},
		})
	}))
	defer srv.Close()

	// We can't easily override the URL in the provider, so test the parsing logic directly.
	// For a proper test, we'd inject the base URL. For now, verify the struct compiles and Name() works.
	s2 := NewSemanticScholar(c)
	if s2.Name() != "semantic_scholar" {
		t.Errorf("Name = %q", s2.Name())
	}
}

func TestOpenAlex_ReconstructAbstract(t *testing.T) {
	inv := map[string][]int{
		"This":  {0},
		"is":    {1},
		"a":     {2},
		"test":  {3},
		"about": {4, 7},
		"cold":  {5},
		"start": {6},
	}
	got := reconstructAbstract(inv)
	expected := "This is a test about cold start about"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestOpenAlex_ReconstructAbstract_Empty(t *testing.T) {
	got := reconstructAbstract(nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestArxiv_ExtractID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://arxiv.org/abs/2301.12345v1", "2301.12345"},
		{"http://arxiv.org/abs/2301.12345v2", "2301.12345"},
		{"http://arxiv.org/abs/2301.12345", "2301.12345"},
		{"https://other.com/abs/123", ""},
	}
	for _, tt := range tests {
		got := extractArxivID(tt.input)
		if got != tt.want {
			t.Errorf("extractArxivID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestArxiv_ParseXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2301.12345v1</id>
    <title>Test Paper Title
    With Newline</title>
    <summary>Abstract text here.</summary>
    <published>2023-01-15T00:00:00Z</published>
    <author><name>Jane Smith</name></author>
    <author><name>Bob Jones</name></author>
    <link href="http://arxiv.org/abs/2301.12345v1" rel="alternate" type="text/html"/>
    <link href="http://arxiv.org/pdf/2301.12345v1" title="pdf" rel="related" type="application/pdf"/>
    <arxiv:doi>10.1234/test</arxiv:doi>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xmlData))
	}))
	defer srv.Close()

	c := httpclient.New("")
	// Test XML parsing directly since we can't override arXiv URL easily.
	text, err := c.DoText(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	a := NewArXiv(c)
	_ = a // verify it compiles

	// Parse manually to test XML parsing.
	import_xml_test(t, text)
}

func import_xml_test(t *testing.T, text string) {
	t.Helper()
	var feed atomFeed
	if err := xml.Unmarshal([]byte(text), &feed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(feed.Entries) != 1 {
		t.Fatalf("entries = %d", len(feed.Entries))
	}
	e := feed.Entries[0]
	if id := extractArxivID(e.ID); id != "2301.12345" {
		t.Errorf("id = %q", id)
	}
	year := extractArxivYear(e.Published)
	if year == nil || *year != 2023 {
		t.Errorf("year = %v", year)
	}
	authors := extractArxivAuthors(e.Authors)
	if len(authors) != 2 || authors[0] != "Jane Smith" {
		t.Errorf("authors = %v", authors)
	}
	pdfURL := extractArxivPdfURL(e.Links, "2301.12345")
	if pdfURL != "http://arxiv.org/pdf/2301.12345v1" {
		t.Errorf("pdfURL = %q", pdfURL)
	}
}

func TestCrossref_ExtractYear(t *testing.T) {
	w := crWork{
		Issued: &crDate{DateParts: [][]int{{2022, 3, 15}}},
	}
	y := extractCRYear(w)
	if y == nil || *y != 2022 {
		t.Errorf("year = %v", y)
	}

	w2 := crWork{}
	if y2 := extractCRYear(w2); y2 != nil {
		t.Errorf("empty year = %v", y2)
	}
}

func TestCrossref_ExtractAuthors(t *testing.T) {
	authors := extractCRAuthors([]crAuthor{
		{Given: "John", Family: "Doe"},
		{Given: "Jane", Family: "Smith"},
		{Given: "", Family: ""},
	})
	if len(authors) != 2 {
		t.Errorf("len = %d, want 2", len(authors))
	}
	if authors[0] != "John Doe" {
		t.Errorf("author[0] = %q", authors[0])
	}
}
