package db

import "time"

// SetAxisRadarRun updates the last_radar_run timestamp for an axis.
func (d *DB) SetAxisRadarRun(axisID int64, t time.Time) error {
	_, err := d.Exec(`UPDATE axes SET last_radar_run=? WHERE id=?`, t.UTC().Format("2006-01-02 15:04:05"), axisID)
	return err
}

// EnsureTag returns the ID of a tag with the given name and color, creating it if needed.
func (d *DB) EnsureTag(profileID int64, name, color string) (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM tags WHERE profile_id=? AND name=?`, profileID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	// Create the tag.
	res, err := d.Exec(`INSERT INTO tags (profile_id, name, color) VALUES (?,?,?)`, profileID, name, color)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
