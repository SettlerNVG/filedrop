package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	KeySize   = 32 // AES-256
	NonceSize = 12 // GCM nonce
	SaltSize  = 16
)

// DeriveKey creates AES key from password using PBKDF2
func DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, 100000, KeySize, sha256.New)
}

// GenerateSalt creates random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// GenerateKey creates random AES-256 key
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// KeyToString encodes key to base64 for sharing
func KeyToString(key []byte) string {
	return base64.URLEncoding.EncodeToString(key)
}

// KeyFromString decodes key from base64
func KeyFromString(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

// EncryptedWriter wraps io.Writer with AES-GCM encryption
type EncryptedWriter struct {
	writer io.Writer
	aead   cipher.AEAD
	buf    []byte
}

// NewEncryptedWriter creates encrypted writer
func NewEncryptedWriter(w io.Writer, key []byte) (*EncryptedWriter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &EncryptedWriter{
		writer: w,
		aead:   aead,
		buf:    make([]byte, 0, 64*1024+aead.Overhead()+NonceSize),
	}, nil
}

// WriteChunk encrypts and writes a chunk
func (ew *EncryptedWriter) WriteChunk(data []byte) error {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt: nonce + ciphertext
	ciphertext := ew.aead.Seal(nil, nonce, data, nil)

	// Write nonce
	if _, err := ew.writer.Write(nonce); err != nil {
		return err
	}

	// Write ciphertext
	if _, err := ew.writer.Write(ciphertext); err != nil {
		return err
	}

	return nil
}

// EncryptedReader wraps io.Reader with AES-GCM decryption
type EncryptedReader struct {
	reader io.Reader
	aead   cipher.AEAD
}

// NewEncryptedReader creates encrypted reader
func NewEncryptedReader(r io.Reader, key []byte) (*EncryptedReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &EncryptedReader{
		reader: r,
		aead:   aead,
	}, nil
}

// ReadChunk reads and decrypts a chunk
func (er *EncryptedReader) ReadChunk(chunkSize int) ([]byte, error) {
	// Read nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(er.reader, nonce); err != nil {
		return nil, err
	}

	// Read ciphertext (chunk + overhead)
	ciphertext := make([]byte, chunkSize+er.aead.Overhead())
	n, err := io.ReadFull(er.reader, ciphertext)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	ciphertext = ciphertext[:n]

	// Decrypt
	plaintext, err := er.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}
