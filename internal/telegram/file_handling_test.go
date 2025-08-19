package telegram

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"
)

// Mock bot for file handling tests
func createMockBot() *Bot {
	return &Bot{
		parseMode: "Markdown",
	}
}

func TestBot_ProcessZipArchive(t *testing.T) {
	bot := createMockBot()

	// Create a test ZIP archive in memory
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add test files to ZIP
	files := map[string]string{
		"main.py":          "print('Hello World')",
		"requirements.txt": "flask==2.0.1\nrequests==2.28.0",
		"test.py":          "import unittest\n\nclass TestMain(unittest.TestCase):\n    pass",
	}

	for filename, content := range files {
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			t.Fatalf("Failed to create file in ZIP: %v", err)
		}
		_, err = fileWriter.Write([]byte(content))
		if err != nil {
			t.Fatalf("Failed to write content to ZIP file: %v", err)
		}
	}

	zipWriter.Close()
	zipData := zipBuffer.Bytes()

	// Test processing the ZIP archive
	result, err := bot.processZipArchive(zipData, "test.zip")
	if err != nil {
		t.Errorf("processZipArchive() error = %v", err)
		return
	}

	// Verify all files were extracted
	if len(result) != len(files) {
		t.Errorf("Expected %d files, got %d", len(files), len(result))
	}

	// Check each extracted file
	for filename, expectedContent := range files {
		if extractedContent, exists := result[filename]; exists {
			if extractedContent != expectedContent {
				t.Errorf("File %s: expected content %q, got %q", filename, expectedContent, extractedContent)
			}
		} else {
			t.Errorf("Expected file %s not found in result", filename)
		}
	}
}

func TestBot_ProcessTarArchive(t *testing.T) {
	bot := createMockBot()

	// Create a test TAR archive in memory
	var tarBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuffer)

	// Add test files to TAR
	files := map[string]string{
		"index.js":     "console.log('Hello World');",
		"package.json": `{"name": "test-app", "version": "1.0.0"}`,
		"README.md":    "# Test Application\nThis is a test.",
	}

	for filename, content := range files {
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write TAR header: %v", err)
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write TAR content: %v", err)
		}
	}

	tarWriter.Close()
	tarData := tarBuffer.Bytes()

	// Test processing the TAR archive
	result, err := bot.processTarArchive(tarData, "test.tar")
	if err != nil {
		t.Errorf("processTarArchive() error = %v", err)
		return
	}

	// Verify all files were extracted
	if len(result) != len(files) {
		t.Errorf("Expected %d files, got %d", len(files), len(result))
	}

	// Check each extracted file
	for filename, expectedContent := range files {
		if extractedContent, exists := result[filename]; exists {
			if extractedContent != expectedContent {
				t.Errorf("File %s: expected content %q, got %q", filename, expectedContent, extractedContent)
			}
		} else {
			t.Errorf("Expected file %s not found in result", filename)
		}
	}
}

func TestBot_ProcessTarGzArchive(t *testing.T) {
	bot := createMockBot()

	// Create a TAR archive first
	var tarBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuffer)

	files := map[string]string{
		"main.go": "package main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
		"go.mod":  "module test\n\ngo 1.21",
		"go.sum":  "// checksums would go here",
	}

	for filename, content := range files {
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write TAR header: %v", err)
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write TAR content: %v", err)
		}
	}

	tarWriter.Close()
	tarData := tarBuffer.Bytes()

	// Compress with gzip
	var gzipBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuffer)
	_, err := gzipWriter.Write(tarData)
	if err != nil {
		t.Fatalf("Failed to write to gzip: %v", err)
	}
	gzipWriter.Close()

	targzData := gzipBuffer.Bytes()

	// Test processing the TAR.GZ archive
	result, err := bot.processTarGzArchive(targzData, "test.tar.gz")
	if err != nil {
		t.Errorf("processTarGzArchive() error = %v", err)
		return
	}

	// Verify all files were extracted
	if len(result) != len(files) {
		t.Errorf("Expected %d files, got %d", len(files), len(result))
	}

	// Check each extracted file
	for filename, expectedContent := range files {
		if extractedContent, exists := result[filename]; exists {
			if extractedContent != expectedContent {
				t.Errorf("File %s: expected content %q, got %q", filename, expectedContent, extractedContent)
			}
		} else {
			t.Errorf("Expected file %s not found in result", filename)
		}
	}
}

