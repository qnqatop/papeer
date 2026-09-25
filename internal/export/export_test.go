package export

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

func testPapers() []db.Paper {
	return []db.Paper{
		{
			ID:            1,
			Title:         "Deep Learning for Recommendations",
			Abstract:      "We propose a novel approach.",
			Year:          ptr.Ptr(2021),
			Venue:         "RecSys",
			Authors:       db.JSONStringSlice{"Alice Smith", "Bob Jones"},
			DOI:           ptr.Ptr("10.1234/test1"),
			PdfURL:        ptr.Ptr("https://example.com/paper1.pdf"),
			CitationCount: 42,
			PreScore:      8,
			Status:        "approved",
			Sources:       db.JSONStringSlice{"scholar", "semantic_scholar"},
		},
		{
			ID:       2,
			Title:    "Cold Start Solutions",
			Abstract: "Addressing the cold start problem.",
			Year:     ptr.Ptr(2023),
			Venue:    "SIGIR",
			Authors:  db.JSONStringSlice{"Carol White"},
			ArxivID:  ptr.Ptr("2301.00001"),
			PreScore: 5,
			Status:   "new",
			Sources:  db.JSONStringSlice{"arxiv"},
		},
	}
}

func TestExportBibTeX(t *testing.T) {
	var buf bytes.Buffer
	papers := testPapers()

	if err := ExportBibTeX(papers, &buf); err != nil {
		t.Fatalf("ExportBibTeX: %v", err)
	}

	out := buf.String()

	// Check both entries present.
	if !strings.Contains(out, "@article{") {
		t.Error("expected @article entries")
	}
	if !strings.Contains(out, "Deep Learning for Recommendations") {
		t.Error("expected first paper title")
	}
	if !strings.Contains(out, "Alice Smith and Bob Jones") {
		t.Error("expected first paper authors")
	}
	if !strings.Contains(out, "Cold Start Solutions") {
		t.Error("expected second paper title")
	}
	if !strings.Contains(out, "2021") {
		t.Error("expected year 2021")
	}
	if !strings.Contains(out, "10.1234/test1") {
		t.Error("expected DOI")
	}
	// Second paper has no DOI/url — should not appear.
	if strings.Contains(out, "doi = {}") {
		t.Error("empty doi field should be skipped")
	}
}

func TestExportCSV(t *testing.T) {
	var buf bytes.Buffer
	papers := testPapers()

	if err := ExportCSV(papers, &buf); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll: %v", err)
	}

	if len(records) != 3 { // header + 2 rows
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	header := records[0]
	if header[0] != "title" || header[1] != "authors" {
		t.Errorf("unexpected header: %v", header)
	}

	row1 := records[1]
	if row1[0] != "Deep Learning for Recommendations" {
		t.Errorf("unexpected title: %s", row1[0])
	}
	if row1[1] != "Alice Smith; Bob Jones" {
		t.Errorf("unexpected authors: %s", row1[1])
	}
	if row1[2] != "2021" {
		t.Errorf("unexpected year: %s", row1[2])
	}

	row2 := records[2]
	if row2[5] != "2301.00001" {
		t.Errorf("unexpected arxiv_id: %s", row2[5])
	}
}

func TestImportAxesFromYAML(t *testing.T) {
	yamlData := `
axes:
  cold_start:
    description: "Cold start problem in recommender systems"
    year_min: 2018
    max_per_query: 25
    queries:
      - "cold start recommender system"
      - "new user cold start"
    keywords_must:
      - "cold start"
    keywords_boost:
      - "recommender"
      - "collaborative filtering"
  deep_learning:
    description: "Deep learning approaches"
    year_min: 2020
    max_per_query: 30
    queries:
      - "deep learning recommendation"
    keywords_must:
      - "neural"
`

	axes, err := ImportAxesFromYAML(strings.NewReader(yamlData), 42)
	if err != nil {
		t.Fatalf("ImportAxesFromYAML: %v", err)
	}

	if len(axes) != 2 {
		t.Fatalf("expected 2 axes, got %d", len(axes))
	}

	// Find cold_start axis.
	var cs *db.Axis
	for i := range axes {
		if axes[i].AxisKey == "cold_start" {
			cs = &axes[i]
			break
		}
	}
	if cs == nil {
		t.Fatal("cold_start axis not found")
	}

	if cs.ProfileID != 42 {
		t.Errorf("expected profile_id 42, got %d", cs.ProfileID)
	}
	if cs.Description != "Cold start problem in recommender systems" {
		t.Errorf("unexpected description: %s", cs.Description)
	}
	if ptr.Val(cs.YearMin) != 2018 {
		t.Errorf("expected year_min 2018, got %d", ptr.Val(cs.YearMin))
	}
	if ptr.Val(cs.MaxPerQuery) != 25 {
		t.Errorf("expected max_per_query 25, got %d", ptr.Val(cs.MaxPerQuery))
	}
	if len(cs.Queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(cs.Queries))
	}
	if cs.Queries[0].Text != "cold start recommender system" {
		t.Errorf("unexpected query: %s", cs.Queries[0].Text)
	}

	mustCount := 0
	boostCount := 0
	for _, kw := range cs.Keywords {
		switch kw.Type {
		case "must":
			mustCount++
		case "boost":
			boostCount++
		}
	}
	if mustCount != 1 {
		t.Errorf("expected 1 must keyword, got %d", mustCount)
	}
	if boostCount != 2 {
		t.Errorf("expected 2 boost keywords, got %d", boostCount)
	}
}

