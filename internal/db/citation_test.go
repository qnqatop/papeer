package db

import (
	"testing"
	"time"

	"github.com/qnqatop/papeer/internal/ptr"
)

func seedTestPaper(t *testing.T, d *DB, profileID int64, title string) *Paper {
	t.Helper()
	p := &Paper{
		ProfileID:       profileID,
		Title:           title,
		TitleNormalized: title,
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	if err := d.UpsertPaper(p); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	return p
}

// ─── UpsertCitationLink ───────────────────────────────────────────────────

func TestUpsertCitationLink_InsertOrIgnore(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	src := seedTestPaper(t, d, p.ID, "Source")
	tgt := seedTestPaper(t, d, p.ID, "Target")

	if err := d.UpsertCitationLink(p.ID, src.ID, tgt.ID); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	// Duplicate insert should be silently ignored (UNIQUE constraint).
	if err := d.UpsertCitationLink(p.ID, src.ID, tgt.ID); err != nil {
		t.Fatalf("UpsertCitationLink (dup): %v", err)
	}

	links, err := d.GetCitationLinks(p.ID)
	if err != nil {
		t.Fatalf("GetCitationLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(links))
	}
}

// ─── UpsertExternalCitation ───────────────────────────────────────────────

func TestUpsertExternalCitation_IncrementsMentionCountAndMerges(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	ec := &ExternalCitation{S2PaperID: "abc", Title: "Short", CitationCount: 5}
	if err := d.UpsertExternalCitation(p.ID, ec); err != nil {
		t.Fatalf("UpsertExternalCitation: %v", err)
	}

	// Second mention: longer title wins, citation_count takes the max, and
	// mention_count increments.
	ec2 := &ExternalCitation{S2PaperID: "abc", Title: "A Much Longer Title", CitationCount: 3}
	if err := d.UpsertExternalCitation(p.ID, ec2); err != nil {
		t.Fatalf("UpsertExternalCitation (2nd): %v", err)
	}

	got, err := d.GetExternalCitations(p.ID, 0, 10)
	if err != nil {
		t.Fatalf("GetExternalCitations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].MentionCount != 2 {
		t.Errorf("MentionCount = %d, want 2", got[0].MentionCount)
	}
	if got[0].Title != "A Much Longer Title" {
		t.Errorf("Title = %q, want the longer title to win", got[0].Title)
	}
	if got[0].CitationCount != 5 {
		t.Errorf("CitationCount = %d, want max(5,3)=5", got[0].CitationCount)
	}
}

func TestUpsertExternalCitationGetID_ReturnsStableID(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	id1, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "x", Title: "X"})
	if err != nil {
		t.Fatalf("UpsertExternalCitationGetID: %v", err)
	}
	id2, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "x", Title: "X"})
	if err != nil {
		t.Fatalf("UpsertExternalCitationGetID (2nd): %v", err)
	}
	if id1 != id2 {
		t.Errorf("ids differ across upserts of the same s2_paper_id: %d vs %d", id1, id2)
	}
}

// ─── citation_mentions / GetCoverageGaps ──────────────────────────────────

func TestGetCoverageGaps_AnnotatesMentionedBy(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	paperA := seedTestPaper(t, d, p.ID, "Paper A")
	paperB := seedTestPaper(t, d, p.ID, "Paper B")

	extID, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "gap1", Title: "Missing Work"})
	if err != nil {
		t.Fatalf("UpsertExternalCitationGetID: %v", err)
	}
	// Bump mention_count to 2 (default minMentions filter in GetCoverageGaps).
	if _, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "gap1", Title: "Missing Work"}); err != nil {
		t.Fatalf("UpsertExternalCitationGetID (bump): %v", err)
	}

	if err := d.UpsertCitationMention(paperA.ID, extID); err != nil {
		t.Fatalf("UpsertCitationMention A: %v", err)
	}
	if err := d.UpsertCitationMention(paperB.ID, extID); err != nil {
		t.Fatalf("UpsertCitationMention B: %v", err)
	}
	// Idempotent: re-adding the same mention must not create a duplicate.
	if err := d.UpsertCitationMention(paperA.ID, extID); err != nil {
		t.Fatalf("UpsertCitationMention A (dup): %v", err)
	}

	gaps, err := d.GetCoverageGaps(p.ID, 2, 10)
	if err != nil {
		t.Fatalf("GetCoverageGaps: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("len(gaps) = %d, want 1", len(gaps))
	}
	if len(gaps[0].MentionedBy) != 2 {
		t.Fatalf("MentionedBy = %v, want 2 entries", gaps[0].MentionedBy)
	}
	seen := map[int64]bool{gaps[0].MentionedBy[0]: true, gaps[0].MentionedBy[1]: true}
	if !seen[paperA.ID] || !seen[paperB.ID] {
		t.Errorf("MentionedBy = %v, want [%d,%d]", gaps[0].MentionedBy, paperA.ID, paperB.ID)
	}
}

