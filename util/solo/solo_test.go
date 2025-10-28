package solo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriter_BasicWrite(t *testing.T) {
	tempDir := t.TempDir()

	w, err := NewWriter(tempDir, "test", 100, 10, nil)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer w.Close()

	data := []byte("Hello, World!")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	// Verify file exists
	filePath := filepath.Join(tempDir, "test_0")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("Expected content %q, got %q", string(data), string(content))
	}
}

func TestWriter_FileRotation(t *testing.T) {
	tempDir := t.TempDir()

	maxLen := int64(20)
	w, err := NewWriter(tempDir, "test", maxLen, 10, nil)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer w.Close()

	// Write data that will exceed maxFileLength
	data1 := []byte("12345678901234567890") // 20 bytes
	data2 := []byte("ABCDEFGHIJ")          // 10 bytes

	_, err = w.Write(data1)
	if err != nil {
		t.Fatalf("Failed to write data1: %v", err)
	}

	_, err = w.Write(data2)
	if err != nil {
		t.Fatalf("Failed to write data2: %v", err)
	}

	// Verify first file
	file0 := filepath.Join(tempDir, "test_0")
	content0, err := os.ReadFile(file0)
	if err != nil {
		t.Fatalf("Failed to read test_0: %v", err)
	}
	if string(content0) != string(data1) {
		t.Errorf("Expected test_0 content %q, got %q", string(data1), string(content0))
	}

	// Verify second file
	file1 := filepath.Join(tempDir, "test_1")
	content1, err := os.ReadFile(file1)
	if err != nil {
		t.Fatalf("Failed to read test_1: %v", err)
	}
	if string(content1) != string(data2) {
		t.Errorf("Expected test_1 content %q, got %q", string(data2), string(content1))
	}
}

func TestWriter_ExistingFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing files
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("existing0"), 0644)
	os.WriteFile(filepath.Join(tempDir, "test_1"), []byte("existing1"), 0644)

	// Create writer with output channel to read existing files
	outputChan := make(chan []byte, 10)
	w, err := NewWriter(tempDir, "test", 100, 10, outputChan)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer w.Close()

	// Read existing content from channel
	var received []byte
	for chunk := range outputChan {
		received = append(received, chunk...)
	}

	expected := "existing0existing1"
	if string(received) != expected {
		t.Errorf("Expected %q, got %q", expected, string(received))
	}

	// Verify writer starts with next index
	if w.currentIndex != 2 {
		t.Errorf("Expected currentIndex 2, got %d", w.currentIndex)
	}
}

func TestReader_BasicRead(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("Hello, "), 0644)
	os.WriteFile(filepath.Join(tempDir, "test_1"), []byte("World!"), 0644)

	r := NewReader(tempDir, "test", 10, 50*time.Millisecond)
	outputChan := make(chan []byte, 10)

	ctx, cancel := context.WithCancel(context.Background())

	go r.Read(ctx, outputChan)

	// Read chunks until we have the expected content
	var received []byte
	expected := "Hello, World!"
	timeout := time.After(500 * time.Millisecond)

	for len(received) < len(expected) {
		select {
		case chunk := <-outputChan:
			received = append(received, chunk...)
		case <-timeout:
			t.Fatalf("Timeout waiting for data. Got %q, expected %q", string(received), expected)
		}
	}

	// Cancel context and drain channel
	cancel()
	time.Sleep(50 * time.Millisecond)
	for len(outputChan) > 0 {
		<-outputChan
	}

	if string(received) != expected {
		t.Errorf("Expected %q, got %q", expected, string(received))
	}
}

