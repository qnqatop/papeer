package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
	"github.com/qnqatop/papeer/internal/search"
)

// ─── truncateTitle ────────────────────────────────────────────────────────

func TestTruncateTitle(t *testing.T) {
	cases := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"}, // boundary, returns as-is
		{"this is way too long", 10, "this is..."},
		{"abcdef", 5, "ab..."},
	}
	for _, tc := range cases {
		got := truncateTitle(tc.in, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncateTitle(%q, %d) = %q, want %q", tc.in, tc.maxLen, got, tc.want)
		}
	}
}

// ─── seedPaper inserts a minimal paper and returns its ID. ───────────────

func seedPaper(t *testing.T, a *App, profileID int64, title, doi string, status string) int64 {
	t.Helper()
	p := &db.Paper{
		ProfileID:       profileID,
		Title:           title,
		TitleNormalized: search.NormalizeTitle(title),
		Status:          status,
	}
	if doi != "" {
		p.DOI = ptr.Ptr(doi)
	}
	if err := a.db.UpsertPaper(p); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	return p.ID
}

// ─── FetchCitations boundary cases ───────────────────────────────────────

func TestFetchCitations_NoPapers(t *testing.T) {
	a := newTestApp(t)
	p := createValidProfile(t, a, "P", "real@univ.edu")

	err := a.FetchCitations(p.ID)
	if err == nil || !strings.Contains(err.Error(), "no approved or downloaded papers") {
		t.Errorf("err = %v, want 'no approved or downloaded papers'", err)
	}
}

func TestFetchCitations_GuardsInvalidEmailBeforeDB(t *testing.T) {
	a := newTestApp(t)
	p := insertProfileRaw(t, a, "Legacy", "")

	// Even though there are no papers, guard fires first → ErrInvalidEmail, not "no approved".
	err := a.FetchCitations(p.ID)
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("err = %v, want ErrInvalidEmail", err)
	}
}

// ─── processCitationEntries — internal link via DOI ──────────────────────

func TestProcessCitationEntries_MatchesByDOI(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source Paper", "10.1/source", "approved")
	tgt := seedPaper(t, a, prof.ID, "Target Paper", "10.2/target", "approved")

	doiMap := map[string]int64{"10.1/source": src, "10.2/target": tgt}
	titleMap := map[string]int64{
		search.NormalizeTitle("Source Paper"): src,
		search.NormalizeTitle("Target Paper"): tgt,
	}

	entries := []search.S2CitationEntry{
		search.NewS2CitationEntryWithDOI("Target Paper", "10.2/TARGET"), // upper-case DOI → matched via lowercase lookup
	}
	a.processCitationEntries(prof.ID, src, entries, titleMap, doiMap, true)

	links, err := a.db.GetCitationLinks(prof.ID)
	if err != nil {
		t.Fatalf("GetCitationLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	if links[0].FromPaperID != src || links[0].ToPaperID != tgt {
		t.Errorf("link = %+v, want %d→%d", links[0], src, tgt)
	}
}

// ─── processCitationEntries — internal link via title fallback ───────────

func TestProcessCitationEntries_MatchesByTitleFallback(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source", "", "approved")
	tgt := seedPaper(t, a, prof.ID, "Foo Bar Baz", "", "approved")

	doiMap := map[string]int64{}
	titleMap := map[string]int64{
		search.NormalizeTitle("Source"):      src,
		search.NormalizeTitle("Foo Bar Baz"): tgt,
	}

	entries := []search.S2CitationEntry{
		{Title: "FOO! BAR. BAZ"}, // normalises to same key
	}
	a.processCitationEntries(prof.ID, src, entries, titleMap, doiMap, true)

	links, _ := a.db.GetCitationLinks(prof.ID)
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	if links[0].ToPaperID != tgt {
		t.Errorf("ToPaperID = %d, want %d", links[0].ToPaperID, tgt)
	}
}

// ─── processCitationEntries — citedBy direction inverts source/target ────

func TestProcessCitationEntries_CitedByInvertsDirection(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source", "10.1/source", "approved")
	tgt := seedPaper(t, a, prof.ID, "Target", "10.2/target", "approved")

	doiMap := map[string]int64{"10.1/source": src, "10.2/target": tgt}
	titleMap := map[string]int64{}

	entries := []search.S2CitationEntry{
		search.NewS2CitationEntryWithDOI("Target", "10.2/target"),
	}
	// isReference=false → matchedPaper cites source ⇒ edge target→source.
	a.processCitationEntries(prof.ID, src, entries, titleMap, doiMap, false)

	links, _ := a.db.GetCitationLinks(prof.ID)
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	if links[0].FromPaperID != tgt || links[0].ToPaperID != src {
		t.Errorf("link = %+v, want %d→%d (citedBy inverted)", links[0], tgt, src)
	}
}

// ─── processCitationEntries — unmatched entries become external citations ─

func TestProcessCitationEntries_UnmatchedToExternal(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source", "10.1/source", "approved")

	titleMap := map[string]int64{search.NormalizeTitle("Source"): src}
	doiMap := map[string]int64{"10.1/source": src}

	entries := []search.S2CitationEntry{
		{S2PaperID: "abc", Title: "External 1", Year: ptr.Ptr(2020)},
		{S2PaperID: "def", Title: "External 2"},
		{S2PaperID: "abc", Title: "External 1"}, // duplicate → mention_count increment
	}
	a.processCitationEntries(prof.ID, src, entries, titleMap, doiMap, true)

	exts, err := a.db.GetExternalCitations(prof.ID, 1, 100)
	if err != nil {
		t.Fatalf("GetExternalCitations: %v", err)
	}
	if len(exts) != 2 {
		t.Fatalf("external count = %d, want 2 (dedupe by s2 id)", len(exts))
	}

	// External 1 should have mention_count == 2 since it appeared twice.
	for _, ec := range exts {
		if ec.S2PaperID == "abc" && ec.MentionCount != 2 {
			t.Errorf("External 1 mention_count = %d, want 2", ec.MentionCount)
		}
	}
}

// ─── processCitationEntries — self-references skipped ────────────────────

func TestProcessCitationEntries_SkipsSelfReference(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source", "10.1/source", "approved")

	doiMap := map[string]int64{"10.1/source": src}
	titleMap := map[string]int64{search.NormalizeTitle("Source"): src}

	entries := []search.S2CitationEntry{
		search.NewS2CitationEntryWithDOI("Source", "10.1/source"), // points at itself
	}
	a.processCitationEntries(prof.ID, src, entries, titleMap, doiMap, true)

	links, _ := a.db.GetCitationLinks(prof.ID)
	if len(links) != 0 {
		t.Errorf("expected 0 links (self-ref skipped), got %d", len(links))
	}
	exts, _ := a.db.GetExternalCitations(prof.ID, 1, 100)
	if len(exts) != 0 {
		t.Errorf("expected 0 externals (matched to self, not external), got %d", len(exts))
	}
}

// ─── GetCitationGraph — assembles internal + external nodes ──────────────

func TestGetCitationGraph_BuildsNodesAndEdges(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "Source", "10.1/source", "approved")
	tgt := seedPaper(t, a, prof.ID, "Target", "10.2/target", "approved")

	// One internal link + one external citation.
	if err := a.db.UpsertCitationLink(prof.ID, src, tgt); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	if err := a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{
		S2PaperID: "ext1", Title: "External Paper",
	}); err != nil {
		t.Fatalf("UpsertExternalCitation: %v", err)
	}

	graph, err := a.GetCitationGraph(prof.ID, 0)
	if err != nil {
		t.Fatalf("GetCitationGraph: %v", err)
	}

	// 2 internal nodes (src, tgt) + 1 external = 3 nodes.
	if len(graph.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(graph.Edges))
	}

	// Edges use p_<id> format.
	expectedEdge := fmt.Sprintf("p_%d→p_%d", src, tgt)
	got := graph.Edges[0].Source + "→" + graph.Edges[0].Target
	if got != expectedEdge {
		t.Errorf("edge = %q, want %q", got, expectedEdge)
	}

	// Each node has correct NodeType.
	internal, external := 0, 0
	for _, n := range graph.Nodes {
		switch n.NodeType {
		case "internal":
			internal++
		case "external":
			external++
		}
	}
	if internal != 2 || external != 1 {
		t.Errorf("nodes: internal=%d external=%d, want 2/1", internal, external)
	}
}

