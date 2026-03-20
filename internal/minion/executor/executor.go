package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ExecuteCommand runs a shell command and sends each line of output on the output channel.
// The channel is NOT closed by this function — the caller manages it.
// Respects context cancellation.
func ExecuteCommand(ctx context.Context, command string, output chan<- string) error {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	done := make(chan struct{}, 2)

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			output <- scanner.Text()
		}
		done <- struct{}{}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			output <- scanner.Text()
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// SubstituteVars replaces template placeholders in a command string.
func SubstituteVars(command string, vars map[string]string) string {
	oldnew := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		oldnew = append(oldnew, "{{."+k+"}}", v)
	}
	return strings.NewReplacer(oldnew...).Replace(command)
}
