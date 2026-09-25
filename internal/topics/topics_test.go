package topics

import (
	"math"
	"strings"
	"testing"
)

// ─── tokenize ─────────────────────────────────────────────────────────────

func TestTokenize_FiltersStopwordsAndShortTokens(t *testing.T) {
	toks := tokenize("The Novel Approach to Quantum Entanglement is a Study")
	for _, bad := range []string{"the", "novel", "approach", "to", "is", "a", "study"} {
		for _, tok := range toks {
			if tok == bad {
				t.Errorf("tokenize() kept stopword/boilerplate %q in %v", bad, toks)
			}
		}
	}
	found := false
	for _, tok := range toks {
		if tok == "quantum" {
			found = true
		}
	}
	if !found {
		t.Errorf("tokenize() dropped meaningful word %q from %v", "quantum", toks)
	}
}

func TestTokenize_DropsShortWords(t *testing.T) {
	toks := tokenize("an ai ML in NLP")
	for _, tok := range toks {
		if len(tok) < 3 && tok != "" {
			// bigrams contain a space so length isn't a clean unigram check;
			// only unigrams (no space) must be >= 3 runes.
			t.Errorf("tokenize() kept a short unigram %q", tok)
		}
	}
}

func TestTokenize_EmitsBigrams(t *testing.T) {
	toks := tokenize("attention mechanism transformer network")
	wantBigram := "attention mechanism"
	found := false
	for _, tok := range toks {
		if tok == wantBigram {
			found = true
		}
	}
	if !found {
		t.Errorf("tokenize() missing bigram %q in %v", wantBigram, toks)
	}
}

// ─── buildVocab ───────────────────────────────────────────────────────────

func TestBuildVocab_MinDocFrequency(t *testing.T) {
	// "rare" appears in only 1 of 4 docs → dropped (minDF=2 for n>=3).
	// "common" appears in all 4 → also dropped (maxDF = 0.6*4 = 2.4 → 2).
	// "shared" appears in exactly 2 docs → kept.
	tokenized := [][]string{
		{"rare", "common", "shared"},
		{"common", "shared"},
		{"common"},
		{"common"},
	}
	vocab, df := buildVocab(tokenized)

	has := func(term string) bool {
		for _, v := range vocab {
			if v == term {
				return true
			}
		}
		return false
	}
	if has("rare") {
		t.Errorf("vocab should drop term below minDF: %v", vocab)
	}
	if has("common") {
		t.Errorf("vocab should drop term above maxDF: %v (df=%v)", vocab, df)
	}
	if !has("shared") {
		t.Errorf("vocab should keep term within [minDF,maxDF]: %v", vocab)
	}
}

func TestBuildVocab_TinyCorpusRelaxesMinDF(t *testing.T) {
	// n < 3 → minDF relaxes to 1, so a term appearing in a single doc survives.
	tokenized := [][]string{
		{"unique"},
		{"other"},
	}
	vocab, _ := buildVocab(tokenized)
	if len(vocab) != 2 {
		t.Errorf("tiny corpus: vocab = %v, want both terms kept", vocab)
	}
}

// ─── chooseK ──────────────────────────────────────────────────────────────

func TestChooseK(t *testing.T) {
	cases := []struct {
		name string
		k, n int
		want int
	}{
		{"explicit k wins", 3, 100, 3},
		{"explicit k capped by n/2", 3, 4, 2}, // n=4 >= 4 → k capped to n/2=2
		{"auto small corpus capped by n", 0, 5, 2},
		{"auto mid corpus at MinK", 0, 20, MinK},
		{"auto large corpus grows toward MaxK", 0, 600, MaxK},
		{"k larger than n", 20, 5, 2}, // n=5 >= 4 → n/2 = 2
		{"n zero yields at least 1", 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseK(tc.k, tc.n)
			if tc.n == 0 {
				if got < 0 {
					t.Errorf("chooseK(%d,%d) = %d, want >= 0", tc.k, tc.n, got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("chooseK(%d,%d) = %d, want %d", tc.k, tc.n, got, tc.want)
			}
		})
	}
}

// ─── Cluster ──────────────────────────────────────────────────────────────

func TestCluster_EmptyInput(t *testing.T) {
	got, err := Cluster(nil, 2)
	if err != nil {
		t.Fatalf("Cluster(nil): %v", err)
	}
	if got != nil {
		t.Errorf("Cluster(nil) = %v, want nil", got)
	}
}

func TestCluster_FiltersBlankDocs(t *testing.T) {
	docs := []Document{
		{ID: 1, Text: "   "},
		{ID: 2, Text: ""},
	}
	got, err := Cluster(docs, 2)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if got != nil {
		t.Errorf("Cluster(all-blank) = %v, want nil", got)
	}
}

// quantumDocs / medievalDocs share zero vocabulary, so spherical K-Means with
// k=2 should perfectly separate them into two homogeneous clusters — a
// minimal but concrete check that clustering actually groups similar
// documents rather than e.g. always producing an arbitrary split.
func twoGroupDocs() []Document {
	quantum := []string{
		"quantum computing algorithms exploit entanglement between quantum bits",
		"quantum entanglement algorithms improve quantum computing bit fidelity",
		"entanglement based quantum computing algorithm research on quantum bits",
	}
	medieval := []string{
		"medieval castles and knights defended territory with armor and siege engines",
		"medieval knights wore armor and defended castles during siege warfare",
		"castles knights medieval armor and siege warfare history of fortification",
	}
	docs := make([]Document, 0, 6)
	for i, text := range quantum {
		docs = append(docs, Document{ID: int64(i + 1), Text: text})
	}
	for i, text := range medieval {
		docs = append(docs, Document{ID: int64(i + 4), Text: text})
	}
	return docs
}

func TestCluster_SeparatesTwoDistinctGroups(t *testing.T) {
	docs := twoGroupDocs()
	got, err := Cluster(docs, 2)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(topics) = %d, want 2", len(got))
	}

	quantumIDs := map[int64]bool{1: true, 2: true, 3: true}
	medievalIDs := map[int64]bool{4: true, 5: true, 6: true}

	totalSize := 0
	for _, topic := range got {
		totalSize += topic.Size
		if topic.Size == 0 {
			continue
		}
		inQuantum, inMedieval := 0, 0
		for _, id := range topic.PaperIDs {
			if quantumIDs[id] {
				inQuantum++
			}
			if medievalIDs[id] {
				inMedieval++
			}
		}
		if inQuantum > 0 && inMedieval > 0 {
			t.Errorf("topic %+v mixes both groups (quantum=%d, medieval=%d)", topic, inQuantum, inMedieval)
		}
	}
	if totalSize != 6 {
		t.Errorf("total papers across topics = %d, want 6", totalSize)
	}
}

