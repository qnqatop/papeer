// Package textproc turns free text (paper titles and abstracts) into
// normalized tokens for the local text-analytics features: topic clustering
// (internal/topics) and smart relevance scoring (internal/recsys).
//
// It handles mixed Russian/English corpora: words are split on letter runs,
// lowercased, stripped of English and Russian stop words (general-purpose and
// academic boilerplate) and stemmed with the Snowball stemmer of their script,
// so "нейронные"/"нейронных" or "network"/"networks" collapse into one term.
// Each token also keeps its surface form, because stems are not
// human-readable and must never be shown to the user.
package textproc

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kljensen/snowball/english"
	"github.com/kljensen/snowball/russian"
	"golang.org/x/text/unicode/norm"
)

// MinRunes is the minimum word length (in runes, not bytes) a token must
// have; shorter words ("на", "of", "ai") carry almost no topical signal.
const MinRunes = 3

// Token is one surviving word of the input text.
type Token struct {
	// Stem is the normalized matching key: lowercase, ё folded to е, and
	// reduced by the Snowball stemmer for its script (Cyrillic → Russian,
	// Latin → English). Words in other scripts are kept unstemmed.
	Stem string
	// Surface is the word as it appeared in the text, lowercased (ё kept).
	// It is what should be displayed when a stem has to be shown to a user.
	Surface string
}

// script classifies a word by the alphabet all of its letters belong to.
type script int

const (
	scriptOther script = iota // mixed alphabets or neither Latin nor Cyrillic
	scriptLatin
	scriptCyrillic
)

// wordRe matches runs of letters, including combining marks so that an
// accented letter left decomposed after NFC (e.g. a stress mark) stays inside
// its word instead of splitting it in two. Digits and punctuation, including
// hyphens, separate words.
var wordRe = regexp.MustCompile(`[\p{L}\p{M}]+`)

// Tokenize splits text into stemmed, stopword-free tokens in their original
// order. Two tokens adjacent in the result were adjacent in the text modulo
// dropped words, which is what bigram construction relies on.
//
// The pipeline per word: NFC + lowercase → drop combining marks → length
// check (MinRunes, after ё→е folding) → stopword check on the surface form →
// stem → stopword check on the stem (catches inflected forms of academic
// boilerplate such as "методом" or "approaches").
//
// Tokenize is deterministic and safe for concurrent use.
func Tokenize(text string) []Token {
	text = strings.ToLower(norm.NFC.String(text))
	raw := wordRe.FindAllString(text, -1)
	out := make([]Token, 0, len(raw))
	for _, w := range raw {
		surface := stripMarks(w)
		folded := foldYo(surface)
		if utf8.RuneCountInString(folded) < MinRunes {
			continue
		}
		sc := scriptOf(folded)
		if isSurfaceStopword(surface, folded, sc) {
			continue
		}
		stem := stemWord(folded, sc)
		if stemStopwords[stem] {
			continue
		}
		out = append(out, Token{Stem: stem, Surface: surface})
	}
	return out
}

// Stems is Tokenize reduced to the stem of each token, for callers that only
// need matching keys (bag-of-words scoring).
func Stems(text string) []string {
	toks := Tokenize(text)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Stem
	}
	return out
}

// stemWord stems an already lowercased, ё-folded word with the stemmer of
// its script. Stop words are stemmed too (stemStopWords=true): the stopword
// filter is ours, not the stemmer's.
func stemWord(w string, sc script) string {
	switch sc {
	case scriptCyrillic:
		return russian.Stem(w, true)
	case scriptLatin:
		return english.Stem(w, true)
	default:
		return w
	}
}

// foldYo replaces ё with е: Russian texts use both spellings for the same
// word, and the Snowball Russian stemmer does not treat them as equal.
func foldYo(w string) string {
	return strings.ReplaceAll(w, "ё", "е")
}

// stripMarks drops nonspacing combining marks (stress accents and the like)
// that NFC could not compose into a precomposed letter.
func stripMarks(w string) string {
	if !strings.ContainsFunc(w, isMark) {
		return w
	}
	return strings.Map(func(r rune) rune {
		if isMark(r) {
			return -1
		}
		return r
	}, w)
}

func isMark(r rune) bool { return unicode.Is(unicode.Mn, r) }

// scriptOf reports whether every letter of w is Latin or every letter is
// Cyrillic; anything else (Greek, CJK, homoglyph mixes like a Cyrillic word
// with a Latin "e") is scriptOther and left unstemmed.
func scriptOf(w string) script {
	sc := scriptOther
	for _, r := range w {
		var cur script
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cur = scriptCyrillic
		case unicode.Is(unicode.Latin, r):
			cur = scriptLatin
		default:
			return scriptOther
		}
		if sc != scriptOther && sc != cur {
			return scriptOther
		}
		sc = cur
	}
	return sc
}
