package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
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
	for _, sub := range []string{"", "cloudinit", "builds"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}
	return &DB{dir: dir}, nil
}

// Close is a no-op (no DB connection to close).
func (db *DB) Close() error { return nil }

// --- helpers ---

func (db *DB) path(name string) string          { return filepath.Join(db.dir, name) }
func (db *DB) ciDir() string                     { return filepath.Join(db.dir, "cloudinit") }
func (db *DB) buildsDir() string                 { return filepath.Join(db.dir, "builds") }

func readYAML(path string, v interface{}) error {
	data, err := os.ReadFile(path)
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
	return os.WriteFile(path, data, 0644)
}

// ============================================================
// Cloud-Init (directory of .yaml files)
// ============================================================

func (db *DB) CreateCloudInit(name, content string) (int64, error) {
	p := filepath.Join(db.ciDir(), name)
	if _, err := os.Stat(p); err == nil {
		return 0, fmt.Errorf("insert cloudinit: UNIQUE constraint failed: %s", name)
	}
	return 0, os.WriteFile(p, []byte(content), 0644)
}

func (db *DB) GetCloudInit(name string) (*models.CloudInitConfig, error) {
	p := filepath.Join(db.ciDir(), name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("get cloudinit %q: %w", name, err)
	}
	info, _ := os.Stat(p)
	return &models.CloudInitConfig{
		Name:      name,
		Content:   string(data),
		CreatedAt: info.ModTime(),
		UpdatedAt: info.ModTime(),
	}, nil
}

func (db *DB) GetCloudInitByID(name string) (*models.CloudInitConfig, error) {
	return db.GetCloudInit(name)
}

func (db *DB) ListCloudInits() ([]models.CloudInitConfig, error) {
	entries, err := os.ReadDir(db.ciDir())
	if err != nil {
		return nil, err
	}
	var configs []models.CloudInitConfig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		info, _ := e.Info()
		configs = append(configs, models.CloudInitConfig{
			Name:      e.Name(),
			CreatedAt: info.ModTime(),
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil
}

func (db *DB) UpdateCloudInit(name, content string) error {
	p := filepath.Join(db.ciDir(), name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return fmt.Errorf("cloudinit %q not found", name)
	}
	return os.WriteFile(p, []byte(content), 0644)
}

func (db *DB) DeleteCloudInit(name string) error {
	p := filepath.Join(db.ciDir(), name)
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("cloudinit %q not found", name)
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
	db.mu.Lock()
	defer db.mu.Unlock()

	templates, _ := db.loadTemplates()
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
	templates, _ := db.loadTemplates()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	templates, _ := db.loadTemplates()
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
			return db.saveTemplates(templates)
		}
	}
	return fmt.Errorf("template %q not found", name)
}

func (db *DB) DeleteTemplate(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	templates, _ := db.loadTemplates()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	steps, _ := db.loadSteps()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	steps, _ := db.loadSteps()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	steps, _ := db.loadSteps()
	for i, s := range steps {
		if s.Name == name {
			steps = append(steps[:i], steps[i+1:]...)
			return db.saveSteps(steps)
		}
	}
	return fmt.Errorf("build step %q not found", name)
}

func (db *DB) SwapBuildStepOrder(idxA, idxB int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	steps, _ := db.loadSteps()
	if idxA < 0 || idxA >= len(steps) || idxB < 0 || idxB >= len(steps) {
		return fmt.Errorf("index out of range")
	}
	steps[idxA], steps[idxB] = steps[idxB], steps[idxA]
	return db.saveSteps(steps)
}

func (db *DB) ResetBuildSteps(defaults []models.Step) error {
	db.mu.Lock()
	defer db.mu.Unlock()

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

func (db *DB) CreateNode(name, displayName, address, apiKey string) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	nodes, _ := db.loadNodes()
	for _, n := range nodes {
		if n.Name == name {
			return 0, fmt.Errorf("insert node: UNIQUE constraint failed: %s", name)
		}
	}
	nodes = append(nodes, models.Node{
		Name:        name,
		DisplayName: displayName,
		Address:     address,
		APIKey:      apiKey,
	})
	return 0, db.saveNodes(nodes)
}

