package search

import (
	"sort"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Cold Start in Recommender Systems", "cold start in recommender systems"},
		{"A Survey: Deep Learning!!!", "a survey deep learning"},
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"Привет, Мир!", "привет мир"},
		{"Mixed English & Русский Text", "mixed english русский text"},
		{"", ""},
	}
	for _, tt := range tests {
		got := NormalizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDedupe_ByDOI(t *testing.T) {
	records := []RawPaper{
		{
			Title: "Paper One", DOI: "10.1234/test", Abstract: "Short",
			PdfURL: "https://example.com/a.pdf", CitationCount: 10, Source: "crossref",
		},
		{
			Title: "Paper One (variant)", DOI: "10.1234/test", Abstract: "Longer abstract text here",
			PdfURL: "https://arxiv.org/pdf/123.pdf", CitationCount: 42, Source: "arxiv",
		},
	}

	out := Dedupe(records)
	if len(out) != 1 {
		t.Fatalf("got %d papers, want 1", len(out))
	}
	p := out[0]

	// Should keep max citations.
	if p.CitationCount != 42 {
		t.Errorf("CitationCount = %d, want 42", p.CitationCount)
	}
	// Should keep longest abstract.
	if p.Abstract != "Longer abstract text here" {
		t.Errorf("Abstract = %q", p.Abstract)
	}
	// arxiv has higher priority (0) than crossref (4), so PDF should be arxiv's.
	if p.PdfURL != "https://arxiv.org/pdf/123.pdf" {
		t.Errorf("PdfURL = %q, want arxiv URL", p.PdfURL)
	}
	// Sources should contain both.
	sources := strings.Split(p.Source, ",")
	sort.Strings(sources)
	if len(sources) != 2 {
		t.Errorf("sources = %v, want 2", sources)
	}
}

func TestDedupe_ByTitle(t *testing.T) {
	records := []RawPaper{
		{
			Title: "Deep Learning for NLP", Abstract: "About DL",
			CitationCount: 5, Source: "openalex",
		},
		{
			Title: "deep learning for nlp", Abstract: "About deep learning for NLP tasks",
			DOI: "10.5678/dl", CitationCount: 100, Source: "semantic_scholar",
		},
	}

	out := Dedupe(records)
	if len(out) != 1 {
		t.Fatalf("got %d papers, want 1", len(out))
	}
	p := out[0]

	// DOI should be filled from second record.
	if p.DOI != "10.5678/dl" {
		t.Errorf("DOI = %q, want 10.5678/dl", p.DOI)
	}
	if p.CitationCount != 100 {
		t.Errorf("CitationCount = %d, want 100", p.CitationCount)
	}
}

func TestDedupe_PdfPriority(t *testing.T) {
	records := []RawPaper{
		{Title: "Test", DOI: "10.1/x", PdfURL: "https://cr.com/x.pdf", Source: "crossref"},
		{Title: "Test", DOI: "10.1/x", PdfURL: "https://s2.com/x.pdf", Source: "semantic_scholar"},
		{Title: "Test", DOI: "10.1/x", PdfURL: "https://arxiv.org/pdf/x.pdf", Source: "arxiv"},
	}

	out := Dedupe(records)
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	// arxiv (prio 0) should win.
	if out[0].PdfURL != "https://arxiv.org/pdf/x.pdf" {
		t.Errorf("PdfURL = %q, want arxiv", out[0].PdfURL)
	}
}

func TestDedupe_SkipsEmptyTitle(t *testing.T) {
	records := []RawPaper{
		{Title: "", DOI: "10.1/empty", Source: "crossref"},
		{Title: "   ", DOI: "10.1/spaces", Source: "crossref"},
		{Title: "Real Paper", DOI: "10.1/real", Source: "crossref"},
	}

	out := Dedupe(records)
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if out[0].Title != "Real Paper" {
		t.Errorf("Title = %q", out[0].Title)
	}
}

func TestDedupe_PreservesOrder(t *testing.T) {
	records := []RawPaper{
		{Title: "First", DOI: "10.1/a", Source: "arxiv"},
		{Title: "Second", DOI: "10.1/b", Source: "arxiv"},
		{Title: "Third", DOI: "10.1/c", Source: "arxiv"},
	}

	out := Dedupe(records)
	if len(out) != 3 {
		t.Fatalf("got %d, want 3", len(out))
	}
	if out[0].Title != "First" || out[1].Title != "Second" || out[2].Title != "Third" {
		t.Errorf("order broken: %q, %q, %q", out[0].Title, out[1].Title, out[2].Title)
	}
}

func TestDedupe_FillsMissingFields(t *testing.T) {
	y := intPtr(2023)
	records := []RawPaper{
		{Title: "Paper", DOI: "10.1/x", Source: "crossref"},
		{Title: "Paper", DOI: "10.1/x", ArxivID: "2301.12345", Venue: "NeurIPS", Year: y, Source: "arxiv"},
	}

	out := Dedupe(records)
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	p := out[0]
	if p.ArxivID != "2301.12345" {
		t.Errorf("ArxivID = %q", p.ArxivID)
	}
	if p.Venue != "NeurIPS" {
		t.Errorf("Venue = %q", p.Venue)
	}
	if p.Year == nil || *p.Year != 2023 {
		t.Errorf("Year = %v", p.Year)
	}
}