func TestBot_ProcessZipArchive_LargeFiles(t *testing.T) {
	bot := createMockBot()

	// Create a ZIP with a file that exceeds size limit
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add a normal file
	fileWriter, err := zipWriter.Create("small.txt")
	if err != nil {
		t.Fatalf("Failed to create small file: %v", err)
	}
	_, err = fileWriter.Write([]byte("Small content"))
	if err != nil {
		t.Fatalf("Failed to write small file: %v", err)
	}

	// Add a large file that should be skipped (over 1MB)
	fileWriter, err = zipWriter.Create("large.txt")
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}
	largeContent := make([]byte, 2*1024*1024) // 2MB
	for i := range largeContent {
		largeContent[i] = 'x'
	}
	_, err = fileWriter.Write(largeContent)
	if err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	zipWriter.Close()
	zipData := zipBuffer.Bytes()

	// Test processing - large file should be skipped
	result, err := bot.processZipArchive(zipData, "test.zip")
	if err != nil {
		t.Errorf("processZipArchive() error = %v", err)
		return
	}

	// Should only have the small file
	if len(result) != 1 {
		t.Errorf("Expected 1 file (large file should be skipped), got %d", len(result))
	}

	if _, exists := result["small.txt"]; !exists {
		t.Error("Expected small.txt to be extracted")
	}

	if _, exists := result["large.txt"]; exists {
		t.Error("Large file should have been skipped")
	}
}

func TestBot_ProcessZipArchive_TooManyFiles(t *testing.T) {
	bot := createMockBot()

	// Create a ZIP with too many files (over the 50 file limit)
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	// Add 55 files (5 more than the limit)
	for i := 0; i < 55; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", filename, err)
		}
		content := fmt.Sprintf("Content of file %d", i)
		_, err = fileWriter.Write([]byte(content))
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", filename, err)
		}
	}

	zipWriter.Close()
	zipData := zipBuffer.Bytes()

	// Test processing - should be limited to 50 files
	result, err := bot.processZipArchive(zipData, "test.zip")
	if err != nil {
		t.Errorf("processZipArchive() error = %v", err)
		return
	}

	// Should have exactly 50 files (the limit)
	if len(result) != 50 {
		t.Errorf("Expected 50 files (file limit), got %d", len(result))
	}
}

func TestBot_ProcessZipArchive_HiddenFiles(t *testing.T) {
	bot := createMockBot()

	// Create a ZIP with hidden files that should be skipped
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	files := map[string]string{
		"regular.txt": "Regular file content",
		".hidden":     "Hidden file content",
		".gitignore":  "node_modules/\n*.log",
		"visible.py":  "print('hello')",
	}

	for filename, content := range files {
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", filename, err)
		}
		_, err = fileWriter.Write([]byte(content))
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", filename, err)
		}
	}

	zipWriter.Close()
	zipData := zipBuffer.Bytes()

	// Test processing - hidden files should be skipped
	result, err := bot.processZipArchive(zipData, "test.zip")
	if err != nil {
		t.Errorf("processZipArchive() error = %v", err)
		return
	}

	// Should only have the visible files
	if len(result) != 2 {
		t.Errorf("Expected 2 files (hidden files should be skipped), got %d", len(result))
	}

	if _, exists := result["regular.txt"]; !exists {
		t.Error("Expected regular.txt to be extracted")
	}

	if _, exists := result["visible.py"]; !exists {
		t.Error("Expected visible.py to be extracted")
	}

	if _, exists := result[".hidden"]; exists {
		t.Error("Hidden file should have been skipped")
	}

	if _, exists := result[".gitignore"]; exists {
		t.Error("Hidden .gitignore should have been skipped")
	}
}
