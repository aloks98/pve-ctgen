// Package initresolve loads a template's init config and prepares it for
// shipping to the minion. For cloud-init it returns YAML bytes unchanged.
// For Ignition it transpiles the Butane source to Ignition JSON and changes
// the filename extension to .ign.
package initresolve

import (
	"fmt"
	"strings"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/shared/ignition"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// Resolve returns (content bytes, snippet filename, error) for a template's init config.
// Returns empty values (no error) if the template has no init config attached.
func Resolve(db *store.DB, tmpl models.Template) ([]byte, string, error) {
	if tmpl.CloudInit == "" {
		return nil, "", nil
	}
	ci, err := db.GetCloudInit(tmpl.CloudInit)
	if err != nil {
		return nil, "", fmt.Errorf("load init config %q: %w", tmpl.CloudInit, err)
	}

	initType := tmpl.EffectiveInitType()
	if initType == models.InitTypeIgnition {
		jsonBytes, err := ignition.Transpile([]byte(ci.Content))
		if err != nil {
			return nil, "", fmt.Errorf("transpile %q: %w", tmpl.CloudInit, err)
		}
		return jsonBytes, ignitionFilename(ci.Name), nil
	}
	return []byte(ci.Content), ci.Name, nil
}

// ignitionFilename converts a Butane filename (.bu/.butane) to .ign.
func ignitionFilename(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".butane", ".bu"} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)] + ".ign"
		}
	}
	// Already an .ign or unknown extension — keep as-is
	if !strings.HasSuffix(lower, ".ign") {
		return name + ".ign"
	}
	return name
}
