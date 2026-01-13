package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if len(key) != KeySize {
		t.Errorf("Expected key size %d, got %d", KeySize, len(key))
	}
}

func TestKeyToString(t *testing.T) {
	key, _ := GenerateKey()
	keyStr := KeyToString(key)

	if len(keyStr) == 0 {
		t.Error("KeyToString returned empty string")
	}

	// Test round trip
	parsedKey, err := KeyFromString(keyStr)
	if err != nil {
		t.Fatalf("KeyFromString failed: %v", err)
	}

	if !bytes.Equal(key, parsedKey) {
		t.Error("Key round trip failed")
	}
}

func TestDeriveKey(t *testing.T) {
	password := "test123"
	salt, _ := GenerateSalt()

	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	if !bytes.Equal(key1, key2) {
		t.Error("DeriveKey should produce same key for same input")
	}

	if len(key1) != KeySize {
		t.Errorf("Expected key size %d, got %d", KeySize, len(key1))
	}
}

func TestEncryptedWriterReader(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte("Hello, World!")

	// Test with buffer
	var buf bytes.Buffer

	writer, err := NewEncryptedWriter(&buf, key)
	if err != nil {
		t.Fatalf("NewEncryptedWriter failed: %v", err)
	}

	err = writer.WriteChunk(plaintext)
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	reader, err := NewEncryptedReader(&buf, key)
	if err != nil {
		t.Fatalf("NewEncryptedReader failed: %v", err)
	}

	decrypted, err := reader.ReadChunk(len(plaintext))
	if err != nil {
		t.Fatalf("ReadChunk failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Decrypted text doesn't match original")
	}
}
