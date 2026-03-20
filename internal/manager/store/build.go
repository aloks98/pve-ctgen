package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// CreateBuild inserts a new build record.
func (db *DB) CreateBuild(buildID string, templateID, nodeID int64) (int64, error) {
	now := time.Now()
	res, err := db.Exec(
		"INSERT INTO builds (build_id, template_id, node_id, status, started_at) VALUES (?, ?, ?, 'running', ?)",
		buildID, templateID, nodeID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert build: %w", err)
	}
	return res.LastInsertId()
}

// UpdateBuildStatus updates the status of a build.
func (db *DB) UpdateBuildStatus(buildID, status string) error {
	var query string
	if status == "completed" || status == "failed" {
		query = "UPDATE builds SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE build_id = ?"
	} else {
		query = "UPDATE builds SET status = ? WHERE build_id = ?"
	}
	_, err := db.Exec(query, status, buildID)
	return err
}

// GetBuild retrieves a build by build_id.
func (db *DB) GetBuild(buildID string) (*models.Build, error) {
	row := db.QueryRow(
		"SELECT id, build_id, template_id, node_id, status, started_at, completed_at FROM builds WHERE build_id = ?",
		buildID,
	)
	var b models.Build
	var startedAt, completedAt sql.NullTime
	if err := row.Scan(&b.ID, &b.BuildID, &b.TemplateID, &b.NodeID, &b.Status, &startedAt, &completedAt); err != nil {
		return nil, fmt.Errorf("get build %q: %w", buildID, err)
	}
	if startedAt.Valid {
		b.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}
	return &b, nil
}

// ListBuilds returns builds with optional filters.
func (db *DB) ListBuilds(status, templateName, nodeName string, limit int) ([]models.Build, error) {
	query := `SELECT b.id, b.build_id, b.template_id, b.node_id, b.status, b.started_at, b.completed_at
		FROM builds b`
	var conditions []string
	var args []interface{}

	if templateName != "" {
		query += " JOIN templates t ON b.template_id = t.id"
		conditions = append(conditions, "t.name = ?")
		args = append(args, templateName)
	}
	if nodeName != "" {
		query += " JOIN nodes n ON b.node_id = n.id"
		conditions = append(conditions, "n.name = ?")
		args = append(args, nodeName)
	}
	if status != "" {
		conditions = append(conditions, "b.status = ?")
		args = append(args, status)
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, c := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += c
		}
	}

	query += " ORDER BY b.started_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()

	var builds []models.Build
	for rows.Next() {
		var b models.Build
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.BuildID, &b.TemplateID, &b.NodeID, &b.Status, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			b.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			b.CompletedAt = &completedAt.Time
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// CreateBuildStepResult inserts a build step result.
func (db *DB) CreateBuildStepResult(buildID, stepName string, stepIndex int) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO build_step_results (build_id, step_name, step_index, status, started_at) VALUES (?, ?, ?, 'running', CURRENT_TIMESTAMP)",
		buildID, stepName, stepIndex,
	)
	if err != nil {
		return 0, fmt.Errorf("insert step result: %w", err)
	}
	return res.LastInsertId()
}

// UpdateBuildStepResult updates a step result's status and appends log.
func (db *DB) UpdateBuildStepResult(buildID, stepName, status, logLine string) error {
	var query string
	if status == "completed" || status == "failed" {
		query = "UPDATE build_step_results SET status = ?, log = log || ?, completed_at = CURRENT_TIMESTAMP WHERE build_id = ? AND step_name = ?"
	} else {
		query = "UPDATE build_step_results SET status = ?, log = log || ? WHERE build_id = ? AND step_name = ?"
	}
	_, err := db.Exec(query, status, logLine, buildID, stepName)
	return err
}

// AppendBuildStepLog appends a log line to a step result.
func (db *DB) AppendBuildStepLog(buildID, stepName, logLine string) error {
	_, err := db.Exec(
		"UPDATE build_step_results SET log = log || ? WHERE build_id = ? AND step_name = ?",
		logLine+"\n", buildID, stepName,
	)
	return err
}

// GetBuildStepResults returns all step results for a build.
func (db *DB) GetBuildStepResults(buildID string) ([]models.BuildStepResult, error) {
	rows, err := db.Query(
		`SELECT id, build_id, step_name, step_index, status, log, started_at, completed_at
		 FROM build_step_results WHERE build_id = ? ORDER BY step_index`,
		buildID,
	)
	if err != nil {
		return nil, fmt.Errorf("get step results: %w", err)
	}
	defer rows.Close()

	var results []models.BuildStepResult
	for rows.Next() {
		var r models.BuildStepResult
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.BuildID, &r.StepName, &r.StepIndex, &r.Status, &r.Log, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			r.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
