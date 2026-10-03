package textproc

import (
	"strings"
	"sync"
	"sync/atomic"

	bbalet "github.com/bbalet/stopwords"
	"github.com/kljensen/snowball/english"
	"github.com/kljensen/snowball/russian"
)

// Stop words come from four layers, checked in Tokenize in this order:
//
//  1. surfaceStopwords — our own lists matched against the ё-folded surface
//     form: general English words the third-party lists miss, Russian
//     function words written without ё ("ее", "еще"), and every word of the
//     academic lists below.
//  2. General-purpose lists of the word's language: the Snowball lists
//     (english/russian.IsStopWord, ~170 words each) and the bbalet/stopwords
//     lists (~320 EN / ~420 RU words). bbalet keeps its maps unexported, so
//     membership is probed through CleanString and memoized (see inBbalet).
//     Both are matched on surface forms only: stemming a function word can
//     collide with a content stem.
//  3. keepWords exempts the few content words those general lists contain
//     ("fire", "interest", "человек", "жизнь").
//  4. stemStopwords — the stems of the academic boilerplate lists, so every
//     inflection is caught: "метод/метода/методом/методы" all stem to
//     "метод", "approach/approaches/approached" to "approach".
//
// Academic words whose stem collides with an unrelated content word are put
// in the surface-only lists instead (e.g. "provide" → "provid" would also
// drop "provider", "experiment" → "experi" would drop "experience", "виде" →
// "вид" would drop "виды" — species).

// generalEnglish are common English words missing from the third-party
// lists; matched on surface form only.
var generalEnglish = []string{
	"the", "and", "for", "are", "but", "not", "you", "all", "can", "her",
	"was", "one", "our", "out", "has", "have", "had", "his", "him", "its",
	"who", "did", "yet", "few", "how", "why", "than", "then", "them",
	"they", "this", "that", "these", "those", "with", "from", "into",
	"over", "under", "about", "after", "before", "between", "during",
	"through", "while", "where", "when", "which", "what", "will", "would",
	"could", "should", "shall", "may", "might", "must", "also", "such",
	"some", "any", "each", "both", "more", "most", "other", "same", "own",
	"too", "very", "just", "only", "here", "there", "again",
	"further", "once", "does", "doing", "been", "being", "were", "itself",
	"themselves", "ourselves", "yourself", "because", "against", "above",
	"below", "off", "onto", "upon", "within", "without", "per", "via",
	"across", "among", "along", "toward", "towards", "thus", "hence",
	"however", "therefore", "moreover", "furthermore", "although",
	"though", "either", "neither", "whether", "given", "namely",
	"including", "include", "includes", "included",
}

// generalRussian are Russian function words missing from the third-party
// lists, mostly ё-less spellings and common connectives of scientific prose.
var generalRussian = []string{
	"ее", "еще", "мое", "нее", "твое", "свое", "все",
	"которые", "который", "которая", "которое", "которых", "которым",
	"которыми", "котором", "которой", "этой", "этих", "этим",
	"однако", "кроме", "также", "либо", "причем", "поэтому",
	"следовательно", "например", "таким", "этого", "свою", "своих",
	"одна", "одно", "одни", "одним", "одном",
}

// academicEnglish is boilerplate that is near-universal in abstracts and
// therefore useless for telling topics apart ("this paper proposes a novel
// method to analyze..."). Matched on surface and on stem.
var academicEnglish = []string{
	"study", "studies", "studied", "analysis", "analyses", "analyze",
	"analyzed", "analyzing", "model", "models", "modeling", "modelled",
	"method", "methods", "methodology", "result", "results", "resulting",
	"approach", "approaches", "paper", "papers", "using", "used", "use",
	"uses", "based", "novel", "propose", "proposed", "proposes",
	"proposing", "present", "presents", "presented", "presenting", "work",
	"framework", "frameworks", "system", "systems", "dataset", "datasets",
	"data", "performance", "evaluate", "evaluated", "evaluating",
	"evaluation", "show", "shows", "showed", "shown", "showing",
	"demonstrate", "demonstrates", "demonstrated", "demonstrating",
	"achieve", "achieves", "achieved", "achieving", "compared",
	"comparison", "compare", "comparing", "significant", "significantly",
	"existing", "recent", "recently", "state", "art", "consider",
	"considered", "considering", "various", "several", "different",
	"large", "small", "high", "low", "new", "well", "many", "research",
	"literature", "review", "field", "fields", "problem", "problems",
	"task", "tasks", "application", "applications", "case", "cases", "aim",
	"aims", "aimed", "order", "terms", "term", "number", "numbers", "set",
	"sets", "value", "values", "level", "levels", "purpose", "conclusion",
	"conclusions", "findings", "finding", "discuss", "discussed",
	"discussion", "described", "describe", "describes", "describing",
	"introduce", "introduces", "introduced", "introducing",
}

