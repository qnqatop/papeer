package app

import (
	"strings"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/topics"
)

// GetTopics clusters the profile's approved/downloaded papers into topics
// using TF-IDF + K-Means over title+abstract (see internal/topics). k<=0
// auto-selects a cluster count (8-15, fewer for small corpora). Runs
// entirely locally — no network calls, no LLM.
func (a *App) GetTopics(profileID int64, k int) ([]topics.Topic, error) {
	papers, err := a.approvedAndDownloadedFull(profileID)
	if err != nil {
		return nil, err
	}

	docs := make([]topics.Document, 0, len(papers))
	for _, p := range papers {
		text := strings.TrimSpace(p.Title + " " + p.Abstract)
		if text == "" {
			continue
		}
		docs = append(docs, topics.Document{ID: p.ID, Text: text})
	}
	if len(docs) == 0 {
		return nil, nil
	}

	return topics.Cluster(docs, k)
}

// GetTopicsForReview clusters the profile's approved/downloaded papers into
// topics using summary text (when available) instead of title+abstract for
// finer clustering. Falls back to title+abstract when no summary exists.
func (a *App) GetTopicsForReview(profileID int64, model string, k int) ([]topics.Topic, error) {
	papers, err := a.approvedAndDownloadedFull(profileID)
	if err != nil {
		return nil, err
	}

	pids := make([]int64, len(papers))
	for i, p := range papers {
		pids[i] = p.ID
	}
	summariesByPaper, _ := a.db.GetSummariesByPaperIDs(pids, model)

	docs := make([]topics.Document, 0, len(papers))
	for _, p := range papers {
		text := strings.TrimSpace(p.Title + " " + p.Abstract)
		if sum, ok := summariesByPaper[p.ID]; ok && strings.TrimSpace(sum) != "" {
			text = strings.TrimSpace(p.Title + " " + sum)
		}
		if text == "" {
			continue
		}
		docs = append(docs, topics.Document{ID: p.ID, Text: text})
	}
	if len(docs) == 0 {
		return nil, nil
	}

	return topics.Cluster(docs, k)
}

// approvedAndDownloadedFull returns full paper records (including abstract,
// venue, etc.) for status IN (approved, downloaded) — the same eligibility
// set as FetchCitations, but via ListPapers so all columns are populated
// (db.GetApprovedAndDownloadedPapers only selects the handful of columns the
// citation worker needs). Shared by Topics and Key Papers.
func (a *App) approvedAndDownloadedFull(profileID int64) ([]db.Paper, error) {
	approved, err := a.db.GetApprovedPapers(profileID)
	if err != nil {
		return nil, err
	}
	downloaded, _, err := a.db.ListPapers(db.PaperFilter{
		ProfileID: profileID,
		Status:    "downloaded",
		Limit:     10000,
	})
	if err != nil {
		return nil, err
	}
	return append(approved, downloaded...), nil
}
