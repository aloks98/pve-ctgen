package utils

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/pkg/style"
	"github.com/aloks98/pve-ctgen/pkg/types"
	"github.com/rivo/tview"
)

// --- Progress Writer ---

// ProgressWriter is an io.Writer that reports download progress.
type ProgressWriter struct {
	Total      float64
	Downloaded float64
	LastUpdate time.Time
	App        *tview.Application
	OutputView *tview.TextView
}

// Write implements the io.Writer interface for ProgressWriter.
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += float64(n)

	if time.Since(pw.LastUpdate) > 100*time.Millisecond || pw.Downloaded == pw.Total {
		pw.LastUpdate = time.Now()
		percentage := (pw.Downloaded / pw.Total) * 100
		pw.App.QueueUpdateDraw(func() {
			pw.OutputView.Clear()
			pw.OutputView.SetText(fmt.Sprintf("Downloading: %.2f%%", percentage))
		})
	}
	return n, nil
}

// --- File and Network Operations ---

// LoadImages loads image configurations from a JSON file.
func LoadImages(path string) ([]types.Image, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading images file: %w", err)
	}
	var images []types.Image
	if err := json.Unmarshal(file, &images); err != nil {
		return nil, fmt.Errorf("error parsing images JSON: %w", err)
	}
	return images, nil
}

// LoadSteps loads step configurations from a JSON file.
func LoadSteps(path string) ([]types.Step, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading steps file: %w", err)
	}
	var steps []types.Step
	if err := json.Unmarshal(file, &steps); err != nil {
		return nil, fmt.Errorf("error parsing steps JSON: %w", err)
	}
	return steps, nil
}

// HandleDownloadAndChecksum handles the download and checksum verification of an image.
func HandleDownloadAndChecksum(app *tview.Application, stepView, commandView, outputView *tview.TextView, step *types.UISTep, img types.Image, isoFilePath string, updateNodeStatus func(*tview.TreeNode, string), appendOutput func(string)) (string, error) {
	updateNodeStatus(step.Node, "running")
	app.QueueUpdateDraw(func() {
		stepView.Clear()
		commandView.Clear()
		outputView.Clear()
		stepView.SetText(fmt.Sprintf("Step: %s\n[yellow]Verifying local file and checksum...", step.Name))
		commandView.SetText("No command")
	})

	filePath := filepath.Join(isoFilePath, img.Name)

	if _, err := os.Stat(filePath); err == nil {
		if img.ChecksumURL == "" {
			appendOutput("☑️ File exists, no checksum URL provided. Skipping check and download.\n")
			return filePath, nil
		}

		appendOutput("🔎 Verifying checksum...\n")
		filenameFromURL := filepath.Base(img.URL)
		expectedChecksum, algo, err := GetExpectedChecksum(img.ChecksumURL, filenameFromURL)
		if err != nil {
			appendOutput(fmt.Sprintf("⚠️ Could not get checksum: %v. Re-downloading...\n", err))
			os.Remove(filePath)
		} else {
			localChecksum, err := CalculateFileChecksum(filePath, algo)
			if err != nil {
				appendOutput(fmt.Sprintf("⚠️ Could not calculate local checksum: %v. Re-downloading...\n", err))
				os.Remove(filePath)
			} else if localChecksum == expectedChecksum {
				appendOutput(fmt.Sprintf("✅ Checksum match (%s). Skipping download.\n", algo))
				return filePath, nil
			} else {
				appendOutput(fmt.Sprintf("❌ Checksum mismatch (%s). Re-downloading...\n", algo))
				os.Remove(filePath)
			}
		}
	}

	if err := DownloadFile(app, stepView, commandView, outputView, filePath, img.URL); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	return filePath, nil
}

