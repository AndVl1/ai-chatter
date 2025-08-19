package codevalidation

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
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
	}, nil
}

func (m *MockDockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	log.Printf("🔧 Mock: Removing container %s", containerID)
	return nil
}

// CreateContainer создает и запускает Docker контейнер
func (d *DockerClient) CreateContainer(ctx context.Context, analysis *CodeAnalysisResult) (string, error) {
	log.Printf("🐳 Creating Docker container with image: %s", analysis.DockerImage)

	// Создаем контейнер
	cmd := exec.CommandContext(ctx, d.dockerPath, "run", "-d", "-i",
		"--workdir=/workspace",
		"-e", "DEBIAN_FRONTEND=noninteractive",
		analysis.DockerImage, "sh")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	log.Printf("✅ Container created and started: %s", containerID)
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

	// Используем docker cp для копирования файлов
	cmd := exec.CommandContext(ctx, d.dockerPath, "cp", "-", containerID+":/workspace")
	cmd.Stdin = tarBuffer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy files to container: %w", err)
	}

	log.Printf("✅ Files copied successfully")
	return nil
}

// InstallDependencies устанавливает зависимости в контейнере
func (d *DockerClient) InstallDependencies(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error {
	log.Printf("📦 Installing dependencies in container %s", containerID)

	// Используем LLM-определенные команды установки зависимостей
	if len(analysis.InstallCommands) == 0 {
		log.Printf("📦 No installation commands provided")
		return nil
	}

	// Выполняем каждую команду установки
	for i, cmd := range analysis.InstallCommands {
		log.Printf("📦 Running install command %d/%d: %s", i+1, len(analysis.InstallCommands), cmd)

		execCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "sh", "-c", cmd)
		output, err := execCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("install command '%s' failed: %w\nOutput: %s", cmd, err, string(output))
		}

		log.Printf("📦 Install command output: %s", string(output))
	}

	log.Printf("✅ All installation commands completed successfully")
	return nil
}

// ExecuteValidation выполняет команды валидации в контейнере
func (d *DockerClient) ExecuteValidation(ctx context.Context, containerID string, analysis *CodeAnalysisResult) (*ValidationResult, error) {
	log.Printf("⚡ Executing validation commands in container %s", containerID)

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

		execCmd := exec.CommandContext(ctx, d.dockerPath, "exec", containerID, "sh", "-c", cmd)
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
