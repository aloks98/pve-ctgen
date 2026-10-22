package store

import (
	"context"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/filelock"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// DB is the file-based store. Named DB for backward compatibility with callers.
type DB struct {
	dir string // base directory (~/.config/pvectgen)
	mu  sync.Mutex
}

// BuildWithDetails includes template and node names for display.
type BuildWithDetails struct {
	models.Build
	TemplateName string
	NodeName     string
}

// Open creates (if needed) the data directory and returns a store.
func Open(dir string) (*DB, error) {
	for _, sub := range []string{"", "cloudinit", "ignition", "builds"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}
	if st, err := os.Lstat(filepath.Join(dir, "nodes.yaml")); err == nil && !st.Mode().IsRegular() {
		return nil, fmt.Errorf("nodes.yaml must be a regular file")
	}
	if err := os.Chmod(filepath.Join(dir, "nodes.yaml"), 0600); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return &DB{dir: dir}, nil
}

// Close is a no-op (no DB connection to close).
func (db *DB) Close() error { return nil }

// --- helpers ---

func (db *DB) path(name string) string { return filepath.Join(db.dir, name) }
func (db *DB) ciDir() string           { return filepath.Join(db.dir, "cloudinit") }
func (db *DB) ignDir() string          { return filepath.Join(db.dir, "ignition") }
func (db *DB) buildsDir() string       { return filepath.Join(db.dir, "builds") }

// initDir returns the directory for the given init type.
func (db *DB) initDir(initType string) string {
	if initType == models.InitTypeIgnition {
		return db.ignDir()
	}
	return db.ciDir()
}

// detectInitType returns "ignition" if filename looks like a Butane file, else "cloudinit".
func detectInitType(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".bu") || strings.HasSuffix(lower, ".butane") || strings.HasSuffix(lower, ".ign") {
		return models.InitTypeIgnition
	}
	return models.InitTypeCloudInit
}

func readYAML(path string, v interface{}) error {
	data, err := readRegular(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty is fine
		}
		return err
	}
	return yaml.Unmarshal(data, v)
}

func writeYAML(path string, v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, data, 0600)
}

// ============================================================
// Init Configs (cloud-init YAML in cloudinit/ + Butane in ignition/)
// ============================================================
// The legacy method names (CreateCloudInit, GetCloudInit, etc.) are preserved
// for backward compat with callers; they now auto-route based on filename.

// resolveInitPath returns (full path, init type) for an init config by name.
// It checks both directories; if the file exists in ignition/, that wins,
// otherwise falls back to cloudinit/.
func (db *DB) resolveInitPath(name string) (string, string) {
	if p := filepath.Join(db.ignDir(), name); fileExists(p) {
		return p, models.InitTypeIgnition
	}
	if p := filepath.Join(db.ciDir(), name); fileExists(p) {
		return p, models.InitTypeCloudInit
	}
	// Doesn't exist yet — pick the directory based on extension
	return filepath.Join(db.initDir(detectInitType(name)), name), detectInitType(name)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (db *DB) CreateCloudInit(name, content string) (int64, error) {
	if err := validation.Name(name); err != nil {
		return 0, err
	}
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	p, _ := db.resolveInitPath(name)
	if fileExists(p) {
		return 0, fmt.Errorf("insert cloudinit: UNIQUE constraint failed: %s", name)
	}
	return 0, fileutil.AtomicWrite(p, []byte(content), 0600)
}

func (db *DB) GetCloudInit(name string) (*models.CloudInitConfig, error) {
	if err := validation.Name(name); err != nil {
		return nil, err
	}
	p, t := db.resolveInitPath(name)
	data, err := readRegular(p)
	if err != nil {
		return nil, fmt.Errorf("get init %q: %w", name, err)
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	return &models.CloudInitConfig{
		Name:      name,
		Type:      t,
		Content:   string(data),
		CreatedAt: info.ModTime(),
		UpdatedAt: info.ModTime(),
	}, nil
}

func (db *DB) GetCloudInitByID(name string) (*models.CloudInitConfig, error) {
	return db.GetCloudInit(name)
}

func (db *DB) ListCloudInits() ([]models.CloudInitConfig, error) {
	var configs []models.CloudInitConfig
	// cloud-init configs
	if entries, err := os.ReadDir(db.ciDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			configs = append(configs, models.CloudInitConfig{
				Name:      e.Name(),
				Type:      models.InitTypeCloudInit,
				CreatedAt: info.ModTime(),
				UpdatedAt: info.ModTime(),
			})
		}
	} else {
		return nil, err
	}
	// ignition configs (Butane source files)
	if entries, err := os.ReadDir(db.ignDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			lower := strings.ToLower(e.Name())
			if !strings.HasSuffix(lower, ".bu") && !strings.HasSuffix(lower, ".butane") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			configs = append(configs, models.CloudInitConfig{
				Name:      e.Name(),
				Type:      models.InitTypeIgnition,
				CreatedAt: info.ModTime(),
				UpdatedAt: info.ModTime(),
			})
		}
	} else {
		return nil, err
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil
}

func (db *DB) UpdateCloudInit(name, content string) error {
	if err := validation.Name(name); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	p, _ := db.resolveInitPath(name)
	if !fileExists(p) {
		return fmt.Errorf("init %q not found", name)
	}
	return fileutil.AtomicWrite(p, []byte(content), 0600)
}

func (db *DB) DeleteCloudInit(name string) error {
	if err := validation.Name(name); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	p, _ := db.resolveInitPath(name)
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("init %q not found", name)
	}
	return nil
}

