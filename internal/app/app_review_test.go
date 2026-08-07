package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
	"github.com/qnqatop/papeer/internal/topics"
)

// ─── buildIntroPrompt ─────────────────────────────────────────────────────

func TestBuildIntroPrompt_MentionsCountsAndLabels(t *testing.T) {
	papers := []db.Paper{{ID: 1}, {ID: 2}, {ID: 3}}
	topicList := []topics.Topic{
		{ID: 0, Label: "Neural Networks"},
		{ID: 1, Label: "Cold Start"},
	}
	got := buildIntroPrompt(papers, topicList, nil)

	if !strings.Contains(got, "3 papers") {
		t.Errorf("prompt = %q, want paper count 3", got)
	}
	if !strings.Contains(got, "2 topic clusters") {
		t.Errorf("prompt = %q, want topic count 2", got)
	}
	if !strings.Contains(got, "Neural Networks") || !strings.Contains(got, "Cold Start") {
		t.Errorf("prompt = %q, want both topic labels", got)
	}
}

func TestBuildIntroPrompt_NoTopics(t *testing.T) {
	got := buildIntroPrompt(nil, nil, nil)
	if !strings.Contains(got, "0 papers") || !strings.Contains(got, "0 topic clusters") {
		t.Errorf("prompt = %q, want zero counts", got)
	}
}

// ─── buildSectionPrompt ───────────────────────────────────────────────────

func TestBuildSectionPrompt_IncludesLabelTermsAndPapers(t *testing.T) {
	byID := map[int64]db.Paper{
		1: {ID: 1, Title: "Paper One", Year: ptr.Ptr(2020), Abstract: "Short abstract."},
		2: {ID: 2, Title: "Paper Two", Year: nil, Abstract: ""},
	}
	topic := topics.Topic{
		Label:    "Cold Start",
		TopTerms: []string{"cold start", "recommender"},
		PaperIDs: []int64{1, 2},
	}
	got := buildSectionPrompt(topic, byID, nil)

	if !strings.Contains(got, `"Cold Start"`) {
		t.Errorf("prompt = %q, want quoted label", got)
	}
	if !strings.Contains(got, "cold start, recommender") {
		t.Errorf("prompt = %q, want joined top terms", got)
	}
	if !strings.Contains(got, "Paper One (2020)") {
		t.Errorf("prompt = %q, want paper one with year", got)
	}
	if !strings.Contains(got, "Paper Two (n/a)") {
		t.Errorf("prompt = %q, want 'n/a' year fallback for Paper Two", got)
	}
}

func TestBuildSectionPrompt_SkipsMissingPaperIDs(t *testing.T) {
	byID := map[int64]db.Paper{1: {ID: 1, Title: "Present"}}
	topic := topics.Topic{Label: "T", PaperIDs: []int64{1, 999}}
	got := buildSectionPrompt(topic, byID, nil)
	if !strings.Contains(got, "Present") {
		t.Errorf("prompt = %q, want the present paper included", got)
	}
	if strings.Count(got, "- ") != 1 {
		t.Errorf("prompt = %q, want exactly one paper row", got)
	}
}

func TestBuildSectionPrompt_UsesSummaryWhenAvailable(t *testing.T) {
	byID := map[int64]db.Paper{
		1: {ID: 1, Title: "Paper One", Year: ptr.Ptr(2020), Abstract: "Old abstract."},
	}
	summariesByPaper := map[int64]string{
		1: "AI-generated summary content.",
	}
	topic := topics.Topic{Label: "T", PaperIDs: []int64{1}}
	got := buildSectionPrompt(topic, byID, summariesByPaper)

	if !strings.Contains(got, "Summary: AI-generated summary content.") {
		t.Errorf("prompt = %q, want summary content instead of abstract", got)
	}
	if strings.Contains(got, "Old abstract.") {
		t.Errorf("prompt = %q, should NOT contain old abstract when summary exists", got)
	}
}

func TestBuildSectionPrompt_FallsBackToAbstractWhenNoSummary(t *testing.T) {
	byID := map[int64]db.Paper{
		1: {ID: 1, Title: "Paper One", Year: ptr.Ptr(2020), Abstract: "Short abstract."},
	}
	topic := topics.Topic{Label: "T", PaperIDs: []int64{1}}
	got := buildSectionPrompt(topic, byID, nil)

	if !strings.Contains(got, "Summary: Short abstract.") {
		t.Errorf("prompt = %q, want fallback to abstract when no summary map", got)
	}
}

