package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultRelay is the default relay server address
// This can be overridden by:
// 1. -relay flag
// 2. FILEDROP_RELAY environment variable
// 3. ~/.filedrop/relay file
const FallbackRelay = "localhost:9000"

// GetDefaultRelay returns the default relay address
func GetDefaultRelay() string {
	// 1. Check environment variable
	if relay := os.Getenv("FILEDROP_RELAY"); relay != "" {
		return relay
	}

	// 2. Check config file
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".filedrop", "relay")
		if data, err := os.ReadFile(configPath); err == nil {
			relay := strings.TrimSpace(string(data))
			if relay != "" {
				return relay
			}
		}
	}

	// 3. Fallback
	return FallbackRelay
}

// SaveRelay saves relay address to config file
func SaveRelay(relay string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".filedrop")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "relay")
	return os.WriteFile(configPath, []byte(relay), 0644)
}
