package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filedrop/internal/config"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2)
)

// User represents online user
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Online   bool   `json:"online"`
	LastSeen string `json:"last_seen"`
}

func (u User) Title() string       { return u.Name }
func (u User) Description() string { return dimStyle.Render("ID: " + u.ID[:8] + "...") }
func (u User) FilterValue() string { return u.Name }

// View states
type viewState int

const (
	viewMain viewState = iota
	viewUsers
	viewFilePicker
	viewTransfer
	viewPending
	viewSetName
)

// Messages
type usersMsg []User
type pendingMsg []string
type transferStartMsg struct{}
type transferProgressMsg float64
type transferDoneMsg struct{ err error }
type tickMsg time.Time
type errMsg error

// Model
type model struct {
	relayAddr    string
	userID       string
	username     string
	presenceConn net.Conn

	view         viewState
	users        []User
	pending      []string
	selectedUser *User
	selectedFile string

	userList   list.Model
	filePicker filepicker.Model
	nameInput  textinput.Model
	progress   progress.Model

	transferring bool
	transferPct  float64
	transferErr  error
	status       string

	width  int
	height int
}

func generateUserID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func initialModel(relayAddr, username string) model {
	// File picker
	fp := filepicker.New()
	fp.CurrentDirectory, _ = os.Getwd()
	fp.AllowedTypes = []string{} // Allow all types

	// Name input
	ti := textinput.New()
	ti.Placeholder = "Enter your name"
	ti.Focus()
	ti.CharLimit = 32
	ti.Width = 30
	if username != "" {
		ti.SetValue(username)
	}

	// Progress bar
	prog := progress.New(progress.WithDefaultGradient())

	// User list
	delegate := list.NewDefaultDelegate()
	userList := list.New([]list.Item{}, delegate, 0, 0)
	userList.Title = "Online Users"
	userList.SetShowStatusBar(false)

	m := model{
		relayAddr:  relayAddr,
		userID:     generateUserID(),
		username:   username,
		view:       viewSetName,
		filePicker: fp,
		nameInput:  ti,
		progress:   prog,
		userList:   userList,
		status:     "Enter your name to start",
	}

	if username != "" {
		m.view = viewMain
		m.status = "Press Enter to continue"
	}

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.filePicker.Init(),
	)
}


func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.view == viewMain {
				m.disconnect()
				return m, tea.Quit
			}
			m.view = viewMain
			return m, nil
		case "esc":
			if m.view != viewMain && m.view != viewSetName {
				m.view = viewMain
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.userList.SetSize(msg.Width-4, msg.Height-10)
		m.filePicker.Height = msg.Height - 10
		return m, nil

	case usersMsg:
		m.users = msg
		items := make([]list.Item, len(msg))
		for i, u := range msg {
			items[i] = u
		}
		m.userList.SetItems(items)
		return m, nil

	case pendingMsg:
		m.pending = msg
		if len(msg) > 0 {
			m.status = fmt.Sprintf("📥 %d pending transfer(s)!", len(msg))
		}
		return m, nil

	case transferProgressMsg:
		m.transferPct = float64(msg)
		return m, nil

	case transferDoneMsg:
		m.transferring = false
		if msg.err != nil {
			m.transferErr = msg.err
			m.status = errorStyle.Render("Transfer failed: " + msg.err.Error())
		} else {
			m.status = successStyle.Render("✅ Transfer complete!")
		}
		m.view = viewMain
		return m, nil

	case tickMsg:
		if m.presenceConn != nil {
			return m, tea.Batch(
				m.refreshUsers(),
				m.checkPending(),
				tickCmd(),
			)
		}
		return m, tickCmd()

	case errMsg:
		m.status = errorStyle.Render(msg.Error())
		return m, nil
	}

	// Handle view-specific updates
	switch m.view {
	case viewSetName:
		return m.updateSetName(msg)
	case viewMain:
		return m.updateMain(msg)
	case viewUsers:
		return m.updateUsers(msg)
	case viewFilePicker:
		return m.updateFilePicker(msg)
	case viewPending:
		return m.updatePending(msg)
	case viewTransfer:
		return m.updateTransfer(msg)
	}

	return m, nil
}

func (m model) updateSetName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.status = errorStyle.Render("Name cannot be empty")
				return m, nil
			}
			m.username = name
			m.view = viewMain
			// Connect to relay
			return m, tea.Batch(
				m.connectPresence(),
				tickCmd(),
			)
		}
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "u", "1":
			m.view = viewUsers
			return m, m.refreshUsers()
		case "p", "2":
			m.view = viewPending
			return m, m.checkPending()
		case "r":
			return m, m.refreshUsers()
		}
	}
	return m, nil
}

func (m model) updateUsers(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.userList.SelectedItem().(User); ok {
				m.selectedUser = &item
				m.view = viewFilePicker
				return m, m.filePicker.Init()
			}
		case "r":
			return m, m.refreshUsers()
		}
	}

	var cmd tea.Cmd
	m.userList, cmd = m.userList.Update(msg)
	return m, cmd
}

