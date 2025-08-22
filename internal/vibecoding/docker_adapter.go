package vibecoding

import (
	"context"

	"ai-chatter/internal/codevalidation"
)

// DockerAdapter адаптирует DockerManager для работы с unified CodeAnalysisResult
type DockerAdapter struct {
	dockerManager codevalidation.DockerManager
}

// NewDockerAdapter создает новый адаптер
func NewDockerAdapter(dockerManager codevalidation.DockerManager) *DockerAdapter {
	return &DockerAdapter{
		dockerManager: dockerManager,
	}
}

// CreateContainer создает контейнер напрямую используя CodeAnalysisResult
func (a *DockerAdapter) CreateContainer(ctx context.Context, analysis *codevalidation.CodeAnalysisResult) (string, error) {
	return a.dockerManager.CreateContainer(ctx, analysis)
}

// CopyFilesToContainer копирует файлы в контейнер
func (a *DockerAdapter) CopyFilesToContainer(ctx context.Context, containerID string, files map[string]string) error {
	return a.dockerManager.CopyFilesToContainer(ctx, containerID, files)
}

// InstallDependencies устанавливает зависимости напрямую используя CodeAnalysisResult
func (a *DockerAdapter) InstallDependencies(ctx context.Context, containerID string, analysis *codevalidation.CodeAnalysisResult) error {
	return a.dockerManager.InstallDependencies(ctx, containerID, analysis)
}

// ExecuteValidation выполняет валидацию напрямую используя CodeAnalysisResult
func (a *DockerAdapter) ExecuteValidation(ctx context.Context, containerID string, analysis *codevalidation.CodeAnalysisResult) (*codevalidation.ValidationResult, error) {
	return a.dockerManager.ExecuteValidation(ctx, containerID, analysis)
}

// RemoveContainer удаляет контейнер
func (a *DockerAdapter) RemoveContainer(ctx context.Context, containerID string) error {
	return a.dockerManager.RemoveContainer(ctx, containerID)
}
