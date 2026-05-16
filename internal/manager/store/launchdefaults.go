package store

import "fmt"

// launchDefaultsFile stores last-used VM launch form values keyed by template name.
// Layout: map[templateName]map[fieldKey]value
const launchDefaultsFile = "launch-defaults.yaml"

// GetLaunchDefaults returns the last-used launch field values for a template.
// Returns an empty map (no error) if none recorded yet.
func (db *DB) GetLaunchDefaults(templateName string) (map[string]string, error) {
	all := map[string]map[string]string{}
	if err := readYAML(db.path(launchDefaultsFile), &all); err != nil {
		return nil, fmt.Errorf("read launch defaults: %w", err)
	}
	if v, ok := all[templateName]; ok && v != nil {
		return v, nil
	}
	return map[string]string{}, nil
}

// SaveLaunchDefaults persists the launch field values for a template.
func (db *DB) SaveLaunchDefaults(templateName string, vals map[string]string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	all := map[string]map[string]string{}
	_ = readYAML(db.path(launchDefaultsFile), &all)
	all[templateName] = vals
	return writeYAML(db.path(launchDefaultsFile), all)
}
