package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"filedrop/internal/relay"
)

type Session struct {
	Code        string
	SenderConn  net.Conn
	RecvConn    net.Conn
	Ready       chan struct{}
	Done        chan struct{}
	BytesSent   int64
	StartTime   time.Time
	UserID      string
}

// DirectTransfer represents a direct user-to-user transfer request
type DirectTransfer struct {
	FromUser   string
	ToUser     string
	SenderConn net.Conn
	RecvConn   net.Conn
	Ready      chan struct{}
	Done       chan struct{}
	Accepted   bool
}

type RelayServer struct {
	sessions        map[string]*Session
	directTransfers map[string]*DirectTransfer // key: "fromUser:toUser"
	mu              sync.RWMutex
	auth            *relay.AuthManager
	presence        *relay.PresenceManager
	requireAuth     bool
	stats           *Stats
}

type Stats struct {
	TotalConnections int64
	ActiveSessions   int64
	BytesTransferred int64
}

func NewRelayServer(requireAuth bool) *RelayServer {
	auth := relay.NewAuthManager()
	auth.StartCleanupRoutine()

	return &RelayServer{
		sessions:        make(map[string]*Session),
		directTransfers: make(map[string]*DirectTransfer),
		auth:            auth,
		presence:        relay.NewPresenceManager(),
		requireAuth:     requireAuth,
		stats:           &Stats{},
	}
}

func (r *RelayServer) Run(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("🚀 Relay server listening on %s", addr)
	if r.requireAuth {
		log.Println("🔐 Authentication required")
	}

	// Start stats reporter
	go r.reportStats()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		atomic.AddInt64(&r.stats.TotalConnections, 1)
		go r.handleConnection(conn)
	}
}

func (r *RelayServer) reportStats() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		log.Printf("📊 Stats: connections=%d, active=%d, transferred=%s",
			atomic.LoadInt64(&r.stats.TotalConnections),
			atomic.LoadInt64(&r.stats.ActiveSessions),
			formatBytes(atomic.LoadInt64(&r.stats.BytesTransferred)))
	}
}

func (r *RelayServer) handleConnection(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()

	// Rate limiting
	ip := strings.Split(remoteAddr, ":")[0]
	if !r.auth.CheckRateLimit(ip) {
		conn.Write([]byte("ERROR Rate limited\n"))
		conn.Close()
		return
	}

	buf := make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	cmd := strings.TrimSpace(string(buf[:n]))
	parts := strings.SplitN(cmd, " ", 3)
	if len(parts) < 2 {
		conn.Write([]byte("ERROR Invalid command\n"))
		conn.Close()
		return
	}

	action := parts[0]
	var code, token string

	if r.requireAuth {
		if len(parts) < 3 {
			conn.Write([]byte("ERROR Auth required: SEND/RECV <code> <token>\n"))
			conn.Close()
			return
		}
		code = parts[1]
		token = parts[2]

		if _, valid := r.auth.ValidateToken(token); !valid {
			conn.Write([]byte("ERROR Invalid token\n"))
			conn.Close()
			return
		}
	} else {
		code = parts[1]
	}

	switch action {
	case "SEND":
		r.handleSender(conn, code, token)
	case "RECV":
		r.handleReceiver(conn, code)
	case "AUTH":
		r.handleAuth(conn, code) // code is actually apiKey here
	case "STATS":
		r.handleStats(conn)
	case "REGISTER":
		// REGISTER <userID> <username>
		if len(parts) >= 2 {
			username := code
			if len(parts) >= 3 {
				username = parts[2]
			}
			r.handleRegister(conn, code, username)
		}
	case "HEARTBEAT":
		r.handleHeartbeat(conn, code)
	case "USERS":
		r.handleUsers(conn)
	case "UNREGISTER":
		r.handleUnregister(conn, code)
	case "SENDTO":
		// SENDTO <targetUserID> <fromUserID>
		if len(parts) >= 3 {
			r.handleSendTo(conn, code, parts[2])
		} else {
			conn.Write([]byte("ERROR Usage: SENDTO <targetUserID> <fromUserID>\n"))
			conn.Close()
		}
	case "ACCEPT":
		// ACCEPT <fromUserID> <myUserID>
		if len(parts) >= 3 {
			r.handleAccept(conn, code, parts[2])
		} else {
			conn.Write([]byte("ERROR Usage: ACCEPT <fromUserID> <myUserID>\n"))
			conn.Close()
		}
	case "PENDING":
		r.handlePending(conn, code)
	default:
		conn.Write([]byte("ERROR Unknown command\n"))
		conn.Close()
	}
}

