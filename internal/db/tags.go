package db

import (
	"fmt"
	"strings"
)

// ListTags returns all tags for a profile.
func (d *DB) ListTags(profileID int64) ([]Tag, error) {
	rows, err := d.Query(`SELECT id, profile_id, name, color FROM tags WHERE profile_id=? ORDER BY name`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.ProfileID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTag creates a new tag. Returns the created tag with ID.
func (d *DB) CreateTag(t *Tag) error {
	res, err := d.Exec(`INSERT INTO tags (profile_id, name, color) VALUES (?, ?, ?)`, t.ProfileID, t.Name, t.Color)
	if err != nil {
		return fmt.Errorf("create tag: %w", err)
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

// UpdateTag updates a tag's name and color.
func (d *DB) UpdateTag(t *Tag) error {
	_, err := d.Exec(`UPDATE tags SET name=?, color=? WHERE id=?`, t.Name, t.Color, t.ID)
	return err
}

// DeleteTag removes a tag and its paper associations.
func (d *DB) DeleteTag(id int64) error {
	_, err := d.Exec(`DELETE FROM tags WHERE id=?`, id)
	return err
}

// AddTagToPaper links a tag to a paper.
func (d *DB) AddTagToPaper(tagID, paperID int64) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO paper_tags (tag_id, paper_id) VALUES (?, ?)`, tagID, paperID)
	return err
}

// RemoveTagFromPaper unlinks a tag from a paper.
func (d *DB) RemoveTagFromPaper(tagID, paperID int64) error {
	_, err := d.Exec(`DELETE FROM paper_tags WHERE tag_id=? AND paper_id=?`, tagID, paperID)
	return err
}

// ListPaperTags returns all tags for a specific paper.
func (d *DB) ListPaperTags(paperID int64) ([]PaperTag, error) {
	rows, err := d.Query(`SELECT t.id, t.name, t.color FROM tags t INNER JOIN paper_tags pt ON pt.tag_id=t.id WHERE pt.paper_id=? ORDER BY t.name`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PaperTag
	for rows.Next() {
		var pt PaperTag
		if err := rows.Scan(&pt.TagID, &pt.Name, &pt.Color); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}

// SetPaperTags replaces all tags on a paper with the given tag IDs.
func (d *DB) SetPaperTags(paperID int64, tagIDs []int64) error {
	if _, err := d.Exec(`DELETE FROM paper_tags WHERE paper_id=?`, paperID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		if _, err := d.Exec(`INSERT INTO paper_tags (tag_id, paper_id) VALUES (?, ?)`, tid, paperID); err != nil {
			return err
		}
	}
	return nil
}

// buildTagFilter returns a subquery clause for filtering by tag IDs.
func buildTagFilter(tagIDs []int64) (string, []interface{}) {
	if len(tagIDs) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(tagIDs))
	args := make([]interface{}, len(tagIDs))
	for i, id := range tagIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf("id IN (SELECT paper_id FROM paper_tags WHERE tag_id IN (%s))", strings.Join(placeholders, ",")), args
}
