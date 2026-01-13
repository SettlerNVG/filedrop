package config

import (
	"os"
	"testing"
)

func TestGetDefaultRelay(t *testing.T) {
	// Test default behavior
	relay := GetDefaultRelay()
	if relay == "" {
		t.Error("GetDefaultRelay returned empty string")
	}

	// Test environment variable override
	testRelay := "test.example.com:9000"
	os.Setenv("FILEDROP_RELAY", testRelay)
	defer os.Unsetenv("FILEDROP_RELAY")

	relay = GetDefaultRelay()
	if relay != testRelay {
		t.Errorf("Expected %s, got %s", testRelay, relay)
	}
}
