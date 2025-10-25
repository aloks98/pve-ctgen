package main

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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// --- Data Structures ---

type Image struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ChecksumURL string `json:"checksum_url"`
	Tags        string `json:"tags"`
	Vendor      string `json:"vendor"`
}

type Step struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type UISTep struct {
	Node   *tview.TreeNode
	Name   string
}

type UIImage struct {
	Node  *tview.TreeNode
	Image Image
	Steps []*UISTep
}

// --- UI Components ---

var (
	app           = tview.NewApplication()
	stepsTree     = tview.NewTreeView()
	cmdView       = tview.NewTextView()
	outputView    = tview.NewTextView()
)

func init() {
	app.SetMouseCapture(nil)
}

// --- Main Application Logic ---

func main() {
	// --- UI Setup ---
	stepsTree.SetRoot(tview.NewTreeNode("Cloud-Init Template Generation").SetColor(tcell.ColorRed))
	stepsTree.SetCurrentNode(stepsTree.GetRoot())
	stepsTree.SetBorder(true).SetTitle("Progress")
	stepsTree.SetChangedFunc(func(node *tview.TreeNode) {
		// Output display for selected steps has been removed as per user request.
	})

	cmdView.SetBorder(true)
	cmdView.SetTitle("Current Command")
	cmdView.SetDynamicColors(true)

	outputView.SetBorder(true)
	outputView.SetTitle("Live Output")
	outputView.SetDynamicColors(true)
	outputView.SetScrollable(true)

	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cmdView, 3, 1, false).
		AddItem(outputView, 0, 1, true)

	layout := tview.NewFlex().
		AddItem(stepsTree, 0, 1, true).AddItem(rightPanel, 0, 3, true)

	// --- Generator Goroutine ---
	go func() {
		if err := runGenerator(); err != nil {
			app.Stop()
		}
	}()

	// --- Run Application ---
	if err := app.SetRoot(layout, true).Run(); err != nil {
		panic(err)
	}
}

func runGenerator() error {
	isoFilePath := "/var/lib/vz/template/iso"
	snippetsFilePath := "/var/lib/vz/snippets"

	images, err := loadImages("config/os_list.json")
	if err != nil {
		return err
	}
	steps, err := loadSteps("config/steps.json")
	if err != nil {
		return err
	}

	uiImages := buildUITree(images, steps)

	if err := os.MkdirAll(isoFilePath, 0755); err != nil {
		return fmt.Errorf("error creating iso folder: %w", err)
	}
	if err := os.MkdirAll(snippetsFilePath, 0755); err != nil {
		return fmt.Errorf("error creating snippets folder: %w", err)
	}
	if err := os.MkdirAll("logs", 0755); err != nil {
		return fmt.Errorf("error creating logs folder: %w", err)
	}

	var previousImageNode *tview.TreeNode
	for _, uiImage := range uiImages {
		app.QueueUpdateDraw(func() {
			if previousImageNode != nil {
				previousImageNode.Collapse()
			}
			uiImage.Node.SetColor(tcell.ColorYellow)
			stepsTree.SetCurrentNode(uiImage.Node)
			uiImage.Node.Expand()
			previousImageNode = uiImage.Node
		})

		var hasFailed bool
		var filePath string

		// --- Download & Verify Step ---
		downloadStep := uiImage.Steps[0]
		filePath, err = handleDownloadAndChecksum(downloadStep, uiImage.Image, isoFilePath)
		if err != nil {
			logError(uiImage.Image.Name, err)
			updateNodeStatus(downloadStep.Node, "failed")
			hasFailed = true
		} else {
			updateNodeStatus(downloadStep.Node, "success")
			time.Sleep(1 * time.Second)
		}

		// --- Copy Image Step ---
		baseFilePath := "base.qcow2"
		if !hasFailed {
			copyStep := uiImage.Steps[1]
			updateNodeStatus(copyStep.Node, "running")
			app.QueueUpdateDraw(func() {
				cmdView.Clear()
				outputView.Clear()
				cmdView.SetText(fmt.Sprintf("Step: %s\n[yellow]Command: cp %s %s", copyStep.Name, filePath, baseFilePath))
			})
			appendOutput(fmt.Sprintf("Copying %s to %s...\n", filePath, baseFilePath))
			if err := copyFile(filePath, baseFilePath); err != nil {
				logError(uiImage.Image.Name, err)
				updateNodeStatus(copyStep.Node, "failed")
				hasFailed = true
			} else {
				appendOutput("Copy complete.\n")
				updateNodeStatus(copyStep.Node, "success")
				time.Sleep(1 * time.Second)
			}
		}

		// --- Dynamic Execution Steps ---
		if !hasFailed {
			executionSteps := uiImage.Steps[2:]
			if err := executeCommands(executionSteps, baseFilePath, uiImage.Image, steps); err != nil {
				logError(uiImage.Image.Name, err)
				hasFailed = true
			}
		}

		// --- Final Status ---
		if hasFailed {
			markRemainingStepsAsSkipped(uiImage.Steps)
			uiImage.Node.SetText(fmt.Sprintf("❌ %s", uiImage.Image.Name))
		} else {
			uiImage.Node.SetText(fmt.Sprintf("✅ %s", uiImage.Image.Name))
		}
		uiImage.Node.SetColor(tcell.ColorDefault)
	}

	return nil
}

