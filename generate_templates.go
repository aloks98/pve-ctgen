package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"gopkg.in/yaml.v2"
)

type Image struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Tags   string `json:"tags"`
	Vendor string `json:"vendor"`
}

type Vendor struct {
	RunCmd []string `yaml:"runcmd"`
}

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func main() {
	jsonFilePath := "os_list.json"

	fileContent, err := os.ReadFile(jsonFilePath)
	if err != nil {
		log.Fatal().Err(err).Msg("Error reading JSON file")
	}

	var images []Image
	if err := json.Unmarshal(fileContent, &images); err != nil {
		log.Fatal().Err(err).Msg("Error parsing JSON")
	}

	for _, img := range images {
		filePath := filepath.Join(".", img.Name)

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Info().Str("file", img.Name).Msg("Starting download")

			if err := downloadFile(filePath, img.URL); err != nil {
				log.Error().Err(err).Str("file", img.Name).Msg("Download failed")
				continue
			}

			log.Info().Str("file", img.Name).Msg("Download completed")
		} else {
			log.Info().Str("file", img.Name).Msg("File already exists")
		}

		// Create a copy of the downloaded file with the name base.qcow2
		baseFilePath := "base.qcow2"
		if err := copyFile(filePath, baseFilePath); err != nil {
			log.Error().Err(err).Str("file", img.Name).Msg("Copying failed")
			continue
		}

		// Execute the commands
		if err := executeCommands(baseFilePath, img); err != nil {
			log.Error().Err(err).Str("file", img.Name).Msg("Command execution failed")
			continue
		}
	}
}

func downloadFile(filePath string, url string) error {
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

	bar := progressbar.NewOptions64(
		resp.ContentLength,
		progressbar.OptionSetDescription("Downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
		progressbar.OptionSpinnerType(14),
	)

	if _, err := io.Copy(io.MultiWriter(file, bar), resp.Body); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	return nil
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

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("copy file content failed: %w", err)
	}

	return nil
}

