package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ExecuteCommand runs a shell command and sends each line of output on the output channel.
// The channel is NOT closed by this function — the caller manages it.
// Respects context cancellation.
func ExecuteCommand(ctx context.Context, command string, output chan<- string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	writer := &lineWriter{ctx: ctx, output: output, cancel: cancel}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	// Also reap detached descendants that inherited the shell's process group.
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return writer.flush()
}

type lineWriter struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	output  chan<- string
	pending []byte
}

func (w *lineWriter) send(line []byte) error {
	select {
	case w.output <- string(line):
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}
func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		if end < 0 {
			end = len(p)
		}
		if len(w.pending)+end > 1024*1024 {
			w.cancel()
			return 0, fmt.Errorf("command output line exceeds 1 MiB")
		}
		w.pending = append(w.pending, p[:end]...)
		if end == len(p) {
			break
		}
		if err := w.send(w.pending); err != nil {
			return 0, err
		}
		w.pending = w.pending[:0]
		p = p[end+1:]
	}
	return n, nil
}
func (w *lineWriter) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		return w.send(w.pending)
	}
	return nil
}

// SubstituteVars replaces template placeholders in a command string.
func SubstituteVars(command string, vars map[string]string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(command); {
		if strings.HasPrefix(command[i:], "{{.") {
			if end := strings.Index(command[i:], "}}"); end >= 0 {
				key := command[i+3 : i+end]
				if value, ok := vars[key]; ok {
					switch quote {
					case '\'':
						out.WriteString(strings.ReplaceAll(value, "'", "'\\''"))
					case '"':
						out.WriteString(strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`").Replace(value))
					default:
						out.WriteString("'" + strings.ReplaceAll(value, "'", "'\\''") + "'")
					}
					i += end + 2
					continue
				}
			}
		}
		ch := command[i]
		out.WriteByte(ch)
		i++
		if ch == '\\' && quote != '\'' && i < len(command) {
			out.WriteByte(command[i])
			i++
			continue
		}
		if (ch == '\'' || ch == '"') && (quote == 0 || quote == ch) {
			if quote == ch {
				quote = 0
			} else {
				quote = ch
			}
		}
	}
	return out.String()
}