func TestImportTopicsFromYAML(t *testing.T) {
	yamlData := `
topics:
  nlp_basics:
    description: "Fundamental NLP techniques"
    year_min: 2019
    max_per_query: 20
    queries:
      - "natural language processing survey"
      - "transformer attention mechanism"
    keywords_must:
      - "NLP"
    keywords_boost:
      - "transformer"
      - "BERT"
`

	axes, err := ImportAxesFromYAML(strings.NewReader(yamlData), 99)
	if err != nil {
		t.Fatalf("ImportAxesFromYAML (topics key): %v", err)
	}

	if len(axes) != 1 {
		t.Fatalf("expected 1 axis, got %d", len(axes))
	}

	a := axes[0]
	if a.AxisKey != "nlp_basics" {
		t.Errorf("expected axis_key 'nlp_basics', got '%s'", a.AxisKey)
	}
	if a.ProfileID != 99 {
		t.Errorf("expected profile_id 99, got %d", a.ProfileID)
	}
	if a.Description != "Fundamental NLP techniques" {
		t.Errorf("unexpected description: %s", a.Description)
	}
	if ptr.Val(a.YearMin) != 2019 {
		t.Errorf("expected year_min 2019, got %d", ptr.Val(a.YearMin))
	}
	if ptr.Val(a.MaxPerQuery) != 20 {
		t.Errorf("expected max_per_query 20, got %d", ptr.Val(a.MaxPerQuery))
	}
	if len(a.Queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(a.Queries))
	}
	if a.Queries[0].Text != "natural language processing survey" {
		t.Errorf("unexpected query: %s", a.Queries[0].Text)
	}

	mustCount := 0
	boostCount := 0
	for _, kw := range a.Keywords {
		switch kw.Type {
		case "must":
			mustCount++
			if kw.Word != "NLP" {
				t.Errorf("unexpected must keyword: %s", kw.Word)
			}
		case "boost":
			boostCount++
		}
	}
	if mustCount != 1 {
		t.Errorf("expected 1 must keyword, got %d", mustCount)
	}
	if boostCount != 2 {
		t.Errorf("expected 2 boost keywords, got %d", boostCount)
	}
}

func TestImportAxesAndTopicsMerged(t *testing.T) {
	// When both keys present, axes take priority over topics for same key,
	// and distinct keys are merged.
	yamlData := `
axes:
  shared_topic:
    description: "From axes key"
    queries:
      - "axes query"
topics:
  shared_topic:
    description: "From topics key (should be ignored)"
    queries:
      - "topics query"
  extra_topic:
    description: "Only in topics"
    queries:
      - "extra query"
`

	axes, err := ImportAxesFromYAML(strings.NewReader(yamlData), 1)
	if err != nil {
		t.Fatalf("ImportAxesFromYAML (merged): %v", err)
	}

	if len(axes) != 2 {
		t.Fatalf("expected 2 axes, got %d", len(axes))
	}

	// Find shared_topic — should use "axes" version.
	var shared *db.Axis
	var extra *db.Axis
	for i := range axes {
		switch axes[i].AxisKey {
		case "shared_topic":
			shared = &axes[i]
		case "extra_topic":
			extra = &axes[i]
		}
	}
	if shared == nil {
		t.Fatal("shared_topic not found")
	}
	if extra == nil {
		t.Fatal("extra_topic not found")
	}

	// Shared must have the "axes" description, not "topics".
	if shared.Description != "From axes key" {
		t.Errorf("expected axes description to win, got '%s'", shared.Description)
	}
	if len(shared.Queries) != 1 || shared.Queries[0].Text != "axes query" {
		t.Errorf("expected axes query to win, got %v", shared.Queries)
	}

	// Extra must come from topics.
	if extra.Description != "Only in topics" {
		t.Errorf("expected topics description for extra, got '%s'", extra.Description)
	}
	if len(extra.Queries) != 1 || extra.Queries[0].Text != "extra query" {
		t.Errorf("expected topics query for extra, got %v", extra.Queries)
	}
}