// ============================================================
// Templates (templates.yaml)
// ============================================================

func (db *DB) loadTemplates() ([]models.Template, error) {
	var templates []models.Template
	if err := readYAML(db.path("templates.yaml"), &templates); err != nil {
		return nil, err
	}
	return templates, nil
}

func (db *DB) saveTemplates(templates []models.Template) error {
	return writeYAML(db.path("templates.yaml"), templates)
}

func (db *DB) CreateTemplate(t *models.Template) (int64, error) {
	if err := validateTemplate(t); err != nil {
		return 0, err
	}
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	templates, err := db.loadTemplates()
	if err != nil {
		return 0, err
	}
	for _, existing := range templates {
		if existing.Name == t.Name {
			return 0, fmt.Errorf("insert template: UNIQUE constraint failed: templates.name")
		}
		if existing.VMID == t.VMID {
			return 0, fmt.Errorf("insert template: UNIQUE constraint failed: templates.vm_id")
		}
	}
	templates = append(templates, *t)
	return 0, db.saveTemplates(templates)
}

func (db *DB) GetTemplate(name string) (*models.Template, error) {
	templates, err := db.loadTemplates()
	if err != nil {
		return nil, err
	}
	for _, t := range templates {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("get template %q: not found", name)
}

func (db *DB) ListTemplates() ([]models.Template, error) {
	return db.loadTemplates()
}

func (db *DB) UpdateTemplate(name string, updates map[string]interface{}) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	templates, err := db.loadTemplates()
	if err != nil {
		return err
	}
	for i, t := range templates {
		if t.Name == name {
			if v, ok := updates["name"]; ok {
				templates[i].Name = v.(string)
			}
			if v, ok := updates["vm_id"]; ok {
				templates[i].VMID = v.(int)
			}
			if v, ok := updates["url"]; ok {
				templates[i].URL = v.(string)
			}
			if v, ok := updates["checksum_url"]; ok {
				templates[i].ChecksumURL = v.(string)
			}
			if v, ok := updates["tags"]; ok {
				templates[i].Tags = v.(string)
			}
			if v, ok := updates["cloudinit"]; ok {
				if v == nil {
					templates[i].CloudInit = ""
				} else {
					templates[i].CloudInit = v.(string)
				}
			}
			if v, ok := updates["profile"]; ok {
				templates[i].Profile = v.(string)
			}
			if v, ok := updates["init_type"]; ok {
				if v == nil {
					templates[i].InitType = ""
				} else {
					templates[i].InitType = v.(string)
				}
			}
			if err := validateTemplate(&templates[i]); err != nil {
				return err
			}
			for j, other := range templates {
				if i != j && (other.Name == templates[i].Name || other.VMID == templates[i].VMID) {
					return fmt.Errorf("template name or VM ID already exists")
				}
			}
			return db.saveTemplates(templates)
		}
	}
	return fmt.Errorf("template %q not found", name)
}

