package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WebServer представляет HTTP сервер для VibeCoding
type WebServer struct {
	sessionManager *SessionManager
	server         *http.Server
	port           int
	startTime      time.Time
}

// FileNode представляет узел в дереве файлов
type FileNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" или "directory"
	Path     string      `json:"path"`
	Children []*FileNode `json:"children,omitempty"`
	Size     int         `json:"size,omitempty"`
	Content  string      `json:"content,omitempty"`
}

// SessionData представляет данные сессии для веб-интерфейса
type SessionData struct {
	UserID      int64     `json:"user_id"`
	ProjectName string    `json:"project_name"`
	Language    string    `json:"language"`
	StartTime   time.Time `json:"start_time"`
	Duration    string    `json:"duration"`
	FilesTree   *FileNode `json:"files_tree"`
	Stats       struct {
		TotalFiles     int `json:"total_files"`
		GeneratedFiles int `json:"generated_files"`
		TotalSize      int `json:"total_size"`
	} `json:"stats"`
}

// NewWebServer создает новый веб-сервер
func NewWebServer(sessionManager *SessionManager, port int) *WebServer {
	return &WebServer{
		sessionManager: sessionManager,
		port:           port,
		startTime:      time.Now(),
	}
}

// Start запускает веб-сервер
func (ws *WebServer) Start() error {
	mux := http.NewServeMux()

	// Регистрируем обработчики
	mux.HandleFunc("/static/", ws.handleStatic)        // Статические файлы
	mux.HandleFunc("/api/status", ws.handleStatus)     // Health check endpoint
	mux.HandleFunc("/api/sessions", ws.handleSessions) // Список всех сессий (админ)
	mux.HandleFunc("/api/context/", ws.handleContext)  // API для получения контекста сессии
	mux.HandleFunc("/api/save/", ws.handleSaveFile)    // API для сохранения файлов
	mux.HandleFunc("/vibe_", ws.handleVibeSession)     // HTML страницы vibe сессий
	mux.HandleFunc("/admin", ws.handleAdmin)           // Админская страница
	mux.HandleFunc("/", ws.handleRoot)                 // Корневой обработчик (должен быть последним)

	ws.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", ws.port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("🌐 Starting VibeCoding web server on http://localhost:%d (accessible locally)", ws.port)
	return ws.server.ListenAndServe()
}

// Stop останавливает веб-сервер
func (ws *WebServer) Stop() error {
	if ws.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return ws.server.Shutdown(ctx)
}