func TestBuildSectionPrompt_TruncatesLongSummary(t *testing.T) {
	longSummary := strings.Repeat("y", 800)
	byID := map[int64]db.Paper{1: {ID: 1, Title: "Long", Abstract: "short"}}
	summariesByPaper := map[int64]string{1: longSummary}
	topic := topics.Topic{Label: "T", PaperIDs: []int64{1}}
	got := buildSectionPrompt(topic, byID, summariesByPaper)
	if !strings.Contains(got, strings.Repeat("y", 600)+"...") {
		t.Errorf("prompt did not truncate the summary to 600 chars + ellipsis")
	}
	if strings.Contains(got, strings.Repeat("y", 601)) {
		t.Errorf("prompt leaked more than 600 chars of the summary")
	}
}

func TestBuildSectionPrompt_CapsPaperCountAndNotesOverflow(t *testing.T) {
	byID := make(map[int64]db.Paper, maxSectionPapers+10)
	ids := make([]int64, 0, maxSectionPapers+10)
	for i := int64(1); i <= int64(maxSectionPapers+10); i++ {
		byID[i] = db.Paper{ID: i, Title: "P"}
		ids = append(ids, i)
	}
	topic := topics.Topic{Label: "T", PaperIDs: ids}
	got := buildSectionPrompt(topic, byID, nil)

	if strings.Count(got, "- P") != maxSectionPapers {
		t.Errorf("prompt lists %d papers, want %d (capped)", strings.Count(got, "- P"), maxSectionPapers)
	}
	if !strings.Contains(got, "and 10 further papers in this cluster (omitted for brevity)") {
		t.Errorf("prompt = %q, want overflow note for the remaining 10 papers", got)
	}
}

// ─── reviewDisclaimer ─────────────────────────────────────────────────────

func TestReviewDisclaimer_WarnsAboutHallucination(t *testing.T) {
	if !strings.Contains(reviewDisclaimer, "Disclaimer") {
		t.Errorf("reviewDisclaimer = %q, want it to mention 'Disclaimer'", reviewDisclaimer)
	}
	if !strings.Contains(strings.ToLower(reviewDisclaimer), "verify") {
		t.Errorf("reviewDisclaimer = %q, want a call to verify claims", reviewDisclaimer)
	}
	if !strings.Contains(strings.ToLower(reviewDisclaimer), "ai-generated summaries") {
		t.Errorf("reviewDisclaimer = %q, want mention of AI-generated summaries", reviewDisclaimer)
	}
}

// ─── GenerateReviewDraft — guard path only, no LLM/network involved ───────

func TestGenerateReviewDraft_NoActiveLLMProfile(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	seedKeyPaper(t, a, prof.ID, "Some Paper", 1, nil)

	err := a.GenerateReviewDraft(prof.ID, "", nil, "")
	if !errors.Is(err, ErrNoActiveProfile) {
		t.Errorf("err = %v, want ErrNoActiveProfile", err)
	}

	// The mutex must have been released on the error path — a second call
	// must fail with the same "no active profile" error, not "already in
	// progress" (which would indicate a leaked lock).
	err = a.GenerateReviewDraft(prof.ID, "", nil, "")
	if !errors.Is(err, ErrNoActiveProfile) {
		t.Errorf("second call err = %v, want ErrNoActiveProfile (lock must not leak)", err)
	}
}

func TestGenerateReviewDraft_CustomPrompt(t *testing.T) {
	a := newTestApp(t)
	prof := createValidProfile(t, a, "P", "real@univ.edu")
	seedKeyPaper(t, a, prof.ID, "Some Paper", 1, nil)

	err := a.GenerateReviewDraft(prof.ID, "", nil, "Be a pirate.")
	if !errors.Is(err, ErrNoActiveProfile) {
		t.Errorf("err = %v, want ErrNoActiveProfile", err)
	}

	// The mutex must have been released on the error path.
	err = a.GenerateReviewDraft(prof.ID, "", nil, "")
	if !errors.Is(err, ErrNoActiveProfile) {
		t.Errorf("second call err = %v, want ErrNoActiveProfile (lock must not leak)", err)
	}
}