func (db *DB) DeleteTemplate(name string) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	templates, err := db.loadTemplates()
	if err != nil {
		return err
	}
	for i, t := range templates {
		if t.Name == name {
			templates = append(templates[:i], templates[i+1:]...)
			return db.saveTemplates(templates)
		}
	}
	return fmt.Errorf("template %q not found", name)
}

// ============================================================
// Build Steps (steps.yaml)
// ============================================================

func (db *DB) loadSteps() ([]models.BuildStep, error) {
	var steps []models.BuildStep
	if err := readYAML(db.path("steps.yaml"), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func (db *DB) saveSteps(steps []models.BuildStep) error {
	return writeYAML(db.path("steps.yaml"), steps)
}

func (db *DB) CreateBuildStep(name, command string, sortOrder int) (int64, error) {
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	steps, err := db.loadSteps()
	if err != nil {
		return 0, err
	}
	// Insert at the right position
	newStep := models.BuildStep{Name: name, Command: command}
	if sortOrder <= 0 || sortOrder > len(steps) {
		steps = append(steps, newStep)
	} else {
		steps = append(steps[:sortOrder-1], append([]models.BuildStep{newStep}, steps[sortOrder-1:]...)...)
	}
	return 0, db.saveSteps(steps)
}

func (db *DB) ListBuildSteps() ([]models.BuildStep, error) {
	return db.loadSteps()
}

func (db *DB) UpdateBuildStep(name string, updates map[string]interface{}) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	steps, err := db.loadSteps()
	if err != nil {
		return err
	}
	for i, s := range steps {
		if s.Name == name {
			if v, ok := updates["name"]; ok {
				steps[i].Name = v.(string)
			}
			if v, ok := updates["command"]; ok {
				steps[i].Command = v.(string)
			}
			return db.saveSteps(steps)
		}
	}
	return fmt.Errorf("build step %q not found", name)
}

func (db *DB) DeleteBuildStep(name string) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	steps, err := db.loadSteps()
	if err != nil {
		return err
	}
	for i, s := range steps {
		if s.Name == name {
			steps = append(steps[:i], steps[i+1:]...)
			return db.saveSteps(steps)
		}
	}
	return fmt.Errorf("build step %q not found", name)
}

func (db *DB) SwapBuildStepOrder(idxA, idxB int) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	steps, err := db.loadSteps()
	if err != nil {
		return err
	}
	if idxA < 0 || idxA >= len(steps) || idxB < 0 || idxB >= len(steps) {
		return fmt.Errorf("index out of range")
	}
	steps[idxA], steps[idxB] = steps[idxB], steps[idxA]
	return db.saveSteps(steps)
}

func (db *DB) ResetBuildSteps(defaults []models.Step) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	steps := make([]models.BuildStep, len(defaults))
	for i, s := range defaults {
		steps[i] = models.BuildStep{Name: s.Name, Command: s.Command}
	}
	return db.saveSteps(steps)
}

// ============================================================
// Nodes (nodes.yaml)
// ============================================================

