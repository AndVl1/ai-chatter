package codevalidation

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

// DockerManager интерфейс для управления Docker контейнерами
type DockerManager interface {
	CreateContainer(ctx context.Context, analysis *CodeAnalysisResult) (string, error)
	CopyCodeToContainer(ctx context.Context, containerID, code, filename string) error
	CopyFilesToContainer(ctx context.Context, containerID string, files map[string]string) error
	InstallDependencies(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error
	ExecuteValidation(ctx context.Context, containerID string, analysis *CodeAnalysisResult) (*ValidationResult, error)
	RemoveContainer(ctx context.Context, containerID string) error
}

// DockerClient реализация DockerManager с использованием Docker CLI
type DockerClient struct {
	dockerPath string
}

// NewDockerClient создает новый Docker client
func NewDockerClient() (*DockerClient, error) {
	log.Printf("🐳 Initializing Docker client")

	// Проверяем наличие Docker
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker not found in PATH: %w", err)
	}

	// Проверяем что Docker работает
	cmd := exec.Command(dockerPath, "version")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker is not running or not accessible: %w", err)
	}

	return &DockerClient{
		dockerPath: dockerPath,
	}, nil
}

// NewMockDockerClient создает mock клиент для случаев когда Docker недоступен
func NewMockDockerClient() DockerManager {
	log.Printf("🔧 Initializing mock Docker client (Docker not available)")
	return &MockDockerClient{}
}

// MockDockerClient реализация DockerManager без реального Docker
type MockDockerClient struct{}

func (m *MockDockerClient) CreateContainer(ctx context.Context, analysis *CodeAnalysisResult) (string, error) {
	log.Printf("🔧 Mock: Creating container with image %s", analysis.DockerImage)
	return "mock-container-id", nil
}

func (m *MockDockerClient) CopyCodeToContainer(ctx context.Context, containerID, code, filename string) error {
	log.Printf("🔧 Mock: Copying code %s to container", filename)
	return nil
}

func (m *MockDockerClient) CopyFilesToContainer(ctx context.Context, containerID string, files map[string]string) error {
	log.Printf("🔧 Mock: Copying %d files to container", len(files))
	return nil
}

