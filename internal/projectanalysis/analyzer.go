package projectanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"ai-chatter/internal/llm"
)

// ProjectInfo содержит информацию о проекте
type ProjectInfo struct {
	Language        string   `json:"language"`
	Framework       string   `json:"framework,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	InstallCommands []string `json:"install_commands"`
	Commands        []string `json:"commands"`
	DockerImage     string   `json:"docker_image"`
	ProjectType     string   `json:"project_type,omitempty"`
	WorkingDir      string   `json:"working_dir,omitempty"`
	Reasoning       string   `json:"reasoning"`
}

// Analyzer анализирует проекты для определения языка и параметров
type Analyzer struct {
	llmClient llm.Client
}

// NewAnalyzer создает новый анализатор проектов
func NewAnalyzer(llmClient llm.Client) *Analyzer {
	return &Analyzer{
		llmClient: llmClient,
	}
}

// AnalyzeProject анализирует проект и возвращает информацию о языке и параметрах
func (a *Analyzer) AnalyzeProject(ctx context.Context, files map[string]string) (*ProjectInfo, error) {
	log.Printf("📊 Analyzing project with %d files for language and framework detection", len(files))

	systemPrompt := `You are a code analysis agent. Analyze the provided project files and determine project parameters.

CRITICAL EXECUTION CONTEXT:
- All files will be copied to /workspace directory in the Docker container
- You need to determine the correct working_dir within /workspace where the project should run
- If files are in a subdirectory (e.g. extracted from archive), specify working_dir (e.g. "project-name")
- All commands (install_commands and validation commands) will be executed in /workspace/working_dir
- Use relative paths or assume files are in the current working directory
- DO NOT use absolute paths like /workspace/file.py - use just file.py

WORKING DIRECTORY ANALYSIS:
- Look at file paths to determine project structure
- ONLY set working_dir if ALL files are in the SAME subdirectory
- If files are at root level, set working_dir to "" (empty string)
- Examples:
  * Files like: "main.py", "requirements.txt" → working_dir: ""
  * Files like: "myapp/main.py", "myapp/requirements.txt" → working_dir: "myapp"

LANGUAGE DETECTION RULES:
1. Python: Look for .py files, requirements.txt, setup.py, pyproject.toml
2. JavaScript/Node.js: Look for .js files, package.json, npm-lock.json
3. Go: Look for .go files, go.mod, go.sum
4. Java: Look for .java files, pom.xml, build.gradle
5. C++: Look for .cpp, .hpp, .cc files, CMakeLists.txt, Makefile
6. Rust: Look for .rs files, Cargo.toml, Cargo.lock

DOCKER IMAGE SELECTION:
- Python: "python:3.9-slim"
- Node.js: "node:16-alpine"
- Go: "golang:1.19-alpine"
- Java: "openjdk:11-jdk-slim" or "maven:3.8-openjdk-11" if pom.xml present
- C++: "gcc:latest" or "ubuntu:20.04" 
- Rust: "rust:1.70"
- Default: "ubuntu:20.04"

INSTALL COMMANDS:
- For Python with requirements.txt: ["pip install -r requirements.txt"]
- For Node.js with package.json: ["npm install"] or ["yarn install"]
- For Go with go.mod: ["go mod download"]
- For Java with pom.xml: ["mvn dependency:resolve"]
- For C++ with CMakeLists.txt: ["cmake .", "make"]

VALIDATION COMMANDS:
Choose SIMPLE commands that can verify the project works:
- Python: ["python -m py_compile *.py"] for syntax check or ["python main.py --help"] if main exists
- Node.js: ["npm test"] if test script exists, otherwise ["node -c main.js"] for syntax check
- Go: ["go build"] or ["go test ./..."] if tests exist
- Java: ["mvn compile"] or ["javac *.java"] for simple projects
- C++: ["make"] or ["g++ -o main main.cpp"] for simple projects

OUTPUT FORMAT:
Respond ONLY with a valid JSON object matching this exact schema:
{
  "language": "string (required)",
  "framework": "string (optional)",
  "dependencies": ["string array (optional)"],
  "install_commands": ["string array (required, can be empty)"],
  "commands": ["string array (required, at least one command)"],
  "docker_image": "string (required)",
  "project_type": "string (optional)",
  "working_dir": "string (required, empty if files at root)",
  "reasoning": "string (required, explain your analysis)"
}`

	// Подготавливаем описание файлов для анализа
	var fileDescriptions []string
	for filename, content := range files {
		// Ограничиваем содержимое файла для анализа
		preview := content
		if len(content) > 500 {
			preview = content[:500] + "...[truncated]"
		}

		fileDescriptions = append(fileDescriptions, fmt.Sprintf("File: %s\n%s\n", filename, preview))
	}

	userPrompt := fmt.Sprintf("Analyze these project files and provide project information:\n\n%s", strings.Join(fileDescriptions, "\n---\n"))

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	log.Printf("🧠 Sending analysis request to LLM with %d files", len(files))

	response, err := a.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project with LLM: %w", err)
	}

	log.Printf("📝 LLM analysis response length: %d characters", len(response.Content))

	// Парсим JSON ответ
	var result ProjectInfo
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		log.Printf("⚠️ Failed to parse LLM response as JSON: %v", err)
		log.Printf("Raw response: %s", response.Content)
		return nil, fmt.Errorf("failed to parse LLM analysis response: %w", err)
	}

	// Валидируем обязательные поля
	if result.Language == "" {
		return nil, fmt.Errorf("language field is required but was empty")
	}
	if result.DockerImage == "" {
		return nil, fmt.Errorf("docker_image field is required but was empty")
	}
	if len(result.Commands) == 0 {
		return nil, fmt.Errorf("at least one validation command is required")
	}

	log.Printf("✅ Project analysis completed: language=%s, docker_image=%s, working_dir='%s'",
		result.Language, result.DockerImage, result.WorkingDir)

	return &result, nil
}

// GetSupportedLanguages возвращает список поддерживаемых языков
func GetSupportedLanguages() []string {
	return []string{"Python", "JavaScript", "Go", "Java", "C++", "Rust"}
}

// IsLanguageSupported проверяет, поддерживается ли язык
func IsLanguageSupported(language string) bool {
	supported := GetSupportedLanguages()
	for _, lang := range supported {
		if strings.EqualFold(lang, language) {
			return true
		}
	}
	return false
}