func executeCommands(filePath string, img Image) error {
	// Check and destroy existing VM
	if err := checkAndDestroyVM(img.ID); err != nil {
		return fmt.Errorf("VM cleanup failed: %w", err)
	}

	commands := []struct {
		name string
		cmd  *exec.Cmd
	}{
		{
			"Resize disk",
			exec.Command("qemu-img", "resize", "-f", "qcow2", filePath, "32G"),
		},
		{
			"First boot command to truncate machine-id",
			exec.Command("virt-customize", "-a", filePath, "--firstboot-command", "sudo truncate -s 0 /etc/machine-id"),
		},
		{
			"First boot command to remove machine-id",
			exec.Command("virt-customize", "-a", filePath, "--firstboot-command", "sudo rm /var/lib/dbus/machine-id"),
		},
		{
			"First boot command to link machine-id",
			exec.Command("virt-customize", "-a", filePath, "--firstboot-command", "sudo ln -s /etc/machine-id /var/lib"),
		},
		{
			"Set Timezone",
			exec.Command("virt-customize", "-a", filePath, "--timezone", "Asia/Kolkata"),
		},
	}

	// Load vendor commands
	vendorFilePath := filepath.Join("vendors", img.Vendor)
	vendorFileContent, err := ioutil.ReadFile(vendorFilePath)
	if err != nil {
		log.Error().Err(err).Str("vendor", img.Vendor).Msg("Error reading vendor file")
		return err
	}

	var vendor Vendor
	if err := yaml.Unmarshal(vendorFileContent, &vendor); err != nil {
		log.Error().Err(err).Str("vendor", img.Vendor).Msg("Error parsing vendor file")
		return err
	}

	for _, cmd := range vendor.RunCmd {
		command := fmt.Sprintf("sudo %s", cmd)
		commands = append(commands, struct {
			name string
			cmd  *exec.Cmd
		}{
			name: fmt.Sprintf("Vendor command: %s", cmd),
			cmd:  exec.Command("virt-customize", "-a", filePath, "--firstboot-command", command),
		})
	}

	commands = append(commands, []struct {
		name string
		cmd  *exec.Cmd
	}{
		{
			"Create VM",
			exec.Command("qm", "create", fmt.Sprintf("%d", img.ID),
				"--name", fmt.Sprintf("%s-%d-cloudinit", img.Name, img.ID),
				"--ostype", "l26",
				"--memory", "1024",
				"--agent", "1",
				"--bios", "ovmf",
				"--machine", "q35",
				"--efidisk0", "local-lvm:0,pre-enrolled-keys=0",
				"--cpu", "host",
				"--socket", "1",
				"--cores", "2",
				"--vga", "serial0",
				"--serial0", "socket",
				"--net0", "virtio,bridge=vmbr0"),
		},
		{
			"Import disk",
			exec.Command("qm", "importdisk", fmt.Sprintf("%d", img.ID), filePath, "local-lvm"),
		},
		{
			"Set disk options",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID),
				"--scsihw", "virtio-scsi-pci",
				"--virtio0", fmt.Sprintf("local-lvm:vm-%d-disk-1,discard=on", img.ID)),
		},
		{
			"Set boot options",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--boot", "order=virtio0"),
		},
		{
			"Set cloud-init",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--scsi1", "local-lvm:cloudinit"),
		},
		{
			"Set IP configuration",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--ipconfig0", "ip=dhcp"),
		},
		{
			"Set tags",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--tags", img.Tags),
		},
		{
			"Set credentials",
			exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--ciuser", "root", "--cipassword", "alok@admin1"),
		},
	}...)

	// Execute all commands
	for _, c := range commands {
		if err := runCommandWithStreaming(c.cmd, c.name); err != nil {
			return err
		}
	}

	// Configure SSH keys
	if sshPath, err := resolveSSHKeysPath(); err == nil {
		sshCmd := exec.Command("qm", "set", fmt.Sprintf("%d", img.ID), "--sshkeys", sshPath)
		if err := runCommandWithStreaming(sshCmd, "Set SSH keys"); err != nil {
			log.Warn().Err(err).Msg("Failed to set SSH keys")
		}
	} else {
		log.Warn().Err(err).Msg("SSH keys not configured")
	}

	templateCmd := exec.Command("qm", "template", fmt.Sprintf("%d", img.ID))
	if err := runCommandWithStreaming(templateCmd, "Convert to template"); err != nil {
		log.Warn().Err(err).Msg("Failed to convert to template")
	}

	// Cleanup
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove base.qcow2 file: %w", err)
	}
	log.Info().Str("file", filePath).Msg("Cleanup complete")

	return nil
}

func checkAndDestroyVM(vmid int) error {
	// Check if VM exists
	statusCmd := exec.Command("qm", "status", fmt.Sprintf("%d", vmid))
	if err := statusCmd.Run(); err != nil {
		// VM doesn't exist
		return nil
	}

	log.Info().Int("vmid", vmid).Msg("Existing VM found, destroying...")
	destroyCmd := exec.Command("qm", "destroy", fmt.Sprintf("%d", vmid))
	if err := runCommandWithStreaming(destroyCmd, "Destroy VM"); err != nil {
		return fmt.Errorf("failed to destroy VM: %w", err)
	}
	return nil
}

func runCommandWithStreaming(cmd *exec.Cmd, name string) error {
	log.Info().Str("name", name).Str("command", cmd.String()).Msg("Executing command")

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	streamOutput := func(pipe io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			log.Info().
				Str("", scanner.Text()).
				Msg("stdout")
		}
	}

	go streamOutput(stdout)
	go streamOutput(stderr)

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	log.Info().Str("name", name).Msg("Command completed successfully")
	return nil
}

func resolveSSHKeysPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	path := filepath.Join(usr.HomeDir, ".ssh", "authorized_keys")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	return path, nil
}