func (m *MockDockerClient) InstallDependencies(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error {
	log.Printf("🔧 Mock: Installing dependencies: %v", analysis.InstallCommands)
	return nil
}

func (m *MockDockerClient) ExecuteValidation(ctx context.Context, containerID string, analysis *CodeAnalysisResult) (*ValidationResult, error) {
	log.Printf("🔧 Mock: Executing validation commands: %v", analysis.Commands)

	// Возвращаем mock результат с поддержкой новых полей
	return &ValidationResult{
		Success:  true,
		Output:   "Mock validation completed - Docker is not available for actual execution",
		Errors:   []string{},
		Warnings: []string{"Code validation completed in mock mode (Docker not available)"},
		ExitCode: 0,
		Duration: "0.5s",
		Suggestions: []string{
			"Install Docker to enable real code validation",
			"Code analysis was performed but not executed",
			"Consider setting up Docker for full validation capabilities",
		},
		// Новые поля поддерживаются, но пока пустые в mock режиме
		UserQuestion:   "",
		QuestionAnswer: "",
		ErrorAnalysis:  "",
		RetryAttempt:   0,
		BuildProblems:  []string{},
		CodeProblems:   []string{},
		TotalTokens:    0, // В mock режиме токены не тратятся
	}, nil
}

func (m *MockDockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	log.Printf("🔧 Mock: Removing container %s", containerID)
	return nil
}

// CreateContainer создает и запускает Docker контейнер
func (d *DockerClient) CreateContainer(ctx context.Context, analysis *CodeAnalysisResult) (string, error) {
	log.Printf("🐳 Creating Docker container with image: %s", analysis.DockerImage)

	// Создаем контейнер с сетевыми настройками и VibeCoding MCP сервером
	cmd := exec.CommandContext(ctx, d.dockerPath, "run", "-d", "-i",
		"--workdir=/workspace",
		"--network=host",  // Используем host сеть для доступа к интернету
		"--dns=8.8.8.8",   // Добавляем Google DNS
		"--dns=8.8.4.4",   // Резервный DNS
		"-p", "8080:8080", // Порт для веб-интерфейса
		"-p", "8090:8090", // Порт для VibeCoding MCP сервера
		"-e", "DEBIAN_FRONTEND=noninteractive",
		"-v", "/tmp/vibecoding-mcp:/tmp/vibecoding-mcp", // Монтируем директорию для MCP сокетов
		analysis.DockerImage, "sh")

	log.Printf("🔧 Docker command: %s", cmd.String())

	output, err := cmd.Output()
	if err != nil {
		// Получаем stderr для диагностики
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr := string(exitError.Stderr)
			log.Printf("❌ Docker command failed with stderr: %s", stderr)
			return "", fmt.Errorf("failed to create container: %w (stderr: %s)", err, stderr)
		}
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	log.Printf("✅ Container created and started: %s", containerID)

	// Проверяем сетевое подключение в контейнере
	if err := d.verifyNetworkAccess(ctx, containerID); err != nil {
		log.Printf("⚠️ Network connectivity check failed: %v", err)
	}

	return containerID, nil
}

// CopyCodeToContainer копирует код в контейнер
func (d *DockerClient) CopyCodeToContainer(ctx context.Context, containerID, code, filename string) error {
	log.Printf("📋 Copying code to container %s as %s", containerID, filename)

	return d.CopyFilesToContainer(ctx, containerID, map[string]string{
		filename: code,
	})
}

// CopyFilesToContainer копирует множественные файлы в контейнер
func (d *DockerClient) CopyFilesToContainer(ctx context.Context, containerID string, files map[string]string) error {
	log.Printf("📋 Copying %d files to container %s", len(files), containerID)

	// Отладка: показываем какие файлы копируем
	for filename, content := range files {
		log.Printf("🔍 File to copy: %s (size: %d bytes)", filename, len(content))
	}

	tarBuffer := &bytes.Buffer{}
	tw := tar.NewWriter(tarBuffer)

	for filename, content := range files {
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", filename, err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			return fmt.Errorf("failed to write file content for %s: %w", filename, err)
		}
	}

	tw.Close()

	log.Printf("📦 Created TAR archive with size: %d bytes", tarBuffer.Len())

	// Используем docker cp для копирования файлов
	cmd := exec.CommandContext(ctx, d.dockerPath, "cp", "-", containerID+":/workspace")
	cmd.Stdin = tarBuffer

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ Docker cp command failed: %v", err)
		log.Printf("❌ Docker cp output: %s", string(output))
		return fmt.Errorf("failed to copy files to container: %w", err)
	}

	if len(output) > 0 {
		log.Printf("📋 Docker cp output: %s", string(output))
	}

	// Проверяем что файлы действительно скопированы
	if err := d.verifyFilesCopied(ctx, containerID, files); err != nil {
		log.Printf("⚠️ File verification failed: %v", err)
		// Не возвращаем ошибку, просто предупреждение
	}

	log.Printf("✅ Files copied successfully")
	return nil
}

// verifyFilesCopied проверяет что файлы действительно скопированы в контейнер
func (d *DockerClient) verifyFilesCopied(ctx context.Context, containerID string, files map[string]string) error {
	log.Printf("🔍 Verifying files were copied to container")

	for filename := range files {
		filePath := fmt.Sprintf("/workspace/%s", filename)

		// Проверяем существование файла
		checkCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "test", "-f", filePath)
		if err := checkCmd.Run(); err != nil {
			return fmt.Errorf("file %s not found in container at %s", filename, filePath)
		}

		// Получаем размер файла для дополнительной проверки
		sizeCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "wc", "-c", filePath)
		output, err := sizeCmd.CombinedOutput()
		if err != nil {
			log.Printf("⚠️ Could not get size for %s: %v", filePath, err)
		} else {
			log.Printf("✅ File %s exists in container, size: %s", filename, strings.TrimSpace(string(output)))
		}
	}

	// Показываем древовидную структуру /workspace для отладки
	d.showWorkspaceTree(ctx, containerID)

	return nil
}

