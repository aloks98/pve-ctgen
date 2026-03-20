package store

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// CreateCloudInit inserts a new cloud-init config.
func (db *DB) CreateCloudInit(name, content string) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO cloudinit_configs (name, content) VALUES (?, ?)",
		name, content,
	)
	if err != nil {
		return 0, fmt.Errorf("insert cloudinit: %w", err)
	}
	return res.LastInsertId()
}

// GetCloudInit retrieves a cloud-init config by name.
func (db *DB) GetCloudInit(name string) (*models.CloudInitConfig, error) {
	row := db.QueryRow(
		"SELECT id, name, content, created_at, updated_at FROM cloudinit_configs WHERE name = ?",
		name,
	)
	var c models.CloudInitConfig
	if err := row.Scan(&c.ID, &c.Name, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get cloudinit %q: %w", name, err)
	}
	return &c, nil
}

// GetCloudInitByID retrieves a cloud-init config by ID.
func (db *DB) GetCloudInitByID(id int64) (*models.CloudInitConfig, error) {
	row := db.QueryRow(
		"SELECT id, name, content, created_at, updated_at FROM cloudinit_configs WHERE id = ?",
		id,
	)
	var c models.CloudInitConfig
	if err := row.Scan(&c.ID, &c.Name, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get cloudinit id=%d: %w", id, err)
	}
	return &c, nil
}

// ListCloudInits returns all cloud-init configs.
func (db *DB) ListCloudInits() ([]models.CloudInitConfig, error) {
	rows, err := db.Query("SELECT id, name, content, created_at, updated_at FROM cloudinit_configs ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list cloudinits: %w", err)
	}
	defer rows.Close()

	var configs []models.CloudInitConfig
	for rows.Next() {
		var c models.CloudInitConfig
		if err := rows.Scan(&c.ID, &c.Name, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// UpdateCloudInit updates a cloud-init config's content.
func (db *DB) UpdateCloudInit(name, content string) error {
	res, err := db.Exec(
		"UPDATE cloudinit_configs SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?",
		content, name,
	)
	if err != nil {
		return fmt.Errorf("update cloudinit: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cloudinit %q not found", name)
	}
	return nil
}

// DeleteCloudInit removes a cloud-init config by name.
func (db *DB) DeleteCloudInit(name string) error {
	res, err := db.Exec("DELETE FROM cloudinit_configs WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete cloudinit: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cloudinit %q not found", name)
	}
	return nil
}
