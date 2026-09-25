package search

import "unicode"

// minCyrillicForRu is how many Cyrillic letters make a text Russian regardless
// of how much Latin it also contains. Russian titles routinely carry English
// terms ("Fine-tuning LLaMA и RuBERT для Question Answering" is 4 Cyrillic vs
// 38 Latin letters), while English titles practically never contain Cyrillic,
// so a few Cyrillic letters are strong evidence on their own. A lone letter
// (a variable name, "Ж" in a linguistics paper) is not enough.
const minCyrillicForRu = 3

// detectLang guesses a paper's language from the script of its text: Cyrillic →
// "ru", Latin → "en". It is deliberately coarse (script-based, not
// language-model based) — enough to tell a Russian article from an English one,
// which is all the axis language scope needs. Returns "" when the text carries
// no letters to judge by (e.g. only digits/punctuation), so callers can choose
// not to drop such records.
func detectLang(s string) string {
	var cyr, lat int
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case unicode.Is(unicode.Latin, r):
			lat++
		}
	}
	if cyr == 0 && lat == 0 {
		return ""
	}
	if cyr >= minCyrillicForRu || cyr >= lat {
		return "ru"
	}
	return "en"
}

// matchesLangScope reports whether a paper belongs in an axis with the given
// language scope. Language is judged from the title, falling back to the
// abstract when the title has no letters. An undetectable language is kept
// (returns true) rather than silently dropped.
func matchesLangScope(p RawPaper, scope string) bool {
	lang := detectLang(p.Title)
	if lang == "" {
		lang = detectLang(p.Abstract)
	}
	if lang == "" {
		return true
	}
	return lang == scope
}

// filterByLangScope returns only the papers whose detected language matches the
// scope (keeping undetectable ones). It allocates a new slice and never mutates
// the input.
func filterByLangScope(papers []RawPaper, scope string) []RawPaper {
	out := make([]RawPaper, 0, len(papers))
	for _, p := range papers {
		if matchesLangScope(p, scope) {
			out = append(out, p)
		}
	}
	return out
}
