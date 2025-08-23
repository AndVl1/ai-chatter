const express = require('express');
const cors = require('cors');
const path = require('path');

// Add fetch for Node.js
global.fetch = require('node-fetch');

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(cors());
app.use(express.json());
app.use(express.static(path.join(__dirname, 'web')));

// VibeCoding API Client для коммуникации с внутренним сервером
class VibeCodingAPIClient {
    constructor() {
        // Используем localhost для локального тестирования, host.docker.internal для Docker
        this.baseURL = process.env.VIBECODING_API_URL || (process.env.NODE_ENV === 'production' ? 'http://host.docker.internal:8080' : 'http://localhost:8080');
        this.isConnected = false;
    }

    // Проверка доступности внутреннего API
    async checkConnection() {
        try {
            const response = await fetch(`${this.baseURL}/`, {
                method: 'GET',
                timeout: 5000
            });
            
            this.isConnected = response.ok;
            if (this.isConnected) {
                console.log('✅ Connected to VibeCoding internal API');
            } else {
                console.log('⚠️ VibeCoding internal API responded with error');
            }
            return this.isConnected;
        } catch (error) {
            console.log('❌ VibeCoding internal API not available:', error.message);
            this.isConnected = false;
            return false;
        }
    }

    // Получить информацию о сессии
    async getSessionInfo(userId) {
        try {
            const response = await fetch(`${this.baseURL}/api/vibe_${userId}`, {
                method: 'GET',
                timeout: 10000
            });
            
            if (!response.ok) {
                throw new Error(`Session not found: ${response.status}`);
            }
            
            const data = await response.json();
            return { success: true, data };
        } catch (error) {
            console.error(`Failed to get session info for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Получить список файлов
    async getFiles(userId) {
        try {
            const sessionInfo = await this.getSessionInfo(userId);
            if (!sessionInfo.success) {
                throw new Error(sessionInfo.error);
            }

            const files = [];
            // sessionInfo.data содержит SessionData напрямую, не session wrapper
            const sessionData = sessionInfo.data;
            
            // SessionData не содержит файлы напрямую, получаем через files_tree
            if (sessionData.files_tree) {
                this.extractFilesFromTree(sessionData.files_tree, files);
            }
            
            return { success: true, files };
        } catch (error) {
            console.error(`Failed to get files for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Извлекаем файлы из дерева файлов
    extractFilesFromTree(node, files) {
        if (node.type === 'file') {
            files.push(node.path);
        }
        
        if (node.children) {
            node.children.forEach(child => {
                this.extractFilesFromTree(child, files);
            });
        }
    }

    // Получить список файлов (альтернативный метод)
    async getFilesAlternative(userId) {
        try {
            const sessionInfo = await this.getSessionInfo(userId);
            if (!sessionInfo.success) {
                throw new Error(sessionInfo.error);
            }

            const files = [];
            // Получаем SessionData напрямую
            const sessionData = sessionInfo.data;
            
            // Добавляем заглушку для обычных файлов (SessionData не содержит файлы напрямую)
            // В будущем можно использовать отдельный API endpoint для получения списка файлов
            if (sessionData.files_tree) {
                this.extractFilesFromTree(sessionData.files_tree, files);
            }
            
            // Добавляем сгенерированные файлы (если есть в дереве)
            // В SessionData структуре сгенерированные файлы помечены префиксом "[generated]"
            
            return { success: true, files, totalFiles: files.length };
        } catch (error) {
            console.error(`Failed to get files for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Прочитать файл
    async readFile(userId, filename) {
        try {
            // Убираем "[generated] " префикс и "(generated)" суффикс если есть
            let cleanFilename = filename.replace(/^\[generated\]\s+/, '').replace(' (generated)', '');
            
            const response = await fetch(`${this.baseURL}/api/vibe_${userId}/file/${encodeURIComponent(cleanFilename)}`, {
                method: 'GET',
                timeout: 10000
            });
            
            if (!response.ok) {
                throw new Error(`File not found: ${response.status}`);
            }
            
            const content = await response.text();
            return { success: true, content, filename: cleanFilename, size: content.length };
        } catch (error) {
            console.error(`Failed to read file ${filename} for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Записать файл (пока через session info API)
    async writeFile(userId, filename, content, generated = false) {
        try {
            // Для записи файлов нужно использовать internal API сессии
            // Пока возвращаем заглушку
            console.log(`📝 Writing file ${filename} for user ${userId} (${content.length} bytes)`);
            return { 
                success: true, 
                message: `File ${filename} would be written (${content.length} bytes)`,
                filename,
                size: content.length 
            };
        } catch (error) {
            console.error(`Failed to write file ${filename} for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Выполнить команду (пока заглушка)
    async executeCommand(userId, command) {
        try {
            console.log(`⚡ Executing command "${command}" for user ${userId}`);
            return {
                success: true,
                output: `Mock execution of: ${command}\nCommand would be executed in VibeCoding session.`,
                exitCode: 0,
                message: `Command "${command}" executed (mock)`
            };
        } catch (error) {
            console.error(`Failed to execute command for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }

    // Запустить тесты (пока заглушка)
    async runTests(userId, testFile = '') {
        try {
            console.log(`🧪 Running tests for user ${userId}`);
            return {
                success: true,
                output: `Mock test execution for user ${userId}\nTests would be executed in VibeCoding session.`,
                testCommand: 'pytest' || 'npm test' || 'go test',
                exitCode: 0
            };
        } catch (error) {
            console.error(`Failed to run tests for user ${userId}:`, error);
            return { success: false, error: error.message };
        }
    }
}

// Создаем глобальный API клиент
const apiClient = new VibeCodingAPIClient();

// API endpoints для взаимодействия с VibeCoding через внутренний API

// Получить список файлов
app.get('/api/files/:userId', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        console.log(`📁 Getting files for user ${userId}`);

        const result = await apiClient.getFiles(userId);
        
        if (result.success) {
            res.json({
                success: true,
                files: result.files,
                totalFiles: result.totalFiles,
                message: 'Files retrieved successfully'
            });
        } else {
            res.status(500).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error getting files:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Прочитать файл
app.get('/api/files/:userId/:filename', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        const filename = req.params.filename;
        console.log(`📄 Reading file ${filename} for user ${userId}`);

        const result = await apiClient.readFile(userId, filename);
        
        if (result.success) {
            res.json({
                success: true,
                content: result.content,
                filename: result.filename,
                size: result.size
            });
        } else {
            res.status(404).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error reading file:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Записать файл
app.post('/api/files/:userId/:filename', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        const filename = req.params.filename;
        const { content, generated = false } = req.body;
        
        console.log(`✏️ Writing file ${filename} for user ${userId}`);

        const result = await apiClient.writeFile(userId, filename, content, generated);
        
        if (result.success) {
            res.json({
                success: true,
                message: result.message,
                filename: result.filename,
                size: result.size
            });
        } else {
            res.status(500).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error writing file:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Выполнить команду
app.post('/api/execute/:userId', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        const { command } = req.body;
        
        console.log(`⚡ Executing command for user ${userId}: ${command}`);

        const result = await apiClient.executeCommand(userId, command);
        
        if (result.success !== false) {
            res.json({
                success: result.success,
                output: result.output,
                exitCode: result.exitCode,
                message: result.message
            });
        } else {
            res.status(500).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error executing command:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Запустить тесты
app.post('/api/test/:userId', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        const { testFile } = req.body;
        
        console.log(`🧪 Running tests for user ${userId}`);

        const result = await apiClient.runTests(userId, testFile || '');
        
        if (result.success !== false) {
            res.json({
                success: result.success,
                output: result.output,
                testCommand: result.testCommand,
                exitCode: result.exitCode
            });
        } else {
            res.status(500).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error running tests:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Получить информацию о сессии
app.get('/api/session/:userId', async (req, res) => {
    try {
        const userId = parseInt(req.params.userId);
        console.log(`ℹ️ Getting session info for user ${userId}`);

        const result = await apiClient.getSessionInfo(userId);
        
        if (result.success) {
            // result.data содержит SessionData напрямую, не вложенную в session
            const sessionData = result.data;
            res.json({
                success: true,
                session: {
                    user_id: sessionData.user_id || userId,
                    status: 'Active', // Если сессия найдена, она активна
                    container_id: '', // SessionData не содержит container_id
                    test_command: '', // SessionData не содержит test_command
                    created_at: sessionData.start_time || new Date().toISOString(),
                    project_name: sessionData.project_name || 'Unknown',
                    language: sessionData.language || 'Unknown'
                },
                message: 'Session info retrieved'
            });
        } else {
            res.status(404).json({
                success: false,
                error: result.error
            });
        }
    } catch (error) {
        console.error('Error getting session info:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// Главная страница
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'web', 'index.html'));
});

// Статус сервера
app.get('/api/status', async (req, res) => {
    const connected = await apiClient.checkConnection();
    res.json({
        success: true,
        status: 'running',
        apiConnected: connected,
        apiURL: apiClient.baseURL,
        timestamp: new Date().toISOString()
    });
});

// Запуск сервера
async function startServer() {
    try {
        // Проверяем подключение к внутреннему API
        const connected = await apiClient.checkConnection();
        if (!connected) {
            console.log('⚠️ VibeCoding internal API not available, but starting web interface anyway');
        }
        
        app.listen(PORT, '0.0.0.0', () => {
            console.log(`🌐 VibeCoding Web Interface running on http://0.0.0.0:${PORT}`);
            console.log(`🔗 Internal API URL: ${apiClient.baseURL}`);
            console.log(`📡 API Status: ${connected ? 'Connected' : 'Disconnected'}`);
        });
    } catch (error) {
        console.error('❌ Failed to start server:', error);
        process.exit(1);
    }
}

// Graceful shutdown
process.on('SIGTERM', () => {
    console.log('🔌 Received SIGTERM, shutting down gracefully');
    process.exit(0);
});

process.on('SIGINT', () => {
    console.log('🔌 Received SIGINT, shutting down gracefully');
    process.exit(0);
});

// Запускаем сервер
startServer();