func (db *DB) loadNodes() ([]models.Node, error) {
	var nodes []models.Node
	if err := readYAML(db.path("nodes.yaml"), &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (db *DB) saveNodes(nodes []models.Node) error {
	return writeYAML(db.path("nodes.yaml"), nodes)
}

func (db *DB) CreateNode(name, displayName, address, apiKey string, fingerprints ...string) (int64, error) {
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	nodes, err := db.loadNodes()
	if err != nil {
		return 0, err
	}
	for _, n := range nodes {
		if n.Name == name {
			return 0, fmt.Errorf("insert node: UNIQUE constraint failed: %s", name)
		}
	}
	var pin string
	if len(fingerprints) > 0 {
		pin = fingerprints[0]
	}
	nodes = append(nodes, models.Node{
		Name:           name,
		DisplayName:    displayName,
		Address:        address,
		APIKey:         apiKey,
		TLSFingerprint: pin,
	})
	return 0, db.saveNodes(nodes)
}

func (db *DB) GetNode(name string) (*models.Node, error) {
	nodes, err := db.loadNodes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if n.Name == name || n.DisplayName == name {
			return &n, nil
		}
	}
	return nil, fmt.Errorf("get node %q: not found", name)
}

func (db *DB) ListNodes() ([]models.Node, error) {
	return db.loadNodes()
}

func (db *DB) UpdateNodeDisplayName(name, displayName string) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	nodes, err := db.loadNodes()
	if err != nil {
		return err
	}
	for i, n := range nodes {
		if n.Name == name || n.DisplayName == name {
			nodes[i].DisplayName = displayName
			return db.saveNodes(nodes)
		}
	}
	return fmt.Errorf("node %q not found", name)
}

func (db *DB) DeleteNode(name string) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	nodes, err := db.loadNodes()
	if err != nil {
		return err
	}
	for i, n := range nodes {
		if n.Name == name || n.DisplayName == name {
			nodes = append(nodes[:i], nodes[i+1:]...)
			return db.saveNodes(nodes)
		}
	}
	return fmt.Errorf("node %q not found", name)
}

// ============================================================
// Builds (builds/<build-id>.yaml)
// ============================================================

func (db *DB) buildPath(buildID string) string {
	return filepath.Join(db.buildsDir(), buildID+".yaml")
}

func (db *DB) CreateBuild(buildID, templateName, nodeName string) (int64, error) {
	if err := validation.Name(buildID); err != nil {
		return 0, err
	}
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	if _, err := os.Stat(db.buildPath(buildID)); err == nil {
		return 0, fmt.Errorf("build %q already exists", buildID)
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	now := time.Now()
	b := models.Build{
		BuildID:   buildID,
		Template:  templateName,
		Node:      nodeName,
		Status:    "running",
		StartedAt: &now,
	}
	return 0, writeYAML(db.buildPath(buildID), b)
}

func (db *DB) GetBuild(buildID string) (*models.Build, error) {
	if err := validation.Name(buildID); err != nil {
		return nil, err
	}
	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return nil, fmt.Errorf("get build %q: %w", buildID, err)
	}
	if b.BuildID == "" {
		return nil, fmt.Errorf("get build %q: not found", buildID)
	}
	return &b, nil
}

func (db *DB) UpdateBuildStatus(buildID, status string) error {
	if err := validation.Name(buildID); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return err
	}
	if b.BuildID == "" {
		return fmt.Errorf("build %q not found", buildID)
	}
	b.Status = status
	if status == "completed" || status == "failed" || status == "cancelled" {
		now := time.Now()
		b.CompletedAt = &now
	}
	return writeYAML(db.buildPath(buildID), b)
}

func (db *DB) ListBuilds(status, template, node string, limit int) ([]models.Build, error) {
	entries, err := os.ReadDir(db.buildsDir())
	if err != nil {
		return nil, err
	}
	var builds []models.Build
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var b models.Build
		if err := readYAML(filepath.Join(db.buildsDir(), e.Name()), &b); err != nil {
			return nil, fmt.Errorf("read build %s: %w", e.Name(), err)
		}
		if status != "" && b.Status != status {
			continue
		}
		if template != "" && b.Template != template {
			continue
		}
		if node != "" && b.Node != node {
			continue
		}
		builds = append(builds, b)
	}
	// Sort by started_at descending
	sort.Slice(builds, func(i, j int) bool {
		if builds[i].StartedAt == nil {
			return false
		}
		if builds[j].StartedAt == nil {
			return true
		}
		return builds[i].StartedAt.After(*builds[j].StartedAt)
	})
	if limit > 0 && len(builds) > limit {
		builds = builds[:limit]
	}
	return builds, nil
}

