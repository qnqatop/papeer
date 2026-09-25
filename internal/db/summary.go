package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// CreateSummary inserts a new summary. Returns error on UNIQUE conflict.
func (d *DB) CreateSummary(s *Summary) error {
	res, err := d.Exec(`INSERT INTO summaries (paper_id, provider, model, prompt_hash, content, tokens_in, tokens_out, status, error_msg) VALUES (?,?,?,?,?,?,?,?,?)`,
		s.PaperID, s.Provider, s.Model, s.PromptHash, s.Content, s.TokensIn, s.TokensOut, s.Status, s.ErrorMsg)
	if err != nil {
		return fmt.Errorf("create summary: %w", err)
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

// UpdateSummary updates content, tokens, status, error_msg, and updated_at.
func (d *DB) UpdateSummary(s *Summary) error {
	_, err := d.Exec(`UPDATE summaries SET content=?, tokens_in=?, tokens_out=?, status=?, error_msg=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		s.Content, s.TokensIn, s.TokensOut, s.Status, s.ErrorMsg, s.ID)
	return err
}

// UpsertSummary inserts or updates (for regeneration).
func (d *DB) UpsertSummary(s *Summary) error {
	// RETURNING yields the row id on both the insert and the conflict-update
	// path (LastInsertId is stale on the update path).
	err := d.QueryRow(`INSERT INTO summaries (paper_id, provider, model, prompt_hash, content, tokens_in, tokens_out, status, error_msg)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(paper_id, model) DO UPDATE SET
			content=excluded.content, tokens_in=excluded.tokens_in, tokens_out=excluded.tokens_out,
			status=excluded.status, error_msg=excluded.error_msg, updated_at=CURRENT_TIMESTAMP
		RETURNING id`,
		s.PaperID, s.Provider, s.Model, s.PromptHash, s.Content, s.TokensIn, s.TokensOut, s.Status, s.ErrorMsg).Scan(&s.ID)
	if err != nil {
		return fmt.Errorf("upsert summary: %w", err)
	}
	return nil
}

// GetSummary returns a summary by paper_id and model.
func (d *DB) GetSummary(paperID int64, model string) (*Summary, error) {
	var s Summary
	err := d.QueryRow(`SELECT id, paper_id, provider, model, prompt_hash, content, tokens_in, tokens_out, status, error_msg, created_at, updated_at
		FROM summaries WHERE paper_id=? AND model=?`, paperID, model).
		Scan(&s.ID, &s.PaperID, &s.Provider, &s.Model, &s.PromptHash, &s.Content, &s.TokensIn, &s.TokensOut, &s.Status, &s.ErrorMsg, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSummariesByPaper returns all summaries for a paper.
func (d *DB) GetSummariesByPaper(paperID int64) ([]Summary, error) {
	rows, err := d.Query(`SELECT id, paper_id, provider, model, prompt_hash, content, tokens_in, tokens_out, status, error_msg, created_at, updated_at
		FROM summaries WHERE paper_id=? ORDER BY provider`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.PaperID, &s.Provider, &s.Model, &s.PromptHash, &s.Content, &s.TokensIn, &s.TokensOut, &s.Status, &s.ErrorMsg, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSummariesByProfile returns all summaries for papers in a profile.
func (d *DB) ListSummariesByProfile(profileID int64) ([]SummaryWithPaper, error) {
	rows, err := d.Query(`SELECT s.id, s.paper_id, s.provider, s.model, s.prompt_hash, s.content, s.tokens_in, s.tokens_out, s.status, s.error_msg, s.created_at, s.updated_at,
		p.title, p.year, p.authors, p.axis_id, COALESCE(a.axis_key, '')
		FROM summaries s JOIN papers p ON s.paper_id = p.id
		LEFT JOIN axes a ON p.axis_id = a.id
		WHERE p.profile_id = ?
		ORDER BY s.created_at DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SummaryWithPaper
	for rows.Next() {
		var sw SummaryWithPaper
		if err := rows.Scan(&sw.ID, &sw.PaperID, &sw.Provider, &sw.Model, &sw.PromptHash, &sw.Content, &sw.TokensIn, &sw.TokensOut, &sw.Status, &sw.ErrorMsg, &sw.CreatedAt, &sw.UpdatedAt,
			&sw.PaperTitle, &sw.PaperYear, &sw.PaperAuthors, &sw.AxisID, &sw.AxisKey); err != nil {
			return nil, err
		}
		out = append(out, sw)
	}
	return out, rows.Err()
}

// DeleteSummary deletes a summary by ID.
func (d *DB) DeleteSummary(id int64) error {
	_, err := d.Exec(`DELETE FROM summaries WHERE id=?`, id)
	return err
}

// GetSummariesByPaperIDs returns a map paperID → content for summaries with
// status='done' and the given model, for the specified paper IDs.
func (d *DB) GetSummariesByPaperIDs(paperIDs []int64, model string) (map[int64]string, error) {
	if len(paperIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(paperIDs))
	args := make([]interface{}, 0, len(paperIDs)+1)
	args = append(args, model)
	for i, id := range paperIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := d.Query(
		`SELECT paper_id, content FROM summaries WHERE model=? AND status='done' AND paper_id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]string)
	for rows.Next() {
		var paperID int64
		var content string
		if err := rows.Scan(&paperID, &content); err != nil {
			return nil, err
		}
		out[paperID] = content
	}
	return out, rows.Err()
}

// HasActiveSummary returns the current status for a (paper_id, model) pair, or empty string if not found.
func (d *DB) HasActiveSummary(paperID int64, model string) (string, error) {
	var status string
	err := d.QueryRow(`SELECT status FROM summaries WHERE paper_id=? AND model=?`, paperID, model).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return status, err
}
