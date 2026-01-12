package transfer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"filedrop/internal/crypto"

	"github.com/schollz/progressbar/v3"
)

// Receiver handles file receiving
type Receiver struct {
	conn         io.ReadWriter
	key          []byte
	outputDir    string
	currentFile  int
	bytesWritten int64
	mu           sync.Mutex
	shutdown     chan struct{}
}

// NewReceiver creates new receiver
func NewReceiver(conn io.ReadWriter, key []byte, outputDir string) *Receiver {
	return &Receiver{
		conn:      conn,
		key:       key,
		outputDir: outputDir,
		shutdown:  make(chan struct{}),
	}
}

// ReceiveMetadata получает метаданные передачи
func (r *Receiver) ReceiveMetadata() (*TransferMetadata, error) {
	return ReadMetadata(r.conn)
}

// ReceiveFiles receives multiple files
func (r *Receiver) ReceiveFiles() error {
	// Read metadata
	meta, err := ReadMetadata(r.conn)
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}
	return r.ReceiveFilesWithMetadata(meta)
}

// ReceiveFilesWithMetadata receives files using provided metadata
func (r *Receiver) ReceiveFilesWithMetadata(meta *TransferMetadata) error {

	fmt.Printf("📥 Receiving %d files (%s)\n", meta.TotalFiles, formatSize(meta.TotalSize))
	if meta.Encrypted {
		fmt.Println("🔒 Transfer is encrypted")
	}
	if meta.Compressed {
		fmt.Println("📦 Transfer is compressed")
	}

	// Check for existing partial transfer
	resumeInfo := r.checkResume(meta)
	if resumeInfo != nil {
		_, _ = r.conn.Write([]byte("R"))
		if err := WriteResumeInfo(r.conn, resumeInfo); err != nil {
			return fmt.Errorf("send resume info: %w", err)
		}
		fmt.Printf("⏩ Resuming transfer from file %d\n", resumeInfo.CurrentFile)
	} else {
		_, _ = r.conn.Write([]byte("A")) // ACK, start fresh
	}

	// Create progress bar
	bar := progressbar.NewOptions64(
		meta.TotalSize,
		progressbar.OptionSetDescription("Receiving"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// Skip already received bytes in progress
	if resumeInfo != nil {
		_ = bar.Add64(resumeInfo.BytesWritten)
	}

	startIdx := 0
	var resumeFrom int64 = 0
	if resumeInfo != nil {
		startIdx = resumeInfo.CurrentFile
		resumeFrom = resumeInfo.BytesWritten
	}

	// Receive files
	for i := startIdx; i < len(meta.Files); i++ {
		// Обновляем текущий файл и сбрасываем счётчик байт
		r.mu.Lock()
		r.currentFile = i
		r.bytesWritten = resumeFrom
		r.mu.Unlock()

		fi := meta.Files[i]

		outputPath := filepath.Join(r.outputDir, fi.Path)

		if fi.IsDir {
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", outputPath, err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}

		if err := r.receiveFile(outputPath, fi, meta, bar, resumeFrom); err != nil {
			// Save progress for resume
			r.saveProgress(meta, i, resumeFrom)
			return fmt.Errorf("receive %s: %w", fi.Path, err)
		}

		// После успешного завершения файла сбрасываем прогресс
		r.mu.Lock()
		r.bytesWritten = 0
		r.mu.Unlock()

		resumeFrom = 0
	}

	// Clean up progress file
	r.cleanProgress(meta.TransferID)

	fmt.Printf("\n✅ Files saved to: %s\n", r.outputDir)
	return nil
}

func (r *Receiver) receiveFile(path string, info FileInfo, meta *TransferMetadata, bar *progressbar.ProgressBar, resumeFrom int64) error {
	flags := os.O_CREATE | os.O_WRONLY
	if resumeFrom > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	var encReader *crypto.EncryptedReader
	if r.key != nil {
		encReader, err = crypto.NewEncryptedReader(r.conn, r.key)
		if err != nil {
			return err
		}
	}

	for {
		// Проверяем сигнал завершения перед каждым чтением
		select {
		case <-r.shutdown:
			return fmt.Errorf("передача прервана пользователем")
		default:
		}

		// Read chunk size
		var chunkSize uint32
		if err := binary.Read(r.conn, binary.BigEndian, &chunkSize); err != nil {
			return err
		}

		if chunkSize == 0 {
			break // End of file
		}

		var data []byte
		if encReader != nil {
			data, err = encReader.ReadChunk(int(chunkSize))
			if err != nil {
				return err
			}
		} else {
			data = make([]byte, chunkSize)
			if _, err := io.ReadFull(r.conn, data); err != nil {
				return err
			}
		}

		if _, err := file.Write(data); err != nil {
			return err
		}

		// Обновляем прогресс
		r.mu.Lock()
		r.bytesWritten += int64(len(data))
		r.mu.Unlock()

		_ = bar.Add(len(data))
	}

	// Set modification time
	// os.Chtimes(path, time.Now(), time.Unix(info.ModTime, 0))

	return nil
}

func (r *Receiver) checkResume(meta *TransferMetadata) *ResumeInfo {
	progressFile := filepath.Join(r.outputDir, ".filedrop_progress_"+meta.TransferID)
	data, err := os.ReadFile(progressFile)
	if err != nil {
		return nil
	}

	var info ResumeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}

	return &info
}

func (r *Receiver) saveProgress(meta *TransferMetadata, fileIdx int, bytesWritten int64) {
	info := ResumeInfo{
		TransferID:   meta.TransferID,
		CurrentFile:  fileIdx,
		BytesWritten: bytesWritten,
	}

	data, _ := json.Marshal(info)
	progressFile := filepath.Join(r.outputDir, ".filedrop_progress_"+meta.TransferID)
	_ = os.WriteFile(progressFile, data, 0644)
}

func (r *Receiver) cleanProgress(transferID string) {
	progressFile := filepath.Join(r.outputDir, ".filedrop_progress_"+transferID)
	os.Remove(progressFile)
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

// SetupShutdown настраивает обработку сигналов завершения
func (r *Receiver) SetupShutdown(meta *TransferMetadata) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Получен сигнал %v, сохраняю прогресс...", sig)

		r.mu.Lock()
		r.saveProgress(meta, r.currentFile, r.bytesWritten)
		r.mu.Unlock()

		close(r.shutdown)
		log.Println("Прогресс сохранён, выход")
		os.Exit(0)
	}()
}

// SaveProgressTest - тестовый метод для проверки сохранения прогресса
func (r *Receiver) SaveProgressTest(meta *TransferMetadata, fileIdx int, bytesWritten int64) {
	r.saveProgress(meta, fileIdx, bytesWritten)
}
