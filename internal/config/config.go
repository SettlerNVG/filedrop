package config

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RelayAddressURL is the URL to fetch current relay address
const RelayAddressURL = "https://raw.githubusercontent.com/SettlerNVG/filedrop/main/relay-address.txt"

// FallbackRelay is used if remote fetch fails
const FallbackRelay = "localhost:9000"

// GetDefaultRelay returns the default relay address
func GetDefaultRelay() string {
	// 1. Check environment variable (highest priority)
	if relay := os.Getenv("FILEDROP_RELAY"); relay != "" {
		return relay
	}

	// 2. Check local config file
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

	// 3. Fetch from remote
	relay := fetchRemoteRelay()
	if relay != "" {
		return relay
	}

	// 4. Fallback
	return FallbackRelay
}

// fetchRemoteRelay gets current relay address from GitHub
func fetchRemoteRelay() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(RelayAddressURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	relay := strings.TrimSpace(string(body))
	if relay == "" || relay == "localhost:9000" {
		return "" // Not configured yet
	}

	return relay
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
