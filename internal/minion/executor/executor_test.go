package executor

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestSubstitutionIsLiteralInShell(t *testing.T) {
	value := "a ' quote \" $(printf injected) `id` $HOME; end"
	for _, s := range []string{"printf %s {{.Value}}", `printf %s "{{.Value}}"`, `printf %s '{{.Value}}'`} {
		cmd := SubstituteVars(s, map[string]string{"Value": value})
		out, err := exec.CommandContext(context.Background(), "bash", "-c", cmd).Output()
		if err != nil || string(out) != value {
			t.Fatalf("%s: got=%q err=%v", cmd, out, err)
		}
	}
}

func TestCancelKillsChildrenAndUnblocksOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := ExecuteCommand(ctx, "echo blocked; sleep 60 & wait", make(chan string))
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("cancel: %v after %v", err, time.Since(started))
	}
}

func TestBackgroundChildDoesNotHoldBuildOpen(t *testing.T) {
	start := time.Now()
	err := ExecuteCommand(context.Background(), "sleep 60 &", make(chan string, 1))
	if err == nil || time.Since(start) > 4*time.Second {
		t.Fatalf("background child cleanup: %v after %v", err, time.Since(start))
	}
}
