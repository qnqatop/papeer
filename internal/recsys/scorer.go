package recsys

import (
	"github.com/qnqatop/papeer/internal/db"
	"math"
	"strings"
)

const (
	// Веса сигналов в финальном скоре.
	weightCosine  = 0.60 // TF-IDF cosine similarity к центроиду approved
	weightJaccard = 0.25 // Jaccard overlap ключевых слов
	weightPenalty = 0.15 // Штраф за близость к rejected статьям
)

// CalculateSmartScores принимает статьи одной оси и возвращает map[paperID]Score.
// Score — мульти-сигнальный процент совпадения (от 1 до 99).
//
// Алгоритм:
//  1. TF-IDF (sublinear TF) + cosine similarity к центроиду approved статей
//  2. Jaccard overlap — прямое пересечение словарей
//  3. Rejected penalty — штраф за близость к отклонённым
//  4. Anchor normalization — нормализация относительно approved (их средний raw = эталонные 85%)
func CalculateSmartScores(papers []db.Paper, boostKeywords []string) map[int64]int {
	engine := NewTFIDFEngine()

	// 1. Токенизация и построение IDF.
	docs := make(map[int64][]string)
	for _, p := range papers {
		text := p.Title + " " + p.Abstract
		tokens := CleanText(text)
		docs[p.ID] = tokens
		engine.AddDocument(tokens)
	}

	// 2. Векторизация + разделение по статусам.
	var approvedVectors []map[string]float64
	var rejectedVectors []map[string]float64
	var approvedTokenSets []map[string]bool
	vectors := make(map[int64]map[string]float64)

	for _, p := range papers {
		vec := engine.Vectorize(docs[p.ID])
		vectors[p.ID] = vec
		switch p.Status {
		case "approved":
			approvedVectors = append(approvedVectors, vec)
			approvedTokenSets = append(approvedTokenSets, TokenSet(docs[p.ID]))
		case "rejected":
			rejectedVectors = append(rejectedVectors, vec)
		}
	}

	scores := make(map[int64]int)

	// Холодный старт: нужно минимум 3 approved.
	if len(approvedVectors) < 3 {
		return scores
	}

	// 3. Центроид approved статей.
	approvedCentroid := GetCentroid(approvedVectors)

	// Boost keywords: подмешиваем с весом 20%.
	if len(boostKeywords) > 0 {
		boostTokens := CleanText(strings.Join(boostKeywords, " "))
		boostVec := engine.Vectorize(boostTokens)
		for term, weight := range boostVec {
			approvedCentroid[term] += weight * 0.2
		}
	}

	// 4. Объединённое множество токенов approved (для Jaccard).
	mergedApprovedTokens := make(map[string]bool)
	for _, ts := range approvedTokenSets {
		for t := range ts {
			mergedApprovedTokens[t] = true
		}
	}

	// 5. Центроид rejected (если есть — для штрафа).
	var rejectedCentroid map[string]float64
	if len(rejectedVectors) >= 2 {
		rejectedCentroid = GetCentroid(rejectedVectors)
	}

	// 6. Вычисляем anchor — средний raw-скор approved статей к центроиду.
	// Это наш "эталонный максимум": approved статья ≈ 85%.
	var anchorSum float64
	for i, vec := range approvedVectors {
		cs := CosineSimilarity(approvedCentroid, vec)
		js := JaccardSimilarity(mergedApprovedTokens, approvedTokenSets[i])
		anchorSum += cs*weightCosine + js*weightJaccard
	}
	anchor := anchorSum / float64(len(approvedVectors))
	if anchor < 0.01 {
		anchor = 0.01
	}

	// 7. Скоринг новых статей.
	for _, p := range papers {
		if p.Status != "new" {
			continue
		}

		// Сигнал 1: TF-IDF cosine к approved центроиду.
		cosineSim := CosineSimilarity(approvedCentroid, vectors[p.ID])

		// Сигнал 2: Jaccard overlap токенов.
		paperTokens := TokenSet(docs[p.ID])
		jaccardSim := JaccardSimilarity(mergedApprovedTokens, paperTokens)

		// Сигнал 3: Штраф за близость к rejected.
		penaltyFactor := 1.0
		if rejectedCentroid != nil {
			rejSim := CosineSimilarity(rejectedCentroid, vectors[p.ID])
			// При rejSim=0 → 1.0, rejSim=0.5 → 0.75, rejSim=1.0 → 0.5
			penaltyFactor = 1.0 - rejSim*0.5
		}

		// Комбинируем сигналы.
		raw := (cosineSim*weightCosine + jaccardSim*weightJaccard) * penaltyFactor

		// Anchor normalization: raw/anchor даёт долю от "идеальной approved".
		// New статьи обычно имеют ratio 0.15–0.6 (они не формировали центроид).
		ratio := raw / anchor

		// Sigmoid stretch: мягкая S-кривая.
		// ratio=0.05 → ~7%, ratio=0.20 → ~38%, ratio=0.35 → ~62%, ratio=0.6 → ~88%.
		stretched := SigmoidStretch(ratio, 0.25, 7.0)

		percent := int(math.Round(stretched * 100))
		if percent > 99 {
			percent = 99
		}
		if percent < 1 {
			percent = 1
		}
		scores[p.ID] = percent
	}

	return scores
}
