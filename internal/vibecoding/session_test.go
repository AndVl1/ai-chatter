package vibecoding

import (
	"testing"
	"time"

	"ai-chatter/internal/codevalidation"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager()
	if sm == nil {
		t.Error("Expected session manager to be created")
	}

	if sm.GetActiveSessions() != 0 {
		t.Error("Expected 0 active sessions initially")
	}
}

func TestSessionManager_CreateSession(t *testing.T) {
	sm := NewSessionManager()

	files := map[string]string{
		"main.py": "print('hello world')",
		"test.py": "import unittest",
	}

	session, err := sm.CreateSession(123, 456, "test-project", files, nil)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.UserID != 123 {
		t.Errorf("Expected UserID 123, got %d", session.UserID)
	}

	if session.ChatID != 456 {
		t.Errorf("Expected ChatID 456, got %d", session.ChatID)
	}

	if session.ProjectName != "test-project" {
		t.Errorf("Expected project name 'test-project', got %s", session.ProjectName)
	}

	if len(session.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(session.Files))
	}

	if sm.GetActiveSessions() != 1 {
		t.Error("Expected 1 active session")
	}
}

func TestSessionManager_DuplicateSession(t *testing.T) {
	sm := NewSessionManager()

	files := map[string]string{
		"main.py": "print('hello world')",
	}

	// Create first session
	_, err := sm.CreateSession(123, 456, "project1", files, nil)
	if err != nil {
		t.Fatalf("Failed to create first session: %v", err)
	}

	// Try to create second session for same user
	_, err = sm.CreateSession(123, 456, "project2", files, nil)
	if err == nil {
		t.Error("Expected error when creating duplicate session")
	}

	if sm.GetActiveSessions() != 1 {
		t.Error("Expected only 1 active session")
	}
}

func TestSessionManager_GetSession(t *testing.T) {
	sm := NewSessionManager()

	files := map[string]string{
		"main.py": "print('hello world')",
	}

	// Create session
	originalSession, err := sm.CreateSession(123, 456, "test-project", files, nil)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Get session
	retrievedSession := sm.GetSession(123)
	if retrievedSession == nil {
		t.Error("Session should exist")
	}

	if retrievedSession.UserID != originalSession.UserID {
		t.Error("Retrieved session doesn't match original")
	}

	// Try to get non-existent session
	nonExistentSession := sm.GetSession(999)
	if nonExistentSession != nil {
		t.Error("Non-existent session should not exist")
	}
}

func TestSessionManager_EndSession(t *testing.T) {
	sm := NewSessionManager()

	files := map[string]string{
		"main.py": "print('hello world')",
	}

	// Create session
	_, err := sm.CreateSession(123, 456, "test-project", files, nil)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if sm.GetActiveSessions() != 1 {
		t.Error("Expected 1 active session")
	}

	// End session
	err = sm.EndSession(123)
	if err != nil {
		t.Errorf("Failed to end session: %v", err)
	}

	if sm.GetActiveSessions() != 0 {
		t.Error("Expected 0 active sessions after ending")
	}

	// Try to end non-existent session
	err = sm.EndSession(999)
	if err == nil {
		t.Error("Expected error when ending non-existent session")
	}
}

func TestSessionManager_HasActiveSession(t *testing.T) {
	sm := NewSessionManager()

	if sm.HasActiveSession(123) {
		t.Error("Should not have active session initially")
	}

	files := map[string]string{
		"main.py": "print('hello world')",
	}

	// Create session
	_, err := sm.CreateSession(123, 456, "test-project", files, nil)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if !sm.HasActiveSession(123) {
		t.Error("Should have active session after creation")
	}

	if sm.HasActiveSession(999) {
		t.Error("Should not have active session for different user")
	}
}

// Note: Tests for detectLanguageFromFile and getDockerImageForLanguage removed
// These methods have been replaced with unified LLM-based project analysis

func TestVibeCodingSession_GenerateTestCommand(t *testing.T) {
	session := &VibeCodingSession{
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
			Commands: []string{}, // No test commands in analysis
		},
	}

	expected := "python -m pytest -v || python -m unittest discover -v"
	result := session.generateTestCommand()
	if result != expected {
		t.Errorf("generateTestCommand() = %s, expected %s", result, expected)
	}
}

func TestVibeCodingSession_AddGeneratedFile(t *testing.T) {
	session := &VibeCodingSession{
		GeneratedFiles: make(map[string]string),
	}

	session.AddGeneratedFile("test_main.py", "import unittest\nprint('test')")

	if len(session.GeneratedFiles) != 1 {
		t.Error("Expected 1 generated file")
	}

	content, exists := session.GeneratedFiles["test_main.py"]
	if !exists {
		t.Error("Generated file should exist")
	}

	if content != "import unittest\nprint('test')" {
		t.Error("Generated file content doesn't match")
	}
}

func TestVibeCodingSession_GetAllFiles(t *testing.T) {
	session := &VibeCodingSession{
		Files: map[string]string{
			"main.py":  "print('hello')",
			"utils.py": "def helper(): pass",
		},
		GeneratedFiles: map[string]string{
			"test_main.py": "import unittest",
		},
	}

	allFiles := session.GetAllFiles()

	if len(allFiles) != 3 {
		t.Errorf("Expected 3 files total, got %d", len(allFiles))
	}

	// Check original files
	if allFiles["main.py"] != "print('hello')" {
		t.Error("Original file content doesn't match")
	}

	// Check generated files
	if allFiles["test_main.py"] != "import unittest" {
		t.Error("Generated file content doesn't match")
	}
}

func TestVibeCodingSession_GetSessionInfo(t *testing.T) {
	startTime := time.Now()
	session := &VibeCodingSession{
		ProjectName: "test-project",
		StartTime:   startTime,
		Files: map[string]string{
			"main.py": "print('hello')",
		},
		GeneratedFiles: map[string]string{
			"test_main.py": "import unittest",
		},
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
		TestCommand: "python -m pytest -v",
		ContainerID: "test-container-123",
	}

	info := session.GetSessionInfo()

	if info["project_name"] != "test-project" {
		t.Error("Project name doesn't match")
	}

	if info["language"] != "Python" {
		t.Error("Language doesn't match")
	}

	if info["files_count"] != 1 {
		t.Error("Files count doesn't match")
	}

	if info["generated_count"] != 1 {
		t.Error("Generated count doesn't match")
	}

	if info["test_command"] != "python -m pytest -v" {
		t.Error("Test command doesn't match")
	}

	if info["container_id"] != "test-container-123" {
		t.Error("Container ID doesn't match")
	}
}
