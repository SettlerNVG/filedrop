package transfer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// FileInfo represents single file metadata
type FileInfo struct {
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	IsDir        bool   `json:"is_dir"`
	ModTime      int64  `json:"mod_time"`
	TransferID   string `json:"transfer_id,omitempty"`
	ChunkIndex   int64  `json:"chunk_index,omitempty"`
	BytesWritten int64  `json:"bytes_written,omitempty"`
}

// TransferMetadata contains all transfer info
type TransferMetadata struct {
	Files       []FileInfo `json:"files"`
	TotalSize   int64      `json:"total_size"`
	TotalFiles  int        `json:"total_files"`
	Compressed  bool       `json:"compressed"`
	Encrypted   bool       `json:"encrypted"`
	TransferID  string     `json:"transfer_id"`
	ChunkSize   int        `json:"chunk_size"`
	Resumable   bool       `json:"resumable"`
}

// WriteMetadata sends metadata over connection
func WriteMetadata(w io.Writer, meta *TransferMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Write length prefix (4 bytes)
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("write length: %w", err)
	}

	// Write JSON data
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// ReadMetadata receives metadata from connection
func ReadMetadata(r io.Reader) (*TransferMetadata, error) {
	// Read length prefix
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}

	// Sanity check
	if length > 10*1024*1024 { // 10MB max
		return nil, fmt.Errorf("metadata too large: %d bytes", length)
	}

	// Read JSON data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	var meta TransferMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return &meta, nil
}

// ResumeInfo contains info for resuming transfer
type ResumeInfo struct {
	TransferID   string     `json:"transfer_id"`
	Files        []FileInfo `json:"files"`
	CurrentFile  int        `json:"current_file"`
	CurrentChunk int64      `json:"current_chunk"`
	BytesWritten int64      `json:"bytes_written"`
}

// WriteResumeInfo sends resume info
func WriteResumeInfo(w io.Writer, info *ResumeInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// ReadResumeInfo receives resume info
func ReadResumeInfo(r io.Reader) (*ResumeInfo, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	var info ResumeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}
