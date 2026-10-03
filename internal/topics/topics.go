// Package topics clusters a corpus of papers into topics using classic
// TF-IDF + spherical K-Means, entirely in pure Go — no external services, no
// heavy ML dependency. It runs in well under a second for a few hundred
// documents, which is the target corpus size for a single research profile.
package topics

import (
	"math"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/qnqatop/papeer/internal/textproc"
)

// Document is one paper's text to cluster (caller combines title+abstract).
type Document struct {
	ID   int64
	Text string
}

// Topic is one cluster, with a human-readable label, its most representative
// terms, member paper IDs, and 2D coordinates (via PCA of cluster centroids)
// for a scatter-plot layout.
type Topic struct {
	ID       int      `json:"id"`
	Label    string   `json:"label"`
	TopTerms []string `json:"top_terms"`
	PaperIDs []int64  `json:"paper_ids"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	Size     int      `json:"size"`
}

// MinK and MaxK bound the auto-selected cluster count.
const (
	MinK = 8
	MaxK = 15
)

// Cluster computes TF-IDF vectors for docs and groups them with spherical
// K-Means (cosine similarity). k<=0 auto-selects a cluster count in
// [MinK, MaxK] based on corpus size (fewer for small corpora, so a 20-paper
// profile doesn't get forced into 8 near-empty buckets).
//
// Deterministic: uses a fixed PRNG seed, so repeated calls on the same input
// return the same clustering (no UI jitter between visits).
func Cluster(docs []Document, k int) ([]Topic, error) {
	docs = filterEmpty(docs)
	if len(docs) == 0 {
		return nil, nil
	}

	surfaces := make(surfaceIndex)
	tokenized := make([][]string, len(docs))
	for i, d := range docs {
		tokenized[i] = tokenize(d.Text, surfaces)
	}

	vocab, df := buildVocab(tokenized)
	if len(vocab) == 0 {
		return nil, nil
	}
	idf := make([]float64, len(vocab))
	n := float64(len(docs))
	for i, term := range vocab {
		idf[i] = math.Log(n/float64(df[term])) + 1
	}
	termIndex := make(map[string]int, len(vocab))
	for i, t := range vocab {
		termIndex[t] = i
	}

	vectors := make([]map[int]float64, len(docs))
	for i, toks := range tokenized {
		vectors[i] = tfidfVector(toks, termIndex, idf)
	}

	k = chooseK(k, len(docs))
	assign, centroids := kmeans(vectors, len(vocab), k, 42)

	topics := buildTopics(assign, centroids, docs, vocab, k, surfaces)
	layoutCentroids(topics, centroids)
	return topics, nil
}

func filterEmpty(docs []Document) []Document {
	out := make([]Document, 0, len(docs))
	for _, d := range docs {
		if strings.TrimSpace(d.Text) != "" {
			out = append(out, d)
		}
	}
	return out
}

// chooseK picks a cluster count. Explicit k (if valid) wins; otherwise it
// scales gently with corpus size, bounded to [MinK, MaxK], but never
// exceeds n/2 (avoids singleton clusters on small corpora) nor n itself.
func chooseK(k, n int) int {
	if k <= 0 {
		k = MinK + (n / 60) // grows slowly: +1 cluster per ~60 extra papers
		if k > MaxK {
			k = MaxK
		}
	}
	if k > n {
		k = n
	}
	if n >= 4 && k > n/2 {
		k = n / 2
	}
	if k < 1 {
		k = 1
	}
	return k
}

// tokenize turns text into clustering terms: the stems of its words (see
// textproc.Tokenize: stopwords dropped, Russian and English stemmed, so
// inflected forms count as one term) plus bigrams of consecutive surviving
// stems, so clusters can surface more specific phrases ("attention
// mechanism") instead of only generic unigrams. A bigram term is the two
// stems joined by a single space.
//
// When surfaces is non-nil, every emitted term is also recorded with the
// surface form it came from, so labels can later be shown as real words.
func tokenize(text string, surfaces surfaceIndex) []string {
	toks := textproc.Tokenize(text)
	out := make([]string, 0, len(toks)*2)
	for _, t := range toks {
		out = append(out, t.Stem)
		surfaces.add(t.Stem, t.Surface)
	}
	for i := 0; i+1 < len(toks); i++ {
		term := toks[i].Stem + " " + toks[i+1].Stem
		out = append(out, term)
		surfaces.add(term, toks[i].Surface+" "+toks[i+1].Surface)
	}
	return out
}

// surfaceIndex counts, per term (stem or "stem stem" bigram), how often each
// surface form produced it across the corpus. Stems are not human-readable
// ("нейрон", "entangl"), so topic labels and top terms are displayed as the
// most frequent surface form of each term instead.
type surfaceIndex map[string]map[string]int

func (s surfaceIndex) add(term, surface string) {
	if s == nil {
		return
	}
	forms := s[term]
	if forms == nil {
		forms = make(map[string]int, 1)
		s[term] = forms
	}
	forms[surface]++
}

// display returns the most frequent surface form of term, breaking ties by
// the lexicographically smallest form so the choice is deterministic. Terms
// that were never recorded are returned unchanged.
func (s surfaceIndex) display(term string) string {
	best, bestN := "", 0
	for form, n := range s[term] {
		if n > bestN || (n == bestN && form < best) {
			best, bestN = form, n
		}
	}
	if bestN == 0 {
		return term
	}
	return best
}

// buildVocab collects terms that appear in at least 2 documents and at most
// 60% of documents (drops both noise and near-universal filler terms), and
// returns them alongside their document frequency.
func buildVocab(tokenized [][]string) ([]string, map[string]int) {
	df := make(map[string]int)
	for _, toks := range tokenized {
		seen := make(map[string]bool, len(toks))
		for _, t := range toks {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}

	n := len(tokenized)
	maxDF := int(0.6 * float64(n))
	if maxDF < 2 {
		maxDF = n // tiny corpora: don't filter by upper bound
	}
	minDF := 2
	if n < 3 {
		minDF = 1
	}

	vocab := make([]string, 0, len(df))
	for term, count := range df {
		if count >= minDF && count <= maxDF {
			vocab = append(vocab, term)
		}
	}
	sort.Strings(vocab) // stable order for reproducibility
	return vocab, df
}

// tfidfVector builds a sparse, L2-normalized TF-IDF vector for one document.
func tfidfVector(tokens []string, termIndex map[string]int, idf []float64) map[int]float64 {
	counts := make(map[int]float64)
	for _, t := range tokens {
		if idx, ok := termIndex[t]; ok {
			counts[idx]++
		}
	}
	vec := make(map[int]float64, len(counts))
	var norm float64
	for idx, c := range counts {
		w := c * idf[idx]
		vec[idx] = w
		norm += w * w
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for idx := range vec {
			vec[idx] /= norm
		}
	}
	return vec
}

// buildTopics assigns papers/top-terms to clusters and derives a label from
// each centroid's highest-weighted terms. Terms are picked on stems and then
// translated to their most frequent surface forms for display.
func buildTopics(assign []int, centroids []map[int]float64, docs []Document, vocab []string, k int, surfaces surfaceIndex) []Topic {
	topics := make([]Topic, k)
	for i := range topics {
		topics[i] = Topic{ID: i}
	}
	for i, cluster := range assign {
		topics[cluster].PaperIDs = append(topics[cluster].PaperIDs, docs[i].ID)
	}
	for i := range topics {
		topics[i].Size = len(topics[i].PaperIDs)
		terms := topTerms(centroids[i], vocab, 6)
		for j, term := range terms {
			terms[j] = surfaces.display(term)
		}
		topics[i].TopTerms = terms
		topics[i].Label = makeLabel(topics[i].TopTerms)
	}
	return topics
}

func topTerms(centroid map[int]float64, vocab []string, n int) []string {
	type kv struct {
		term string
		w    float64
	}
	kvs := make([]kv, 0, len(centroid))
	for idx, w := range centroid {
		if w > 0 {
			kvs = append(kvs, kv{vocab[idx], w})
		}
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].w != kvs[j].w {
			return kvs[i].w > kvs[j].w
		}
		return kvs[i].term < kvs[j].term
	})
	// Prefer bigrams first (more specific labels), then fill with unigrams,
	// capped at n, dropping unigrams that are already part of a chosen bigram.
	var bigrams, unigrams []kv
	for _, e := range kvs {
		if strings.Contains(e.term, " ") {
			bigrams = append(bigrams, e)
		} else {
			unigrams = append(unigrams, e)
		}
	}
	used := make(map[string]bool)
	var out []string
	add := func(e kv) bool {
		if len(out) >= n {
			return false
		}
		for _, w := range strings.Fields(e.term) {
			if used[w] {
				return true // skip, but keep going
			}
		}
		out = append(out, e.term)
		for _, w := range strings.Fields(e.term) {
			used[w] = true
		}
		return true
	}
	for _, e := range bigrams {
		if len(out) >= n {
			break
		}
		add(e)
	}
	for _, e := range unigrams {
		if len(out) >= n {
			break
		}
		add(e)
	}
	return out
}

func makeLabel(terms []string) string {
	if len(terms) == 0 {
		return "Miscellaneous"
	}
	top := terms
	if len(top) > 3 {
		top = top[:3]
	}
	// NoLower keeps strings.Title semantics: only word-initial letters change.
	// A Caser is stateful, so it is created per call rather than shared.
	caser := cases.Title(language.Und, cases.NoLower)
	titled := make([]string, len(top))
	for i, t := range top {
		titled[i] = caser.String(t)
	}
	return strings.Join(titled, " · ")
}
