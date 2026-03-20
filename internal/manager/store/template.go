package store

import (
	"database/sql"
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// CreateTemplate inserts a new template.
func (db *DB) CreateTemplate(t *models.Template) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO templates (vm_id, name, url, checksum_url, tags, cloudinit_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.VMID, t.Name, t.URL, t.ChecksumURL, t.Tags, t.CloudInitID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert template: %w", err)
	}
	return res.LastInsertId()
}

// GetTemplate retrieves a template by name.
func (db *DB) GetTemplate(name string) (*models.Template, error) {
	row := db.QueryRow(
		`SELECT id, vm_id, name, url, checksum_url, tags, cloudinit_id, created_at, updated_at
		 FROM templates WHERE name = ?`,
		name,
	)
	var t models.Template
	var ciID sql.NullInt64
	if err := row.Scan(&t.ID, &t.VMID, &t.Name, &t.URL, &t.ChecksumURL, &t.Tags, &ciID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get template %q: %w", name, err)
	}
	if ciID.Valid {
		t.CloudInitID = &ciID.Int64
	}
	return &t, nil
}

// ListTemplates returns all templates.
func (db *DB) ListTemplates() ([]models.Template, error) {
	rows, err := db.Query(
		`SELECT id, vm_id, name, url, checksum_url, tags, cloudinit_id, created_at, updated_at
		 FROM templates ORDER BY vm_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var t models.Template
		var ciID sql.NullInt64
		if err := rows.Scan(&t.ID, &t.VMID, &t.Name, &t.URL, &t.ChecksumURL, &t.Tags, &ciID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if ciID.Valid {
			t.CloudInitID = &ciID.Int64
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// UpdateTemplate updates a template's fields.
func (db *DB) UpdateTemplate(name string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	query := "UPDATE templates SET updated_at = CURRENT_TIMESTAMP"
	args := make([]interface{}, 0, len(updates)+1)

	for col, val := range updates {
		query += fmt.Sprintf(", %s = ?", col)
		args = append(args, val)
	}
	query += " WHERE name = ?"
	args = append(args, name)

	res, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %q not found", name)
	}
	return nil
}

// DeleteTemplate removes a template by name.
func (db *DB) DeleteTemplate(name string) error {
	res, err := db.Exec("DELETE FROM templates WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %q not found", name)
	}
	return nil
}