func TestReader_FollowMode(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial file
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("initial"), 0644)

	r := NewReader(tempDir, "test", 10, 100*time.Millisecond)
	outputChan := make(chan []byte, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go r.Read(ctx, outputChan)

	// Read initial content
	time.Sleep(50 * time.Millisecond)

	// Append to existing file
	file, err := os.OpenFile(filepath.Join(tempDir, "test_0"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	file.WriteString(" appended")
	file.Close()

	// Wait for reader to pick up new data
	time.Sleep(200 * time.Millisecond)

	// Create new file in sequence
	os.WriteFile(filepath.Join(tempDir, "test_1"), []byte(" newfile"), 0644)

	// Wait for reader to pick up new file
	time.Sleep(200 * time.Millisecond)

	// Cancel and collect results
	cancel()
	time.Sleep(50 * time.Millisecond)

	var received []byte
	for chunk := range outputChan {
		received = append(received, chunk...)
	}

	result := string(received)
	if result != "initial appended newfile" {
		t.Errorf("Expected %q, got %q", "initial appended newfile", result)
	}
}

func TestReader_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("data"), 0644)

	r := NewReader(tempDir, "test", 10, 50*time.Millisecond)
	outputChan := make(chan []byte, 10)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- r.Read(ctx, outputChan)
	}()

	// Let it read initial data and enter follow loop
	time.Sleep(150 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for Read to return
	select {
	case err := <-errChan:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for Read to return after context cancellation")
	}

	// Drain and verify channel is eventually closed
	drainTimeout := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-outputChan:
			if !ok {
				// Channel closed as expected
				return
			}
		case <-drainTimeout:
			t.Error("Channel should be closed after context cancellation")
			return
		}
	}
}

func TestFindSequenceFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create files in random order
	os.WriteFile(filepath.Join(tempDir, "test_2"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("0"), 0644)
	os.WriteFile(filepath.Join(tempDir, "test_1"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tempDir, "other_0"), []byte("other"), 0644)

	files, err := findSequenceFiles(tempDir, "test")
	if err != nil {
		t.Fatalf("Failed to find files: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(files))
	}

	// Verify order
	expected := []string{"test_0", "test_1", "test_2"}
	for i, file := range files {
		base := filepath.Base(file)
		if base != expected[i] {
			t.Errorf("Expected file %s at position %d, got %s", expected[i], i, base)
		}
	}
}

func TestReadFileInChunks(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	data := []byte("1234567890ABCDEFGHIJ")
	os.WriteFile(filePath, data, 0644)

	outputChan := make(chan []byte, 10)
	go func() {
		defer close(outputChan)
		readFileInChunks(filePath, 5, outputChan)
	}()

	var received []byte
	chunkCount := 0
	for chunk := range outputChan {
		received = append(received, chunk...)
		chunkCount++
	}

	if string(received) != string(data) {
		t.Errorf("Expected %q, got %q", string(data), string(received))
	}

	if chunkCount != 4 {
		t.Errorf("Expected 4 chunks, got %d", chunkCount)
	}
}

func TestWriter_AppendToExistingFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing file
	existingData := []byte("existing")
	os.WriteFile(filepath.Join(tempDir, "test_0"), existingData, 0644)

	w, err := NewWriter(tempDir, "test", 100, 10, nil)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer w.Close()

	// Write new data
	newData := []byte(" new")
	_, err = w.Write(newData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify file has both old and new data
	filePath := filepath.Join(tempDir, "test_0")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	expected := "existing new"
	if string(content) != expected {
		t.Errorf("Expected %q, got %q", expected, string(content))
	}
}

func TestReader_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	r := NewReader(tempDir, "test", 10, 100*time.Millisecond)
	outputChan := make(chan []byte, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go r.Read(ctx, outputChan)

	// Create file after reader starts
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(tempDir, "test_0"), []byte("delayed"), 0644)

	// Wait for reader to pick up the file
	time.Sleep(200 * time.Millisecond)

	var received []byte
	done := make(chan bool)
	go func() {
		for chunk := range outputChan {
			received = append(received, chunk...)
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for channel to close")
	}

	if string(received) != "delayed" {
		t.Errorf("Expected %q, got %q", "delayed", string(received))
	}
}

func BenchmarkWriter(b *testing.B) {
	tempDir := b.TempDir()
	w, err := NewWriter(tempDir, "bench", 1024*1024, 1024, nil)
	if err != nil {
		b.Fatalf("Failed to create writer: %v", err)
	}
	defer w.Close()

	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Write(data)
	}
}

func BenchmarkReader(b *testing.B) {
	tempDir := b.TempDir()

	// Create test file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(tempDir, "bench_0"), data, 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewReader(tempDir, "bench", 1024, 10*time.Millisecond)
		outputChan := make(chan []byte, 100)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

		go r.Read(ctx, outputChan)
		for range outputChan {
		}

		cancel()
	}
}