// --- UI & State Management ---

func buildUITree(images []Image, steps []Step) []*UIImage {
	rootNode := stepsTree.GetRoot()
	uiImages := make([]*UIImage, len(images))

	staticSteps := []string{"Download/Verify", "Copy Image"}

	for i, img := range images {
		imgNode := tview.NewTreeNode(fmt.Sprintf("🖼️  %s", img.Name)).SetColor(tcell.ColorGrey)
		rootNode.AddChild(imgNode)

		uiImage := &UIImage{Node: imgNode, Image: img}

		for _, stepName := range staticSteps {
			stepNode := tview.NewTreeNode(stepName)
			uiStep := &UISTep{Node: stepNode, Name: stepName}
			stepNode.SetReference(uiStep)
			updateNodeStatus(stepNode, "pending")
			imgNode.AddChild(stepNode)
			uiImage.Steps = append(uiImage.Steps, uiStep)
		}

		for _, step := range steps {
			stepNode := tview.NewTreeNode(step.Name)
			uiStep := &UISTep{Node: stepNode, Name: step.Name}
			stepNode.SetReference(uiStep)
			updateNodeStatus(stepNode, "pending")
			imgNode.AddChild(stepNode)
			uiImage.Steps = append(uiImage.Steps, uiStep)
		}
		uiImages[i] = uiImage
	}
	app.QueueUpdateDraw(func() {})
	return uiImages
}

func updateNodeStatus(node *tview.TreeNode, status string) {
	var icon string
	var color tcell.Color
	switch status {
	case "running":
		icon = "⚙️"
		color = tcell.ColorYellow
	case "success":
		icon = "✅"
		color = tcell.ColorGreen
	case "failed":
		icon = "❌"
		color = tcell.ColorRed
	case "skipped":
		icon = "➖"
		color = tcell.ColorDarkGrey
	default: // pending
		icon = "❔"
		color = tcell.ColorGrey
	}
	text := strings.TrimLeft(node.GetText(), "✅⚙️❌➖❔ ")
	app.QueueUpdateDraw(func() {
		node.SetText(fmt.Sprintf("%s %s", icon, text)).SetColor(color)
	})
}

func markRemainingStepsAsSkipped(steps []*UISTep) {
	for _, step := range steps {
		text := step.Node.GetText()
		if strings.HasPrefix(text, "❔") {
			updateNodeStatus(step.Node, "skipped")
		}
	}
}

func appendOutput(text string) {
	app.QueueUpdateDraw(func() {
		outputView.Write([]byte(text))
	})
}

func logError(imageName string, err error) {
	logFilePath := filepath.Join("logs", fmt.Sprintf("%s.error.log", imageName))
	f, _ := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		f.WriteString(fmt.Sprintf("[%s] %v\n", time.Now().Format(time.RFC3339), err))
	}
}

// --- Core Logic Functions ---

func loadImages(path string) ([]Image, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading images file: %w", err)
	}
	var images []Image
	if err := json.Unmarshal(file, &images); err != nil {
		return nil, fmt.Errorf("error parsing images JSON: %w", err)
	}
	return images, nil
}

func loadSteps(path string) ([]Step, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading steps file: %w", err)
	}
	var steps []Step
	if err := json.Unmarshal(file, &steps); err != nil {
		return nil, fmt.Errorf("error parsing steps JSON: %w", err)
	}
	return steps, nil
}