func TestExportAndReimportTopics(t *testing.T) {
	original := []db.Axis{
		{
			AxisKey:     "test_topic",
			Description: "Round-trip test",
			YearMin:     ptr.Ptr(2020),
			MaxPerQuery: ptr.Ptr(15),
			Queries:     []db.Query{{Text: "query one"}, {Text: "query two"}},
			Keywords: []db.Keyword{
				{Word: "must1", Type: "must"},
				{Word: "boost1", Type: "boost"},
			},
		},
	}

	// Export to YAML.
	data, err := ExportAxesToYAML(original)
	if err != nil {
		t.Fatalf("ExportAxesToYAML: %v", err)
	}

	// Must contain "topics" key (new format), not "axes".
	if !strings.Contains(string(data), "topics:") {
		t.Error("exported YAML must contain 'topics:' key")
	}
	if strings.Contains(string(data), "axes:") {
		t.Error("exported YAML must NOT contain legacy 'axes:' key when exporting via new code path")
	}

	// Re-import.
	imported, err := ImportAxesFromYAML(bytes.NewReader(data), 7)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}

	if len(imported) != 1 {
		t.Fatalf("expected 1 axis after re-import, got %d", len(imported))
	}

	a := imported[0]
	if a.AxisKey != "test_topic" {
		t.Errorf("AxisKey: expected 'test_topic', got '%s'", a.AxisKey)
	}
	if a.Description != "Round-trip test" {
		t.Errorf("Description mismatch: '%s'", a.Description)
	}
	if ptr.Val(a.YearMin) != 2020 {
		t.Errorf("YearMin mismatch")
	}
	if ptr.Val(a.MaxPerQuery) != 15 {
		t.Errorf("MaxPerQuery mismatch")
	}
	if len(a.Queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(a.Queries))
	}
	if a.Queries[0].Text != "query one" || a.Queries[1].Text != "query two" {
		t.Errorf("query texts mismatch")
	}
	if len(a.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d", len(a.Keywords))
	}
}

func TestImportYAML_LangScope(t *testing.T) {
	yamlData := `
topics:
  ru_topic:
    description: "Russian-scoped topic"
    lang_scope: ru
    queries:
      - "рекомендательная система"
  en_topic:
    description: "Default English topic"
    queries:
      - "recommender system"
`
	axes, err := ImportAxesFromYAML(strings.NewReader(yamlData), 1)
	if err != nil {
		t.Fatalf("ImportAxesFromYAML: %v", err)
	}

	byKey := map[string]db.Axis{}
	for _, a := range axes {
		byKey[a.AxisKey] = a
	}
	if got := byKey["ru_topic"].LangScope; got != "ru" {
		t.Errorf("ru_topic LangScope = %q, want ru", got)
	}
	// en_topic omits lang_scope → the English default.
	if got := byKey["en_topic"].LangScope; got != "en" {
		t.Errorf("en_topic LangScope = %q, want en", got)
	}
}

func TestImportYAML_LangScope_NormalizedAndValidated(t *testing.T) {
	axes, err := ImportAxesFromYAML(strings.NewReader(`
topics:
  upper:
    lang_scope: RU
    queries: ["q"]
`), 1)
	if err != nil {
		t.Fatalf("ImportAxesFromYAML: %v", err)
	}
	if len(axes) != 1 || axes[0].LangScope != "ru" {
		t.Fatalf("axes = %+v, want one axis with LangScope ru", axes)
	}

	_, err = ImportAxesFromYAML(strings.NewReader(`
topics:
  french:
    lang_scope: fr
    queries: ["q"]
`), 1)
	if err == nil || !strings.Contains(err.Error(), "french") {
		t.Errorf("err = %v, want an invalid lang_scope error naming the axis", err)
	}
}

func TestExportYAML_LangScope_OnlyEmittedForRu(t *testing.T) {
	data, err := ExportAxesToYAML([]db.Axis{
		{AxisKey: "ru_topic", LangScope: "ru", Queries: []db.Query{{Text: "q"}}},
		{AxisKey: "en_topic", LangScope: "en", Queries: []db.Query{{Text: "q"}}},
	})
	if err != nil {
		t.Fatalf("ExportAxesToYAML: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "lang_scope: ru") {
		t.Errorf("expected 'lang_scope: ru' in export, got:\n%s", out)
	}
	if strings.Contains(out, "lang_scope: en") {
		t.Errorf("did not expect 'lang_scope: en' (default omitted), got:\n%s", out)
	}
}
