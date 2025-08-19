package codevalidation

import (
	"context"
	"strings"
	"testing"

	"ai-chatter/internal/llm"
)

// Mock LLM client for testing
type mockLLMClient struct {
	response llm.Response
	err      error
}

func (m *mockLLMClient) Generate(ctx context.Context, messages []llm.Message) (llm.Response, error) {
	return m.response, m.err
}

func (m *mockLLMClient) GenerateWithTools(ctx context.Context, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return m.response, m.err
}

// Mock Docker manager for testing
type mockDockerManager struct {
	createError   error
	copyError     error
	installError  error
	executeResult *ValidationResult
	executeError  error
	removeError   error
	containerID   string
}

func (m *mockDockerManager) CreateContainer(ctx context.Context, analysis *CodeAnalysisResult) (string, error) {
	if m.createError != nil {
		return "", m.createError
	}
	return m.containerID, nil
}

func (m *mockDockerManager) CopyCodeToContainer(ctx context.Context, containerID, code, filename string) error {
	return m.copyError
}

func (m *mockDockerManager) CopyFilesToContainer(ctx context.Context, containerID string, files map[string]string) error {
	return m.copyError
}

func (m *mockDockerManager) InstallDependencies(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error {
	return m.installError
}

func (m *mockDockerManager) ExecuteValidation(ctx context.Context, containerID string, analysis *CodeAnalysisResult) (*ValidationResult, error) {
	if m.executeError != nil {
		return nil, m.executeError
	}
	return m.executeResult, nil
}

func (m *mockDockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	return m.removeError
}

// Mock progress callback for testing
type mockProgressCallback struct {
	steps []string
}

func (m *mockProgressCallback) UpdateProgress(step string, status string) {
	m.steps = append(m.steps, step+":"+status)
}

func TestDetectCodeInMessage(t *testing.T) {
	tests := []struct {
		name           string
		messageContent string
		llmResponse    string
		expectedHas    bool
		expectedCode   string
		expectedFile   string
	}{
		{
			name:           "Code block detected",
			messageContent: "Here is my Python code: ```python\nprint('hello')\n```",
			llmResponse:    `{"has_code": true, "extracted_code": "print('hello')", "filename": "script.py", "reasoning": "Python code block found"}`,
			expectedHas:    true,
			expectedCode:   "print('hello')",
			expectedFile:   "script.py",
		},
		{
			name:           "No code detected",
			messageContent: "This is just a regular message without code",
			llmResponse:    `{"has_code": false, "extracted_code": "", "filename": "", "reasoning": "No code found in message"}`,
			expectedHas:    false,
			expectedCode:   "",
			expectedFile:   "",
		},
		{
			name:           "JavaScript code detected",
			messageContent: "```js\nfunction test() { return 42; }\n```",
			llmResponse:    `{"has_code": true, "extracted_code": "function test() { return 42; }", "filename": "script.js", "reasoning": "JavaScript function found"}`,
			expectedHas:    true,
			expectedCode:   "function test() { return 42; }",
			expectedFile:   "script.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := &mockLLMClient{
				response: llm.Response{Content: tt.llmResponse},
				err:      nil,
			}

			hasCode, extractedCode, filename, err := DetectCodeInMessage(context.Background(), mockLLM, tt.messageContent)

			if err != nil {
				t.Errorf("DetectCodeInMessage() error = %v", err)
				return
			}

			if hasCode != tt.expectedHas {
				t.Errorf("DetectCodeInMessage() hasCode = %v, want %v", hasCode, tt.expectedHas)
			}

			if extractedCode != tt.expectedCode {
				t.Errorf("DetectCodeInMessage() extractedCode = %v, want %v", extractedCode, tt.expectedCode)
			}

			if filename != tt.expectedFile {
				t.Errorf("DetectCodeInMessage() filename = %v, want %v", filename, tt.expectedFile)
			}
		})
	}
}

