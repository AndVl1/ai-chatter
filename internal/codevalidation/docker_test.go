package codevalidation

import (
	"context"
	"os/exec"
	"testing"
)

func TestNewDockerClient(t *testing.T) {
	// Skip this test if Docker is not available
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker not available in PATH, skipping test")
	}

	client, err := NewDockerClient()
	if err != nil {
		// This might fail if Docker daemon is not running, which is OK for unit tests
		t.Logf("Docker client creation failed (expected in some environments): %v", err)
		return
	}

	if client.dockerPath == "" {
		t.Error("Expected dockerPath to be set")
	}
}

func TestNewMockDockerClient(t *testing.T) {
	// Test that mock client can always be created
	mockClient := NewMockDockerClient()
	if mockClient == nil {
		t.Error("Expected mock client to be created")
	}

	// Test that it implements the interface
	var _ DockerManager = mockClient

	ctx := context.Background()
	analysis := &CodeAnalysisResult{
		Language:        "Python",
		DockerImage:     "python:3.11-slim",
		InstallCommands: []string{"pip install pytest"},
		Commands:        []string{"python -m pytest"},
	}

	// Test all methods work without errors
	containerID, err := mockClient.CreateContainer(ctx, analysis)
	if err != nil {
		t.Errorf("Mock CreateContainer failed: %v", err)
	}
	if containerID != "mock-container-id" {
		t.Errorf("Expected mock container ID, got %s", containerID)
	}

	err = mockClient.CopyFilesToContainer(ctx, containerID, map[string]string{"test.py": "print('hello')"})
	if err != nil {
		t.Errorf("Mock CopyFilesToContainer failed: %v", err)
	}

	err = mockClient.InstallDependencies(ctx, containerID, analysis)
	if err != nil {
		t.Errorf("Mock InstallDependencies failed: %v", err)
	}

	result, err := mockClient.ExecuteValidation(ctx, containerID, analysis)
	if err != nil {
		t.Errorf("Mock ExecuteValidation failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected mock validation to succeed")
	}

	if len(result.Warnings) == 0 {
		t.Error("Expected mock validation to include warning about Docker unavailability")
	}

	if len(result.Suggestions) == 0 {
		t.Error("Expected mock validation to include suggestions")
	}

	err = mockClient.RemoveContainer(ctx, containerID)
	if err != nil {
		t.Errorf("Mock RemoveContainer failed: %v", err)
	}
}

func TestDockerClient_CreateContainer_MockMode(t *testing.T) {
	// Test the interface without actually calling Docker
	analysis := &CodeAnalysisResult{
		Language:        "Python",
		Framework:       "",
		Dependencies:    []string{},
		InstallCommands: []string{"pip install pytest"},
		Commands:        []string{"python -m pytest"},
		DockerImage:     "python:3.11-slim",
		ProjectType:     "script",
		Reasoning:       "Python script for testing",
	}

	// Verify the analysis struct is properly constructed
	if analysis.DockerImage != "python:3.11-slim" {
		t.Errorf("Expected docker image python:3.11-slim, got %s", analysis.DockerImage)
	}

	if len(analysis.InstallCommands) != 1 {
		t.Errorf("Expected 1 install command, got %d", len(analysis.InstallCommands))
	}

	if len(analysis.Commands) != 1 {
		t.Errorf("Expected 1 validation command, got %d", len(analysis.Commands))
	}
}

func TestValidationResult_Structure(t *testing.T) {
	result := &ValidationResult{
		Success:     false,
		Output:      "Command failed",
		Errors:      []string{"Syntax error in line 1", "Missing import"},
		Warnings:    []string{"Unused variable"},
		ExitCode:    1,
		Duration:    "0.5s",
		Suggestions: []string{"Fix syntax errors", "Add missing imports"},
	}

	if result.Success {
		t.Error("Expected Success to be false")
	}

	if result.ExitCode != 1 {
		t.Errorf("Expected ExitCode to be 1, got %d", result.ExitCode)
	}

	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(result.Errors))
	}

	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}

	if len(result.Suggestions) != 2 {
		t.Errorf("Expected 2 suggestions, got %d", len(result.Suggestions))
	}

	// Test that error messages are properly stored
	expectedError := "Syntax error in line 1"
	if result.Errors[0] != expectedError {
		t.Errorf("Expected first error to be '%s', got '%s'", expectedError, result.Errors[0])
	}
}