// academicEnglishSurfaceOnly is boilerplate whose stem collides with a
// content word (see the package comment above).
var academicEnglishSurfaceOnly = []string{
	"experiment", "experiments", "experimental",
	"provide", "provides", "provided", "providing",
	"important",
}

// academicRussian is the Russian counterpart of academicEnglish. Matched on
// surface and on stem; the Snowball stemmer does not unify stem alternations
// ("уровень" → "уровен", "уровня" → "уровн"), so such pairs list both forms.
var academicRussian = []string{
	"является", "являются", "являться", "данный", "данной", "данные",
	"данных", "работа", "работе", "работы", "работах", "статья", "статье",
	"статьи", "метод", "метода", "методы", "методов", "методом", "подход",
	"подхода", "подходы", "исследование", "исследования", "исследований",
	"результат", "результаты", "результатов", "предлагается",
	"предлагаемый", "предложен", "предложенный", "рассмотрен",
	"рассмотрены", "рассматривается", "может", "могут", "позволяет",
	"позволяют", "основе", "использование", "использования",
	"используется", "используются", "используя", "применение",
	"применения", "анализ", "анализа", "модель", "модели", "система",
	"системы", "задача", "задачи", "проблема", "проблемы", "показано",
	"показаны", "показывает", "получены", "представлен", "представлены",
	"представлена", "описан", "описаны", "различных", "различные",
	"новый", "новых", "новые", "существующих", "современных", "автор",
	"авторы", "вывод", "выводы", "заключение", "обзор", "литература",
	"литературы", "значение", "значения", "уровень", "уровня", "оценка",
	"оценки", "область", "области",
}

// academicRussianSurfaceOnly is Russian boilerplate whose stem collides with
// content words ("виде" → "вид" = species, "целью" → "цел" = "целые").
var academicRussianSurfaceOnly = []string{
	"виде", "целью", "цель", "случае", "образом", "рамках", "помощью",
	"условиях", "качестве", "эксперимент", "эксперименты",
	"экспериментальные", "получение", "необходимо", "следует",
	"существенно", "значительно", "важно",
}

// keepWords are content words that the third-party general lists treat as
// stop words; research corpora about fires, mills or human life need them.
var keepWords = setOf(
	"fire", "bill", "mill", "interest", "bottom", "top", "front", "thin",
	"detail", "жизнь", "человек", "люди", "мира", "мор",
)

var (
	surfaceStopwords = setOf(concat(
		generalEnglish, generalRussian,
		academicEnglish, academicEnglishSurfaceOnly,
		academicRussian, academicRussianSurfaceOnly,
	)...)
	stemStopwords = stemSet(academicEnglish, academicRussian)
)

// isSurfaceStopword checks a word against the surface-form layers. surface
// is the lowercased word as written, folded is surface with ё→е, sc its
// script.
func isSurfaceStopword(surface, folded string, sc script) bool {
	if surfaceStopwords[folded] {
		return true
	}
	if keepWords[folded] {
		return false
	}
	switch sc {
	case scriptLatin:
		return english.IsStopWord(folded) || inBbalet(folded, "en")
	case scriptCyrillic:
		// The Snowball list is spelled without ё, bbalet mixes both.
		return russian.IsStopWord(folded) ||
			inBbalet(folded, "ru") ||
			(surface != folded && inBbalet(surface, "ru"))
	default:
		return false
	}
}

// bbaletCache memoizes inBbalet: CleanString parses a language tag and runs
// a regexp per call, far too slow to repeat for every token of a corpus.
// The vocabulary of a literature corpus is small, but the cache is capped
// anyway so a long-running app cannot grow it without bound.
var (
	bbaletCache     sync.Map // lang+":"+word → bool
	bbaletCacheSize atomic.Int64
)

const bbaletCacheMax = 1 << 17

// inBbalet reports whether word is in bbalet/stopwords' list for lang. The
// package exposes no membership test, but CleanString of a single stop word
// is blank.
func inBbalet(word, lang string) bool {
	key := lang + ":" + word
	if v, ok := bbaletCache.Load(key); ok {
		return v.(bool)
	}
	stop := strings.TrimSpace(bbalet.CleanString(word, lang, false)) == ""
	if bbaletCacheSize.Load() < bbaletCacheMax {
		if _, loaded := bbaletCache.LoadOrStore(key, stop); !loaded {
			bbaletCacheSize.Add(1)
		}
	}
	return stop
}

func concat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// setOf builds a lookup set of ё-folded words.
func setOf(words ...string) map[string]bool {
	out := make(map[string]bool, len(words))
	for _, w := range words {
		out[foldYo(w)] = true
	}
	return out
}

// stemSet builds the set of stems of the given word lists, stemmed exactly
// as Tokenize stems text so lookups match.
func stemSet(lists ...[]string) map[string]bool {
	out := make(map[string]bool)
	for _, l := range lists {
		for _, w := range l {
			w = foldYo(w)
			out[stemWord(w, scriptOf(w))] = true
		}
	}
	return out
}
