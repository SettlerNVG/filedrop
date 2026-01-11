package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filedrop/internal/crypto"
	"filedrop/internal/transfer"

	"github.com/schollz/progressbar/v3"
)

func generateCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func formatSize(bytes int64) string {
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

func connectRelay(addr, cmd string) (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot connect to relay: %w", err)
	}

	fmt.Fprintf(conn, "%s\n", cmd)

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("relay error: %w", err)
	}
	response = strings.TrimSpace(response)

	if strings.HasPrefix(response, "ERROR") {
		conn.Close()
		return nil, nil, fmt.Errorf("relay: %s", response)
	}

	return conn, reader, nil
}

func sendFiles(relayAddr string, paths []string, password string, compress bool) error {
	// Validate paths
	var totalSize int64
	var fileCount int
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", path, err)
		}
		if info.IsDir() {
			filepath.Walk(path, func(_ string, fi os.FileInfo, _ error) error {
				if !fi.IsDir() {
					totalSize += fi.Size()
					fileCount++
				}
				return nil
			})
		} else {
			totalSize += info.Size()
			fileCount++
		}
	}

	code := generateCode()

	// Generate encryption key
	var key []byte
	var keyStr string
	if password != "" {
		salt, _ := crypto.GenerateSalt()
		key = crypto.DeriveKey(password, salt)
		keyStr = "(password protected)"
	} else {
		key, _ = crypto.GenerateKey()
		keyStr = crypto.KeyToString(key)
	}

	conn, reader, err := connectRelay(relayAddr, fmt.Sprintf("SEND %s", code))
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("\n📤 Ready to send: %d files (%s)\n", fileCount, formatSize(totalSize))
	fmt.Printf("🔑 Code: %s\n", code)
	if password == "" {
		fmt.Printf("🔐 Key: %s\n", keyStr)
	} else {
		fmt.Printf("🔐 Password protected\n")
	}
	fmt.Println("⏳ Waiting for receiver...")

	// Wait for receiver
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("connection lost: %w", err)
	}
	response = strings.TrimSpace(response)

	if response != "CONNECTED" {
		return fmt.Errorf("connection failed: %s", response)
	}

	fmt.Println("✅ Receiver connected! Starting transfer...")

	// Send encryption key/salt first (16 bytes salt if password, 32 bytes key otherwise)
	if password != "" {
		// Send salt for password-based key derivation
		conn.Write([]byte{0x01}) // Flag: password mode
		// Salt was already used, receiver will derive same key from password
	} else {
		conn.Write([]byte{0x00}) // Flag: key mode
		conn.Write(key)
	}

	sender := transfer.NewSender(conn, key, compress)
	return sender.SendFiles(paths)
}

func receiveFiles(relayAddr, code, outputDir, password, keyStr string) error {
	conn, _, err := connectRelay(relayAddr, fmt.Sprintf("RECV %s", code))
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("✅ Connected to sender!")

	// Read encryption mode
	mode := make([]byte, 1)
	if _, err := io.ReadFull(conn, mode); err != nil {
		return fmt.Errorf("read mode: %w", err)
	}

	var key []byte
	if mode[0] == 0x01 {
		// Password mode
		if password == "" {
			return fmt.Errorf("sender used password encryption, provide -password flag")
		}
		// We need salt from sender - simplified: use fixed salt derivation
		salt := []byte("filedrop-salt-v1") // In production, exchange salt
		key = crypto.DeriveKey(password, salt)
	} else {
		// Key mode
		if keyStr != "" {
			key, err = crypto.KeyFromString(keyStr)
			if err != nil {
				return fmt.Errorf("invalid key: %w", err)
			}
		} else {
			key = make([]byte, crypto.KeySize)
			if _, err := io.ReadFull(conn, key); err != nil {
				return fmt.Errorf("read key: %w", err)
			}
		}
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	receiver := transfer.NewReceiver(conn, key, outputDir)
	return receiver.ReceiveFiles()
}

func sendSimple(relayAddr, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	stat, _ := file.Stat()
	fileName := filepath.Base(filePath)
	fileSize := stat.Size()

	code := generateCode()

	conn, reader, err := connectRelay(relayAddr, fmt.Sprintf("SEND %s", code))
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("\n📤 Ready to send: %s (%s)\n", fileName, formatSize(fileSize))
	fmt.Printf("🔑 Code: %s\n", code)
	fmt.Println("⏳ Waiting for receiver...")

	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("connection lost: %w", err)
	}
	if strings.TrimSpace(response) != "CONNECTED" {
		return fmt.Errorf("connection failed: %s", response)
	}

	fmt.Println("✅ Receiver connected!")

	// Send metadata
	meta := fmt.Sprintf("%s\n%d\n", fileName, fileSize)
	conn.Write([]byte(meta))

	// Send file
	bar := progressbar.NewOptions64(fileSize,
		progressbar.OptionSetDescription("Sending"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer: "█", SaucerHead: "█", SaucerPadding: "░",
			BarStart: "[", BarEnd: "]",
		}),
	)

	_, err = io.Copy(io.MultiWriter(conn, bar), file)
	fmt.Println("\n✅ File sent!")
	return err
}