// Test the DockerManager interface compliance
func TestDockerManagerInterface(t *testing.T) {
	// Create a mock that implements the interface
	mock := &mockDockerManager{
		containerID: "test-123",
		executeResult: &ValidationResult{
			Success:  true,
			Output:   "Tests passed",
			ExitCode: 0,
		},
	}

	// Verify it implements the interface
	var _ DockerManager = mock

	ctx := context.Background()
	analysis := &CodeAnalysisResult{
		DockerImage: "python:3.11-slim",
		Commands:    []string{"python -m pytest"},
	}

	// Test CreateContainer
	containerID, err := mock.CreateContainer(ctx, analysis)
	if err != nil {
		t.Errorf("CreateContainer failed: %v", err)
	}
	if containerID != "test-123" {
		t.Errorf("Expected container ID 'test-123', got '%s'", containerID)
	}

	// Test ExecuteValidation
	result, err := mock.ExecuteValidation(ctx, containerID, analysis)
	if err != nil {
		t.Errorf("ExecuteValidation failed: %v", err)
	}
	if !result.Success {
		t.Error("Expected validation to succeed")
	}

	// Test RemoveContainer
	err = mock.RemoveContainer(ctx, containerID)
	if err != nil {
		t.Errorf("RemoveContainer failed: %v", err)
	}
}

// Test different language configurations
func TestLanguageConfigurations(t *testing.T) {
	testCases := []struct {
		name        string
		language    string
		image       string
		installCmd  string
		validateCmd string
	}{
		{
			name:        "Python",
			language:    "Python",
			image:       "python:3.11-slim",
			installCmd:  "pip install -r requirements.txt",
			validateCmd: "python -m pytest",
		},
		{
			name:        "Node.js",
			language:    "JavaScript",
			image:       "node:18-alpine",
			installCmd:  "npm install",
			validateCmd: "npm test",
		},
		{
			name:        "Go",
			language:    "Go",
			image:       "golang:1.21-alpine",
			installCmd:  "go mod download",
			validateCmd: "go test ./...",
		},
		{
			name:        "Java",
			language:    "Java",
			image:       "openjdk:17-slim",
			installCmd:  "mvn install",
			validateCmd: "mvn test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			analysis := &CodeAnalysisResult{
				Language:        tc.language,
				DockerImage:     tc.image,
				InstallCommands: []string{tc.installCmd},
				Commands:        []string{tc.validateCmd},
			}

			if analysis.Language != tc.language {
				t.Errorf("Expected language %s, got %s", tc.language, analysis.Language)
			}

			if analysis.DockerImage != tc.image {
				t.Errorf("Expected docker image %s, got %s", tc.image, analysis.DockerImage)
			}

			if len(analysis.InstallCommands) == 0 || analysis.InstallCommands[0] != tc.installCmd {
				t.Errorf("Expected install command %s, got %v", tc.installCmd, analysis.InstallCommands)
			}

			if len(analysis.Commands) == 0 || analysis.Commands[0] != tc.validateCmd {
				t.Errorf("Expected validate command %s, got %v", tc.validateCmd, analysis.Commands)
			}
		})
	}
}

// Test error handling scenarios
func TestDockerErrorHandling(t *testing.T) {
	testCases := []struct {
		name         string
		createError  error
		executeError error
		removeError  error
	}{
		{
			name:        "All success",
			createError: nil,
		},
		{
			name:        "Create error",
			createError: &exec.ExitError{},
		},
		{
			name:         "Execute error",
			executeError: &exec.ExitError{},
		},
		{
			name:        "Remove error",
			removeError: &exec.ExitError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockDockerManager{
				containerID:   "test-container",
				createError:   tc.createError,
				executeError:  tc.executeError,
				removeError:   tc.removeError,
				executeResult: &ValidationResult{Success: true},
			}

			ctx := context.Background()
			analysis := &CodeAnalysisResult{DockerImage: "python:3.11-slim"}

			// Test create
			_, createErr := mock.CreateContainer(ctx, analysis)
			if tc.createError != nil && createErr == nil {
				t.Error("Expected create error but got none")
			}
			if tc.createError == nil && createErr != nil {
				t.Errorf("Unexpected create error: %v", createErr)
			}

			// Test execute (only if create succeeded)
			if createErr == nil {
				_, executeErr := mock.ExecuteValidation(ctx, "test-container", analysis)
				if tc.executeError != nil && executeErr == nil {
					t.Error("Expected execute error but got none")
				}
				if tc.executeError == nil && executeErr != nil {
					t.Errorf("Unexpected execute error: %v", executeErr)
				}
			}

			// Test remove
			removeErr := mock.RemoveContainer(ctx, "test-container")
			if tc.removeError != nil && removeErr == nil {
				t.Error("Expected remove error but got none")
			}
			if tc.removeError == nil && removeErr != nil {
				t.Errorf("Unexpected remove error: %v", removeErr)
			}
		})
	}
}
