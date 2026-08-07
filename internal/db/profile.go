package db

import "fmt"

func (d *DB) ListProfiles() ([]Profile, error) {
	rows, err := d.Query(`SELECT id, name, email, pdf_dir, year_min, vintage_year, max_per_query, download_sources, created_at FROM search_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.PdfDir, &p.YearMin, &p.VintageYear, &p.MaxPerQuery, &p.DownloadSources, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetProfile(id int64) (*Profile, error) {
	var p Profile
	err := d.QueryRow(`SELECT id, name, email, pdf_dir, year_min, vintage_year, max_per_query, download_sources, created_at FROM search_profiles WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Email, &p.PdfDir, &p.YearMin, &p.VintageYear, &p.MaxPerQuery, &p.DownloadSources, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) CreateProfile(p *Profile) error {
	res, err := d.Exec(`INSERT INTO search_profiles (name, email, pdf_dir, year_min, vintage_year, max_per_query, download_sources) VALUES (?,?,?,?,?,?,?)`,
		p.Name, p.Email, p.PdfDir, p.YearMin, p.VintageYear, p.MaxPerQuery, p.DownloadSources)
	if err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (d *DB) UpdateProfile(p *Profile) error {
	_, err := d.Exec(`UPDATE search_profiles SET name=?, email=?, pdf_dir=?, year_min=?, vintage_year=?, max_per_query=?, download_sources=? WHERE id=?`,
		p.Name, p.Email, p.PdfDir, p.YearMin, p.VintageYear, p.MaxPerQuery, p.DownloadSources, p.ID)
	return err
}

func (d *DB) DeleteProfile(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete downloads for all papers in this profile.
	if _, err := tx.Exec(`DELETE FROM downloads WHERE paper_id IN (SELECT id FROM papers WHERE profile_id=?)`, id); err != nil {
		return fmt.Errorf("delete downloads: %w", err)
	}
	// Delete papers (no ON DELETE CASCADE on profile_id).
	if _, err := tx.Exec(`DELETE FROM papers WHERE profile_id=?`, id); err != nil {
		return fmt.Errorf("delete papers: %w", err)
	}
	// Delete profile — axes/queries/keywords cascade via ON DELETE CASCADE.
	if _, err := tx.Exec(`DELETE FROM search_profiles WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}

	return tx.Commit()
}

// ProfileStats returns counts of axes and papers for a profile (used for delete confirmation).
func (d *DB) ProfileStats(id int64) (axes int, papers int, err error) {
	err = d.QueryRow(`SELECT COUNT(*) FROM axes WHERE profile_id=?`, id).Scan(&axes)
	if err != nil {
		return
	}
	err = d.QueryRow(`SELECT COUNT(*) FROM papers WHERE profile_id=?`, id).Scan(&papers)
	return
}