func (r *RelayServer) handleAuth(conn net.Conn, apiKey string) {
	defer conn.Close()

	token, err := r.auth.GenerateToken(apiKey)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("ERROR %s\n", err)))
		return
	}

	conn.Write([]byte(fmt.Sprintf("TOKEN %s\n", token.Value)))
	log.Printf("[AUTH] Token generated for user %s", token.UserID)
}

func (r *RelayServer) handleStats(conn net.Conn) {
	defer conn.Close()

	stats := fmt.Sprintf("STATS connections=%d active=%d bytes=%d\n",
		atomic.LoadInt64(&r.stats.TotalConnections),
		atomic.LoadInt64(&r.stats.ActiveSessions),
		atomic.LoadInt64(&r.stats.BytesTransferred))

	conn.Write([]byte(stats))
}

func (r *RelayServer) handleSender(conn net.Conn, code, token string) {
	r.mu.Lock()
	if _, exists := r.sessions[code]; exists {
		r.mu.Unlock()
		conn.Write([]byte("ERROR Code already in use\n"))
		conn.Close()
		return
	}

	session := &Session{
		Code:       code,
		SenderConn: conn,
		Ready:      make(chan struct{}),
		Done:       make(chan struct{}),
		StartTime:  time.Now(),
	}
	r.sessions[code] = session
	r.mu.Unlock()

	atomic.AddInt64(&r.stats.ActiveSessions, 1)
	defer atomic.AddInt64(&r.stats.ActiveSessions, -1)

	conn.Write([]byte("WAITING\n"))
	log.Printf("[%s] Sender waiting from %s", code, conn.RemoteAddr())

	select {
	case <-session.Ready:
		conn.Write([]byte("CONNECTED\n"))
		log.Printf("[%s] Transfer starting", code)
		r.relay(session)
	case <-time.After(10 * time.Minute):
		conn.Write([]byte("ERROR Timeout\n"))
		conn.Close()
	}

	r.mu.Lock()
	delete(r.sessions, code)
	r.mu.Unlock()
}

func (r *RelayServer) handleReceiver(conn net.Conn, code string) {
	r.mu.RLock()
	session, exists := r.sessions[code]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR Code not found\n"))
		conn.Close()
		return
	}

	session.RecvConn = conn
	conn.Write([]byte("CONNECTED\n"))
	log.Printf("[%s] Receiver connected from %s", code, conn.RemoteAddr())
	close(session.Ready)

	<-session.Done
}

