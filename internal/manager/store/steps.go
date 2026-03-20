package store

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// CreateBuildStep inserts a new build step.
func (db *DB) CreateBuildStep(name, command string, sortOrder int) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO build_steps (name, command, sort_order, is_default) VALUES (?, ?, ?, 0)",
		name, command, sortOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert build step: %w", err)
	}
	return res.LastInsertId()
}

// ListBuildSteps returns all build steps ordered by sort_order.
func (db *DB) ListBuildSteps() ([]models.BuildStep, error) {
	rows, err := db.Query(
		"SELECT id, name, command, sort_order, is_default, created_at, updated_at FROM build_steps ORDER BY sort_order",
	)
	if err != nil {
		return nil, fmt.Errorf("list build steps: %w", err)
	}
	defer rows.Close()

	var steps []models.BuildStep
	for rows.Next() {
		var s models.BuildStep
		if err := rows.Scan(&s.ID, &s.Name, &s.Command, &s.SortOrder, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// UpdateBuildStep updates a build step's fields.
func (db *DB) UpdateBuildStep(name string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	query := "UPDATE build_steps SET updated_at = CURRENT_TIMESTAMP"
	args := make([]interface{}, 0, len(updates)+1)

	for col, val := range updates {
		query += fmt.Sprintf(", %s = ?", col)
		args = append(args, val)
	}
	query += " WHERE name = ?"
	args = append(args, name)

	res, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update build step: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("build step %q not found", name)
	}
	return nil
}

// DeleteBuildStep removes a build step by name.
func (db *DB) DeleteBuildStep(name string) error {
	res, err := db.Exec("DELETE FROM build_steps WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete build step: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("build step %q not found", name)
	}
	return nil
}

// SwapBuildStepOrder swaps the sort_order of two steps by their current indices in the ordered list.
func (db *DB) SwapBuildStepOrder(idA, idB int64, orderA, orderB int) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE build_steps SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", orderB, idA); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE build_steps SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", orderA, idB); err != nil {
		return err
	}
	return tx.Commit()
}

// GetBuildStepByName retrieves a single build step by name.
func (db *DB) GetBuildStepByName(name string) (*models.BuildStep, error) {
	row := db.QueryRow(
		"SELECT id, name, command, sort_order, is_default, created_at, updated_at FROM build_steps WHERE name = ?",
		name,
	)
	var s models.BuildStep
	if err := row.Scan(&s.ID, &s.Name, &s.Command, &s.SortOrder, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get build step %q: %w", name, err)
	}
	return &s, nil
}

// ResetBuildSteps deletes all build steps and re-inserts defaults.
func (db *DB) ResetBuildSteps(defaults []models.Step) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM build_steps"); err != nil {
		return fmt.Errorf("delete build steps: %w", err)
	}

	for i, s := range defaults {
		if _, err := tx.Exec(
			"INSERT INTO build_steps (name, command, sort_order, is_default) VALUES (?, ?, ?, 1)",
			s.Name, s.Command, i+1,
		); err != nil {
			return fmt.Errorf("insert default step: %w", err)
		}
	}

	return tx.Commit()
}