func (db *DB) ListBuildsWithDetails(limit int) ([]BuildWithDetails, error) {
	builds, err := db.ListBuilds("", "", "", limit)
	if err != nil {
		return nil, err
	}
	var result []BuildWithDetails
	for _, b := range builds {
		result = append(result, BuildWithDetails{
			Build:        b,
			TemplateName: b.Template,
			NodeName:     b.Node,
		})
	}
	return result, nil
}

// --- Build step results (stored inline in the build YAML) ---

func (db *DB) CreateBuildStepResult(buildID, stepName string, stepIndex int) (int64, error) {
	if err := validation.Name(buildID); err != nil {
		return 0, err
	}
	unlock, err := db.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return 0, err
	}
	if b.BuildID == "" {
		return 0, fmt.Errorf("build %q not found", buildID)
	}
	now := time.Now()
	b.Steps = append(b.Steps, models.BuildStepResult{
		StepName:  stepName,
		StepIndex: stepIndex,
		Status:    "running",
		StartedAt: &now,
	})
	return 0, writeYAML(db.buildPath(buildID), b)
}

func (db *DB) UpdateBuildStepResult(buildID, stepName, status, logLine string) error {
	if err := validation.Name(buildID); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return err
	}
	if b.BuildID == "" {
		return fmt.Errorf("build %q not found", buildID)
	}
	for i, s := range b.Steps {
		if s.StepName == stepName {
			b.Steps[i].Status = status
			if logLine != "" {
				b.Steps[i].Log += logLine + "\n"
			}
			if status == "completed" || status == "failed" {
				now := time.Now()
				b.Steps[i].CompletedAt = &now
			}
			break
		}
	}
	return writeYAML(db.buildPath(buildID), b)
}

func (db *DB) AppendBuildStepLog(buildID, stepName, logLine string) error {
	if err := validation.Name(buildID); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()

	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return err
	}
	if b.BuildID == "" {
		return fmt.Errorf("build %q not found", buildID)
	}
	for i, s := range b.Steps {
		if s.StepName == stepName {
			b.Steps[i].Log += logLine + "\n"
			break
		}
	}
	return writeYAML(db.buildPath(buildID), b)
}

func (db *DB) GetBuildStepResults(buildID string) ([]models.BuildStepResult, error) {
	if err := validation.Name(buildID); err != nil {
		return nil, err
	}
	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return nil, err
	}
	if b.BuildID == "" {
		return nil, fmt.Errorf("build %q not found", buildID)
	}
	return b.Steps, nil
}

// ============================================================
// Seed (import from legacy JSON/YAML files)
// ============================================================

