package textproc

import (
	"reflect"
	"slices"
	"sync"
	"testing"
	"unicode/utf8"
)

func stemsOf(text string) []string { return Stems(text) }

func surfacesOf(text string) []string {
	toks := Tokenize(text)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Surface
	}
	return out
}

func TestTokenize_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "123 456 — !!!", "и в на of to"} {
		if got := Tokenize(in); len(got) != 0 {
			t.Errorf("Tokenize(%q) = %v, want no tokens", in, got)
		}
	}
}

func TestTokenize_MixedRussianEnglish(t *testing.T) {
	got := Tokenize("Нейронные сети (neural networks) для классификации изображений")
	want := []Token{
		{Stem: "нейрон", Surface: "нейронные"},
		{Stem: "сет", Surface: "сети"},
		{Stem: "neural", Surface: "neural"},
		{Stem: "network", Surface: "networks"},
		{Stem: "классификац", Surface: "классификации"},
		{Stem: "изображен", Surface: "изображений"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize mixed text:\n got %v\nwant %v", got, want)
	}
}

func TestTokenize_InflectionsShareStem(t *testing.T) {
	groups := [][]string{
		{"нейронные", "нейронных", "нейронная", "нейронной"},
		{"катализатор", "катализаторы", "катализаторов", "катализаторами"},
		{"осадки", "осадков", "осадках"},
		{"network", "networks", "networking"},
		{"cluster", "clusters", "clustering", "clustered"},
	}
	for _, g := range groups {
		first := stemsOf(g[0])
		if len(first) != 1 {
			t.Fatalf("Stems(%q) = %v, want exactly one stem", g[0], first)
		}
		for _, w := range g[1:] {
			got := stemsOf(w)
			if len(got) != 1 || got[0] != first[0] {
				t.Errorf("Stems(%q) = %v, want [%s] like %q", w, got, first[0], g[0])
			}
		}
	}
}

// Inflections of academic boilerplate must all be filtered, not only the
// dictionary form listed in the stopword lists — that is what the stem check
// is for.
func TestTokenize_AcademicBoilerplateFilteredInAllForms(t *testing.T) {
	for _, w := range []string{
		"метод", "метода", "методом", "методами", "методу",
		"исследование", "исследованию", "исследованием",
		"подходом", "результатам", "использованием",
		"approach", "approaches", "methods", "proposes", "evaluations",
		"Метод", "ИССЛЕДОВАНИЯ",
	} {
		if got := Tokenize(w); len(got) != 0 {
			t.Errorf("Tokenize(%q) = %v, want boilerplate dropped", w, got)
		}
	}
	// A different word with a related prefix is not boilerplate.
	if got := stemsOf("методика"); len(got) != 1 {
		t.Errorf("Stems(методика) = %v, want it kept", got)
	}
}

func TestTokenize_GeneralStopwords(t *testing.T) {
	text := "это была одна из тех, которые его; the paper of which they are about"
	if got := Tokenize(text); len(got) != 0 {
		t.Errorf("Tokenize(%q) = %v, want only stop words dropped", text, got)
	}
	// Content words the generic third-party lists happen to include survive.
	for _, w := range []string{"fire", "человек", "жизнь"} {
		if got := Tokenize(w); len(got) != 1 {
			t.Errorf("Tokenize(%q) = %v, want the content word kept", w, got)
		}
	}
}

// Two-letter Russian words are 4 bytes in UTF-8; the length check must count
// runes, or "ил"/"ок" would pass while "ai" is dropped.
func TestTokenize_DropsShortWordsByRunes(t *testing.T) {
	got := Tokenize("ил ок ai ML кпд gan ёж")
	for _, tok := range got {
		if utf8.RuneCountInString(tok.Surface) < MinRunes {
			t.Errorf("Tokenize kept short word %+v", tok)
		}
	}
	if want := []string{"кпд", "gan"}; !reflect.DeepEqual(surfacesOf("ил ок ai ML кпд gan ёж"), want) {
		t.Errorf("surfaces = %v, want %v", surfacesOf("ил ок ai ML кпд gan ёж"), want)
	}
}

func TestTokenize_YoFolding(t *testing.T) {
	a := Tokenize("ёлка")
	b := Tokenize("елка")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("Tokenize(ёлка) = %v, Tokenize(елка) = %v", a, b)
	}
	if a[0].Stem != b[0].Stem {
		t.Errorf("ё and е spellings stem differently: %q vs %q", a[0].Stem, b[0].Stem)
	}
	if a[0].Surface != "ёлка" {
		t.Errorf("surface = %q, want ё preserved for display", a[0].Surface)
	}
	// Stop words spelled with or without ё are both dropped.
	for _, w := range []string{"её", "ее", "ещё", "еще"} {
		if got := Tokenize(w + " катализатор"); len(got) != 1 {
			t.Errorf("Tokenize(%q + катализатор) = %v, want stop word dropped", w, got)
		}
	}
}

func TestTokenize_UnicodeNormalization(t *testing.T) {
	// "й" as и + combining breve, ё as е + combining diaeresis, and a stress
	// accent: all must stay inside one word and match the composed spelling.
	decomposed := "нейронныи\u0306 е\u0308мкость катализа\u0301тор"
	composed := "нейронный ёмкость катализатор"
	if got, want := stemsOf(decomposed), stemsOf(composed); !reflect.DeepEqual(got, want) {
		t.Errorf("decomposed stems = %v, composed = %v", got, want)
	}
	if got := stemsOf(composed); len(got) != 3 {
		t.Errorf("Stems(%q) = %v, want 3 stems", composed, got)
	}
}

func TestTokenize_SplitsOnNonLetters(t *testing.T) {
	got := surfacesOf("graph-based Pt/C катализаторы, 2024г.; x86 deep_learning")
	want := []string{"graph", "катализаторы", "deep", "learning"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("surfaces = %v, want %v", got, want)
	}
}

func TestTokenize_OtherScriptsUnstemmed(t *testing.T) {
	got := Tokenize("αλγόριθμος")
	if len(got) != 1 || got[0].Stem != got[0].Surface {
		t.Errorf("Tokenize(greek) = %v, want one unstemmed token", got)
	}
	// Homoglyph mix (Cyrillic word with a Latin "e") is left as is.
	mixed := Tokenize("нeйрон")
	if len(mixed) != 1 || mixed[0].Stem != "нeйрон" {
		t.Errorf("Tokenize(mixed script) = %v, want unstemmed", mixed)
	}
}

func TestTokenize_DeterministicAndConcurrent(t *testing.T) {
	text := "Нейронные сети и deep learning для анализа донных осадков"
	want := Tokenize(text)
	var wg sync.WaitGroup
	errs := make(chan []Token, 16)
	for range 16 {
		wg.Go(func() {
			if got := Tokenize(text); !slices.Equal(got, want) {
				errs <- got
			}
		})
	}
	wg.Wait()
	close(errs)
	for got := range errs {
		t.Errorf("concurrent Tokenize = %v, want %v", got, want)
	}
}

func TestStemSetMatchesTokenize(t *testing.T) {
	// Every academic word must actually be filtered by Tokenize — guards
	// against list entries that the stemming/folding pipeline cannot match.
	for _, list := range [][]string{academicEnglish, academicRussian, academicEnglishSurfaceOnly, academicRussianSurfaceOnly} {
		for _, w := range list {
			if got := Tokenize(w); len(got) != 0 {
				t.Errorf("academic word %q not filtered: %v", w, got)
			}
		}
	}
}
