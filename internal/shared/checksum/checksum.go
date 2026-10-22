package checksum

import (
	"context"
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
	"regexp"
	"strings"
	"time"
)

// GetExpectedChecksum fetches the expected checksum for a given image from its checksum URL.
// It supports multiple checksum file formats:
//   - Standard: "checksum  filename"
//   - Fedora: "## filename" followed by "SHA256: checksum"
//   - Rocky/Alma: "filename (ALGORITHM) = checksum"
//   - Single value: just the checksum string
func GetExpectedChecksum(url string, filename string) (string, string, error) {
	return GetExpectedChecksumContext(context.Background(), url, filename)
}

func GetExpectedChecksumContext(ctx context.Context, url, filename string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("checksum HTTP status: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if len(body) > 1<<20 {
		return "", "", fmt.Errorf("checksum file exceeds 1 MiB")
	}
	if err != nil {
		return "", "", err
	}

	return parseManifest(string(body), filename)
}

func parseManifest(body, filename string) (string, string, error) {
	hashes := map[string]string{}
	add := func(value string) error {
		algo, err := DetectAlgorithm(value)
		if err != nil {
			return nil
		}
		if algo != "sha256" && algo != "sha512" {
			return nil
		}
		if _, err = hex.DecodeString(value); err != nil {
			return fmt.Errorf("checksum is not hexadecimal")
		}
		value = strings.ToLower(value)
		if previous := hashes[algo]; previous != "" && previous != value {
			return fmt.Errorf("conflicting %s checksums for %s", algo, filename)
		}
		hashes[algo] = value
		return nil
	}
	bsd := regexp.MustCompile(`^(SHA256|SHA512) \((.+)\) = ([A-Fa-f0-9]+)$`)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) == 2 && !strings.HasPrefix(line, "#") && strings.TrimPrefix(fields[1], "*") == filename {
			if err := add(fields[0]); err != nil {
				return "", "", err
			}
		}
		if match := bsd.FindStringSubmatch(line); len(match) > 0 && match[2] == filename {
			want := 64
			if match[1] == "SHA512" {
				want = 128
			}
			if len(match[3]) != want {
				return "", "", fmt.Errorf("checksum algorithm/length mismatch")
			}
			if err := add(match[3]); err != nil {
				return "", "", err
			}
		}
		if line == "## "+filename && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(next, "SHA256: ") {
				if err := add(strings.TrimPrefix(next, "SHA256: ")); err != nil {
					return "", "", err
				}
			}
		}
	}
	if fields := strings.Fields(body); len(fields) == 1 {
		if err := add(fields[0]); err != nil {
			return "", "", err
		}
	}
	for _, algo := range []string{"sha512", "sha256"} {
		if hash := hashes[algo]; hash != "" {
			return hash, algo, nil
		}
	}
	return "", "", fmt.Errorf("SHA-256 or SHA-512 checksum for %s not found", filename)
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
