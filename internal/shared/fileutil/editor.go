package fileutil

import (
	"os"
	"os/exec"
)

// Editor returns the user's preferred editor.
// Checks $EDITOR, then falls back to nvim -> vim -> vi.
func Editor() string {
	if e, ok := os.LookupEnv("EDITOR"); ok && e != "" {
		return e
	}
	for _, e := range []string{"nvim", "vim", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return "vi"
}