func TestGetCoverageGaps_FiltersByMinMentions(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	d.UpsertExternalCitation(p.ID, &ExternalCitation{S2PaperID: "low", Title: "Low"}) // mention_count=1
	d.UpsertExternalCitation(p.ID, &ExternalCitation{S2PaperID: "high", Title: "High"})
	d.UpsertExternalCitation(p.ID, &ExternalCitation{S2PaperID: "high", Title: "High"}) // mention_count=2

	gaps, err := d.GetCoverageGaps(p.ID, 2, 10)
	if err != nil {
		t.Fatalf("GetCoverageGaps: %v", err)
	}
	if len(gaps) != 1 || gaps[0].S2PaperID != "high" {
		t.Errorf("gaps = %+v, want only 'high'", gaps)
	}
}

func TestGetCoverageGaps_EmptyWhenNoExternals(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)

	gaps, err := d.GetCoverageGaps(p.ID, 2, 10)
	if err != nil {
		t.Fatalf("GetCoverageGaps: %v", err)
	}
	if gaps != nil {
		t.Errorf("gaps = %v, want nil", gaps)
	}
}

// ─── ClearCitationData ────────────────────────────────────────────────────

func TestClearCitationData_ClearsEverythingTransactionally(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	src := seedTestPaper(t, d, p.ID, "Source")
	tgt := seedTestPaper(t, d, p.ID, "Target")

	if err := d.UpsertCitationLink(p.ID, src.ID, tgt.ID); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	extID, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "ext", Title: "Ext"})
	if err != nil {
		t.Fatalf("UpsertExternalCitationGetID: %v", err)
	}
	if err := d.UpsertCitationMention(src.ID, extID); err != nil {
		t.Fatalf("UpsertCitationMention: %v", err)
	}
	now := time.Now()
	if err := d.SetLastCitationFetchAt(src.ID, now); err != nil {
		t.Fatalf("SetLastCitationFetchAt: %v", err)
	}

	if err := d.ClearCitationData(p.ID); err != nil {
		t.Fatalf("ClearCitationData: %v", err)
	}

	links, _ := d.GetCitationLinks(p.ID)
	if len(links) != 0 {
		t.Errorf("links after clear = %d, want 0", len(links))
	}
	exts, _ := d.GetExternalCitations(p.ID, 0, 10)
	if len(exts) != 0 {
		t.Errorf("externals after clear = %d, want 0", len(exts))
	}
	var mentionCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM citation_mentions WHERE paper_id=?`, src.ID).Scan(&mentionCount); err != nil {
		t.Fatal(err)
	}
	if mentionCount != 0 {
		t.Errorf("citation_mentions after clear = %d, want 0 (cascade via external_citations delete)", mentionCount)
	}

	got, err := d.GetPaper(src.ID)
	if err != nil {
		t.Fatalf("GetPaper: %v", err)
	}
	if got.LastCitationFetchAt != nil {
		t.Errorf("LastCitationFetchAt after clear = %v, want nil (reset)", got.LastCitationFetchAt)
	}
}

// ─── GetCitationStats / PapersProcessed ───────────────────────────────────

func TestGetCitationStats_PapersProcessedCountsFetchAttempts(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	a := seedTestPaper(t, d, p.ID, "A")
	b := seedTestPaper(t, d, p.ID, "B")
	_ = seedTestPaper(t, d, p.ID, "C") // never processed

	if err := d.SetLastCitationFetchAt(a.ID, time.Now()); err != nil {
		t.Fatalf("SetLastCitationFetchAt a: %v", err)
	}
	if err := d.SetLastCitationFetchAt(b.ID, time.Now()); err != nil {
		t.Fatalf("SetLastCitationFetchAt b: %v", err)
	}
	if err := d.UpsertCitationLink(p.ID, a.ID, b.ID); err != nil {
		t.Fatalf("UpsertCitationLink: %v", err)
	}
	d.UpsertExternalCitation(p.ID, &ExternalCitation{S2PaperID: "e1", Title: "E1"})

	stats, err := d.GetCitationStats(p.ID)
	if err != nil {
		t.Fatalf("GetCitationStats: %v", err)
	}
	if stats.PapersProcessed != 2 {
		t.Errorf("PapersProcessed = %d, want 2", stats.PapersProcessed)
	}
	if stats.PapersEligible != 3 {
		t.Errorf("PapersEligible = %d, want 3 (all approved)", stats.PapersEligible)
	}
	if stats.InternalLinks != 1 {
		t.Errorf("InternalLinks = %d, want 1", stats.InternalLinks)
	}
	if stats.ExternalPapers != 1 {
		t.Errorf("ExternalPapers = %d, want 1", stats.ExternalPapers)
	}
}

func TestGetCitationStats_UnprocessedPaperNotCounted(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	// A paper with an s2_paper_id but no fetch attempt recorded must NOT
	// count as processed — processed means "the worker looked at it", not
	// "it resolved to an S2 id" (see PapersProcessed doc comment).
	paper := seedTestPaper(t, d, p.ID, "Has S2 ID, never processed")
	if err := d.SetPaperS2ID(paper.ID, "s2-123"); err != nil {
		t.Fatalf("SetPaperS2ID: %v", err)
	}

	stats, err := d.GetCitationStats(p.ID)
	if err != nil {
		t.Fatalf("GetCitationStats: %v", err)
	}
	if stats.PapersProcessed != 0 {
		t.Errorf("PapersProcessed = %d, want 0", stats.PapersProcessed)
	}
}

// ─── FindPaperByS2ID / FindPaperByDOI ─────────────────────────────────────

func TestFindPaperByS2IDAndDOI(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	paper := &Paper{
		ProfileID:       p.ID,
		Title:           "T",
		TitleNormalized: "t",
		DOI:             ptr.Ptr("10.1/x"),
		Sources:         JSONStringSlice{"test"},
		Authors:         JSONStringSlice{},
		ScoreReasons:    JSONStringSlice{},
		Status:          "approved",
	}
	if err := d.UpsertPaper(paper); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if err := d.SetPaperS2ID(paper.ID, "s2xyz"); err != nil {
		t.Fatalf("SetPaperS2ID: %v", err)
	}

	id, err := d.FindPaperByS2ID(p.ID, "s2xyz")
	if err != nil {
		t.Fatalf("FindPaperByS2ID: %v", err)
	}
	if id != paper.ID {
		t.Errorf("FindPaperByS2ID = %d, want %d", id, paper.ID)
	}

	id, err = d.FindPaperByDOI(p.ID, "10.1/x")
	if err != nil {
		t.Fatalf("FindPaperByDOI: %v", err)
	}
	if id != paper.ID {
		t.Errorf("FindPaperByDOI = %d, want %d", id, paper.ID)
	}

	id, err = d.FindPaperByS2ID(p.ID, "does-not-exist")
	if err != nil {
		t.Fatalf("FindPaperByS2ID (missing): %v", err)
	}
	if id != 0 {
		t.Errorf("FindPaperByS2ID (missing) = %d, want 0", id)
	}
}

// ─── GetExternalCitationByID ──────────────────────────────────────────────

func TestGetExternalCitationByID(t *testing.T) {
	d := testDB(t)
	p := createTestProfile(t, d)
	id, err := d.UpsertExternalCitationGetID(p.ID, &ExternalCitation{S2PaperID: "z", Title: "Z Paper", Year: ptr.Ptr(2020)})
	if err != nil {
		t.Fatalf("UpsertExternalCitationGetID: %v", err)
	}

	got, err := d.GetExternalCitationByID(p.ID, id)
	if err != nil {
		t.Fatalf("GetExternalCitationByID: %v", err)
	}
	if got.Title != "Z Paper" || got.S2PaperID != "z" {
		t.Errorf("got = %+v", got)
	}

	// Wrong profile scope must not leak the row.
	otherProfile := createTestProfile(t, d)
	if _, err := d.GetExternalCitationByID(otherProfile.ID, id); err == nil {
		t.Errorf("expected error fetching external citation from wrong profile")
	}
}
