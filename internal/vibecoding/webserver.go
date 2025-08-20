package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	}
}

// Start запускает веб-сервер
func (ws *WebServer) Start() error {
	mux := http.NewServeMux()

	// Регистрируем обработчики
	mux.HandleFunc("/static/", ws.handleStatic)    // Статические файлы
	mux.HandleFunc("/api/vibe_", ws.handleVibeAPI) // API для vibe сессий
	mux.HandleFunc("/vibe_", ws.handleVibeSession) // HTML страницы vibe сессий
	mux.HandleFunc("/", ws.handleRoot)             // Корневой обработчик (должен быть последним)

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
	if len(parts) == 0 {
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

	// Ищем файл в основных файлах
	if content, exists := session.Files[filePath]; exists {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
		return
	}

	// Ищем в сгенерированных файлах
	if content, exists := session.GeneratedFiles[filePath]; exists {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
		return
	}

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
                    </div>
                    <pre id="file-content" class="file-content">No file selected</pre>
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
            contentElement.textContent = content;
        })
        .catch(error => {
            contentElement.textContent = 'Error loading file: ' + error.message;
            console.error('Error loading file:', error);
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

	// Проверяем, не запрос ли это к vibe_ сессии
	if strings.HasPrefix(r.URL.Path, "/vibe_") {
		ws.handleVibeSession(w, r)
		return
	}

	// Для всех остальных путей возвращаем 404
	http.NotFound(w, r)
}