func (db *DB) Seed(imagesPath, stepsPath, cloudinitDir string, force bool) error {
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()
	// Import cloud-init configs
	if cloudinitDir != "" {
		entries, err := os.ReadDir(cloudinitDir)
		if err != nil {
			return fmt.Errorf("read cloudinit dir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			src := filepath.Join(cloudinitDir, entry.Name())
			content, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("read %s: %w", entry.Name(), err)
			}
			if err := cloudinit.Validate(string(content)); err != nil {
				return fmt.Errorf("invalid cloud-init %s: %w", entry.Name(), err)
			}
			dst := filepath.Join(db.ciDir(), entry.Name())
			if _, err := os.Stat(dst); err == nil && !force {
				continue // skip existing
			}
			if err := fileutil.AtomicWrite(dst, content, 0600); err != nil {
				return fmt.Errorf("write %s: %w", entry.Name(), err)
			}
		}

		// Also import Butane (.bu) files from cloudinit/ignition subdirectory if present.
		ignSrc := filepath.Join(cloudinitDir, "..", "ignition")
		if entries, err := os.ReadDir(ignSrc); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				lower := strings.ToLower(entry.Name())
				if !strings.HasSuffix(lower, ".bu") && !strings.HasSuffix(lower, ".butane") {
					continue
				}
				src := filepath.Join(ignSrc, entry.Name())
				content, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("read %s: %w", entry.Name(), err)
				}
				dst := filepath.Join(db.ignDir(), entry.Name())
				if _, err := os.Stat(dst); err == nil && !force {
					continue
				}
				if err := fileutil.AtomicWrite(dst, content, 0600); err != nil {
					return fmt.Errorf("write %s: %w", entry.Name(), err)
				}
			}
		}
	}

	// Import build steps
	if stepsPath != "" {
		steps, err := fileutil.LoadSteps(stepsPath)
		if err != nil {
			return fmt.Errorf("load steps: %w", err)
		}
		_, statErr := os.Stat(db.path("steps.yaml"))
		if force || os.IsNotExist(statErr) {
			items := make([]models.BuildStep, len(steps))
			for i, s := range steps {
				items[i] = models.BuildStep{Name: s.Name, Command: s.Command}
			}
			err = db.saveSteps(items)
		} else if statErr != nil {
			err = statErr
		}
		if err != nil {
			return err
		}
	}

	// Import images as templates
	if imagesPath != "" {
		images, err := fileutil.LoadImages(imagesPath)
		if err != nil {
			return fmt.Errorf("load images: %w", err)
		}
		templates, err := db.loadTemplates()
		if err != nil {
			return err
		}
		existingNames := make(map[string]int)
		existingVMIDs := make(map[int]int)
		for i, t := range templates {
			existingNames[t.Name] = i
			existingVMIDs[t.VMID] = i
		}

		for _, img := range images {
			initType := img.InitType
			if initType == "" {
				// Infer from filename extension
				initType = detectInitType(img.Vendor)
			}
			t := models.Template{
				VMID:        img.ID,
				Name:        img.Name,
				URL:         img.URL,
				ChecksumURL: img.ChecksumURL,
				Tags:        img.Tags,
				CloudInit:   img.Vendor,
				InitType:    initType,
			}
			if err := validateTemplate(&t); err != nil {
				return err
			}
			nameIdx, nameExists := existingNames[img.Name]
			idIdx, idExists := existingVMIDs[img.ID]
			if nameExists && idExists && nameIdx != idIdx {
				return fmt.Errorf("import %s conflicts with another template's VM ID", img.Name)
			}
			if idx, ok := existingNames[img.Name]; ok {
				if force {
					templates[idx] = t
				}
			} else if idx, ok := existingVMIDs[img.ID]; ok {
				if !force {
					continue
				}
				templates[idx] = t
			} else {
				templates = append(templates, t)
			}
			clear(existingNames)
			clear(existingVMIDs)
			for i, t := range templates {
				existingNames[t.Name] = i
				existingVMIDs[t.VMID] = i
			}
		}
		if err := db.saveTemplates(templates); err != nil {
			return fmt.Errorf("save templates: %w", err)
		}
	}

	return nil
}

func (db *DB) lock() (func(), error) {
	db.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	release, err := filelock.Acquire(ctx, db.path(".store.lock"))
	cancel()
	if err != nil {
		db.mu.Unlock()
		return nil, err
	}
	return func() { release(); db.mu.Unlock() }, nil
}

func validateTemplate(t *models.Template) error {
	if t == nil {
		return fmt.Errorf("template is required")
	}
	if t.VMID < 100 || t.VMID > 999999999 {
		return fmt.Errorf("VM ID must be between 100 and 999999999")
	}
	if err := validation.Name(t.Name); err != nil {
		return err
	}
	if err := validation.URL(t.URL); err != nil {
		return err
	}
	if err := validation.URL(t.ChecksumURL); err != nil {
		return fmt.Errorf("checksum URL: %w", err)
	}
	if t.CloudInit != "" {
		if err := validation.Name(t.CloudInit); err != nil {
			return err
		}
	}
	if t.Profile != "" {
		if err := validation.Name(t.Profile); err != nil {
			return err
		}
	}
	if t.EffectiveInitType() != "cloudinit" && t.EffectiveInitType() != "ignition" {
		return fmt.Errorf("unknown init type")
	}
	return nil
}

func readRegular(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing non-regular file %s", path)
	}
	return os.ReadFile(path)
}
