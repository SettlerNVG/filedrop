package relay

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AuthManager handles relay authentication
type AuthManager struct {
	tokens    map[string]*Token
	apiKeys   map[string]string // apiKey -> userID
	mu        sync.RWMutex
	rateLimit *RateLimiter
}

// Token represents auth token
type Token struct {
	Value     string
	UserID    string
	ExpiresAt time.Time
}

// RateLimiter limits requests per IP
type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
	limit    int
	window   time.Duration
}

// NewAuthManager creates auth manager
func NewAuthManager() *AuthManager {
	return &AuthManager{
		tokens:  make(map[string]*Token),
		apiKeys: make(map[string]string),
		rateLimit: &RateLimiter{
			requests: make(map[string][]time.Time),
			limit:    100,
			window:   time.Minute,
		},
	}
}

// GenerateAPIKey creates new API key
func (am *AuthManager) GenerateAPIKey(userID string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	key := hex.EncodeToString(b)

	am.mu.Lock()
	am.apiKeys[key] = userID
	am.mu.Unlock()

	return key
}

// ValidateAPIKey checks if API key is valid
func (am *AuthManager) ValidateAPIKey(key string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	userID, exists := am.apiKeys[key]
	return userID, exists
}

// GenerateToken creates session token from API key
func (am *AuthManager) GenerateToken(apiKey string) (*Token, error) {
	userID, valid := am.ValidateAPIKey(apiKey)
	if !valid {
		return nil, fmt.Errorf("invalid API key")
	}

	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tokenValue := hex.EncodeToString(b)

	token := &Token{
		Value:     tokenValue,
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	am.mu.Lock()
	am.tokens[tokenValue] = token
	am.mu.Unlock()

	return token, nil
}

// ValidateToken checks if token is valid
func (am *AuthManager) ValidateToken(tokenValue string) (*Token, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	token, exists := am.tokens[tokenValue]
	if !exists {
		return nil, false
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, false
	}

	return token, true
}

// HashPassword creates password hash
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// VerifyPassword checks password against hash
func VerifyPassword(password, hash string) bool {
	computed := HashPassword(password)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(hash)) == 1
}

// CheckRateLimit checks if IP is rate limited
func (am *AuthManager) CheckRateLimit(ip string) bool {
	return am.rateLimit.Check(ip)
}

// Check checks if IP is rate limited
func (rl *RateLimiter) Check(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean old requests
	requests := rl.requests[ip]
	var valid []time.Time
	for _, t := range requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		return false
	}

	rl.requests[ip] = append(valid, now)
	return true
}

// CleanupExpired removes expired tokens
func (am *AuthManager) CleanupExpired() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	for key, token := range am.tokens {
		if now.After(token.ExpiresAt) {
			delete(am.tokens, key)
		}
	}
}

// StartCleanupRoutine starts background cleanup
func (am *AuthManager) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			am.CleanupExpired()
		}
	}()
}
