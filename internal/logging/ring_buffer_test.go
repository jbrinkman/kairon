package logging

import (
	"sync"
	"testing"

	"github.com/charmbracelet/log"
)

// TestNewRingBuffer verifies ring buffer creation with various capacities
func TestNewRingBuffer(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		expected int
	}{
		{"positive capacity", 100, 100},
		{"zero capacity uses default", 0, DefaultMaxBufferLines},
		{"negative capacity uses default", -1, DefaultMaxBufferLines},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewRingBuffer(tt.capacity)
			if rb.Capacity() != tt.expected {
				t.Errorf("expected capacity %d, got %d", tt.expected, rb.Capacity())
			}
			if rb.Size() != 0 {
				t.Errorf("expected size 0 for new buffer, got %d", rb.Size())
			}
			if !rb.IsEmpty() {
				t.Error("expected new buffer to be empty")
			}
			if rb.IsFull() {
				t.Error("expected new buffer to not be full")
			}
		})
	}
}

// TestAddAndGet verifies basic add and get operations
func TestAddAndGet(t *testing.T) {
	rb := NewRingBuffer(5)

	// Add entries
	rb.Add(log.InfoLevel, "message 1")
	rb.Add(log.DebugLevel, "message 2", "key1", "value1")
	rb.Add(log.WarnLevel, "message 3", "key2", 123)

	// Verify size
	if rb.Size() != 3 {
		t.Errorf("expected size 3, got %d", rb.Size())
	}

	// Get all entries
	entries := rb.Get()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify entries
	if entries[0].Message != "message 1" {
		t.Errorf("expected message 1, got %s", entries[0].Message)
	}
	if entries[1].Level != log.DebugLevel {
		t.Errorf("expected debug level, got %v", entries[1].Level)
	}
	if entries[1].Metadata["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %v", entries[1].Metadata["key1"])
	}
	if entries[2].Metadata["key2"] != 123 {
		t.Errorf("expected key2=123, got %v", entries[2].Metadata["key2"])
	}
}

// TestFIFOBehavior verifies oldest entries are overwritten when buffer is full
func TestFIFOBehavior(t *testing.T) {
	rb := NewRingBuffer(3)

	// Fill buffer
	rb.Add(log.InfoLevel, "message 1")
	rb.Add(log.InfoLevel, "message 2")
	rb.Add(log.InfoLevel, "message 3")

	if !rb.IsFull() {
		t.Error("expected buffer to be full")
	}

	// Add one more (should overwrite first entry)
	rb.Add(log.InfoLevel, "message 4")

	// Size should remain at capacity
	if rb.Size() != 3 {
		t.Errorf("expected size 3, got %d", rb.Size())
	}

	// Get entries and verify FIFO
	entries := rb.Get()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// First entry should now be "message 2"
	if entries[0].Message != "message 2" {
		t.Errorf("expected message 2, got %s", entries[0].Message)
	}
	if entries[1].Message != "message 3" {
		t.Errorf("expected message 3, got %s", entries[1].Message)
	}
	if entries[2].Message != "message 4" {
		t.Errorf("expected message 4, got %s", entries[2].Message)
	}
}

// TestGetRecent verifies retrieval of last N entries
func TestGetRecent(t *testing.T) {
	rb := NewRingBuffer(10)

	// Add 5 entries
	for i := 1; i <= 5; i++ {
		rb.Add(log.InfoLevel, "message", "index", i)
	}

	tests := []struct {
		name     string
		n        int
		expected int
	}{
		{"get last 3", 3, 3},
		{"get last 5", 5, 5},
		{"get more than available", 10, 5},
		{"get zero", 0, 0},
		{"get negative", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := rb.GetRecent(tt.n)
			if len(entries) != tt.expected {
				t.Errorf("expected %d entries, got %d", tt.expected, len(entries))
			}
		})
	}

	// Verify GetRecent returns entries in chronological order
	recent := rb.GetRecent(3)
	if recent[0].Metadata["index"] != 3 {
		t.Errorf("expected index 3, got %v", recent[0].Metadata["index"])
	}
	if recent[2].Metadata["index"] != 5 {
		t.Errorf("expected index 5, got %v", recent[2].Metadata["index"])
	}
}

// TestGetRecentWithWraparound verifies GetRecent works correctly with wraparound
func TestGetRecentWithWraparound(t *testing.T) {
	rb := NewRingBuffer(5)

	// Fill buffer completely
	for i := 1; i <= 5; i++ {
		rb.Add(log.InfoLevel, "message", "index", i)
	}

	// Add more entries to cause wraparound
	for i := 6; i <= 8; i++ {
		rb.Add(log.InfoLevel, "message", "index", i)
	}

	// Get last 3 entries (should be 6, 7, 8)
	recent := rb.GetRecent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}

	if recent[0].Metadata["index"] != 6 {
		t.Errorf("expected index 6, got %v", recent[0].Metadata["index"])
	}
	if recent[1].Metadata["index"] != 7 {
		t.Errorf("expected index 7, got %v", recent[1].Metadata["index"])
	}
	if recent[2].Metadata["index"] != 8 {
		t.Errorf("expected index 8, got %v", recent[2].Metadata["index"])
	}
}

