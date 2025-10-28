package solo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Writer writes data sequentially to a series of files with append-only semantics.
// When a file exceeds MaxFileLength, it closes the file and creates a new one.
type Writer struct {
	path          string
	filename      string
	maxFileLength int64
	currentIndex  int
	currentFile   *os.File
	currentSize   int64
	mu            sync.Mutex
}

// Reader reads data from a sequence of files, following new data as it arrives.
type Reader struct {
	path          string
	filename      string
	chunkSize     int
	fileTimeLapse time.Duration
	currentIndex  int
	currentFile   *os.File
	currentOffset int64
	mu            sync.Mutex
}

// NewWriter creates a new sequential file writer.
// If files already exist, it starts by reading them and sending content to the output channel.
// After reading existing files, it switches to write mode.
func NewWriter(path, filename string, maxFileLength int64, chunkSize int, outputChan chan []byte) (*Writer, error) {
	w := &Writer{
		path:          path,
		filename:      filename,
		maxFileLength: maxFileLength,
	}

	// Check for existing files in the sequence
	existingFiles, err := findSequenceFiles(path, filename)
	if err != nil {
		return nil, err
	}

	// If there are existing files, read them first
	if len(existingFiles) > 0 && outputChan != nil {
		go func() {
			defer close(outputChan)
			for _, file := range existingFiles {
				if err := readFileInChunks(file, chunkSize, outputChan); err != nil {
					return
				}
			}
		}()

		// Start with the next index after existing files
		w.currentIndex = len(existingFiles)
	} else if outputChan != nil {
		close(outputChan)
	}

	// Open the current file for writing
	if err := w.openNextFile(); err != nil {
		return nil, err
	}

	return w, nil
}

// Write writes data to the current file. If the file exceeds maxFileLength after writing,
// it closes the current file and opens a new one.
func (w *Writer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to current file
	n, err = w.currentFile.Write(p)
	if err != nil {
		return n, err
	}

	// Update current size
	w.currentSize += int64(n)

	// Check if we need to rotate to a new file
	if w.currentSize >= w.maxFileLength {
		if err := w.currentFile.Close(); err != nil {
			return n, err
		}
		w.currentIndex++
		if err := w.openNextFile(); err != nil {
			return n, err
		}
	}

	return n, nil
}

// Close closes the current file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentFile != nil {
		return w.currentFile.Close()
	}
	return nil
}

// openNextFile opens the next file in the sequence.
func (w *Writer) openNextFile() error {
	filename := fmt.Sprintf("%s_%d", w.filename, w.currentIndex)
	fullPath := filepath.Join(w.path, filename)

	// Check if file exists to get current size
	info, err := os.Stat(fullPath)
	if err == nil {
		w.currentSize = info.Size()
	} else {
		w.currentSize = 0
	}

	// Open file in append mode
	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	w.currentFile = file
	return nil
}

// NewReader creates a new sequential file reader.
// It reads all existing files and then follows new data as it arrives.
func NewReader(path, filename string, chunkSize int, fileTimeLapse time.Duration) *Reader {
	return &Reader{
		path:          path,
		filename:      filename,
		chunkSize:     chunkSize,
		fileTimeLapse: fileTimeLapse,
		currentIndex:  0,
	}
}

// Read reads from the file sequence, sending chunks to the output channel.
// It first reads all existing files, then follows new data as it arrives.
// The function respects the provided context and exits when the context is done.
func (r *Reader) Read(ctx context.Context, outputChan chan []byte) error {
	defer close(outputChan)

	// Find existing files
	existingFiles, err := findSequenceFiles(r.path, r.filename)
	if err != nil {
		return err
	}

	// Read existing files
	for i, filePath := range existingFiles {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.currentIndex = i
		r.currentOffset = 0 // Reset offset for each file
		if err := r.readFile(ctx, filePath, outputChan, i < len(existingFiles)-1); err != nil {
			return err
		}
	}

	// Follow mode: check for new data periodically
	ticker := time.NewTicker(r.fileTimeLapse)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check for more data in current file or new files
			currentFilePath := filepath.Join(r.path, fmt.Sprintf("%s_%d", r.filename, r.currentIndex))
			info, err := os.Stat(currentFilePath)

			// If current file exists and has new data
			if err == nil && info.Size() > r.currentOffset {
				file, err := os.Open(currentFilePath)
				if err != nil {
					continue
				}
				if _, err := file.Seek(r.currentOffset, 0); err != nil {
					file.Close()
					continue
				}
				if err := r.readFromFile(ctx, file, outputChan, false); err != nil && err != io.EOF && err != context.Canceled && err != context.DeadlineExceeded {
					file.Close()
					return err
				}
				file.Close()
			}

			// Check for next file in sequence
			nextFilePath := filepath.Join(r.path, fmt.Sprintf("%s_%d", r.filename, r.currentIndex+1))
			if _, err := os.Stat(nextFilePath); err == nil {
				r.currentIndex++
				r.currentOffset = 0
				if err := r.readFile(ctx, nextFilePath, outputChan, false); err != nil {
					return err
				}
			}
		}
	}
}

// readFile reads a file and sends chunks to the output channel.
func (r *Reader) readFile(ctx context.Context, filePath string, outputChan chan []byte, fullRead bool) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if r.currentOffset > 0 {
		if _, err := file.Seek(r.currentOffset, 0); err != nil {
			return err
		}
	}

	return r.readFromFile(ctx, file, outputChan, fullRead)
}

// readFromFile reads from an open file and sends chunks to the output channel.
func (r *Reader) readFromFile(ctx context.Context, file *os.File, outputChan chan []byte, fullRead bool) error {
	buffer := make([]byte, r.chunkSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := file.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			select {
			case <-ctx.Done():
				return ctx.Err()
			case outputChan <- chunk:
				r.currentOffset += int64(n)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// findSequenceFiles finds all files matching the sequence pattern.
func findSequenceFiles(path, filename string) ([]string, error) {
	pattern := fmt.Sprintf("%s_*", filename)
	matches, err := filepath.Glob(filepath.Join(path, pattern))
	if err != nil {
		return nil, err
	}

	// Parse and sort by index
	var fileIndices []struct {
		path  string
		index int
	}

	for _, match := range matches {
		base := filepath.Base(match)
		if !strings.HasPrefix(base, filename+"_") {
			continue
		}
		indexStr := strings.TrimPrefix(base, filename+"_")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			continue
		}
		fileIndices = append(fileIndices, struct {
			path  string
			index int
		}{match, index})
	}

	// Sort by index
	sort.Slice(fileIndices, func(i, j int) bool {
		return fileIndices[i].index < fileIndices[j].index
	})

	// Extract sorted paths
	result := make([]string, len(fileIndices))
	for i, fi := range fileIndices {
		result[i] = fi.path
	}

	return result, nil
}

// readFileInChunks reads a file in chunks and sends them to the output channel.
func readFileInChunks(filePath string, chunkSize int, outputChan chan []byte) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			outputChan <- chunk
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