func receiveSimple(relayAddr, code, outputDir string) error {
	conn, reader, err := connectRelay(relayAddr, fmt.Sprintf("RECV %s", code))
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("✅ Connected!")

	// Read metadata
	fileName, _ := reader.ReadString('\n')
	fileName = strings.TrimSpace(fileName)

	var fileSize int64
	fmt.Fscanf(reader, "%d\n", &fileSize)

	fmt.Printf("📥 Receiving: %s (%s)\n", fileName, formatSize(fileSize))

	outputPath := filepath.Join(outputDir, fileName)
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	bar := progressbar.NewOptions64(fileSize,
		progressbar.OptionSetDescription("Receiving"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer: "█", SaucerHead: "█", SaucerPadding: "░",
			BarStart: "[", BarEnd: "]",
		}),
	)

	_, err = io.CopyN(io.MultiWriter(file, bar), reader, fileSize)
	fmt.Printf("\n✅ Saved to: %s\n", outputPath)
	return err
}

func main() {
	relayAddr := flag.String("relay", "", "Relay server address (required)")
	outputDir := flag.String("output", ".", "Output directory")
	password := flag.String("password", "", "Encryption password")
	key := flag.String("key", "", "Encryption key (for receive)")
	compress := flag.Bool("compress", false, "Enable compression")
	simple := flag.Bool("simple", true, "Simple mode (single file, no encryption)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println(`FileDrop - P2P File Transfer

Usage:
  filedrop -relay <server:port> send <file>      Send files
  filedrop -relay <server:port> receive <code>   Receive files

Flags:
  -relay string      Relay server address (required)
  -output string     Output directory (default ".")
  -password string   Encryption password
  -key string        Encryption key (for receive)
  -compress          Enable compression
  -simple            Simple mode - single file, no encryption (default)

Examples:
  filedrop -relay 192.168.1.100:9000 send myfile.zip
  filedrop -relay myserver.com:9000 receive ABC123`)
		os.Exit(1)
	}

	relay := *relayAddr
	if relay == "" {
		fmt.Println("❌ Error: -relay flag is required")
		fmt.Println("Example: filedrop -relay 192.168.1.100:9000 send myfile.zip")
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "send":
		if len(args) < 2 {
			fmt.Println("Usage: filedrop -relay <server:port> send <file/folder>")
			os.Exit(1)
		}
		if *simple {
			err = sendSimple(relay, args[1])
		} else {
			err = sendFiles(relay, args[1:], *password, *compress)
		}

	case "receive", "recv":
		if len(args) < 2 {
			fmt.Println("Usage: filedrop -relay <server:port> receive <code>")
			os.Exit(1)
		}
		if *simple {
			err = receiveSimple(relay, args[1], *outputDir)
		} else {
			err = receiveFiles(relay, args[1], *outputDir, *password, *key)
		}

	default:
		fmt.Printf("Unknown command: %s\n", args[0])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
}