// detectProjectRoot анализирует структуру файлов в контейнере и определяет проектную директорию
func (d *DockerClient) detectProjectRoot(ctx context.Context, containerID string) string {
	workspaceBase := "/workspace"

	// 1. Получаем список всех файлов и директорий в /workspace
	findCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "find", workspaceBase, "-type", "f", "-o", "-type", "d")
	output, err := findCmd.CombinedOutput()
	if err != nil {
		log.Printf("⚠️ Failed to analyze workspace structure: %v", err)
		return workspaceBase
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	log.Printf("📁 Found %d items in workspace", len(lines))

	// 2. Ищем конфигурационные файлы проектов
	projectMarkers := []string{
		"build.gradle", "build.gradle.kts", "pom.xml", "package.json",
		"requirements.txt", "go.mod", "Cargo.toml", "CMakeLists.txt",
		"gradlew", "mvnw", "composer.json", "pyproject.toml",
	}

	projectRoots := make(map[string]int)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == workspaceBase {
			continue
		}

		// Проверяем, содержит ли путь маркеры проекта
		fileName := strings.ToLower(filepath.Base(line))
		for _, marker := range projectMarkers {
			if fileName == marker {
				// Получаем директорию, содержащую этот файл
				dir := filepath.Dir(line)
				projectRoots[dir]++
				log.Printf("🎯 Found project marker %s in %s", marker, dir)
			}
		}
	}

	// 3. Если нашли проектные маркеры, выбираем директорию с наибольшим количеством маркеров
	if len(projectRoots) > 0 {
		bestRoot := workspaceBase
		maxMarkers := 0

		for root, count := range projectRoots {
			if count > maxMarkers || (count == maxMarkers && len(root) > len(bestRoot)) {
				bestRoot = root
				maxMarkers = count
			}
		}

		log.Printf("🏆 Selected project root: %s (with %d markers)", bestRoot, maxMarkers)
		return bestRoot
	}

	// 4. Если маркеров нет, ищем наиболее общую директорию с исходными файлами
	sourceDirs := make(map[string]int)
	sourceExts := []string{".java", ".kt", ".py", ".js", ".ts", ".go", ".cpp", ".c", ".rs"}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == workspaceBase {
			continue
		}

		ext := strings.ToLower(filepath.Ext(line))
		for _, sourceExt := range sourceExts {
			if ext == sourceExt {
				dir := filepath.Dir(line)
				sourceDirs[dir]++
				break
			}
		}
	}

	if len(sourceDirs) > 0 {
		// Ищем наиболее общую директорию (shortest path с максимальным количеством файлов)
		bestDir := workspaceBase
		maxFiles := 0

		for dir, count := range sourceDirs {
			// Предпочитаем директории с большим количеством файлов, но не слишком глубокие
			score := count - strings.Count(strings.TrimPrefix(dir, workspaceBase), "/")
			currentScore := maxFiles - strings.Count(strings.TrimPrefix(bestDir, workspaceBase), "/")

			if score > currentScore {
				bestDir = dir
				maxFiles = count
			}
		}

		log.Printf("📂 Selected source directory: %s (with %d source files)", bestDir, maxFiles)
		return bestDir
	}

	log.Printf("🤷 No project structure detected, using workspace root: %s", workspaceBase)
	return workspaceBase
}

// showWorkspaceTree показывает древовидную структуру содержимого /workspace
func (d *DockerClient) showWorkspaceTree(ctx context.Context, containerID string) {
	log.Printf("🌳 Displaying workspace tree structure")

	// Пытаемся использовать tree если доступен
	treeCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "tree", "/workspace", "-a", "-L", "4")
	if output, err := treeCmd.CombinedOutput(); err == nil {
		log.Printf("📁 /workspace tree structure:\n%s", string(output))
		return
	}

	// Fallback: используем find для создания древовидной структуры
	log.Printf("📁 tree command not available, using find to create tree structure")

	findCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "find", "/workspace", "-type", "f", "-o", "-type", "d")
	output, err := findCmd.CombinedOutput()
	if err != nil {
		log.Printf("⚠️ Could not list /workspace with find: %v", err)
		// Fallback на обычный ls
		d.fallbackListWorkspace(ctx, containerID)
		return
	}

	// Парсим и форматируем вывод find в древовидную структуру
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	tree := d.buildTreeFromPaths(lines)
	log.Printf("📁 /workspace tree structure:\n%s", tree)
}

