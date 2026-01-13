package transfer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTransferMetadata(t *testing.T) {
	meta := &TransferMetadata{
		TransferID: "test123",
		TotalFiles: 2,
		TotalSize:  1000,
		Compressed: true,
		Encrypted:  true,
		Files: []FileInfo{
			{Path: "file1.txt", Size: 500, IsDir: false},
			{Path: "file2.txt", Size: 500, IsDir: false},
		},
	}

	// Test JSON serialization
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var parsed TransferMetadata
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if parsed.TransferID != meta.TransferID {
		t.Error("TransferID mismatch")
	}
	if parsed.TotalFiles != meta.TotalFiles {
		t.Error("TotalFiles mismatch")
	}
}

func TestResumeInfo(t *testing.T) {
	info := ResumeInfo{
		TransferID:   "test123",
		CurrentFile:  1,
		BytesWritten: 250,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var parsed ResumeInfo
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if parsed.CurrentFile != info.CurrentFile {
		t.Error("CurrentFile mismatch")
	}
	if parsed.BytesWritten != info.BytesWritten {
		t.Error("BytesWritten mismatch")
	}
}

func TestReceiver_SaveProgress(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "filedrop_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	receiver := NewReceiver(nil, nil, tempDir)

	meta := &TransferMetadata{
		TransferID: "test123",
		TotalFiles: 2,
		TotalSize:  1000,
	}

	// Test save progress
	receiver.SaveProgressTest(meta, 1, 250)

	// Check file exists
	progressFile := filepath.Join(tempDir, ".filedrop_progress_test123")
	if _, err := os.Stat(progressFile); os.IsNotExist(err) {
		t.Error("Progress file was not created")
	}

	// Check file content
	data, err := os.ReadFile(progressFile)
	if err != nil {
		t.Fatalf("Failed to read progress file: %v", err)
	}

	var info ResumeInfo
	err = json.Unmarshal(data, &info)
	if err != nil {
		t.Fatalf("Failed to parse progress file: %v", err)
	}

	if info.CurrentFile != 1 {
		t.Errorf("Expected CurrentFile 1, got %d", info.CurrentFile)
	}
	if info.BytesWritten != 250 {
		t.Errorf("Expected BytesWritten 250, got %d", info.BytesWritten)
	}
}

func TestGenerateTransferID(t *testing.T) {
	id1 := GenerateTransferID()
	id2 := GenerateTransferID()

	if id1 == id2 {
		t.Error("GenerateTransferID should produce unique IDs")
	}

	if len(id1) == 0 {
		t.Error("GenerateTransferID returned empty string")
	}
}

func TestWriteMetadata(t *testing.T) {
	meta := &TransferMetadata{
		TransferID: "test123",
		TotalFiles: 1,
		TotalSize:  100,
	}

	var buf bytes.Buffer
	err := WriteMetadata(&buf, meta)
	if err != nil {
		t.Fatalf("WriteMetadata failed: %v", err)
	}

	// Test ReadMetadata
	parsed, err := ReadMetadata(&buf)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}

	if parsed.TransferID != meta.TransferID {
		t.Error("TransferID mismatch after round trip")
	}
}