// TestClear verifies buffer clearing
func TestClear(t *testing.T) {
	rb := NewRingBuffer(5)

	// Add entries
	rb.Add(log.InfoLevel, "message 1")
	rb.Add(log.InfoLevel, "message 2")

	// Clear buffer
	rb.Clear()

	if rb.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", rb.Size())
	}
	if !rb.IsEmpty() {
		t.Error("expected buffer to be empty after clear")
	}

	entries := rb.Get()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

// TestIterator verifies iterator functionality
func TestIterator(t *testing.T) {
	rb := NewRingBuffer(5)

	// Add entries
	rb.Add(log.InfoLevel, "message 1")
	rb.Add(log.DebugLevel, "message 2")
	rb.Add(log.WarnLevel, "message 3")

	// Create iterator
	iter := rb.NewIterator()

	// Count entries
	count := 0
	for iter.HasNext() {
		entry := iter.Next()
		if entry == nil {
			t.Error("expected non-nil entry")
			break
		}
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 entries, got %d", count)
	}

	// Next should return nil after consuming all entries
	if iter.Next() != nil {
		t.Error("expected nil after consuming all entries")
	}
}

// TestIteratorWithWraparound verifies iterator works correctly with wraparound
func TestIteratorWithWraparound(t *testing.T) {
	rb := NewRingBuffer(3)

	// Fill and wraparound
	for i := 1; i <= 5; i++ {
		rb.Add(log.InfoLevel, "message", "index", i)
	}

	// Iterator should see entries 3, 4, 5
	iter := rb.NewIterator()

	entry1 := iter.Next()
	if entry1.Metadata["index"] != 3 {
		t.Errorf("expected index 3, got %v", entry1.Metadata["index"])
	}

	entry2 := iter.Next()
	if entry2.Metadata["index"] != 4 {
		t.Errorf("expected index 4, got %v", entry2.Metadata["index"])
	}

	entry3 := iter.Next()
	if entry3.Metadata["index"] != 5 {
		t.Errorf("expected index 5, got %v", entry3.Metadata["index"])
	}

	if iter.Next() != nil {
		t.Error("expected nil after consuming all entries")
	}
}

// TestConcurrentAccess verifies thread-safety
func TestConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer(1000)
	var wg sync.WaitGroup

	// Concurrent writes
	numWriters := 10
	writesPerWriter := 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				rb.Add(log.InfoLevel, "message", "writer", writerID, "count", j)
			}
		}(i)
	}

	// Concurrent reads
	numReaders := 5
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = rb.Get()
				_ = rb.GetRecent(10)
				_ = rb.Size()
			}
		}()
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify buffer is in valid state
	size := rb.Size()
	if size > rb.Capacity() {
		t.Errorf("size %d exceeds capacity %d", size, rb.Capacity())
	}

	entries := rb.Get()
	if len(entries) != size {
		t.Errorf("Get() returned %d entries but Size() is %d", len(entries), size)
	}
}

// TestConcurrentIterators verifies multiple concurrent iterators
func TestConcurrentIterators(t *testing.T) {
	rb := NewRingBuffer(100)

	// Fill buffer
	for i := 0; i < 50; i++ {
		rb.Add(log.InfoLevel, "message", "index", i)
	}

	var wg sync.WaitGroup
	numIterators := 5

	for i := 0; i < numIterators; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			iter := rb.NewIterator()
			count := 0
			for iter.HasNext() {
				if iter.Next() == nil {
					t.Error("expected non-nil entry")
					return
				}
				count++
			}
			if count != 50 {
				t.Errorf("expected 50 entries, got %d", count)
			}
		}()
	}

	wg.Wait()
}

// TestEmptyBufferOperations verifies operations on empty buffer
func TestEmptyBufferOperations(t *testing.T) {
	rb := NewRingBuffer(10)

	if !rb.IsEmpty() {
		t.Error("expected empty buffer")
	}

	entries := rb.Get()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}

	recent := rb.GetRecent(5)
	if len(recent) != 0 {
		t.Errorf("expected 0 recent entries, got %d", len(recent))
	}

	iter := rb.NewIterator()
	if iter.HasNext() {
		t.Error("expected iterator to have no entries")
	}
	if iter.Next() != nil {
		t.Error("expected nil from Next() on empty buffer")
	}
}

// TestMalformedKeyVals verifies that malformed key-value lists are handled:
// a trailing key without a value is dropped and a non-string key is skipped.
func TestMalformedKeyVals(t *testing.T) {
	tests := []struct {
		name     string
		keyvals  []interface{}
		wantMeta map[string]interface{}
	}{
		{
			name:     "odd number of keyvals drops the trailing key",
			keyvals:  []interface{}{"key1", "value1", "key2"},
			wantMeta: map[string]interface{}{"key1": "value1"},
		},
		{
			name:     "non-string key is skipped",
			keyvals:  []interface{}{123, "value", "key2", "value2"},
			wantMeta: map[string]interface{}{"key2": "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewRingBuffer(10)
			rb.Add(log.InfoLevel, "test", tt.keyvals...)

			entries := rb.Get()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if len(entries[0].Metadata) != len(tt.wantMeta) {
				t.Errorf("expected %d metadata entries, got %d", len(tt.wantMeta), len(entries[0].Metadata))
			}
			for k, want := range tt.wantMeta {
				if entries[0].Metadata[k] != want {
					t.Errorf("expected %v for key %q, got %v", want, k, entries[0].Metadata[k])
				}
			}
		})
	}
}
