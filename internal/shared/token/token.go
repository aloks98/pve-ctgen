package token

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const tokenPrefix = "pvectgen-token://"

// ConnectionInfo holds the minion connection details encoded in a token.
type ConnectionInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	APIKey  string `json:"api_key"`
}

// Encode creates a connection token from the given connection info.
func Encode(info ConnectionInfo) (string, error) {
	data, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("failed to marshal connection info: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return tokenPrefix + encoded, nil
}

// Decode parses a connection token and returns the connection info.
func Decode(tokenStr string) (ConnectionInfo, error) {
	if !strings.HasPrefix(tokenStr, tokenPrefix) {
		return ConnectionInfo{}, fmt.Errorf("invalid token format: must start with %s", tokenPrefix)
	}

	encoded := strings.TrimPrefix(tokenStr, tokenPrefix)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to decode token: %w", err)
	}

	var info ConnectionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to parse token data: %w", err)
	}

	if info.Name == "" || info.Address == "" || info.APIKey == "" {
		return ConnectionInfo{}, fmt.Errorf("invalid token: missing required fields")
	}

	return info, nil
}
