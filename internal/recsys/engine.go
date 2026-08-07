package recsys

import (
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/bbalet/stopwords"
)

// rgx удаляет всё кроме букв (Unicode), цифр и пробелов.
var rgx = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// CleanText очищает текст: нижний регистр, удаление знаков и стоп-слов.
// Поддерживает латиницу и кириллицу.
func CleanText(text string) []string {
	text = strings.Map(unicode.ToLower, text)
	text = rgx.ReplaceAllString(text, " ")
	// Удаляем английские стоп-слова
	clean := stopwords.CleanString(text, "en", false)
	return strings.Fields(clean)
}

// TFIDFEngine хранит статистику корпуса для расчёта IDF.
type TFIDFEngine struct {
	TotalDocs int
	DF        map[string]int // Document Frequency: в скольких документах встречается слово
}

func NewTFIDFEngine() *TFIDFEngine {
	return &TFIDFEngine{
		DF: make(map[string]int),
	}
}

// AddDocument добавляет токены документа в статистику корпуса.
func (e *TFIDFEngine) AddDocument(tokens []string) {
	e.TotalDocs++
	seen := make(map[string]bool)
	for _, token := range tokens {
		if !seen[token] {
			e.DF[token]++
			seen[token] = true
		}
	}
}

// Vectorize превращает массив слов в вектор TF-IDF.
// Использует sublinear TF: 1 + log(count) — даёт лучшую дискриминацию на коротких текстах.
func (e *TFIDFEngine) Vectorize(tokens []string) map[string]float64 {
	vector := make(map[string]float64)
	if len(tokens) == 0 {
		return vector
	}

	termCounts := make(map[string]int)
	for _, token := range tokens {
		termCounts[token]++
	}

	for term, count := range termCounts {
		// Sublinear TF: 1 + log(count) вместо count/total.
		// Одно вхождение даёт TF=1, два — 1.69, три — 2.1 и т.д.
		tf := 1.0 + math.Log(float64(count))

		// Smooth IDF: log(1 + N/df)
		df := e.DF[term]
		if df == 0 {
			df = 1
		}
		idf := math.Log(1.0+float64(e.TotalDocs)/float64(df)) + 1.0

		vector[term] = tf * idf
	}
	return vector
}

// GetCentroid вычисляет усреднённый вектор (центроид) из массива векторов.
func GetCentroid(vectors []map[string]float64) map[string]float64 {
	centroid := make(map[string]float64)
	if len(vectors) == 0 {
		return centroid
	}

	for _, vec := range vectors {
		for term, weight := range vec {
			centroid[term] += weight
		}
	}

	n := float64(len(vectors))
	for term := range centroid {
		centroid[term] /= n
	}
	return centroid
}

// CosineSimilarity возвращает косинусное сходство между векторами (0.0 — 1.0).
func CosineSimilarity(v1, v2 map[string]float64) float64 {
	var dotProduct, normV1, normV2 float64

	for term, w1 := range v1 {
		w2 := v2[term]
		dotProduct += w1 * w2
		normV1 += w1 * w1
	}

	for _, w2 := range v2 {
		normV2 += w2 * w2
	}

	if normV1 == 0 || normV2 == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normV1) * math.Sqrt(normV2))
}

// JaccardSimilarity считает пересечение/объединение множеств токенов (0.0 — 1.0).
func JaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// TokenSet конвертирует слайс токенов в множество.
func TokenSet(tokens []string) map[string]bool {
	s := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		s[t] = true
	}
	return s
}

// SigmoidStretch нелинейно масштабирует значение из [0,1] так, чтобы
// средние значения (0.15–0.4) растягивались в более широкий диапазон.
// midpoint — точка перегиба, steepness — крутизна.
func SigmoidStretch(x, midpoint, steepness float64) float64 {
	return 1.0 / (1.0 + math.Exp(-steepness*(x-midpoint)))
}
