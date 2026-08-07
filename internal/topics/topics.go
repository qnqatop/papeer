// Package topics clusters a corpus of papers into topics using classic
// TF-IDF + spherical K-Means, entirely in pure Go — no external services, no
// heavy ML dependency. It runs in well under a second for a few hundred
// documents, which is the target corpus size for a single research profile.
package topics

import (
	"math"
	"regexp"
	"sort"
	"strings"
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

var tokenRe = regexp.MustCompile(`[a-zA-Zа-яА-ЯёЁ]+`)

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

	tokenized := make([][]string, len(docs))
	for i, d := range docs {
		tokenized[i] = tokenize(d.Text)
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

	topics := buildTopics(assign, centroids, docs, vocab, k)
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

// tokenize lowercases, extracts alphabetic runs, and drops stopwords/short
// tokens. Also emits bigrams of consecutive surviving tokens so clusters can
// surface more specific phrases ("attention mechanism") instead of only
// generic unigrams.
func tokenize(text string) []string {
	raw := tokenRe.FindAllString(strings.ToLower(text), -1)
	words := make([]string, 0, len(raw))
	for _, w := range raw {
		if len(w) < 3 || stopwords[w] {
			continue
		}
		words = append(words, w)
	}

	out := make([]string, 0, len(words)*2)
	out = append(out, words...)
	for i := 0; i+1 < len(words); i++ {
		out = append(out, words[i]+" "+words[i+1])
	}
	return out
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
// each centroid's highest-weighted terms.
func buildTopics(assign []int, centroids []map[int]float64, docs []Document, vocab []string, k int) []Topic {
	topics := make([]Topic, k)
	for i := range topics {
		topics[i] = Topic{ID: i}
	}
	for i, cluster := range assign {
		topics[cluster].PaperIDs = append(topics[cluster].PaperIDs, docs[i].ID)
	}
	for i := range topics {
		topics[i].Size = len(topics[i].PaperIDs)
		topics[i].TopTerms = topTerms(centroids[i], vocab, 6)
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
	titled := make([]string, len(top))
	for i, t := range top {
		titled[i] = strings.Title(t) //nolint:staticcheck // simple heuristic title-casing is fine for cluster labels
	}
	return strings.Join(titled, " · ")
}