// handleContext обрабатывает API запросы на получение контекста сессии
func (ws *WebServer) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем userID из URL: /api/context/{userID}
	path := strings.TrimPrefix(r.URL.Path, "/api/context/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "User ID is required in path /api/context/{userID}", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session := ws.sessionManager.GetSession(userID)
	if session == nil {
		http.Error(w, "VibeCoding session not found", http.StatusNotFound)
		return
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Читаем context JSON файл из рабочей директории
	workingDir := "."
	if session.Analysis != nil && session.Analysis.WorkingDir != "" {
		workingDir = session.Analysis.WorkingDir
	}

	contextPath := filepath.Join(workingDir, "vibecoding-context.json")
	contextData, err := os.ReadFile(contextPath)
	if err != nil {
		log.Printf("❌ Failed to read context file %s: %v", contextPath, err)
		http.Error(w, "Context file not found or not readable", http.StatusNotFound)
		return
	}

	// Возвращаем JSON контекст
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(contextData)
}

// handleSaveFile обрабатывает сохранение файлов через веб-интерфейс
func (ws *WebServer) handleSaveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем userID из URL: /api/save/{userID}
	path := strings.TrimPrefix(r.URL.Path, "/api/save/")
	userID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session := ws.sessionManager.GetSession(userID)
	if session == nil {
		http.Error(w, "VibeCoding session not found", http.StatusNotFound)
		return
	}

	// Парсим JSON запрос
	var saveRequest struct {
		Filename  string `json:"filename"`
		Content   string `json:"content"`
		Generated bool   `json:"generated,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&saveRequest); err != nil {
		http.Error(w, "Invalid JSON request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if saveRequest.Filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Сохраняем файл в соответствующую карту
	if saveRequest.Generated {
		session.GeneratedFiles[saveRequest.Filename] = saveRequest.Content
		log.Printf("💾 Saved generated file via web interface: %s (user %d)", saveRequest.Filename, userID)
	} else {
		session.Files[saveRequest.Filename] = saveRequest.Content
		log.Printf("💾 Saved original file via web interface: %s (user %d)", saveRequest.Filename, userID)
	}

	// Синхронизируем с Docker контейнером
	if session.ContainerID != "" {
		filesToSync := map[string]string{
			saveRequest.Filename: saveRequest.Content,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := session.Docker.CopyFilesToContainer(ctx, session.ContainerID, filesToSync); err != nil {
			log.Printf("⚠️ Failed to sync file to container: %v", err)
		} else {
			log.Printf("🔄 Synced file to container: %s", saveRequest.Filename)
		}
	}

	// Возвращаем успешный ответ
	response := map[string]interface{}{
		"success":  true,
		"message":  "File saved successfully",
		"filename": saveRequest.Filename,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVibeSession обрабатывает запросы на страницы сессий
func (ws *WebServer) handleVibeSession(w http.ResponseWriter, r *http.Request) {
	log.Printf("🌐 VibeCoding web request: %s", r.URL.Path)

	// Извлекаем userID из URL
	path := strings.TrimPrefix(r.URL.Path, "/vibe_")
	if path == "" || path == r.URL.Path {
		// Путь не содержит /vibe_ или содержит только /vibe_
		http.Error(w, "User ID is required in path /vibe_{userID}", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		log.Printf("❌ Invalid user ID in path: %s", path)
		http.Error(w, fmt.Sprintf("Invalid user ID '%s'", path), http.StatusBadRequest)
		return
	}

	log.Printf("🔍 Looking for VibeCoding session for user %d", userID)

	// Получаем сессию
	session := ws.sessionManager.GetSession(userID)
	if session == nil {
		log.Printf("❌ VibeCoding session not found for user %d", userID)
		availableSessions := ws.sessionManager.GetActiveSessions()
		http.Error(w, fmt.Sprintf("VibeCoding session not found for user %d. Active sessions: %d", userID, availableSessions), http.StatusNotFound)
		return
	}

	log.Printf("✅ Found VibeCoding session for user %d: %s", userID, session.ProjectName)

	// Подготавливаем данные
	data := ws.prepareSessionData(session)

	// Отрендерим HTML страницу
	ws.renderSessionPage(w, data)
}

// handleVibeAPI обрабатывает API запросы
func (ws *WebServer) handleVibeAPI(w http.ResponseWriter, r *http.Request) {
	// Извлекаем userID из URL
	path := strings.TrimPrefix(r.URL.Path, "/api/vibe_")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Invalid API path", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session := ws.sessionManager.GetSession(userID)
	if session == nil {
		http.Error(w, "VibeCoding session not found", http.StatusNotFound)
		return
	}

	// Обрабатываем API запросы
	if len(parts) > 1 && parts[1] == "file" {
		ws.handleFileContent(w, r, session, strings.Join(parts[2:], "/"))
		return
	}

	// По умолчанию возвращаем данные сессии в JSON
	data := ws.prepareSessionData(session)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleFileContent обрабатывает запросы содержимого файлов
func (ws *WebServer) handleFileContent(w http.ResponseWriter, r *http.Request, session *VibeCodingSession, filePath string) {
	if filePath == "" {
		http.Error(w, "File path is required", http.StatusBadRequest)
		return
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Декодируем URL-encoded путь
	decodedPath, err := url.QueryUnescape(filePath)
	if err != nil {
		decodedPath = filePath // Fallback to original if decoding fails
	}

	// Ищем файл в основных файлах
	if content, exists := session.Files[decodedPath]; exists {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
		return
	}

	// Ищем в сгенерированных файлах - сначала как есть
	if content, exists := session.GeneratedFiles[decodedPath]; exists {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
		return
	}

	// Если не нашли, пробуем очистить префиксы для сгенерированных файлов
	cleanPath := strings.TrimPrefix(decodedPath, "[generated] ")
	cleanPath = strings.TrimSuffix(cleanPath, " (generated)")

	if content, exists := session.GeneratedFiles[cleanPath]; exists {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
		return
	}

	// Если всё ещё не нашли, попробуем найти файл в файловой структуре как есть
	// Это для случаев, когда путь уже содержит [generated] префикс
	for path, content := range session.Files {
		if path == decodedPath || "[generated] "+path == decodedPath {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(content))
			return
		}
	}

	for path, content := range session.GeneratedFiles {
		if "[generated] "+path == decodedPath {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(content))
			return
		}
	}

	log.Printf("❌ File not found: %s (original: %s)", decodedPath, filePath)
	http.Error(w, "File not found", http.StatusNotFound)
}

// handleStatic обрабатывает статические файлы
func (ws *WebServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Простая обработка CSS/JS - в реальном проекте лучше использовать embed или внешние файлы
	path := strings.TrimPrefix(r.URL.Path, "/static/")

	switch path {
	case "style.css":
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(getCSS()))
	case "script.js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(getJS()))
	default:
		http.NotFound(w, r)
	}
}

// prepareSessionData подготавливает данные сессии для отображения
func (ws *WebServer) prepareSessionData(session *VibeCodingSession) *SessionData {
	session.mutex.RLock()
	defer session.mutex.RUnlock()

	data := &SessionData{
		UserID:      session.UserID,
		ProjectName: session.ProjectName,
		StartTime:   session.StartTime,
		Duration:    time.Since(session.StartTime).Round(time.Second).String(),
	}

	if session.Analysis != nil {
		data.Language = session.Analysis.Language
	}

	// Строим дерево файлов
	data.FilesTree = ws.buildFileTree(session.Files, session.GeneratedFiles)

	// Подсчитываем статистику
	data.Stats.TotalFiles = len(session.Files)
	data.Stats.GeneratedFiles = len(session.GeneratedFiles)

	for _, content := range session.Files {
		data.Stats.TotalSize += len(content)
	}
	for _, content := range session.GeneratedFiles {
		data.Stats.TotalSize += len(content)
	}

	return data
}

// buildFileTree строит дерево файлов из карты файлов
func (ws *WebServer) buildFileTree(originalFiles, generatedFiles map[string]string) *FileNode {
	root := &FileNode{
		Name:     "project",
		Type:     "directory",
		Path:     "",
		Children: make([]*FileNode, 0),
	}

	// Объединяем все файлы
	allFiles := make(map[string]string)
	for path, content := range originalFiles {
		allFiles[path] = content
	}
	for path, content := range generatedFiles {
		allFiles["[generated] "+path] = content
	}

	// Группируем файлы по директориям
	directories := make(map[string]*FileNode)
	directories[""] = root

	// Сортируем пути для правильного порядка создания директорий
	var sortedPaths []string
	for path := range allFiles {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		content := allFiles[path]
		ws.addFileToTree(directories, path, content)
	}

	// Сортируем узлы в каждой директории
	ws.sortTreeNodes(root)

	return root
}

// addFileToTree добавляет файл в дерево
func (ws *WebServer) addFileToTree(directories map[string]*FileNode, filePath, content string) {
	parts := strings.Split(filePath, "/")

	// Создаем промежуточные директории
	currentPath := ""
	for i, part := range parts[:len(parts)-1] {
		if i > 0 {
			currentPath += "/"
		}
		currentPath += part

		if _, exists := directories[currentPath]; !exists {
			newDir := &FileNode{
				Name:     part,
				Type:     "directory",
				Path:     currentPath,
				Children: make([]*FileNode, 0),
			}

			// Добавляем в родительскую директорию
			parentPath := filepath.Dir(currentPath)
			if parentPath == "." {
				parentPath = ""
			}

			if parent, exists := directories[parentPath]; exists {
				parent.Children = append(parent.Children, newDir)
			}

			directories[currentPath] = newDir
		}
	}

	// Добавляем сам файл
	fileName := parts[len(parts)-1]
	fileNode := &FileNode{
		Name:    fileName,
		Type:    "file",
		Path:    filePath,
		Size:    len(content),
		Content: content,
	}

	// Определяем родительскую директорию
	parentPath := filepath.Dir(filePath)
	if parentPath == "." {
		parentPath = ""
	}

	if parent, exists := directories[parentPath]; exists {
		parent.Children = append(parent.Children, fileNode)
	}
}

// sortTreeNodes сортирует узлы в дереве
func (ws *WebServer) sortTreeNodes(node *FileNode) {
	if len(node.Children) == 0 {
		return
	}

	// Сортируем: сначала директории, потом файлы, внутри каждой группы - по алфавиту
	sort.Slice(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]

		if a.Type != b.Type {
			return a.Type == "directory" // Директории сначала
		}

		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	// Рекурсивно сортируем дочерние узлы
	for _, child := range node.Children {
		ws.sortTreeNodes(child)
	}
}

// renderSessionPage рендерит HTML страницу сессии
func (ws *WebServer) renderSessionPage(w http.ResponseWriter, data *SessionData) {
	tmpl := template.Must(template.New("session").Parse(getHTMLTemplate()))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// getHTMLTemplate возвращает HTML шаблон
func getHTMLTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VibeCoding - {{.ProjectName}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header class="header">
            <h1>🔥 VibeCoding Session</h1>
            <div class="project-info">
                <h2>{{.ProjectName}}</h2>
                <div class="meta">
                    <span class="language">{{.Language}}</span>
                    <span class="duration">{{.Duration}}</span>
                </div>
            </div>
        </header>
        
        <div class="main-content">
            <aside class="sidebar">
                <div class="stats">
                    <h3>📊 Statistics</h3>
                    <div class="stat-item">
                        <span class="stat-label">Files:</span>
                        <span class="stat-value">{{.Stats.TotalFiles}}</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-label">Generated:</span>
                        <span class="stat-value">{{.Stats.GeneratedFiles}}</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-label">Size:</span>
                        <span class="stat-value">{{.Stats.TotalSize}} bytes</span>
                    </div>
                </div>
                
                <div class="file-tree">
                    <h3>📁 Project Structure</h3>
                    <div id="tree-container"></div>
                </div>
            </aside>
            
            <main class="content">
                <div class="file-viewer">
                    <div class="file-header">
                        <span id="current-file">Select a file to view</span>
                        <button id="save-file-btn" onclick="saveCurrentFile()" class="btn save-btn" style="display: none;">💾 Save File</button>
                    </div>
                    <textarea id="file-content" class="file-content" placeholder="No file selected" readonly></textarea>
                </div>
            </main>
        </div>
    </div>
    
    <script src="/static/script.js"></script>
    <script>
        // Инициализируем дерево файлов
        const treeData = {{.FilesTree}};
        const userId = {{.UserID}};
        initializeFileTree(treeData, userId);
    </script>
</body>
</html>`
}

// getCSS возвращает CSS стили
func getCSS() string {
	return `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
    background-color: #1a1a1a;
    color: #e0e0e0;
    line-height: 1.6;
}

.container {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

.header {
    background: linear-gradient(90deg, #ff6b35, #f7931e);
    padding: 1rem 2rem;
    border-bottom: 3px solid #333;
}

.header h1 {
    color: white;
    margin-bottom: 0.5rem;
}

.project-info h2 {
    color: white;
    margin-bottom: 0.5rem;
}

.meta {
    display: flex;
    gap: 1rem;
}

.language, .duration {
    background: rgba(255, 255, 255, 0.2);
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    font-size: 0.9rem;
}

.main-content {
    display: flex;
    flex: 1;
    min-height: 0;
}

.sidebar {
    width: 300px;
    background: #2a2a2a;
    border-right: 2px solid #333;
    padding: 1rem;
    overflow-y: auto;
}

.stats {
    margin-bottom: 2rem;
}

.stats h3, .file-tree h3 {
    color: #ff6b35;
    margin-bottom: 1rem;
    font-size: 1rem;
}

.stat-item {
    display: flex;
    justify-content: space-between;
    margin-bottom: 0.5rem;
    padding: 0.25rem 0;
}

.stat-label {
    color: #ccc;
}

.stat-value {
    color: #fff;
    font-weight: bold;
}

.file-tree {
    flex: 1;
}

#tree-container {
    font-size: 0.9rem;
}

.tree-node {
    margin-left: 1rem;
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 3px;
    transition: background-color 0.2s;
}

.tree-node:hover {
    background-color: #333;
}

.tree-node.selected {
    background-color: #ff6b35;
    color: white;
}

.tree-folder {
    font-weight: bold;
    color: #f7931e;
}

.tree-file {
    color: #e0e0e0;
}

.tree-file.generated {
    color: #4CAF50;
}

.tree-toggle {
    display: inline-block;
    width: 1rem;
    text-align: center;
    margin-right: 0.25rem;
}

.content {
    flex: 1;
    padding: 1rem;
    overflow: hidden;
    display: flex;
    flex-direction: column;
}

.file-viewer {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: #222;
    border-radius: 8px;
    overflow: hidden;
}

.file-header {
    background: #333;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid #444;
    color: #ff6b35;
    font-weight: bold;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.file-content {
    flex: 1;
    padding: 1rem;
    background: #1a1a1a;
    overflow: auto;
    white-space: pre-wrap;
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
    font-size: 0.9rem;
    line-height: 1.4;
    border: none;
    resize: none;
}

@media (max-width: 768px) {
    .main-content {
        flex-direction: column;
    }
    
    .sidebar {
        width: 100%;
        order: 2;
    }
    
    .content {
        order: 1;
        min-height: 60vh;
    }
}
`
}

// getJS возвращает JavaScript код
func getJS() string {
	return `
function initializeFileTree(treeData, userId) {
    const container = document.getElementById('tree-container');
    renderTreeNode(treeData, container, '', userId);
}

function renderTreeNode(node, container, level, userId) {
    const nodeElement = document.createElement('div');
    nodeElement.className = 'tree-node';
    
    const toggleElement = document.createElement('span');
    toggleElement.className = 'tree-toggle';
    
    if (node.type === 'directory') {
        nodeElement.className += ' tree-folder';
        toggleElement.textContent = '▶';
        toggleElement.onclick = (e) => {
            e.stopPropagation();
            toggleDirectory(nodeElement, toggleElement);
        };
    } else {
        nodeElement.className += ' tree-file';
        if (node.name.includes('[generated]')) {
            nodeElement.className += ' generated';
        }
        toggleElement.textContent = '📄';
        nodeElement.onclick = () => loadFileContent(node.path, node.name, userId);
    }
    
    nodeElement.appendChild(toggleElement);
    nodeElement.appendChild(document.createTextNode(node.name));
    container.appendChild(nodeElement);
    
    if (node.children && node.children.length > 0) {
        const childContainer = document.createElement('div');
        childContainer.style.display = 'none';
        childContainer.className = 'tree-children';
        
        for (const child of node.children) {
            renderTreeNode(child, childContainer, level + 1, userId);
        }
        
        container.appendChild(childContainer);
        nodeElement.childContainer = childContainer;
    }
}

function toggleDirectory(nodeElement, toggleElement) {
    const childContainer = nodeElement.childContainer;
    if (childContainer) {
        if (childContainer.style.display === 'none') {
            childContainer.style.display = 'block';
            toggleElement.textContent = '▼';
        } else {
            childContainer.style.display = 'none';
            toggleElement.textContent = '▶';
        }
    }
}

function loadFileContent(filePath, fileName, userId) {
    // Удаляем предыдущее выделение
    const previousSelected = document.querySelector('.tree-node.selected');
    if (previousSelected) {
        previousSelected.classList.remove('selected');
    }
    
    // Выделяем текущий узел
    event.target.classList.add('selected');
    
    // Обновляем заголовок
    document.getElementById('current-file').textContent = fileName;
    
    // Загружаем содержимое файла
    const contentElement = document.getElementById('file-content');
    contentElement.textContent = 'Loading...';
    
    const encodedPath = encodeURIComponent(filePath);
    fetch('/api/vibe_' + userId + '/file/' + encodedPath)
        .then(response => {
            if (!response.ok) {
                throw new Error('Failed to load file: ' + response.statusText);
            }
            return response.text();
        })
        .then(content => {
            contentElement.value = content;
            contentElement.readOnly = false;
            
            // Показываем кнопку сохранения
            const saveBtn = document.getElementById('save-file-btn');
            saveBtn.style.display = 'block';
            
            // Сохраняем текущие параметры для последующего сохранения
            window.currentFile = {
                path: filePath,
                name: fileName,
                userId: userId,
                generated: filePath.startsWith('[generated]')
            };
        })
        .catch(error => {
            contentElement.value = 'Error loading file: ' + error.message;
            contentElement.readOnly = true;
            document.getElementById('save-file-btn').style.display = 'none';
            console.error('Error loading file:', error);
        });
}

function saveCurrentFile() {
    if (!window.currentFile) {
        alert('No file is currently loaded');
        return;
    }
    
    const saveBtn = document.getElementById('save-file-btn');
    const contentElement = document.getElementById('file-content');
    
    // Отключаем кнопку во время сохранения
    saveBtn.disabled = true;
    saveBtn.textContent = '💾 Saving...';
    
    const saveData = {
        filename: window.currentFile.path.replace('[generated] ', ''), // убираем префикс для generated файлов
        content: contentElement.value,
        generated: window.currentFile.generated
    };
    
    fetch('/api/save/' + window.currentFile.userId, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(saveData)
    })
    .then(response => response.json())
    .then(result => {
        if (result.success) {
            saveBtn.textContent = '✅ Saved!';
            setTimeout(() => {
                saveBtn.textContent = '💾 Save File';
                saveBtn.disabled = false;
            }, 2000);
        } else {
            throw new Error(result.message || 'Save failed');
        }
    })
    .catch(error => {
        alert('Failed to save file: ' + error.message);
        saveBtn.textContent = '❌ Save Failed';
        setTimeout(() => {
            saveBtn.textContent = '💾 Save File';
            saveBtn.disabled = false;
        }, 3000);
        console.error('Save error:', error);
    });
}

// Автообновление каждые 30 секунд
setInterval(() => {
    location.reload();
}, 30000);
`
}

// handleRoot обрабатывает корневые запросы для отладки
func (ws *WebServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Только обрабатываем запросы к корню или к несуществующим путям
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<html>
<head><title>VibeCoding Web Server</title></head>
<body>
<h1>VibeCoding Web Server</h1>
<p>Server is running. Use URLs like <code>/vibe_123</code> to access sessions.</p>
<p>Active sessions: %d</p>
</body>
</html>`, ws.sessionManager.GetActiveSessions())
		return
	}

	// Проверяем, не API запрос ли это к vibe_ сессии
	if strings.HasPrefix(r.URL.Path, "/api/vibe_") {
		ws.handleVibeAPI(w, r)
		return
	}

	// Проверяем, не запрос ли это к vibe_ сессии
	if strings.HasPrefix(r.URL.Path, "/vibe_") {
		ws.handleVibeSession(w, r)
		return
	}

	// Для всех остальных путей возвращаем 404
	http.NotFound(w, r)
}

// handleStatus обрабатывает health check запросы
func (ws *WebServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "ai-chatter",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    time.Since(ws.startTime).String(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleSessions обрабатывает запросы на получение списка всех активных сессий (админ API)
func (ws *WebServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	sessions := ws.sessionManager.GetAllSessions()
	sessionList := make([]map[string]interface{}, 0, len(sessions))

	for userID, session := range sessions {
		// Безопасно получаем язык программирования
		language := "Unknown"
		if session.Analysis != nil {
			language = session.Analysis.Language
		}

		sessionInfo := map[string]interface{}{
			"user_id":         userID,
			"project_name":    session.ProjectName,
			"language":        language,
			"start_time":      session.StartTime,
			"duration":        time.Since(session.StartTime).Round(time.Second).String(),
			"container_id":    session.ContainerID,
			"test_command":    session.TestCommand,
			"files_count":     len(session.Files),
			"generated_count": len(session.GeneratedFiles),
		}
		sessionList = append(sessionList, sessionInfo)
	}

	response := map[string]interface{}{
		"success":        true,
		"sessions":       sessionList,
		"total_sessions": len(sessionList),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleAdmin обрабатывает админскую страницу для просмотра всех сессий
func (ws *WebServer) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	adminHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VibeCoding Admin - Active Sessions</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-bottom: 20px; }
        .stats { background: #e3f2fd; padding: 15px; border-radius: 5px; margin-bottom: 20px; }
        .session-card { border: 1px solid #ddd; margin: 10px 0; padding: 15px; border-radius: 5px; background: #fafafa; }
        .session-header { font-weight: bold; color: #1976d2; margin-bottom: 10px; }
        .session-meta { font-size: 14px; color: #666; }
        .session-actions { margin-top: 10px; }
        .btn { padding: 8px 15px; background: #1976d2; color: white; text-decoration: none; border-radius: 3px; margin-right: 10px; border: none; cursor: pointer; }
        .btn:hover { background: #1565c0; }
        .refresh-btn { background: #4caf50; }
        .context-btn { background: #ff9800; }
        .context-btn:hover { background: #f57c00; }
        .save-btn { background: #4caf50; font-size: 14px; }
        .save-btn:hover { background: #45a049; }
        .save-btn:disabled { background: #666; cursor: not-allowed; }
        .no-sessions { text-align: center; color: #666; font-style: italic; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔥 VibeCoding Admin Panel</h1>
        
        <div class="stats">
            <h3>System Status</h3>
            <p id="session-count">Loading...</p>
            <button onclick="loadSessions()" class="btn refresh-btn">🔄 Refresh</button>
        </div>

        <div id="sessions-container">
            <p>Loading sessions...</p>
        </div>
    </div>

    <script>
        async function loadSessions() {
            try {
                const response = await fetch('/api/sessions');
                const data = await response.json();
                
                document.getElementById('session-count').innerHTML = 
                    ` + "`Active Sessions: ${data.total_sessions}`" + `;

                const container = document.getElementById('sessions-container');
                
                if (data.sessions.length === 0) {
                    container.innerHTML = '<div class="no-sessions">No active VibeCoding sessions</div>';
                    return;
                }

                let html = '';
                data.sessions.forEach(session => {
                    html += ` + "`" + `
                        <div class="session-card">
                            <div class="session-header">
                                👤 User ID: ${session.user_id} - ${session.project_name}
                            </div>
                            <div class="session-meta">
                                📝 Language: ${session.language} | 
                                ⏱️ Duration: ${session.duration} |
                                📁 Files: ${session.files_count} + ${session.generated_count} generated
                            </div>
                            <div class="session-meta">
                                🧪 Test Command: ${session.test_command || 'Not set'}
                            </div>
                            <div class="session-actions">
                                <a href="/vibe_${session.user_id}" class="btn" target="_blank">🌐 View Session</a>
                                <a href="http://localhost:3000?user=${session.user_id}" class="btn" target="_blank">🎨 External Interface</a>
                                <button onclick="viewContext(${session.user_id})" class="btn context-btn">📄 View Context</button>
                            </div>
                        </div>
                    ` + "`" + `;
                });
                
                container.innerHTML = html;
                
            } catch (error) {
                document.getElementById('sessions-container').innerHTML = 
                    ` + "`<div style='color: red;'>Error loading sessions: ${error.message}</div>`" + `;
            }
        }

        async function viewContext(userId) {
            try {
                const response = await fetch(` + "`/api/context/${userId}`" + `);
                if (!response.ok) {
                    throw new Error(` + "`Context not found: ${response.statusText}`" + `);
                }
                
                const contextData = await response.json();
                
                // Создаем новое окно с контекстом
                const newWindow = window.open('', '_blank', 'width=800,height=600,scrollbars=yes');
                newWindow.document.write(` + "`" + `
                    <html>
                    <head>
                        <title>VibeCoding Context - User ${userId}</title>
                        <style>
                            body { font-family: Monaco, monospace; padding: 20px; background: #1e1e1e; color: #e0e0e0; }
                            pre { background: #2a2a2a; padding: 15px; border-radius: 5px; overflow-x: auto; white-space: pre-wrap; }
                            .header { background: #ff6b35; color: white; padding: 15px; margin: -20px -20px 20px -20px; }
                            .section { margin: 20px 0; }
                            .section-title { color: #ff9800; font-weight: bold; margin-bottom: 10px; }
                        </style>
                    </head>
                    <body>
                        <div class="header">
                            <h1>🔥 VibeCoding Context - User ${userId}</h1>
                            <p>Compressed Project Context JSON</p>
                        </div>
                        <div class="section">
                            <div class="section-title">📄 Context Data:</div>
                            <pre>${JSON.stringify(contextData, null, 2)}</pre>
                        </div>
                    </body>
                    </html>
                ` + "`" + `);
                newWindow.document.close();
                
            } catch (error) {
                alert(` + "`Error loading context: ${error.message}`" + `);
            }
        }

        // Load sessions on page load
        loadSessions();
        
        // Auto-refresh every 30 seconds
        setInterval(loadSessions, 30000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(adminHTML))
}