// fallbackListWorkspace показывает простой список если tree недоступен
func (d *DockerClient) fallbackListWorkspace(ctx context.Context, containerID string) {
	listCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "ls", "-la", "/workspace")
	if output, err := listCmd.CombinedOutput(); err != nil {
		log.Printf("⚠️ Could not list /workspace: %v", err)
	} else {
		log.Printf("📁 /workspace contents (fallback):\n%s", string(output))
	}
}

// TreeNode представляет узел в древовидной структуре
type TreeNode struct {
	Name     string
	IsDir    bool
	Children map[string]*TreeNode
	Level    int
}

// buildTreeFromPaths строит древовидную структуру из списка путей
func (d *DockerClient) buildTreeFromPaths(paths []string) string {
	if len(paths) == 0 {
		return "No files found in /workspace"
	}

	root := &TreeNode{
		Name:     "/workspace",
		IsDir:    true,
		Children: make(map[string]*TreeNode),
		Level:    0,
	}

	// Добавляем все пути в дерево
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || path == "/workspace" {
			continue
		}

		// Убираем префикс /workspace/
		relativePath := strings.TrimPrefix(path, "/workspace/")
		if relativePath == "" {
			continue
		}

		parts := strings.Split(relativePath, "/")
		current := root

		// Проходим по каждой части пути
		for i, part := range parts {
			if part == "" {
				continue
			}

			if current.Children[part] == nil {
				current.Children[part] = &TreeNode{
					Name:     part,
					IsDir:    i < len(parts)-1 || strings.HasSuffix(path, "/"), // Директория если не последний элемент
					Children: make(map[string]*TreeNode),
					Level:    current.Level + 1,
				}
			}
			current = current.Children[part]
		}
	}

	// Рендерим дерево
	var result strings.Builder
	result.WriteString("/workspace\n")
	d.renderTreeNode(root, "", true, &result)
	return result.String()
}

// renderTreeNode рекурсивно рендерит узел дерева
func (d *DockerClient) renderTreeNode(node *TreeNode, prefix string, isLast bool, result *strings.Builder) {
	// Сортируем детей: сначала директории, потом файлы, по алфавиту внутри групп
	type SortableChild struct {
		Name string
		Node *TreeNode
	}

	var children []SortableChild
	for name, child := range node.Children {
		children = append(children, SortableChild{Name: name, Node: child})
	}

	// Сортируем: директории сначала, потом файлы, внутри групп по алфавиту
	for i := 0; i < len(children)-1; i++ {
		for j := i + 1; j < len(children); j++ {
			a, b := children[i], children[j]

			// Директории идут первыми
			if a.Node.IsDir && !b.Node.IsDir {
				continue // a уже на правильном месте
			}
			if !a.Node.IsDir && b.Node.IsDir {
				children[i], children[j] = children[j], children[i]
				continue
			}

			// Внутри одной группы (директории или файлы) сортируем по алфавиту
			if strings.ToLower(a.Name) > strings.ToLower(b.Name) {
				children[i], children[j] = children[j], children[i]
			}
		}
	}

	// Рендерим детей
	for i, child := range children {
		isLastChild := i == len(children)-1

		// Выбираем символ для отображения структуры
		var connector, childPrefix string
		if isLastChild {
			connector = "└── "
			childPrefix = prefix + "    "
		} else {
			connector = "├── "
			childPrefix = prefix + "│   "
		}

		// Добавляем иконку для типа файла
		icon := "📄"
		if child.Node.IsDir {
			icon = "📁"
		} else {
			// Определяем тип файла по расширению
			ext := strings.ToLower(filepath.Ext(child.Name))
			switch ext {
			case ".py":
				icon = "🐍"
			case ".js", ".ts":
				icon = "⚡"
			case ".go":
				icon = "🐹"
			case ".java":
				icon = "☕"
			case ".cpp", ".c", ".h":
				icon = "⚙️"
			case ".rs":
				icon = "🦀"
			case ".json":
				icon = "📋"
			case ".md":
				icon = "📝"
			case ".txt":
				icon = "📄"
			case ".yml", ".yaml":
				icon = "⚙️"
			case ".xml":
				icon = "🏷️"
			default:
				icon = "📄"
			}
		}

		result.WriteString(prefix + connector + icon + " " + child.Name)
		if child.Node.IsDir {
			result.WriteString("/")
		}
		result.WriteString("\n")

		// Рекурсивно рендерим дочерние элементы
		if len(child.Node.Children) > 0 {
			d.renderTreeNode(child.Node, childPrefix, isLastChild, result)
		}
	}
}