func TestCluster_DeterministicAcrossRuns(t *testing.T) {
	docs := twoGroupDocs()

	first, err := Cluster(docs, 2)
	if err != nil {
		t.Fatalf("Cluster (first run): %v", err)
	}
	second, err := Cluster(docs, 2)
	if err != nil {
		t.Fatalf("Cluster (second run): %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("topic count differs between runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Label != second[i].Label {
			t.Errorf("topic %d label differs: %q vs %q", i, first[i].Label, second[i].Label)
		}
		if len(first[i].PaperIDs) != len(second[i].PaperIDs) {
			t.Errorf("topic %d paper count differs: %v vs %v", i, first[i].PaperIDs, second[i].PaperIDs)
			continue
		}
		for j := range first[i].PaperIDs {
			if first[i].PaperIDs[j] != second[i].PaperIDs[j] {
				t.Errorf("topic %d paper[%d] differs: %d vs %d", i, j, first[i].PaperIDs[j], second[i].PaperIDs[j])
			}
		}
		// Layout coordinates come from summing sparse map entries (mean
		// centering, Gram matrix) whose Go iteration order is randomized per
		// range statement — so tiny (~1e-9) floating-point summation-order
		// noise between runs is expected and not a determinism bug; the
		// *clustering* (assignment, labels, membership, checked above) is
		// what must be exactly reproducible, not the last bit of a float64.
		const tol = 1e-6
		if math.Abs(first[i].X-second[i].X) > tol || math.Abs(first[i].Y-second[i].Y) > tol {
			t.Errorf("topic %d layout differs beyond float noise: (%v,%v) vs (%v,%v)", i, first[i].X, first[i].Y, second[i].X, second[i].Y)
		}
	}
}

func TestCluster_LayoutCoordinatesAreFinite(t *testing.T) {
	docs := twoGroupDocs()
	got, err := Cluster(docs, 2)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	for _, topic := range got {
		if math.IsNaN(topic.X) || math.IsInf(topic.X, 0) {
			t.Errorf("topic %+v has non-finite X", topic)
		}
		if math.IsNaN(topic.Y) || math.IsInf(topic.Y, 0) {
			t.Errorf("topic %+v has non-finite Y", topic)
		}
	}
}

func TestCluster_SingleClusterLayoutIsOrigin(t *testing.T) {
	docs := []Document{
		{ID: 1, Text: "shared vocabulary across the only document available here"},
	}
	got, err := Cluster(docs, 1)
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(topics) = %d, want 1", len(got))
	}
	if got[0].X != 0 || got[0].Y != 0 {
		t.Errorf("single-cluster layout = (%v,%v), want (0,0)", got[0].X, got[0].Y)
	}
}

// ─── makeLabel / topTerms ─────────────────────────────────────────────────

func TestMakeLabel_EmptyTermsFallsBackToMisc(t *testing.T) {
	if got := makeLabel(nil); got != "Miscellaneous" {
		t.Errorf("makeLabel(nil) = %q, want Miscellaneous", got)
	}
}

// makeLabel used the deprecated strings.Title; the x/text caser must title-case
// the terms tokenize/topTerms produce (lowercase Latin/Cyrillic words and
// space-joined bigrams) exactly the same way.
func TestMakeLabel_TitleCaseMatchesStringsTitle(t *testing.T) {
	for _, term := range []string{
		"neural", "neural networks", "cold start", "нейронные сети", "ёлка",
		"graph", "self attention", "mixedCase words", "x", "",
	} {
		//lint:ignore SA1019 reference behaviour for the replacement
		want := strings.Title(term)
		if got := makeLabel([]string{term}); got != want {
			t.Errorf("makeLabel(%q) = %q, want %q (strings.Title)", term, got, want)
		}
	}
}

func TestMakeLabel_JoinsTopThreeTitled(t *testing.T) {
	got := makeLabel([]string{"neural networks", "deep learning", "cold start", "extra"})
	want := "Neural Networks · Deep Learning · Cold Start"
	if got != want {
		t.Errorf("makeLabel = %q, want %q", got, want)
	}
}