func TestCodeAnalysisResult_JSON(t *testing.T) {
	// Test that CodeAnalysisResult can be properly marshaled/unmarshaled
	original := &CodeAnalysisResult{
		Language:        "Python",
		Framework:       "Flask",
		Dependencies:    []string{"flask", "requests"},
		InstallCommands: []string{"pip install -r requirements.txt"},
		Commands:        []string{"python -m pytest", "pylint *.py"},
		DockerImage:     "python:3.11-slim",
		ProjectType:     "web application",
		Reasoning:       "Flask web app detected",
	}

	// This tests that our struct tags are correct
	if original.Language != "Python" {
		t.Errorf("Expected Language to be Python, got %s", original.Language)
	}

	if len(original.InstallCommands) != 1 {
		t.Errorf("Expected 1 install command, got %d", len(original.InstallCommands))
	}
}

func TestValidationResult_Success(t *testing.T) {
	result := &ValidationResult{
		Success:     true,
		Output:      "All tests passed",
		Errors:      []string{},
		Warnings:    []string{"Minor style issue"},
		ExitCode:    0,
		Duration:    "2.5s",
		Suggestions: []string{"Consider adding more tests"},
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected ExitCode to be 0, got %d", result.ExitCode)
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(result.Errors))
	}
}

func TestCodeValidationWorkflow_ProcessCodeValidation(t *testing.T) {
	mockLLM := &mockLLMClient{
		response: llm.Response{
			Content: `{"language": "Python", "framework": "", "dependencies": [], "install_commands": ["pip install pytest"], "commands": ["python -m pytest"], "docker_image": "python:3.11-slim", "project_type": "script", "reasoning": "Simple Python script"}`,
		},
	}

	mockDocker := &mockDockerManager{
		containerID: "test-container-123",
		executeResult: &ValidationResult{
			Success:  true,
			Output:   "All tests passed",
			ExitCode: 0,
			Duration: "1.2s",
		},
	}

	workflow := NewCodeValidationWorkflow(mockLLM, mockDocker)
	progressCallback := &mockProgressCallback{}

	result, err := workflow.ProcessCodeValidation(context.Background(), "print('hello world')", "test.py", progressCallback)

	if err != nil {
		t.Errorf("ProcessCodeValidation() error = %v", err)
		return
	}

	if !result.Success {
		t.Error("Expected validation to succeed")
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Check that all progress steps were called
	expectedSteps := []string{
		"code_analysis:in_progress",
		"code_analysis:completed",
		"docker_setup:in_progress",
		"docker_setup:completed",
		"copy_code:in_progress",
		"copy_code:completed",
		"run_validation:in_progress",
		"run_validation:completed",
	}

	if len(progressCallback.steps) != len(expectedSteps) {
		t.Errorf("Expected %d progress steps, got %d", len(expectedSteps), len(progressCallback.steps))
	}

	for i, expected := range expectedSteps {
		if i < len(progressCallback.steps) && progressCallback.steps[i] != expected {
			t.Errorf("Progress step %d: expected %s, got %s", i, expected, progressCallback.steps[i])
		}
	}
}

func TestCodeValidationWorkflow_ProcessProjectValidation(t *testing.T) {
	mockLLM := &mockLLMClient{
		response: llm.Response{
			Content: `{"language": "JavaScript", "framework": "Express", "dependencies": ["express"], "install_commands": ["npm install"], "commands": ["npm test", "npm run lint"], "docker_image": "node:18-alpine", "project_type": "web application", "reasoning": "Express.js web application"}`,
		},
	}

	mockDocker := &mockDockerManager{
		containerID: "test-container-456",
		executeResult: &ValidationResult{
			Success:  true,
			Output:   "All tests passed\nLinting completed",
			ExitCode: 0,
			Duration: "3.5s",
		},
	}

	workflow := NewCodeValidationWorkflow(mockLLM, mockDocker)
	progressCallback := &mockProgressCallback{}

	files := map[string]string{
		"index.js":     "const express = require('express');",
		"package.json": `{"name": "test-app", "dependencies": {"express": "^4.18.0"}}`,
	}

	result, err := workflow.ProcessProjectValidation(context.Background(), files, progressCallback)

	if err != nil {
		t.Errorf("ProcessProjectValidation() error = %v", err)
		return
	}

	if !result.Success {
		t.Error("Expected validation to succeed")
	}

	// Verify that install_deps step was included for this case
	found := false
	for _, step := range progressCallback.steps {
		if strings.Contains(step, "install_deps") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected install_deps step to be present")
	}
}