func handleDownloadAndChecksum(step *UISTep, img Image, isoFilePath string) (string, error) {
	updateNodeStatus(step.Node, "running")
	app.QueueUpdateDraw(func() {
		cmdView.Clear()
		outputView.Clear()
		cmdView.SetText(fmt.Sprintf("Step: %s\n[yellow]Verifying local file and checksum...", step.Name))
	})

	filePath := filepath.Join(isoFilePath, img.Name)

	if _, err := os.Stat(filePath); err == nil {
		if img.ChecksumURL == "" {
			appendOutput("☑️ File exists, no checksum URL provided. Skipping check and download.\n")
			return filePath, nil
		}

		appendOutput("🔎 Verifying checksum...\n")
		filenameFromURL := filepath.Base(img.URL)
		expectedChecksum, algo, err := getExpectedChecksum(img.ChecksumURL, filenameFromURL)
		if err != nil {
			appendOutput(fmt.Sprintf("⚠️ Could not get checksum: %v. Re-downloading...\n", err))
			os.Remove(filePath)
		} else {
			localChecksum, err := calculateFileChecksum(filePath, algo)
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

	if err := downloadFile(filePath, img.URL); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	return filePath, nil
}

func getExpectedChecksum(url string, filename string) (string, string, error) {
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

	// --- Strategy 1: Standard format (Debian, Ubuntu, AlmaLinux) ---
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

	// --- Strategy 2: Fedora format ---
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

	// --- Strategy 3: Rocky Linux format ---
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

	// --- Strategy 4: Single value in file ---
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

func calculateFileChecksum(filePath string, algorithm string) (string, error) {
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

func downloadFile(filePath string, url string) error {
	app.QueueUpdateDraw(func() {
		cmdView.SetText(fmt.Sprintf("Downloading %s", url))
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
	var downloaded float64
	var written int64

	writer := io.MultiWriter(file, &progressWriter{
		total:      totalSize,
		downloaded: &downloaded,
	})

	written, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("response body read failed: %w", err)
	}
	if totalSize != 0 && float64(written) != totalSize {
		return fmt.Errorf("download incomplete: wrote %d bytes, expected %f", written, totalSize)
	}

	return nil
}

type progressWriter struct {
	total      float64
	downloaded *float64
	lastUpdate time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	*pw.downloaded += float64(n)

	if time.Since(pw.lastUpdate) > 100*time.Millisecond || *pw.downloaded == pw.total {
		pw.lastUpdate = time.Now()
		percentage := (*pw.downloaded / pw.total) * 100
		app.QueueUpdateDraw(func() {
			outputView.Clear()
			outputView.SetText(fmt.Sprintf("Downloading: %.2f%%", percentage))
		})
	}
	return n, nil
}

func copyFile(src, dst string) error {
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

func executeCommands(steps []*UISTep, filePath string, img Image, stepData []Step) error {
	cloudinitFilePath := filepath.Join("/var/lib/vz/snippets/", img.Vendor)
	configFilePath := filepath.Join("cloudinit", img.Vendor)
	if err := copyFile(configFilePath, cloudinitFilePath); err != nil {
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

		if err := runCommandWithStreaming(cmd, step.Name); err != nil {
			updateNodeStatus(uiStep.Node, "failed")
			hasFailed = true
			logError(img.Name, fmt.Errorf("step '%s' failed: %w. Command: %s", step.Name, err, commandString))
		} else {
			updateNodeStatus(uiStep.Node, "success")
			time.Sleep(1 * time.Second)
		}
	}

	if err := os.Remove(filePath); err != nil {
		// Log this but don't fail the whole process for it
		logError(img.Name, fmt.Errorf("failed to remove base.qcow2 file: %w", err))
	}

	if hasFailed {
		return fmt.Errorf("one or more steps failed for %s", img.Name)
	}
	return nil
}

func runCommandWithStreaming(cmd *exec.Cmd, name string) error {
	app.QueueUpdateDraw(func() {
		cmdView.Clear()
		outputView.Clear()
		var displayCmd string
		if len(cmd.Args) > 2 && cmd.Args[0] == "bash" && cmd.Args[1] == "-c" {
			displayCmd = cmd.Args[2]
		} else {
			displayCmd = strings.Join(cmd.Args, " ")
		}
		cmdView.SetText(fmt.Sprintf("Step: %s\n[yellow]Command: %s", name, displayCmd))
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

