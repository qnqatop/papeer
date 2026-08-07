package db

import (
	"database/sql"
	"fmt"
)

const llmProfileCols = `id, name, base_url, models, default_model, temperature, is_active, created_at`

func scanLLMProfile(s interface {
	Scan(dest ...any) error
}) (*LLMProfile, error) {
	var p LLMProfile
	if err := s.Scan(&p.ID, &p.Name, &p.BaseURL, &p.Models, &p.DefaultModel, &p.Temperature, &p.IsActive, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListLLMProfiles returns all LLM profiles ordered by id.
func (d *DB) ListLLMProfiles() ([]LLMProfile, error) {
	rows, err := d.Query(`SELECT ` + llmProfileCols + ` FROM llm_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LLMProfile
	for rows.Next() {
		p, err := scanLLMProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetLLMProfile returns a profile by id.
func (d *DB) GetLLMProfile(id int64) (*LLMProfile, error) {
	row := d.QueryRow(`SELECT `+llmProfileCols+` FROM llm_profiles WHERE id=?`, id)
	return scanLLMProfile(row)
}

// GetActiveLLMProfile returns the currently active profile, or nil if none.
func (d *DB) GetActiveLLMProfile() (*LLMProfile, error) {
	row := d.QueryRow(`SELECT ` + llmProfileCols + ` FROM llm_profiles WHERE is_active=1`)
	p, err := scanLLMProfile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// CreateLLMProfile inserts a new profile and sets p.ID.
func (d *DB) CreateLLMProfile(p *LLMProfile) error {
	res, err := d.Exec(`INSERT INTO llm_profiles (name, base_url, models, default_model, temperature, is_active) VALUES (?,?,?,?,?,?)`,
		p.Name, p.BaseURL, p.Models, p.DefaultModel, p.Temperature, p.IsActive)
	if err != nil {
		return fmt.Errorf("insert llm profile: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// UpdateLLMProfile updates an existing profile (is_active is managed separately).
func (d *DB) UpdateLLMProfile(p *LLMProfile) error {
	_, err := d.Exec(`UPDATE llm_profiles SET name=?, base_url=?, models=?, default_model=?, temperature=? WHERE id=?`,
		p.Name, p.BaseURL, p.Models, p.DefaultModel, p.Temperature, p.ID)
	return err
}

// DeleteLLMProfile removes a profile by id.
func (d *DB) DeleteLLMProfile(id int64) error {
	_, err := d.Exec(`DELETE FROM llm_profiles WHERE id=?`, id)
	return err
}

// SetActiveLLMProfile makes the given profile the only active one, atomically.
func (d *DB) SetActiveLLMProfile(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Ensure the target exists to avoid silently activating nothing.
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM llm_profiles WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("llm profile %d not found", id)
	}

	// Clear active flag first so the partial unique index does not conflict.
	if _, err := tx.Exec(`UPDATE llm_profiles SET is_active=0 WHERE is_active=1`); err != nil {
		return fmt.Errorf("clear active: %w", err)
	}
	if _, err := tx.Exec(`UPDATE llm_profiles SET is_active=1 WHERE id=?`, id); err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return tx.Commit()
}