func (r *RelayServer) relay(session *Session) {
	defer close(session.Done)
	defer session.SenderConn.Close()
	defer session.RecvConn.Close()

	done := make(chan struct{}, 2)

	// Sender -> Receiver with byte counting
	go func() {
		n, _ := io.Copy(session.RecvConn, session.SenderConn)
		atomic.AddInt64(&session.BytesSent, n)
		atomic.AddInt64(&r.stats.BytesTransferred, n)
		done <- struct{}{}
	}()

	// Receiver -> Sender (ACKs)
	go func() {
		io.Copy(session.SenderConn, session.RecvConn)
		done <- struct{}{}
	}()

	<-done

	duration := time.Since(session.StartTime)
	speed := float64(session.BytesSent) / duration.Seconds() / 1024 / 1024

	log.Printf("[%s] Transfer complete: %s in %v (%.2f MB/s)",
		session.Code,
		formatBytes(session.BytesSent),
		duration.Round(time.Second),
		speed)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// handleRegister registers user for presence
func (r *RelayServer) handleRegister(conn net.Conn, userID, username string) {
	ip := strings.Split(conn.RemoteAddr().String(), ":")[0]
	r.presence.Register(userID, username, ip)
	conn.Write([]byte("OK\n"))
	log.Printf("[PRESENCE] User %s (%s) registered", username, userID)

	// Keep connection alive for heartbeats
	go func() {
		reader := bufio.NewReader(conn)
		for {
			conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			line, err := reader.ReadString('\n')
			if err != nil {
				r.presence.Unregister(userID)
				conn.Close()
				log.Printf("[PRESENCE] User %s disconnected", userID)
				return
			}
			cmd := strings.TrimSpace(line)
			if cmd == "PING" {
				r.presence.Heartbeat(userID)
				conn.Write([]byte("PONG\n"))
			} else if cmd == "QUIT" {
				r.presence.Unregister(userID)
				conn.Close()
				return
			}
		}
	}()
}

// handleHeartbeat updates user presence
func (r *RelayServer) handleHeartbeat(conn net.Conn, userID string) {
	r.presence.Heartbeat(userID)
	conn.Write([]byte("OK\n"))
	conn.Close()
}

// handleUsers returns list of online users
func (r *RelayServer) handleUsers(conn net.Conn) {
	users := r.presence.ToJSON()
	conn.Write([]byte(fmt.Sprintf("USERS %s\n", string(users))))
	conn.Close()
}

// handleUnregister removes user from presence
func (r *RelayServer) handleUnregister(conn net.Conn, userID string) {
	r.presence.Unregister(userID)
	conn.Write([]byte("OK\n"))
	conn.Close()
}

// handleSendTo initiates direct transfer to user
func (r *RelayServer) handleSendTo(conn net.Conn, targetUserID, fromUserID string) {
	// Check if target user is online
	targetUser := r.presence.GetUser(targetUserID)
	if targetUser == nil || !targetUser.Online {
		conn.Write([]byte("ERROR User not online\n"))
		conn.Close()
		return
	}

	key := fromUserID + ":" + targetUserID

	r.mu.Lock()
	transfer := &DirectTransfer{
		FromUser:   fromUserID,
		ToUser:     targetUserID,
		SenderConn: conn,
		Ready:      make(chan struct{}),
		Done:       make(chan struct{}),
	}
	r.directTransfers[key] = transfer
	r.mu.Unlock()

	conn.Write([]byte("WAITING\n"))
	log.Printf("[DIRECT] %s wants to send to %s", fromUserID, targetUserID)

	// Wait for receiver to accept
	select {
	case <-transfer.Ready:
		if transfer.Accepted {
			conn.Write([]byte("CONNECTED\n"))
			log.Printf("[DIRECT] Transfer %s -> %s starting", fromUserID, targetUserID)
			r.relayDirect(transfer)
		} else {
			conn.Write([]byte("ERROR Transfer rejected\n"))
		}
	case <-time.After(5 * time.Minute):
		conn.Write([]byte("ERROR Timeout waiting for receiver\n"))
		conn.Close()
	}

	r.mu.Lock()
	delete(r.directTransfers, key)
	r.mu.Unlock()
}

// handleAccept accepts incoming transfer
func (r *RelayServer) handleAccept(conn net.Conn, fromUserID, myUserID string) {
	key := fromUserID + ":" + myUserID

	r.mu.RLock()
	transfer, exists := r.directTransfers[key]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR No pending transfer\n"))
		conn.Close()
		return
	}

	transfer.RecvConn = conn
	transfer.Accepted = true
	conn.Write([]byte("CONNECTED\n"))
	log.Printf("[DIRECT] %s accepted transfer from %s", myUserID, fromUserID)
	close(transfer.Ready)

	<-transfer.Done
}

// handlePending returns pending transfers for user
func (r *RelayServer) handlePending(conn net.Conn, userID string) {
	r.mu.RLock()
	var pending []string
	for _, transfer := range r.directTransfers {
		if transfer.ToUser == userID {
			pending = append(pending, transfer.FromUser)
		}
	}
	r.mu.RUnlock()

	data, _ := json.Marshal(pending)
	conn.Write([]byte(fmt.Sprintf("PENDING %s\n", string(data))))
	conn.Close()
}

// relayDirect relays data between two users
func (r *RelayServer) relayDirect(transfer *DirectTransfer) {
	defer close(transfer.Done)
	defer transfer.SenderConn.Close()
	defer transfer.RecvConn.Close()

	done := make(chan struct{}, 2)

	go func() {
		n, _ := io.Copy(transfer.RecvConn, transfer.SenderConn)
		atomic.AddInt64(&r.stats.BytesTransferred, n)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(transfer.SenderConn, transfer.RecvConn)
		done <- struct{}{}
	}()

	<-done
	log.Printf("[DIRECT] Transfer %s -> %s complete", transfer.FromUser, transfer.ToUser)
}

func main() {
	port := flag.String("port", "9000", "Relay server port")
	requireAuth := flag.Bool("auth", false, "Require authentication")
	genKey := flag.String("genkey", "", "Generate API key for user ID")
	flag.Parse()

	server := NewRelayServer(*requireAuth)

	// Generate API key if requested
	if *genKey != "" {
		key := server.auth.GenerateAPIKey(*genKey)
		fmt.Printf("API Key for %s: %s\n", *genKey, key)

		// Save to file
		f, _ := os.OpenFile("api_keys.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		defer f.Close()
		fmt.Fprintf(f, "%s:%s\n", *genKey, key)
		return
	}

	// Load existing API keys
	if *requireAuth {
		if f, err := os.Open("api_keys.txt"); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				parts := strings.SplitN(scanner.Text(), ":", 2)
				if len(parts) == 2 {
					server.auth.GenerateAPIKey(parts[0])
				}
			}
			f.Close()
		}
	}

	if err := server.Run(":" + *port); err != nil {
		log.Fatal(err)
	}
}