func (m model) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)

	if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
		m.selectedFile = path
		m.view = viewTransfer
		m.transferring = true
		m.transferPct = 0
		return m, m.startTransfer()
	}

	return m, cmd
}

func (m model) updatePending(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if idx < len(m.pending) {
				fromUser := m.pending[idx]
				m.view = viewTransfer
				m.transferring = true
				return m, m.acceptTransfer(fromUser)
			}
		case "r":
			return m, m.checkPending()
		}
	}
	return m, nil
}

func (m model) updateTransfer(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Just show progress
	return m, nil
}


func (m model) View() string {
	var s strings.Builder

	// Header
	header := titleStyle.Render("📁 FileDrop TUI")
	s.WriteString(header + "\n")
	s.WriteString(dimStyle.Render(fmt.Sprintf("User: %s | Relay: %s", m.username, m.relayAddr)) + "\n\n")

	switch m.view {
	case viewSetName:
		s.WriteString(boxStyle.Render(
			"Welcome to FileDrop!\n\n" +
				"Enter your display name:\n\n" +
				m.nameInput.View() + "\n\n" +
				dimStyle.Render("Press Enter to continue"),
		))

	case viewMain:
		menu := boxStyle.Render(
			"Main Menu\n\n" +
				selectedStyle.Render("[1/u]") + " Browse online users\n" +
				selectedStyle.Render("[2/p]") + " Check pending transfers\n" +
				selectedStyle.Render("[r]") + "   Refresh\n" +
				selectedStyle.Render("[q]") + "   Quit\n\n" +
				fmt.Sprintf("Online users: %d", len(m.users)),
		)
		s.WriteString(menu)

	case viewUsers:
		if len(m.users) == 0 {
			s.WriteString(boxStyle.Render(
				"No users online\n\n" +
					dimStyle.Render("Press [r] to refresh, [esc] to go back"),
			))
		} else {
			s.WriteString(m.userList.View())
			s.WriteString("\n" + dimStyle.Render("Press Enter to select, [esc] to go back"))
		}

	case viewFilePicker:
		s.WriteString("Select a file to send to " + selectedStyle.Render(m.selectedUser.Name) + "\n\n")
		s.WriteString(m.filePicker.View())
		s.WriteString("\n" + dimStyle.Render("Press [esc] to cancel"))

	case viewPending:
		if len(m.pending) == 0 {
			s.WriteString(boxStyle.Render(
				"No pending transfers\n\n" +
					dimStyle.Render("Press [r] to refresh, [esc] to go back"),
			))
		} else {
			var pendingList strings.Builder
			pendingList.WriteString("Pending Transfers:\n\n")
			for i, from := range m.pending {
				pendingList.WriteString(fmt.Sprintf("[%d] From: %s\n", i+1, from))
			}
			pendingList.WriteString("\n" + dimStyle.Render("Press number to accept, [esc] to go back"))
			s.WriteString(boxStyle.Render(pendingList.String()))
		}

	case viewTransfer:
		var transferView strings.Builder
		if m.selectedFile != "" {
			transferView.WriteString(fmt.Sprintf("Sending: %s\n", filepath.Base(m.selectedFile)))
			transferView.WriteString(fmt.Sprintf("To: %s\n\n", m.selectedUser.Name))
		} else {
			transferView.WriteString("Receiving file...\n\n")
		}
		transferView.WriteString(m.progress.ViewAs(m.transferPct) + "\n\n")
		if m.transferring {
			transferView.WriteString(dimStyle.Render("Transfer in progress..."))
		}
		s.WriteString(boxStyle.Render(transferView.String()))
	}

	// Status bar
	s.WriteString("\n\n" + m.status)

	return s.String()
}

// Network commands
func (m *model) connectPresence() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", m.relayAddr, 10*time.Second)
		if err != nil {
			return errMsg(fmt.Errorf("cannot connect to relay: %w", err))
		}

		// Register
		fmt.Fprintf(conn, "REGISTER %s %s\n", m.userID, m.username)
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(response) != "OK" {
			conn.Close()
			return errMsg(fmt.Errorf("registration failed"))
		}

		m.presenceConn = conn

		// Start heartbeat goroutine
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if m.presenceConn == nil {
					return
				}
				m.presenceConn.Write([]byte("PING\n"))
			}
		}()

		return usersMsg{}
	}
}

func (m *model) disconnect() {
	if m.presenceConn != nil {
		m.presenceConn.Write([]byte("QUIT\n"))
		m.presenceConn.Close()
		m.presenceConn = nil
	}
}

