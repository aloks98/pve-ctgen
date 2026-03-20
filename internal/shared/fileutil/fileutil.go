package fileutil

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

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

// LoadImages loads image configurations from a JSON file.
func LoadImages(path string) ([]models.Image, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading images file: %w", err)
	}
	var images []models.Image
	if err := json.Unmarshal(file, &images); err != nil {
		return nil, fmt.Errorf("error parsing images JSON: %w", err)
	}
	return images, nil
}

// LoadSteps loads step configurations from a JSON file.
func LoadSteps(path string) ([]models.Step, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading steps file: %w", err)
	}
	var steps []models.Step
	if err := json.Unmarshal(file, &steps); err != nil {
		return nil, fmt.Errorf("error parsing steps JSON: %w", err)
	}
	return steps, nil
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
