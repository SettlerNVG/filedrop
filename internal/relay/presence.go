package relay

import (
	"encoding/json"
	"sync"
	"time"
)

// User represents online user
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IP        string    `json:"ip,omitempty"`
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"last_seen"`
	Available bool      `json:"available"` // ready to receive files
}

// PresenceManager tracks online users
type PresenceManager struct {
	users map[string]*User
	mu    sync.RWMutex
}

// NewPresenceManager creates presence manager
func NewPresenceManager() *PresenceManager {
	pm := &PresenceManager{
		users: make(map[string]*User),
	}
	go pm.cleanupRoutine()
	return pm
}

// Register adds or updates user
func (pm *PresenceManager) Register(id, name, ip string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.users[id] = &User{
		ID:        id,
		Name:      name,
		IP:        ip,
		Online:    true,
		LastSeen:  time.Now(),
		Available: true,
	}
}

// Heartbeat updates user's last seen time
func (pm *PresenceManager) Heartbeat(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if user, exists := pm.users[id]; exists {
		user.LastSeen = time.Now()
		user.Online = true
	}
}

// SetAvailable sets user availability
func (pm *PresenceManager) SetAvailable(id string, available bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if user, exists := pm.users[id]; exists {
		user.Available = available
	}
}

// Unregister removes user
func (pm *PresenceManager) Unregister(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if user, exists := pm.users[id]; exists {
		user.Online = false
	}
}

// GetOnlineUsers returns list of online users
func (pm *PresenceManager) GetOnlineUsers() []*User {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var users []*User
	for _, u := range pm.users {
		if u.Online && u.Available {
			users = append(users, &User{
				ID:       u.ID,
				Name:     u.Name,
				Online:   u.Online,
				LastSeen: u.LastSeen,
			})
		}
	}
	return users
}

// GetUser returns user by ID
func (pm *PresenceManager) GetUser(id string) *User {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if user, exists := pm.users[id]; exists {
		return user
	}
	return nil
}

// ToJSON serializes users list
func (pm *PresenceManager) ToJSON() []byte {
	users := pm.GetOnlineUsers()
	data, _ := json.Marshal(users)
	return data
}

// cleanupRoutine removes stale users
func (pm *PresenceManager) cleanupRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		pm.mu.Lock()
		cutoff := time.Now().Add(-2 * time.Minute)
		for id, user := range pm.users {
			if user.LastSeen.Before(cutoff) {
				user.Online = false
			}
			// Remove users offline for more than 10 minutes
			if user.LastSeen.Before(time.Now().Add(-10 * time.Minute)) {
				delete(pm.users, id)
			}
		}
		pm.mu.Unlock()
	}
}
