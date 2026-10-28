package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileHandler_MultipleWrites(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filehandler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := FileOutputConfig{
		LogDir:        tempDir,
		MaxFileSizeMB: 1,
	}

	fh, err := NewFileHandler(config)
	if err != nil {
		t.Fatalf("NewFileHandler failed: %v", err)
	}
	defer fh.Close()

	// Write multiple entries
	entries := []string{
		"log entry 1\n",
		"log entry 2\n",
		"log entry 3\n",
	}

	totalBytes := 0
	for _, entry := range entries {
		n, err := fh.Write([]byte(entry))
		if err != nil {
			t.Errorf("Write failed for entry %q: %v", entry, err)
		}
		totalBytes += n
	}

	// Verify size
	currentSize := fh.GetCurrentSize()
	if currentSize != int64(totalBytes) {
		t.Errorf("current size is %d, expected %d", currentSize, totalBytes)
	}

	// Verify all entries are in the file
	currentPath := fh.GetCurrentFilePath()
	content, err := os.ReadFile(currentPath)
	if err != nil {
		t.Errorf("failed to read log file: %v", err)
	}

	expectedContent := strings.Join(entries, "")
	if string(content) != expectedContent {
		t.Errorf("file content is %q, expected %q", string(content), expectedContent)
	}
}

func TestFileHandler_Close(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filehandler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := FileOutputConfig{
		LogDir:        tempDir,
		MaxFileSizeMB: 1,
	}

	fh, err := NewFileHandler(config)
	if err != nil {
		t.Fatalf("NewFileHandler failed: %v", err)
	}

	currentPath := fh.GetCurrentFilePath()

	// Close the handler
	if err := fh.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify file still exists
	if _, err := os.Stat(currentPath); os.IsNotExist(err) {
		t.Errorf("log file was deleted after close")
	}

	// Verify writes fail after close
	_, err = fh.Write([]byte("should fail\n"))
	if err == nil {
		t.Errorf("Write succeeded after close, expected error")
	}
}

func TestFileHandler_DirectoryCreation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filehandler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Use nested directory that doesn't exist yet
	nestedDir := filepath.Join(tempDir, "nested", "logs")

	config := FileOutputConfig{
		LogDir:        nestedDir,
		MaxFileSizeMB: 1,
	}

	fh, err := NewFileHandler(config)
	if err != nil {
		t.Fatalf("NewFileHandler failed: %v", err)
	}
	defer fh.Close()

	// Verify nested directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Errorf("nested log directory was not created")
	}

	// Verify log file was created in nested directory
	currentPath := fh.GetCurrentFilePath()
	if !strings.HasPrefix(currentPath, nestedDir) {
		t.Errorf("log file not in expected directory: %s", currentPath)
	}
}

func TestFileHandler_ConcurrentWrites(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filehandler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := FileOutputConfig{
		LogDir:        tempDir,
		MaxFileSizeMB: 10,
	}

	fh, err := NewFileHandler(config)
	if err != nil {
		t.Fatalf("NewFileHandler failed: %v", err)
	}
	defer fh.Close()

	// Spawn multiple goroutines writing concurrently
	numGoroutines := 10
	entriesPerGoroutine := 100

	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < entriesPerGoroutine; j++ {
				entry := fmt.Sprintf("goroutine %d entry %d\n", id, j)
				_, err := fh.Write([]byte(entry))
				if err != nil {
					t.Errorf("concurrent write failed: %v", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify file size is reasonable (all writes succeeded)
	currentSize := fh.GetCurrentSize()
	expectedMinSize := int64(numGoroutines * entriesPerGoroutine * 10) // Rough estimate

	if currentSize < expectedMinSize {
		t.Errorf("current size %d is less than expected minimum %d", currentSize, expectedMinSize)
	}
}

func TestFileHandler_Rotation(t *testing.T) {
	t.Run("size threshold rotates to a new file", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "filehandler-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Use very small max size to trigger rotation
		config := FileOutputConfig{
			LogDir:        tempDir,
			MaxFileSizeMB: 1, // Will convert to bytes: 1 * 1024 * 1024 = 1048576
		}

		fh, err := NewFileHandler(config)
		if err != nil {
			t.Fatalf("NewFileHandler failed: %v", err)
		}
		defer fh.Close()

		initialPath := fh.GetCurrentFilePath()

		// Write data that exceeds max size to trigger rotation
		// Create 1MB + 1 byte of data
		largeData := make([]byte, 1024*1024+1)
		for i := range largeData {
			largeData[i] = 'A'
		}

		_, err = fh.Write(largeData)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}

		// Verify rotation occurred
		currentPath := fh.GetCurrentFilePath()
		if currentPath == initialPath {
			t.Errorf("rotation did not occur: path unchanged")
		}

		// Verify both files exist
		if _, err := os.Stat(initialPath); os.IsNotExist(err) {
			t.Errorf("initial log file was deleted after rotation")
		}

		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			t.Errorf("new log file was not created after rotation")
		}

		// Verify old file is closed (can be opened again)
		oldFile, err := os.OpenFile(initialPath, os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("failed to open old log file: %v", err)
		} else {
			oldFile.Close()
		}
	})

	t.Run("same-minute handlers get distinct files", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "filehandler-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		config := FileOutputConfig{
			LogDir:        tempDir,
			MaxFileSizeMB: 1,
		}

		// Create multiple handlers in quick succession (within same minute)
		handlers := make([]*FileHandler, 3)
		for i := 0; i < 3; i++ {
			fh, err := NewFileHandler(config)
			if err != nil {
				t.Fatalf("NewFileHandler %d failed: %v", i, err)
			}
			handlers[i] = fh

			// Write some data
			fh.Write([]byte(fmt.Sprintf("handler %d\n", i)))
		}

		// Close all handlers
		for i, fh := range handlers {
			if err := fh.Close(); err != nil {
				t.Errorf("Close handler %d failed: %v", i, err)
			}
		}

		// Verify multiple log files were created
		files, err := os.ReadDir(tempDir)
		if err != nil {
			t.Fatalf("failed to read log directory: %v", err)
		}

		if len(files) != 3 {
			t.Errorf("expected exactly 3 log files, got %d", len(files))
		}

		// Verify files have sequence numbers (or base name)
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), "debug-") {
				t.Errorf("unexpected filename: %s", file.Name())
			}
		}
	})
}
