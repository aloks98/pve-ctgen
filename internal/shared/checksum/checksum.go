package checksum

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"
)

// GetExpectedChecksum fetches the expected checksum for a given image from its checksum URL.
// It supports multiple checksum file formats:
//   - Standard: "checksum  filename"
//   - Fedora: "## filename" followed by "SHA256: checksum"
//   - Rocky/Alma: "filename (ALGORITHM) = checksum"
//   - Single value: just the checksum string
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

	// Standard format: "checksum  filename" or "checksum *filename"
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

	// Fedora format: "## filename" followed by "SHA256: checksum"
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

	// Rocky/Alma format: "filename (ALGORITHM) = checksum"
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

	// Single value format
	if checksum == "" && len(strings.Fields(bodyString)) == 1 {
		checksum = strings.Fields(bodyString)[0]
	}

	if checksum == "" {
		return "", "", fmt.Errorf("for %s not found in checksum file", filename)
	}

	algo, err := DetectAlgorithm(checksum)
	if err != nil {
		return "", "", err
	}

	return checksum, algo, nil
}

// DetectAlgorithm detects the hash algorithm based on checksum length.
func DetectAlgorithm(checksum string) (string, error) {
	switch len(checksum) {
	case 128:
		return "sha512", nil
	case 64:
		return "sha256", nil
	case 40:
		return "sha1", nil
	case 32:
		return "md5", nil
	default:
		return "", fmt.Errorf("unsupported checksum length: %d", len(checksum))
	}
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
