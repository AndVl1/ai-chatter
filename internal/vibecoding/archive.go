package vibecoding

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxFileSize  = 10 * 1024 * 1024 // 10MB максимальный размер файла
	MaxTotalSize = 50 * 1024 * 1024 // 50MB максимальный размер архива
	MaxFiles     = 1000             // Максимальное количество файлов
)

// ExtractFilesFromArchive извлекает файлы из ZIP архива
func ExtractFilesFromArchive(archiveData []byte, archiveName string) (map[string]string, string, error) {
	log.Printf("🔥 Extracting files from archive: %s (%d bytes)", archiveName, len(archiveData))

	if len(archiveData) > MaxTotalSize {
		return nil, "", fmt.Errorf("архив слишком большой: %d bytes (максимум %d)", len(archiveData), MaxTotalSize)
	}

	reader := bytes.NewReader(archiveData)
	zipReader, err := zip.NewReader(reader, int64(len(archiveData)))
	if err != nil {
		return nil, "", fmt.Errorf("не удалось открыть ZIP архив: %w", err)
	}

	files := make(map[string]string)
	var projectName string

	// Определяем название проекта из имени архива
	projectName = strings.TrimSuffix(archiveName, filepath.Ext(archiveName))
	if projectName == "" {
		projectName = "vibecoding-project"
	}

	if len(zipReader.File) > MaxFiles {
		return nil, "", fmt.Errorf("слишком много файлов в архиве: %d (максимум %d)", len(zipReader.File), MaxFiles)
	}

	totalSize := 0
	for _, file := range zipReader.File {
		// Пропускаем директории и служебные файлы
		if file.FileInfo().IsDir() {
			continue
		}

		filename := file.Name
		if shouldSkipFile(filename) {
			log.Printf("🔥 Skipping file: %s", filename)
			continue
		}

		if file.UncompressedSize64 > MaxFileSize {
			log.Printf("⚠️ File %s is too large (%d bytes), skipping", filename, file.UncompressedSize64)
			continue
		}

		totalSize += int(file.UncompressedSize64)
		if totalSize > MaxTotalSize {
			log.Printf("⚠️ Total archive size exceeded limit, stopping extraction")
			break
		}

		rc, err := file.Open()
		if err != nil {
			log.Printf("⚠️ Failed to open file %s: %v", filename, err)
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()

		if err != nil {
			log.Printf("⚠️ Failed to read file %s: %v", filename, err)
			continue
		}

		// Нормализуем путь файла (убираем префиксы директорий если есть)
		normalizedName := normalizeFilename(filename)
		files[normalizedName] = string(content)

		log.Printf("🔥 Extracted file: %s (%d bytes)", normalizedName, len(content))
	}

	if len(files) == 0 {
		return nil, "", fmt.Errorf("архив не содержит подходящих файлов для анализа")
	}

	log.Printf("🔥 Successfully extracted %d files from %s", len(files), archiveName)
	return files, projectName, nil
}

// shouldSkipFile определяет, нужно ли пропустить файл
func shouldSkipFile(filename string) bool {
	// Системные файлы и директории
	if strings.HasPrefix(filename, "__MACOSX/") ||
		strings.HasPrefix(filename, ".git/") ||
		strings.HasPrefix(filename, ".svn/") ||
		strings.HasPrefix(filename, ".hg/") ||
		strings.HasPrefix(filename, "node_modules/") ||
		strings.HasPrefix(filename, ".env") ||
		strings.Contains(filename, ".DS_Store") {
		return true
	}

	// Временные и компилированные файлы
	lowerFilename := strings.ToLower(filename)
	skipExtensions := []string{
		".pyc", ".pyo", ".class", ".o", ".so", ".dll", ".exe",
		".log", ".tmp", ".cache", ".bak", ".swp", ".lock",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".ico",
		".mp3", ".mp4", ".avi", ".mov", ".pdf", ".zip", ".tar", ".gz",
	}

	for _, ext := range skipExtensions {
		if strings.HasSuffix(lowerFilename, ext) {
			return true
		}
	}

	// Директории сборки
	if strings.Contains(lowerFilename, "/build/") ||
		strings.Contains(lowerFilename, "/dist/") ||
		strings.Contains(lowerFilename, "/target/") ||
		strings.Contains(lowerFilename, "/.next/") ||
		strings.Contains(lowerFilename, "/coverage/") {
		return true
	}

	return false
}

// normalizeFilename нормализует имя файла, убирая лишние префиксы
func normalizeFilename(filename string) string {
	// Убираем ведущие слеши и точки
	filename = strings.TrimPrefix(filename, "/")
	filename = strings.TrimPrefix(filename, "./")

	// Если файл находится в единой корневой директории проекта,
	// убираем этот префикс
	parts := strings.Split(filename, "/")
	if len(parts) > 1 {
		// Если все файлы начинаются с одной директории, это корень проекта
		return strings.Join(parts[1:], "/")
	}

	return filename
}

// CreateResultArchive создает архив с результатами сессии
func CreateResultArchive(session *VibeCodingSession) ([]byte, error) {
	log.Printf("🔥 Creating result archive for session: %s", session.ProjectName)

	allFiles := session.GetAllFiles()
	if len(allFiles) == 0 {
		return nil, fmt.Errorf("нет файлов для архивирования")
	}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Добавляем файлы в архив
	for filename, content := range allFiles {
		// Пропускаем пустые файлы и слишком большие
		if len(content) == 0 || len(content) > MaxFileSize {
			continue
		}

		// Создаем файл в архиве
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			log.Printf("⚠️ Failed to create file %s in archive: %v", filename, err)
			continue
		}

		_, err = fileWriter.Write([]byte(content))
		if err != nil {
			log.Printf("⚠️ Failed to write content for file %s: %v", filename, err)
			continue
		}

		log.Printf("🔥 Added to archive: %s (%d bytes)", filename, len(content))
	}

	// Добавляем метаданные сессии
	sessionInfo := fmt.Sprintf(`# VibeCoding Session Summary

Project: %s
Language: %s
Started: %s
Duration: %s
Files: %d original + %d generated
Test Command: %s

Generated by AI Chatter VibeCoding Mode
Session ended: %s
`,
		session.ProjectName,
		session.Analysis.Language,
		session.StartTime.Format("2006-01-02 15:04:05"),
		time.Since(session.StartTime).Round(time.Second),
		len(session.Files),
		len(session.GeneratedFiles),
		session.TestCommand,
		time.Now().Format("2006-01-02 15:04:05"))

	// Создаем файл с информацией о сессии
	infoWriter, err := zipWriter.Create("VIBECODING_SESSION.md")
	if err == nil {
		infoWriter.Write([]byte(sessionInfo))
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	archiveData := buf.Bytes()
	log.Printf("🔥 Result archive created: %d bytes with %d files", len(archiveData), len(allFiles))

	return archiveData, nil
}

// IsValidProjectArchive проверяет, содержит ли архив подходящие для анализа файлы
func IsValidProjectArchive(files map[string]string) bool {
	if len(files) == 0 {
		return false
	}

	// Ищем файлы с кодом
	codeFileCount := 0
	for filename := range files {
		if isCodeFile(filename) {
			codeFileCount++
		}
	}

	// Должен быть хотя бы один файл с кодом
	return codeFileCount > 0
}

// isCodeFile определяет, является ли файл файлом с кодом
func isCodeFile(filename string) bool {
	lowerFilename := strings.ToLower(filename)

	codeExtensions := []string{
		".py", ".js", ".ts", ".go", ".java", ".cpp", ".c", ".h",
		".rs", ".rb", ".php", ".cs", ".swift", ".kt", ".scala",
		".r", ".m", ".pl", ".sh", ".bash", ".ps1", ".yaml", ".yml",
		".json", ".xml", ".html", ".css", ".scss", ".less",
		".sql", ".dockerfile", ".makefile",
	}

	for _, ext := range codeExtensions {
		if strings.HasSuffix(lowerFilename, ext) {
			return true
		}
	}

	// Также проверяем специальные имена файлов
	specialNames := []string{
		"makefile", "dockerfile", "requirements.txt", "package.json",
		"cargo.toml", "go.mod", "pom.xml", "build.gradle", "setup.py",
	}

	for _, name := range specialNames {
		if strings.Contains(lowerFilename, name) {
			return true
		}
	}

	return false
}

// GetProjectStats возвращает статистику проекта
func GetProjectStats(files map[string]string) map[string]interface{} {
	stats := map[string]interface{}{
		"total_files": len(files),
		"languages":   make(map[string]int),
		"total_size":  0,
	}

	languages := make(map[string]int)
	totalSize := 0

	for filename, content := range files {
		totalSize += len(content)

		// Определяем язык по расширению
		lang := detectLanguageFromExtension(filename)
		if lang != "" {
			languages[lang]++
		}
	}

	stats["languages"] = languages
	stats["total_size"] = totalSize

	return stats
}

// detectLanguageFromExtension определяет язык программирования по расширению файла
func detectLanguageFromExtension(filename string) string {
	lowerFilename := strings.ToLower(filename)

	switch {
	case strings.HasSuffix(lowerFilename, ".py"):
		return "Python"
	case strings.HasSuffix(lowerFilename, ".js") || strings.HasSuffix(lowerFilename, ".ts"):
		return "JavaScript/TypeScript"
	case strings.HasSuffix(lowerFilename, ".go"):
		return "Go"
	case strings.HasSuffix(lowerFilename, ".java"):
		return "Java"
	case strings.HasSuffix(lowerFilename, ".cpp") || strings.HasSuffix(lowerFilename, ".c") || strings.HasSuffix(lowerFilename, ".h"):
		return "C/C++"
	case strings.HasSuffix(lowerFilename, ".rs"):
		return "Rust"
	case strings.HasSuffix(lowerFilename, ".rb"):
		return "Ruby"
	case strings.HasSuffix(lowerFilename, ".php"):
		return "PHP"
	case strings.HasSuffix(lowerFilename, ".cs"):
		return "C#"
	case strings.HasSuffix(lowerFilename, ".swift"):
		return "Swift"
	case strings.HasSuffix(lowerFilename, ".kt"):
		return "Kotlin"
	case strings.HasSuffix(lowerFilename, ".scala"):
		return "Scala"
	case strings.HasSuffix(lowerFilename, ".html") || strings.HasSuffix(lowerFilename, ".css"):
		return "Web"
	default:
		return ""
	}
}
