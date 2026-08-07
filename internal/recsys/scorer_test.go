package recsys

import (
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

func TestCalculateSmartScores_MathWorks(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Deep Learning for Recommender Systems", Abstract: "Neural networks and embeddings for collaborative filtering", Status: "approved"},
		{ID: 2, Title: "Neural Networks in RecSys", Abstract: "Using deep learning for sparse data recommendations", Status: "approved"},
		{ID: 3, Title: "Deep Neural Models for Recommendations", Abstract: "Matrix factorization and neural networks for user preferences", Status: "approved"},

		// Похожая статья — ожидаем высокий скор
		{ID: 4, Title: "Advanced Deep Learning in RecSys", Abstract: "State of the art neural networks for recommendation engines", Status: "new"},

		// Нерелевантная статья — ожидаем низкий скор
		{ID: 5, Title: "Ancient Roman History", Abstract: "Julius Caesar and the fall of the Roman empire", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	if len(scores) != 2 {
		t.Fatalf("Ожидалось 2 оценки, получено: %d", len(scores))
	}

	scoreSimilar := scores[4]
	scoreDifferent := scores[5]

	t.Logf("Скор похожей статьи (ID 4): %d%%", scoreSimilar)
	t.Logf("Скор чужеродной статьи (ID 5): %d%%", scoreDifferent)

	if scoreSimilar <= scoreDifferent {
		t.Errorf("Алгоритм сломан: скор похожей (%d) <= чужеродной (%d)", scoreSimilar, scoreDifferent)
	}

	// Похожая статья должна иметь высокий скор (>50%)
	if scoreSimilar < 50 {
		t.Errorf("Скор похожей статьи слишком низкий: %d%% (ожидаем >50%%)", scoreSimilar)
	}

	// Нерелевантная — низкий (<40%)
	if scoreDifferent > 40 {
		t.Errorf("Скор чужеродной статьи слишком высокий: %d%% (ожидаем <40%%)", scoreDifferent)
	}
}

func TestCalculateSmartScores_RejectedPenalty(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Deep Learning for NLP", Abstract: "Transformers and attention mechanisms", Status: "approved"},
		{ID: 2, Title: "BERT and Language Models", Abstract: "Pre-training deep bidirectional transformers", Status: "approved"},
		{ID: 3, Title: "GPT Language Generation", Abstract: "Generative pre-trained transformer models", Status: "approved"},

		// Rejected — тема компьютерного зрения
		{ID: 4, Title: "Image Classification with CNNs", Abstract: "Convolutional neural networks for image recognition", Status: "rejected"},
		{ID: 5, Title: "Object Detection Deep Learning", Abstract: "Region based convolutional neural networks for objects", Status: "rejected"},

		// Новая статья, похожая на rejected тему
		{ID: 6, Title: "Visual Object Recognition with CNNs", Abstract: "Deep convolutional networks for image classification", Status: "new"},

		// Новая статья, похожая на approved тему
		{ID: 7, Title: "Attention Mechanisms in NLP", Abstract: "Self-attention and transformer architectures for language", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	scoreRejLike := scores[6]
	scoreAppLike := scores[7]

	t.Logf("Скор статьи похожей на rejected (ID 6): %d%%", scoreRejLike)
	t.Logf("Скор статьи похожей на approved (ID 7): %d%%", scoreAppLike)

	if scoreAppLike <= scoreRejLike {
		t.Errorf("Штраф за rejected не работает: approved-like (%d) <= rejected-like (%d)", scoreAppLike, scoreRejLike)
	}
}

func TestCalculateSmartScores_BoostKeywords(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Machine Learning Basics", Abstract: "Introduction to supervised learning algorithms", Status: "approved"},
		{ID: 2, Title: "Statistical Learning Theory", Abstract: "Foundations of machine learning and generalization", Status: "approved"},
		{ID: 3, Title: "Applied Machine Learning", Abstract: "Practical applications of learning algorithms", Status: "approved"},

		{ID: 4, Title: "Reinforcement Learning for Robotics", Abstract: "Robot control using reinforcement learning", Status: "new"},
		{ID: 5, Title: "Cooking Italian Pasta", Abstract: "Traditional recipes for Italian cuisine", Status: "new"},
	}

	boosts := []string{"reinforcement", "robotics", "control"}
	scores := CalculateSmartScores(papers, boosts)

	scoreBoosted := scores[4]
	scoreIrrelevant := scores[5]

	t.Logf("Скор boosted статьи (ID 4): %d%%", scoreBoosted)
	t.Logf("Скор нерелевантной (ID 5): %d%%", scoreIrrelevant)

	if scoreBoosted <= scoreIrrelevant {
		t.Errorf("Boost не работает: boosted (%d) <= irrelevant (%d)", scoreBoosted, scoreIrrelevant)
	}
}

func TestCalculateSmartScores_ColdStart(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Paper A", Status: "approved"},
		{ID: 2, Title: "Paper B", Status: "approved"},
		{ID: 3, Title: "Paper C", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	if len(scores) != 0 {
		t.Errorf("Ожидалась пустая мапа из-за холодного старта, получено: %d", len(scores))
	}
}

// Реалистичный сценарий: диссертация по рекомендательным системам.
// Много approved, несколько rejected, спектр новых от очень похожих до мусора.
func TestCalculateSmartScores_RealisticCorpus(t *testing.T) {
	papers := []db.Paper{
		// ── Approved: рекомендательные системы ──
		{ID: 1, Title: "Collaborative Filtering for Implicit Feedback Datasets", Abstract: "We present a scalable approach to collaborative filtering using implicit feedback signals such as clicks and views for large-scale recommender systems", Status: "approved"},
		{ID: 2, Title: "Deep Learning based Recommender System: A Survey and New Perspectives", Abstract: "This survey reviews recent advances in deep learning based recommender systems including autoencoders recurrent neural networks and attention mechanisms", Status: "approved"},
		{ID: 3, Title: "Neural Collaborative Filtering", Abstract: "We propose a neural network architecture to model user-item interactions by replacing inner product with a multi-layer perceptron for collaborative filtering", Status: "approved"},
		{ID: 4, Title: "Matrix Factorization Techniques for Recommender Systems", Abstract: "Matrix factorization methods decompose the user-item interaction matrix into low dimensional latent factors capturing preferences and item characteristics", Status: "approved"},
		{ID: 5, Title: "Wide and Deep Learning for Recommender Systems", Abstract: "We present jointly trained wide linear models and deep neural networks combining memorization and generalization for app recommendations", Status: "approved"},
		{ID: 6, Title: "AutoRec: Autoencoders Meet Collaborative Filtering", Abstract: "We propose AutoRec a novel autoencoder framework for collaborative filtering that learns latent representations of users or items", Status: "approved"},
		{ID: 7, Title: "Session-Based Recommendations with Recurrent Neural Networks", Abstract: "We apply recurrent neural networks to session-based recommendation modeling whole sessions to predict user clicks", Status: "approved"},

		// ── Rejected: компьютерное зрение (не по теме) ──
		{ID: 8, Title: "ImageNet Large Scale Visual Recognition Challenge", Abstract: "The ImageNet challenge has driven advances in object recognition and image classification using convolutional neural networks", Status: "rejected"},
		{ID: 9, Title: "YOLO Real-Time Object Detection", Abstract: "You only look once is a unified approach to object detection framing it as a single regression problem from image pixels to bounding boxes", Status: "rejected"},

		// ── New: спектр релевантности ──
		// Очень похожа (прямо в тему рекомендаций)
		{ID: 10, Title: "Graph Neural Networks for Social Recommendation", Abstract: "We propose a graph neural network framework for social recommendation that captures user-item interactions and social influence through graph convolutions", Status: "new"},
		// Частично похожа (ML общего плана, но не рекомендации)
		{ID: 11, Title: "Attention Is All You Need", Abstract: "We propose the transformer architecture based entirely on attention mechanisms dispensing with recurrence and convolutions for sequence transduction", Status: "new"},
		// Слабо похожа (NLP, далеко от рекомендаций)
		{ID: 12, Title: "BERT Pre-training of Deep Bidirectional Transformers", Abstract: "We introduce BERT a method for pre-training language representations that obtains state-of-the-art results on eleven natural language processing tasks", Status: "new"},
		// Похожа на rejected (CV тема)
		{ID: 13, Title: "EfficientNet Rethinking Model Scaling for CNNs", Abstract: "We propose a systematic model scaling method for convolutional neural networks that uniformly scales depth width and resolution for image classification", Status: "new"},
		// Полный мусор
		{ID: 14, Title: "Impact of Climate Change on Coral Reef Ecosystems", Abstract: "Rising ocean temperatures and acidification threaten coral reef biodiversity affecting marine species and coastal communities worldwide", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	if len(scores) != 5 {
		t.Fatalf("Ожидалось 5 оценок, получено: %d", len(scores))
	}

	t.Logf("=== Распределение скоров ===")
	names := map[int64]string{
		10: "Graph NN for Recommendations (прямо в тему)",
		11: "Attention Is All You Need   (ML общий)",
		12: "BERT                        (NLP)",
		13: "EfficientNet CNNs           (CV, ~rejected)",
		14: "Coral Reefs                 (мусор)",
	}
	for _, id := range []int64{10, 11, 12, 13, 14} {
		t.Logf("  ID %d: %3d%% — %s", id, scores[id], names[id])
	}

	// Ранжирование должно быть правильным
	if scores[10] <= scores[11] {
		t.Errorf("Рек.системы (%d) должны быть выше общего ML (%d)", scores[10], scores[11])
	}
	if scores[11] <= scores[14] {
		t.Errorf("Общий ML (%d) должен быть выше мусора (%d)", scores[11], scores[14])
	}
	if scores[10] <= scores[13] {
		t.Errorf("Рек.системы (%d) должны быть выше CV/rejected-like (%d)", scores[10], scores[13])
	}

	// Rejected penalty: CV-статья (EfficientNet) должна получить ниже чем ML (Attention).
	// NB: EfficientNet > BERT — корректно, т.к. CNN-статья делит словарь "neural networks",
	// "deep learning" с approved, а BERT — нет. Bag-of-words не различает тематику слов.
	if scores[13] >= scores[11] {
		t.Errorf("CV/rejected-like (%d) должен быть ниже общего ML (%d) из-за rejected penalty", scores[13], scores[11])
	}

	// Абсолютные диапазоны
	if scores[10] < 40 {
		t.Errorf("Прямое попадание слишком низко: %d%% (ожидаем >40%%)", scores[10])
	}
	if scores[14] > 20 {
		t.Errorf("Мусор слишком высоко: %d%% (ожидаем <20%%)", scores[14])
	}
}

// Граничный случай: ровно 3 approved, все одинаковые.
func TestCalculateSmartScores_MinimalApproved(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Kubernetes Container Orchestration", Abstract: "Automating deployment scaling and management of containerized applications", Status: "approved"},
		{ID: 2, Title: "Docker Container Management", Abstract: "Container runtime and orchestration for microservices deployment", Status: "approved"},
		{ID: 3, Title: "Cloud Native Container Platforms", Abstract: "Orchestrating containers at scale in cloud environments for deployment", Status: "approved"},

		{ID: 4, Title: "Serverless Container Orchestration", Abstract: "Managing containers without dedicated infrastructure for deployment", Status: "new"},
		{ID: 5, Title: "Quantum Computing Algorithms", Abstract: "Shor algorithm and quantum error correction for cryptography", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	t.Logf("Container статья: %d%%, Quantum: %d%%", scores[4], scores[5])

	if scores[4] <= scores[5] {
		t.Errorf("Container (%d) должен быть выше Quantum (%d)", scores[4], scores[5])
	}
	if scores[4] < 40 {
		t.Errorf("При 3 approved прямое попадание слишком низко: %d%%", scores[4])
	}
}

// Все new статьи одинаково похожи — скоры должны быть примерно равными.
func TestCalculateSmartScores_UniformSimilarity(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Database Query Optimization", Abstract: "Cost-based query optimization in relational database management systems", Status: "approved"},
		{ID: 2, Title: "SQL Query Planning and Execution", Abstract: "Efficient query plans for relational database queries", Status: "approved"},
		{ID: 3, Title: "Indexing Strategies for Databases", Abstract: "B-tree and hash indexing for faster query execution in databases", Status: "approved"},

		{ID: 4, Title: "Adaptive Query Processing in Databases", Abstract: "Runtime query optimization for relational database workloads", Status: "new"},
		{ID: 5, Title: "Query Optimization Using Machine Learning", Abstract: "Learned query optimizers for relational database systems", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	t.Logf("Adaptive Query: %d%%, ML Query Opt: %d%%", scores[4], scores[5])

	diff := scores[4] - scores[5]
	if diff < 0 {
		diff = -diff
	}
	// Обе статьи по теме — разница не должна быть огромной
	if diff > 30 {
		t.Errorf("Похожие статьи получили слишком разные скоры: %d vs %d (diff=%d)", scores[4], scores[5], diff)
	}
	// Обе должны быть достаточно высоко
	if scores[4] < 30 || scores[5] < 30 {
		t.Errorf("Релевантные статьи получили слишком низкие скоры: %d, %d", scores[4], scores[5])
	}
}

// Только new статьи (нет rejected) — алгоритм не должен падать.
func TestCalculateSmartScores_NoRejected(t *testing.T) {
	papers := []db.Paper{
		{ID: 1, Title: "Distributed Systems Consensus", Abstract: "Paxos and Raft consensus protocols for distributed computing", Status: "approved"},
		{ID: 2, Title: "Byzantine Fault Tolerance", Abstract: "Practical BFT protocols for distributed systems", Status: "approved"},
		{ID: 3, Title: "Consistency Models in Distributed Databases", Abstract: "Eventual and strong consistency in distributed data stores", Status: "approved"},

		{ID: 4, Title: "Raft Consensus Implementation", Abstract: "Implementing Raft consensus for replicated state machines", Status: "new"},
	}

	scores := CalculateSmartScores(papers, nil)

	t.Logf("Raft consensus (no rejected): %d%%", scores[4])

	if scores[4] < 40 {
		t.Errorf("Без rejected penalty скор должен быть выше: %d%%", scores[4])
	}
}

func TestCleanText_Unicode(t *testing.T) {
	tokens := CleanText("Нейронные сети для рекомендаций: обзор 2024")
	if len(tokens) == 0 {
		t.Error("CleanText убил кириллицу — токенов 0")
	}
	t.Logf("Токены: %v", tokens)

	// Проверяем что латиница тоже работает
	tokensEn := CleanText("Deep Learning for Recommendations")
	if len(tokensEn) == 0 {
		t.Error("CleanText сломал латиницу")
	}
}