func (db *DB) GetNode(name string) (*models.Node, error) {
	nodes, _ := db.loadNodes()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	nodes, _ := db.loadNodes()
	for i, n := range nodes {
		if n.Name == name || n.DisplayName == name {
			nodes[i].DisplayName = displayName
			return db.saveNodes(nodes)
		}
	}
	return fmt.Errorf("node %q not found", name)
}

func (db *DB) DeleteNode(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	nodes, _ := db.loadNodes()
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
	db.mu.Lock()
	defer db.mu.Unlock()

	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return err
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
		return nil, nil
	}
	var builds []models.Build
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var b models.Build
		if err := readYAML(filepath.Join(db.buildsDir(), e.Name()), &b); err != nil {
			continue
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
	db.mu.Lock()
	defer db.mu.Unlock()

	var b models.Build
	readYAML(db.buildPath(buildID), &b)
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
	db.mu.Lock()
	defer db.mu.Unlock()

	var b models.Build
	readYAML(db.buildPath(buildID), &b)
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
	db.mu.Lock()
	defer db.mu.Unlock()

	var b models.Build
	readYAML(db.buildPath(buildID), &b)
	for i, s := range b.Steps {
		if s.StepName == stepName {
			b.Steps[i].Log += logLine + "\n"
			break
		}
	}
	return writeYAML(db.buildPath(buildID), b)
}

func (db *DB) GetBuildStepResults(buildID string) ([]models.BuildStepResult, error) {
	var b models.Build
	if err := readYAML(db.buildPath(buildID), &b); err != nil {
		return nil, err
	}
	return b.Steps, nil
}

// ============================================================
// Seed (import from legacy JSON/YAML files)
// ============================================================

func (db *DB) Seed(imagesPath, stepsPath, cloudinitDir string, force bool) error {
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
				fmt.Fprintf(os.Stderr, "warning: %s: %v (importing anyway)\n", entry.Name(), err)
			}
			dst := filepath.Join(db.ciDir(), entry.Name())
			if _, err := os.Stat(dst); err == nil && !force {
				continue // skip existing
			}
			if err := os.WriteFile(dst, content, 0644); err != nil {
				return fmt.Errorf("write %s: %w", entry.Name(), err)
			}
		}
	}

	// Import build steps
	if stepsPath != "" {
		steps, err := fileutil.LoadSteps(stepsPath)
		if err != nil {
			return fmt.Errorf("load steps: %w", err)
		}
		if err := db.ResetBuildSteps(steps); err != nil {
			return fmt.Errorf("reset steps: %w", err)
		}
	}

	// Import images as templates
	if imagesPath != "" {
		images, err := fileutil.LoadImages(imagesPath)
		if err != nil {
			return fmt.Errorf("load images: %w", err)
		}
		templates, _ := db.loadTemplates()
		existingNames := make(map[string]int)
		existingVMIDs := make(map[int]int)
		for i, t := range templates {
			existingNames[t.Name] = i
			existingVMIDs[t.VMID] = i
		}

		for _, img := range images {
			t := models.Template{
				VMID:        img.ID,
				Name:        img.Name,
				URL:         img.URL,
				ChecksumURL: img.ChecksumURL,
				Tags:        img.Tags,
				CloudInit:   img.Vendor,
			}
			if idx, ok := existingNames[img.Name]; ok {
				if force {
					templates[idx] = t
				}
			} else if _, ok := existingVMIDs[img.ID]; ok {
				if !force {
					continue
				}
			} else {
				templates = append(templates, t)
			}
		}
		if err := db.saveTemplates(templates); err != nil {
			return fmt.Errorf("save templates: %w", err)
		}
	}

	return nil
}