// getWorkingDirectory возвращает полную рабочую директорию для команд
func (d *DockerClient) getWorkingDirectory(ctx context.Context, containerID string, analysis *CodeAnalysisResult) string {
	// Используем автоматическое определение вместо LLM предположений
	detectedRoot := d.detectProjectRoot(ctx, containerID)

	// Если LLM указала working_dir, проверим её, но приоритет у автоматического определения
	if analysis.WorkingDir != "" {
		workspaceBase := "/workspace"
		targetDir := fmt.Sprintf("%s/%s", workspaceBase, analysis.WorkingDir)

		checkCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "test", "-d", targetDir)
		if err := checkCmd.Run(); err != nil {
			log.Printf("⚠️ LLM suggested directory %s does not exist, using detected: %s", targetDir, detectedRoot)
		} else {
			log.Printf("✅ LLM suggested directory %s exists, but using auto-detected: %s", targetDir, detectedRoot)
		}
	}

	return detectedRoot
}

// InstallDependencies устанавливает зависимости в контейнере
func (d *DockerClient) InstallDependencies(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error {
	log.Printf("📦 Installing dependencies in container %s", containerID)

	// Используем LLM-определенные команды установки зависимостей
	if len(analysis.InstallCommands) == 0 {
		log.Printf("📦 No installation commands provided")
		return nil
	}

	workingDir := d.getWorkingDirectory(ctx, containerID, analysis)

	// Выполняем каждую команду установки
	for i, cmd := range analysis.InstallCommands {
		log.Printf("📦 Running install command %d/%d: %s", i+1, len(analysis.InstallCommands), cmd)

		execCmd := exec.CommandContext(ctx, d.dockerPath, "exec", "-w", workingDir, containerID, "sh", "-c", cmd)
		output, err := execCmd.CombinedOutput()
		if err != nil {
			log.Printf("❌ Install command failed: %s", string(output))

			// Проверяем если это сетевая ошибка
			outputStr := string(output)
			if d.isNetworkError(outputStr) {
				log.Printf("🌐 Detected network connectivity issue, running diagnostics...")
				d.diagnoseNetworkIssues(ctx, containerID)
			}

			return fmt.Errorf("install command '%s' failed: %w\nOutput: %s", cmd, err, outputStr)
		}

		log.Printf("📦 Install command output: %s", string(output))
	}

	log.Printf("✅ All installation commands completed successfully")
	return nil
}

// ExecuteValidation выполняет команды валидации в контейнере
func (d *DockerClient) ExecuteValidation(ctx context.Context, containerID string, analysis *CodeAnalysisResult) (*ValidationResult, error) {
	log.Printf("⚡ Executing validation commands in container %s", containerID)

	workingDir := d.getWorkingDirectory(ctx, containerID, analysis)

	result := &ValidationResult{
		Success:  true,
		Output:   "",
		Errors:   []string{},
		Warnings: []string{},
		ExitCode: 0,
	}

	// Выполняем каждую команду валидации
	for i, cmd := range analysis.Commands {
		log.Printf("⚡ Running command %d/%d: %s", i+1, len(analysis.Commands), cmd)

		execCmd := exec.CommandContext(ctx, d.dockerPath, "exec", "-w", workingDir, containerID, "sh", "-c", cmd)
		output, err := execCmd.CombinedOutput()

		commandOutput := string(output)
		result.Output += fmt.Sprintf("=== Command: %s ===\n%s\n\n", cmd, commandOutput)

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = 1
			}
			result.Success = false
			result.Errors = append(result.Errors, fmt.Sprintf("Command '%s' failed: %v", cmd, err))
		}
	}

	if result.Success {
		log.Printf("✅ All validation commands completed successfully")
		result.Suggestions = []string{
			"Code validation passed all checks",
			"Consider adding more comprehensive tests",
			"Ensure proper error handling is implemented",
		}
	} else {
		log.Printf("❌ Some validation commands failed")
	}

	return result, nil
}

