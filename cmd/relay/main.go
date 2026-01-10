package main

import (
	"bufio"
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

type RelayServer struct {
	sessions    map[string]*Session
	mu          sync.RWMutex
	auth        *relay.AuthManager
	requireAuth bool
	stats       *Stats
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
		sessions:    make(map[string]*Session),
		auth:        auth,
		requireAuth: requireAuth,
		stats:       &Stats{},
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
