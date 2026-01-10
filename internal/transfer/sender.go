package transfer

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filedrop/internal/crypto"

	"github.com/schollz/progressbar/v3"
)

const (
	ChunkSize = 64 * 1024 // 64KB
)

// Sender handles file sending
type Sender struct {
	conn       io.ReadWriter
	key        []byte
	compress   bool
	onProgress func(sent, total int64)
}

// NewSender creates new sender
func NewSender(conn io.ReadWriter, key []byte, compress bool) *Sender {
	return &Sender{
		conn:     conn,
		key:      key,
		compress: compress,
	}
}

// GenerateTransferID creates unique transfer ID
func GenerateTransferID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SendFiles sends multiple files/directories
func (s *Sender) SendFiles(paths []string) error {
	// Collect all files
	var files []FileInfo
	var totalSize int64

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			// Walk directory
			err := filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				relPath, _ := filepath.Rel(filepath.Dir(path), p)
				files = append(files, FileInfo{
					Path:    relPath,
					Size:    fi.Size(),
					IsDir:   fi.IsDir(),
					ModTime: fi.ModTime().Unix(),
				})
				if !fi.IsDir() {
					totalSize += fi.Size()
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("walk %s: %w", path, err)
			}
		} else {
			files = append(files, FileInfo{
				Path:    info.Name(),
				Size:    info.Size(),
				IsDir:   false,
				ModTime: info.ModTime().Unix(),
			})
			totalSize += info.Size()
		}
	}

	// Send metadata
	meta := &TransferMetadata{
		Files:      files,
		TotalSize:  totalSize,
		TotalFiles: len(files),
		Compressed: s.compress,
		Encrypted:  s.key != nil,
		TransferID: GenerateTransferID(),
		ChunkSize:  ChunkSize,
		Resumable:  true,
	}

	if err := WriteMetadata(s.conn, meta); err != nil {
		return fmt.Errorf("send metadata: %w", err)
	}

	// Wait for ACK or resume info
	ack := make([]byte, 1)
	if _, err := s.conn.Read(ack); err != nil {
		return fmt.Errorf("read ack: %w", err)
	}

	var resumeFrom int64 = 0
	var startFileIdx = 0

	if ack[0] == 'R' { // Resume requested
		resumeInfo, err := ReadResumeInfo(s.conn)
		if err != nil {
			return fmt.Errorf("read resume info: %w", err)
		}
		startFileIdx = resumeInfo.CurrentFile
		resumeFrom = resumeInfo.BytesWritten
		fmt.Printf("Resuming from file %d, byte %d\n", startFileIdx, resumeFrom)
	}

	// Create progress bar
	bar := progressbar.NewOptions64(
		totalSize,
		progressbar.OptionSetDescription("Sending"),
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

	// Send files
	basePath := filepath.Dir(paths[0])
	for i := startFileIdx; i < len(files); i++ {
		fi := files[i]
		if fi.IsDir {
			continue
		}

		fullPath := filepath.Join(basePath, fi.Path)
		if err := s.sendFile(fullPath, fi, bar, resumeFrom); err != nil {
			return fmt.Errorf("send %s: %w", fi.Path, err)
		}
		resumeFrom = 0 // Only first file can be resumed
	}

	fmt.Println()
	return nil
}

func (s *Sender) sendFile(path string, info FileInfo, bar *progressbar.ProgressBar, resumeFrom int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Seek if resuming
	if resumeFrom > 0 {
		if _, err := file.Seek(resumeFrom, 0); err != nil {
			return fmt.Errorf("seek: %w", err)
		}
		bar.Add64(resumeFrom)
	}

	remaining := info.Size - resumeFrom
	buf := make([]byte, ChunkSize)

	var encWriter *crypto.EncryptedWriter
	if s.key != nil {
		encWriter, err = crypto.NewEncryptedWriter(s.conn, s.key)
		if err != nil {
			return err
		}
	}

	for remaining > 0 {
		toRead := ChunkSize
		if remaining < int64(ChunkSize) {
			toRead = int(remaining)
		}

		n, err := file.Read(buf[:toRead])
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		// Write chunk size first
		if err := binary.Write(s.conn, binary.BigEndian, uint32(n)); err != nil {
			return err
		}

		if encWriter != nil {
			if err := encWriter.WriteChunk(buf[:n]); err != nil {
				return err
			}
		} else {
			if _, err := s.conn.Write(buf[:n]); err != nil {
				return err
			}
		}

		remaining -= int64(n)
		bar.Add(n)
	}

	// Send end marker
	if err := binary.Write(s.conn, binary.BigEndian, uint32(0)); err != nil {
		return err
	}

	return nil
}