// RemoveContainer удаляет контейнер
func (d *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	log.Printf("🗑️ Removing container: %s", containerID)

	cmd := exec.CommandContext(ctx, d.dockerPath, "rm", "-f", containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	log.Printf("✅ Container removed: %s", containerID)
	return nil
}

// verifyNetworkAccess проверяет сетевое подключение в контейнере
func (d *DockerClient) verifyNetworkAccess(ctx context.Context, containerID string) error {
	log.Printf("🌐 Checking network connectivity in container %s", containerID)

	// Проверяем DNS разрешение
	dnsCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "nslookup", "google.com")
	if err := dnsCmd.Run(); err != nil {
		log.Printf("❌ DNS resolution failed: %v", err)

		// Пытаемся проверить основной DNS
		dnsTestCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "nslookup", "8.8.8.8")
		if err := dnsTestCmd.Run(); err != nil {
			return fmt.Errorf("DNS resolution completely failed: %w", err)
		}
	}

	// Проверяем HTTP подключение
	httpCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "sh", "-c",
		"command -v wget >/dev/null 2>&1 && wget -q --spider https://google.com --timeout=10 || "+
			"command -v curl >/dev/null 2>&1 && curl -s --max-time 10 https://google.com >/dev/null || "+
			"echo 'No wget/curl available for HTTP test'")

	if err := httpCmd.Run(); err != nil {
		log.Printf("⚠️ HTTP connectivity test failed: %v", err)
		return fmt.Errorf("HTTP connectivity failed: %w", err)
	}

	log.Printf("✅ Network connectivity verified")
	return nil
}

// isNetworkError проверяет содержит ли вывод признаки сетевых ошибок
func (d *DockerClient) isNetworkError(output string) bool {
	networkErrorPatterns := []string{
		"Failed to establish a new connection",
		"Temporary failure in name resolution",
		"network is unreachable",
		"Connection timed out",
		"Could not resolve host",
		"dial tcp: lookup",
		"connection broken",
		"NewConnectionError",
		"proxy.golang.org",
		"pypi.org",
		"registry.npmjs.org",
	}

	outputLower := strings.ToLower(output)
	for _, pattern := range networkErrorPatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// diagnoseNetworkIssues выполняет диагностику сетевых проблем в контейнере
func (d *DockerClient) diagnoseNetworkIssues(ctx context.Context, containerID string) {
	log.Printf("🔍 Running network diagnostics for container %s", containerID)

	// Проверка сетевых интерфейсов
	ifCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "ip", "addr", "show")
	if output, err := ifCmd.CombinedOutput(); err == nil {
		log.Printf("📡 Network interfaces:\n%s", string(output))
	}

	// Проверка маршрутизации
	routeCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "ip", "route", "show")
	if output, err := routeCmd.CombinedOutput(); err == nil {
		log.Printf("🗺️ Routing table:\n%s", string(output))
	}

	// Проверка DNS настроек
	resolvCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "cat", "/etc/resolv.conf")
	if output, err := resolvCmd.CombinedOutput(); err == nil {
		log.Printf("🌐 DNS configuration:\n%s", string(output))
	}

	// Тест ping к внешним адресам
	pingCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "ping", "-c", "2", "8.8.8.8")
	if err := pingCmd.Run(); err != nil {
		log.Printf("❌ Cannot ping 8.8.8.8: %v", err)
	} else {
		log.Printf("✅ Can ping 8.8.8.8")
	}

	// Проверка доступности портов
	tcpCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "sh", "-c",
		"timeout 5 bash -c '</dev/tcp/8.8.8.8/53' && echo 'Port 53 accessible' || echo 'Port 53 not accessible'")
	if output, err := tcpCmd.CombinedOutput(); err == nil {
		log.Printf("🔌 TCP connectivity test: %s", strings.TrimSpace(string(output)))
	}
}