func (m model) refreshUsers() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", m.relayAddr, 5*time.Second)
		if err != nil {
			return errMsg(err)
		}
		defer conn.Close()

		fmt.Fprintf(conn, "USERS\n")
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return errMsg(err)
		}

		response = strings.TrimSpace(response)
		if !strings.HasPrefix(response, "USERS ") {
			return errMsg(fmt.Errorf("unexpected response: %s", response))
		}

		jsonData := strings.TrimPrefix(response, "USERS ")
		var users []User
		if err := json.Unmarshal([]byte(jsonData), &users); err != nil {
			return errMsg(err)
		}

		// Filter out self
		var filtered []User
		for _, u := range users {
			if u.ID != m.userID {
				filtered = append(filtered, u)
			}
		}

		return usersMsg(filtered)
	}
}

func (m model) checkPending() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", m.relayAddr, 5*time.Second)
		if err != nil {
			return errMsg(err)
		}
		defer conn.Close()

		fmt.Fprintf(conn, "PENDING %s\n", m.userID)
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return errMsg(err)
		}

		response = strings.TrimSpace(response)
		if !strings.HasPrefix(response, "PENDING ") {
			return pendingMsg{}
		}

		jsonData := strings.TrimPrefix(response, "PENDING ")
		var pending []string
		json.Unmarshal([]byte(jsonData), &pending)

		return pendingMsg(pending)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}


func (m model) startTransfer() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", m.relayAddr, 10*time.Second)
		if err != nil {
			return transferDoneMsg{err: err}
		}
		defer conn.Close()

		// Send request
		fmt.Fprintf(conn, "SENDTO %s %s\n", m.selectedUser.ID, m.userID)
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return transferDoneMsg{err: err}
		}

		response = strings.TrimSpace(response)
		if response != "WAITING" {
			return transferDoneMsg{err: fmt.Errorf(response)}
		}

		// Wait for connection
		response, err = reader.ReadString('\n')
		if err != nil {
			return transferDoneMsg{err: err}
		}
		if strings.TrimSpace(response) != "CONNECTED" {
			return transferDoneMsg{err: fmt.Errorf(response)}
		}

		// Send file
		file, err := os.Open(m.selectedFile)
		if err != nil {
			return transferDoneMsg{err: err}
		}
		defer file.Close()

		stat, _ := file.Stat()
		fileName := filepath.Base(m.selectedFile)
		fileSize := stat.Size()

		// Send metadata
		meta := fmt.Sprintf("%s\n%d\n", fileName, fileSize)
		conn.Write([]byte(meta))

		// Send file data
		buf := make([]byte, 64*1024)
		var sent int64
		for {
			n, err := file.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				return transferDoneMsg{err: err}
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return transferDoneMsg{err: err}
			}
			sent += int64(n)
		}

		return transferDoneMsg{err: nil}
	}
}

func (m model) acceptTransfer(fromUserID string) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", m.relayAddr, 10*time.Second)
		if err != nil {
			return transferDoneMsg{err: err}
		}
		defer conn.Close()

		fmt.Fprintf(conn, "ACCEPT %s %s\n", fromUserID, m.userID)
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return transferDoneMsg{err: err}
		}

		if strings.TrimSpace(response) != "CONNECTED" {
			return transferDoneMsg{err: fmt.Errorf(response)}
		}

		// Read metadata
		fileName, _ := reader.ReadString('\n')
		fileName = strings.TrimSpace(fileName)

		var fileSize int64
		fmt.Fscanf(reader, "%d\n", &fileSize)

		// Create output file
		outputPath := filepath.Join(".", fileName)
		file, err := os.Create(outputPath)
		if err != nil {
			return transferDoneMsg{err: err}
		}
		defer file.Close()

		// Receive file
		_, err = io.CopyN(file, reader, fileSize)
		if err != nil {
			return transferDoneMsg{err: err}
		}

		return transferDoneMsg{err: nil}
	}
}

func main() {
	defaultRelay := config.GetDefaultRelay()
	relayAddr := flag.String("relay", defaultRelay, "Relay server address")
	username := flag.String("name", "", "Your display name")
	flag.Parse()

	relay := *relayAddr
	
	fmt.Printf("🔍 Connecting to relay: %s\n", relay)
	
	// Try to connect, fallback to localhost if fails
	conn, err := net.DialTimeout("tcp", relay, 5*time.Second)
	if err != nil {
		// Try localhost as fallback
		localRelay := "localhost:9000"
		fmt.Printf("⚠️  Cannot reach %s, trying localhost...\n", relay)
		conn, err = net.DialTimeout("tcp", localRelay, 2*time.Second)
		if err != nil {
			fmt.Printf("❌ Cannot connect to relay\n\n")
			fmt.Println("Options:")
			fmt.Println("  1. Wait for public relay to come online")
			fmt.Println("  2. Run your own: make run-relay")
			fmt.Println("  3. Specify relay: filedrop-tui -relay <address>")
			os.Exit(1)
		}
		relay = localRelay
	}
	conn.Close()
	fmt.Printf("✅ Connected!\n")
	time.Sleep(300 * time.Millisecond)

	p := tea.NewProgram(initialModel(relay, *username), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