// ─── GetCitationGraph — minMentions filter on externals ──────────────────

func TestGetCitationGraph_FiltersExternalsByMinMentions(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// One external with mention_count=1, another with mention_count=2.
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "low", Title: "Low"})
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "high", Title: "High"})
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "high", Title: "High"}) // bump to 2

	graph, err := a.GetCitationGraph(prof.ID, 2)
	if err != nil {
		t.Fatalf("GetCitationGraph: %v", err)
	}
	// Only "high" should pass minMentions=2.
	highSeen := false
	for _, n := range graph.Nodes {
		if n.Title == "High" {
			highSeen = true
		}
		if n.Title == "Low" {
			t.Errorf("Low external leaked through minMentions=2 filter")
		}
	}
	if !highSeen {
		t.Errorf("High external missing from graph")
	}
}

// ─── GetMissingKeyPapers ─────────────────────────────────────────────────

func TestGetMissingKeyPapers_DefaultsLimit(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")

	// Insert externals with varying mention counts.
	for i := 0; i < 5; i++ {
		a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{
			S2PaperID: fmt.Sprintf("ext%d", i), Title: fmt.Sprintf("E%d", i),
		})
		// Bump high-mention ones twice.
		if i < 3 {
			a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{
				S2PaperID: fmt.Sprintf("ext%d", i), Title: fmt.Sprintf("E%d", i),
			})
		}
	}

	// Default limit when 0.
	got, err := a.GetMissingKeyPapers(prof.ID, 0)
	if err != nil {
		t.Fatalf("GetMissingKeyPapers: %v", err)
	}
	// Only those with mention_count ≥ 2 returned. Only the first 3 qualify.
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

// ─── ClearCitationData ───────────────────────────────────────────────────

func TestClearCitationData(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	src := seedPaper(t, a, prof.ID, "S", "10.1/s", "approved")
	tgt := seedPaper(t, a, prof.ID, "T", "10.2/t", "approved")

	a.db.UpsertCitationLink(prof.ID, src, tgt)
	a.db.UpsertExternalCitation(prof.ID, &db.ExternalCitation{S2PaperID: "x", Title: "X"})

	if err := a.ClearCitationData(prof.ID); err != nil {
		t.Fatalf("ClearCitationData: %v", err)
	}
	links, _ := a.db.GetCitationLinks(prof.ID)
	exts, _ := a.db.GetExternalCitations(prof.ID, 0, 100)
	if len(links) != 0 || len(exts) != 0 {
		t.Errorf("after clear: links=%d, externals=%d, both should be 0", len(links), len(exts))
	}
}
