package store

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// CreateNode inserts a new node.
func (db *DB) CreateNode(name, displayName, address, apiKey string) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO nodes (name, display_name, address, api_key) VALUES (?, ?, ?, ?)",
		name, displayName, address, apiKey,
	)
	if err != nil {
		return 0, fmt.Errorf("insert node: %w", err)
	}
	return res.LastInsertId()
}

// GetNode retrieves a node by name (matches either name or display_name).
func (db *DB) GetNode(name string) (*models.Node, error) {
	row := db.QueryRow(
		"SELECT id, name, display_name, address, api_key, created_at FROM nodes WHERE name = ? OR display_name = ?",
		name, name,
	)
	var n models.Node
	if err := row.Scan(&n.ID, &n.Name, &n.DisplayName, &n.Address, &n.APIKey, &n.CreatedAt); err != nil {
		return nil, fmt.Errorf("get node %q: %w", name, err)
	}
	return &n, nil
}

// ListNodes returns all nodes.
func (db *DB) ListNodes() ([]models.Node, error) {
	rows, err := db.Query("SELECT id, name, display_name, address, api_key, created_at FROM nodes ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var nodes []models.Node
	for rows.Next() {
		var n models.Node
		if err := rows.Scan(&n.ID, &n.Name, &n.DisplayName, &n.Address, &n.APIKey, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// UpdateNodeDisplayName updates a node's display name.
func (db *DB) UpdateNodeDisplayName(name, displayName string) error {
	res, err := db.Exec("UPDATE nodes SET display_name = ? WHERE name = ? OR display_name = ?", displayName, name, name)
	if err != nil {
		return fmt.Errorf("update node display name: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %q not found", name)
	}
	return nil
}

// DeleteNode removes a node by name or display_name.
func (db *DB) DeleteNode(name string) error {
	res, err := db.Exec("DELETE FROM nodes WHERE name = ? OR display_name = ?", name, name)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %q not found", name)
	}
	return nil
}
