package db

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

func (d *DB) ListAxes(profileID int64) ([]Axis, error) {
	rows, err := d.Query(`SELECT id, profile_id, axis_key, description, year_min, max_per_query, position, lang_scope FROM axes WHERE profile_id=? ORDER BY position, id`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Axis
	for rows.Next() {
		var a Axis
		if err := rows.Scan(&a.ID, &a.ProfileID, &a.AxisKey, &a.Description, &a.YearMin, &a.MaxPerQuery, &a.Position, &a.LangScope); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load queries and keywords for each axis.
	for i := range out {
		out[i].Queries, err = d.listQueries(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Keywords, err = d.listKeywords(out[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (d *DB) GetAxis(id int64) (*Axis, error) {
	var a Axis
	err := d.QueryRow(`SELECT id, profile_id, axis_key, description, year_min, max_per_query, position, lang_scope FROM axes WHERE id=?`, id).
		Scan(&a.ID, &a.ProfileID, &a.AxisKey, &a.Description, &a.YearMin, &a.MaxPerQuery, &a.Position, &a.LangScope)
	if err != nil {
		return nil, err
	}
	a.Queries, err = d.listQueries(a.ID)
	if err != nil {
		return nil, err
	}
	a.Keywords, err = d.listKeywords(a.ID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// LangScopes lists the supported axis language scopes.
var LangScopes = []string{"en", "ru"}

// NormalizeLangScope lowercases and trims an axis language scope, maps empty
// to the "en" default, and rejects values outside LangScopes.
func NormalizeLangScope(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "en", nil
	}
	if !slices.Contains(LangScopes, s) {
		return "", fmt.Errorf("invalid lang_scope %q (want one of %s)", s, strings.Join(LangScopes, ", "))
	}
	return s, nil
}

// SaveAxis creates or updates an axis with its queries and keywords.
// If a.ID == 0, a new axis is created. Otherwise, it updates the existing one.
// Queries and keywords are replaced entirely (delete + re-insert).
func (d *DB) SaveAxis(a *Axis) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := saveAxisTx(tx, a); err != nil {
		return err
	}
	return tx.Commit()
}

// AppendAxes inserts new axes for a profile in a single transaction, placing
// them after the profile's existing axes (positions continue from the current
// max). Either all axes are saved or none — a duplicate axis_key aborts the
// whole batch. Input IDs are ignored; IDs are filled in on success.
func (d *DB) AppendAxes(profileID int64, axes []Axis) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var maxPos sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(position) FROM axes WHERE profile_id=?`, profileID).Scan(&maxPos); err != nil {
		return fmt.Errorf("max axis position: %w", err)
	}
	next := 0
	if maxPos.Valid {
		next = int(maxPos.Int64) + 1
	}

	for i := range axes {
		axes[i].ID = 0
		axes[i].ProfileID = profileID
		axes[i].Position = next + i
		if err := saveAxisTx(tx, &axes[i]); err != nil {
			return fmt.Errorf("save axis %q: %w", axes[i].AxisKey, err)
		}
	}
	return tx.Commit()
}

// saveAxisTx creates or updates an axis with its queries and keywords inside
// the given transaction.
func saveAxisTx(tx *sql.Tx, a *Axis) error {
	// Normalize lang_scope so the column's NOT NULL invariant holds, empty
	// (unset by older callers/UI) maps to the English default, and a typo such
	// as "fr" is rejected instead of silently matching no search provider.
	scope, err := NormalizeLangScope(a.LangScope)
	if err != nil {
		return err
	}
	a.LangScope = scope

	if a.ID == 0 {
		// Insert new axis.
		res, err := tx.Exec(`INSERT INTO axes (profile_id, axis_key, description, year_min, max_per_query, position, lang_scope) VALUES (?,?,?,?,?,?,?)`,
			a.ProfileID, a.AxisKey, a.Description, a.YearMin, a.MaxPerQuery, a.Position, a.LangScope)
		if err != nil {
			return fmt.Errorf("insert axis: %w", err)
		}
		a.ID, _ = res.LastInsertId()
	} else {
		// Update existing axis.
		_, err := tx.Exec(`UPDATE axes SET axis_key=?, description=?, year_min=?, max_per_query=?, position=?, lang_scope=? WHERE id=?`,
			a.AxisKey, a.Description, a.YearMin, a.MaxPerQuery, a.Position, a.LangScope, a.ID)
		if err != nil {
			return fmt.Errorf("update axis: %w", err)
		}
	}

	// Replace queries.
	if _, err := tx.Exec(`DELETE FROM queries WHERE axis_id=?`, a.ID); err != nil {
		return err
	}
	for i, q := range a.Queries {
		res, err := tx.Exec(`INSERT INTO queries (axis_id, text, position) VALUES (?,?,?)`, a.ID, q.Text, i)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		a.Queries[i].ID = id
		a.Queries[i].AxisID = a.ID
		a.Queries[i].Position = i
	}

	// Replace keywords.
	if _, err := tx.Exec(`DELETE FROM keywords WHERE axis_id=?`, a.ID); err != nil {
		return err
	}
	for i, k := range a.Keywords {
		res, err := tx.Exec(`INSERT INTO keywords (axis_id, word, type) VALUES (?,?,?)`, a.ID, k.Word, k.Type)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		a.Keywords[i].ID = id
		a.Keywords[i].AxisID = a.ID
	}
	return nil
}

// DeleteAxis removes an axis. papers.axis_id references axes(id) without
// ON DELETE, so papers found by this axis are detached (axis_id=NULL) first;
// otherwise foreign_keys=ON makes the delete fail. paper_axes, queries and
// keywords rows cascade.
func (d *DB) DeleteAxis(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE papers SET axis_id=NULL WHERE axis_id=?`, id); err != nil {
		return fmt.Errorf("detach papers: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM axes WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete axis: %w", err)
	}
	return tx.Commit()
}

// ReorderAxes updates the position field for axes in the given order.
func (d *DB) ReorderAxes(profileID int64, axisIDs []int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for pos, id := range axisIDs {
		if _, err := tx.Exec(`UPDATE axes SET position=? WHERE id=? AND profile_id=?`, pos, id, profileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) listQueries(axisID int64) ([]Query, error) {
	rows, err := d.Query(`SELECT id, axis_id, text, position FROM queries WHERE axis_id=? ORDER BY position`, axisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Query
	for rows.Next() {
		var q Query
		if err := rows.Scan(&q.ID, &q.AxisID, &q.Text, &q.Position); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (d *DB) listKeywords(axisID int64) ([]Keyword, error) {
	rows, err := d.Query(`SELECT id, axis_id, word, type FROM keywords WHERE axis_id=? ORDER BY id`, axisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Keyword
	for rows.Next() {
		var k Keyword
		if err := rows.Scan(&k.ID, &k.AxisID, &k.Word, &k.Type); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CountPapersByAxis returns paper count for a given axis. Used by stats.
func (d *DB) CountPapersByAxis(profileID int64) (map[int64]int, error) {
	rows, err := d.Query(`SELECT axis_id, COUNT(*) FROM papers WHERE profile_id=? AND axis_id IS NOT NULL GROUP BY axis_id`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]int)
	for rows.Next() {
		var axisID sql.NullInt64
		var count int
		if err := rows.Scan(&axisID, &count); err != nil {
			return nil, err
		}
		if axisID.Valid {
			out[axisID.Int64] = count
		}
	}
	return out, rows.Err()
}