// GetExpectedChecksum fetches the expected checksum for a given image from its checksum URL.
func GetExpectedChecksum(url string, filename string) (string, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var checksum string
	bodyString := string(body)
	lines := strings.Split(bodyString, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			checksumFilename := strings.TrimPrefix(fields[1], "*")
			if checksumFilename == filename {
				checksum = fields[0]
				break
			}
		}
	}

	if checksum == "" {
		for i, line := range lines {
			if strings.HasPrefix(line, "## ") && strings.TrimPrefix(line, "## ") == filename {
				if i+1 < len(lines) {
					nextLine := lines[i+1]
					if strings.HasPrefix(nextLine, "SHA256: ") {
						checksum = strings.TrimSpace(strings.TrimPrefix(nextLine, "SHA256: "))
						break
					}
				}
			}
		}
	}

	if checksum == "" {
		for _, line := range lines {
			if strings.Contains(line, "("+filename+")") {
				parts := strings.Split(line, "= ")
				if len(parts) == 2 {
					checksum = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	if checksum == "" && len(strings.Fields(bodyString)) == 1 {
		checksum = strings.Fields(bodyString)[0]
	}

	if checksum == "" {
		return "", "", fmt.Errorf("for %s not found in checksum file", filename)
	}

	var algo string
	switch len(checksum) {
	case 128:
		algo = "sha512"
	case 64:
		algo = "sha256"
	case 40:
		algo = "sha1"
	case 32:
		algo = "md5"
	default:
		return "", "", fmt.Errorf("unsupported checksum length: %d", len(checksum))
	}

	return checksum, algo, nil
}

// CalculateFileChecksum calculates the checksum of a file using the specified algorithm.
func CalculateFileChecksum(filePath string, algorithm string) (string, error) {
	var h hash.Hash
	switch strings.ToLower(algorithm) {
	case "sha512":
		h = sha512.New()
	case "sha256":
		h = sha256.New()
	case "sha1":
		h = sha1.New()
	case "md5":
		h = md5.New()
	default:
		return "", fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DownloadFile downloads a file from the given URL to the specified path.
func DownloadFile(app *tview.Application, stepView, commandView, outputView *tview.TextView, filePath string, url string) error {
	app.QueueUpdateDraw(func() {
		stepView.SetText(fmt.Sprintf("Downloading %s", url))
		commandView.SetText("No command")
	})

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer file.Close()

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	totalSize := float64(resp.ContentLength)
	
	progressWriter := &ProgressWriter{
		Total:      totalSize,
		App:        app,
		OutputView: outputView,
	}

	writer := io.MultiWriter(file, progressWriter)

	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("response body read failed: %w", err)
	}

	return nil
}

// CopyFile copies a file from source to destination.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file failed: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file failed: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ExecuteCommands executes a series of commands for a given image.
func ExecuteCommands(app *tview.Application, stepView, commandView, outputView *tview.TextView, steps []*types.UISTep, filePath string, img types.Image, stepData []types.Step, updateNodeStatus func(*tview.TreeNode, string), appendOutput func(string), logError func(string, error)) error {
	cloudinitFilePath := filepath.Join("/var/lib/vz/snippets/", img.Vendor)
	configFilePath := filepath.Join("cloudinit", img.Vendor)
	if err := CopyFile(configFilePath, cloudinitFilePath); err != nil {
		return fmt.Errorf("copying cloudinit config failed: %w", err)
	}

	replacer := strings.NewReplacer(
		"{{.ID}}", fmt.Sprintf("%d", img.ID),
		"{{.Name}}", img.Name,
		"{{.Tags}}", img.Tags,
		"{{.Vendor}}", img.Vendor,
		"{{.FilePath}}", filePath,
	)

	var hasFailed bool
	for i, step := range stepData {
		uiStep := steps[i]
		if hasFailed {
			updateNodeStatus(uiStep.Node, "skipped")
			continue
		}

		updateNodeStatus(uiStep.Node, "running")
		commandString := replacer.Replace(step.Command)
		cmd := exec.Command("bash", "-c", commandString)

		if err := RunCommandWithStreaming(app, stepView, commandView, outputView, cmd, step.Name, appendOutput, logError); err != nil {
			updateNodeStatus(uiStep.Node, "failed")
			hasFailed = true
			logError(img.Name, fmt.Errorf("step '%s' failed: %w. Command: %s", step.Name, err, commandString))
		} else {
			updateNodeStatus(uiStep.Node, "success")
			time.Sleep(1 * time.Second)
		}
	}

	if err := os.Remove(filePath); err != nil {
		logError(img.Name, fmt.Errorf("failed to remove base.qcow2 file: %w", err))
	}

	if hasFailed {
		return fmt.Errorf("one or more steps failed for %s", img.Name)
	}
	return nil
}

// RunCommandWithStreaming executes a shell command and streams its output to the UI.
func RunCommandWithStreaming(app *tview.Application, stepView, commandView, outputView *tview.TextView, cmd *exec.Cmd, name string, appendOutput func(string), logError func(string, error)) error {
	var displayCmd string
	if len(cmd.Args) > 2 && cmd.Args[0] == "bash" && cmd.Args[1] == "-c" {
		displayCmd = cmd.Args[2]
	} else {
		displayCmd = strings.Join(cmd.Args, " ")
	}

	app.QueueUpdateDraw(func() {
		stepView.Clear()
		commandView.Clear()
		outputView.Clear()
		stepView.SetText(name)
		writer := tview.ANSIWriter(commandView)
		fmt.Fprint(writer, style.Yellow(displayCmd))
	})

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			appendOutput(scanner.Text() + "\n")
		}
		if scanner.Err() != nil {
			logError("command_stdout_stream", fmt.Errorf("error reading stdout: %w", scanner.Err()))
		}
		done <- struct{}{}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			appendOutput(scanner.Text() + "\n")
		}
		if scanner.Err() != nil {
			logError("command_stderr_stream", fmt.Errorf("error reading stderr: %w", scanner.Err()))
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

// LogError logs an error to a file specific to the image name.
func LogError(imageName string, err error) {
	logFilePath := filepath.Join("logs", fmt.Sprintf("%s.error.log", imageName))
	f, _ := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		f.WriteString(fmt.Sprintf("[%s] %v\n", time.Now().Format(time.RFC3339), err))
	}
}
