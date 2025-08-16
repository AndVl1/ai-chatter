# 🤝 Contributing to AI Chatter

Спасибо за интерес к проекту! Мы приветствуем все виды вклада - от исправления багов до добавления новых фич.

## 🚀 Быстрый старт

### Локальная разработка

```bash
# 1. Fork и клонирование
git clone https://github.com/YOUR_USERNAME/ai-chatter.git
cd ai-chatter

# 2. Установка зависимостей
make deps

# 3. Быстрая проверка
make ci-fast
```

### Структура проекта

```
ai-chatter/
├── cmd/                    # Точки входа (main пакеты)
│   ├── bot/               # Основной Telegram бот
│   ├── notion-mcp-server/ # Custom MCP сервер для Notion
│   └── test-custom-mcp/   # Тестовый клиент
├── internal/              # Внутренние пакеты
│   ├── auth/             # Авторизация и white-listing
│   ├── config/           # Конфигурация
│   ├── history/          # История диалогов
│   ├── llm/              # LLM провайдеры (OpenAI, YandexGPT)
│   ├── notion/           # Notion MCP интеграция
│   ├── pending/          # Pending users управление
│   ├── storage/          # Файловое хранилище
│   └── telegram/         # Telegram Bot API
├── scripts/              # Utility скрипты
├── docs/                 # Документация
└── .github/workflows/    # CI/CD
```

## 📋 Workflow разработки

### 1. Создание фичи

```bash
# Создайте feature branch
git checkout -b feature/awesome-feature

# Разрабатывайте с проверками
make ci-fast  # Быстрая проверка
make test     # Unit тесты
make format   # Форматирование
```

### 2. Тестирование

```bash
# Unit тесты
make test

# Integration тесты (нужны Notion secrets)
export NOTION_TOKEN=your_token
export NOTION_TEST_PAGE_ID=your_page_id
make integration

# Полный CI pipeline
make ci
```

### 3. Перед commit

```bash
# Автоматические проверки
make ci-fast

# Если всё ОК:
git add .
git commit -m "feat: add awesome feature"
git push origin feature/awesome-feature
```

### 4. Pull Request

- Создайте PR в основной репозиторий
- CI автоматически проверит ваши изменения
- Опишите что и зачем изменено
- Приложите тесты для новой функциональности

## 🧪 Тестирование

### Типы тестов

#### Unit Tests
```bash
# Быстрые изолированные тесты
go test ./internal/auth
go test ./internal/history
```

#### Integration Tests
```bash
# Тесты с реальным Notion API
./scripts/test-notion-integration.sh
```

#### Performance Tests
```bash
# Бенчмарки и профилирование
make benchmark
make profile-cpu
make profile-mem
```

### Добавление тестов

#### Unit тесты
```go
// internal/mypackage/myfile_test.go
func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test input"
    expected := "expected output"
    
    // Act
    result := MyFunction(input)
    
    // Assert
    if result != expected {
        t.Errorf("Expected %s, got %s", expected, result)
    }
}
```

#### Integration тесты
```go
// internal/mypackage/integration_test.go
func TestMyIntegration(t *testing.T) {
    token := os.Getenv("API_TOKEN")
    if token == "" {
        t.Skip("API_TOKEN not set, skipping integration test")
    }
    
    // Test with real API...
}
```

#### Benchmark тесты
```go
// internal/mypackage/benchmark_test.go
func BenchmarkMyFunction(b *testing.B) {
    for i := 0; i < b.N; i++ {
        MyFunction("test input")
    }
}
```

## 📝 Code Style

### Go стандарты
- Следуйте [Effective Go](https://golang.org/doc/effective_go.html)
- Используйте `go fmt` для форматирования
- Добавляйте комментарии к публичным функциям
- Обрабатывайте ошибки явно

### Naming conventions
```go
// ✅ Хорошо
func CreateNotionPage(title, content string) error
type MCPClient struct { ... }
var ErrInvalidToken = errors.New("invalid token")

// ❌ Плохо  
func createPage(t, c string) error
type mcpClient struct { ... }
var invalidToken = errors.New("invalid token")
```

### Error handling
```go
// ✅ Хорошо
result, err := client.CreatePage(title, content)
if err != nil {
    return fmt.Errorf("failed to create page: %w", err)
}

// ❌ Плохо
result, _ := client.CreatePage(title, content)
```

### Logging
```go
// ✅ Хорошо - структурированное логирование
log.Printf("📝 Creating page: %s", title)
log.Printf("❌ Failed to connect: %v", err)

// ❌ Плохо - неинформативно
log.Println("Creating page")
log.Println("Error:", err)
```

## 🔧 Архитектурные принципы

### Dependency Injection
```go
// ✅ Хорошо - интерфейсы и DI
type NotionClient interface {
    CreatePage(title, content string) error
}

func NewBot(notionClient NotionClient) *Bot {
    return &Bot{notion: notionClient}
}

// ❌ Плохо - прямые зависимости
func NewBot() *Bot {
    notionClient := notion.NewClient() // Жёсткая связь
    return &Bot{notion: notionClient}
}
```

### Error wrapping
```go
// ✅ Хорошо - контекстные ошибки
func (c *Client) CreatePage(title string) error {
    if title == "" {
        return fmt.Errorf("title cannot be empty")
    }
    
    err := c.api.Create(title)
    if err != nil {
        return fmt.Errorf("failed to create page %q: %w", title, err)
    }
    
    return nil
}
```

### Configuration
```go
// ✅ Хорошо - структурированная конфигурация
type Config struct {
    NotionToken      string `env:"NOTION_TOKEN"`
    NotionParentPage string `env:"NOTION_PARENT_PAGE_ID"`
}

// ❌ Плохо - прямые os.Getenv вызовы в коде
token := os.Getenv("NOTION_TOKEN")
```

## 🎯 Типичные задачи

### Добавление нового LLM провайдера

1. **Создайте клиент** в `internal/llm/`:
```go
// internal/llm/myprovider.go
type MyProviderClient struct {
    apiKey string
    model  string
}

func (c *MyProviderClient) Generate(ctx context.Context, messages []Message) (Response, error) {
    // Реализация
}
```

2. **Обновите фабрику** в `internal/llm/factory.go`:
```go
case "myprovider":
    return NewMyProvider(apiKey, model), nil
```

3. **Добавьте тесты**:
```go
// internal/llm/myprovider_test.go
func TestMyProvider(t *testing.T) { ... }
```

### Добавление новой Notion функции

1. **Расширьте MCP сервер** в `cmd/notion-mcp-server/main.go`:
```go
func (s *NotionMCPServer) MyNewFunction(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[MyParams]) (*mcp.CallToolResultFor[any], error) {
    // Реализация
}
```

2. **Добавьте в клиент** в `internal/notion/mcp.go`:
```go
func (m *MCPClient) MyNewFunction(ctx context.Context, ...) MCPResult {
    // Реализация
}
```

3. **Обновите tools** в `internal/llm/tools.go`:
```go
{
    Type: "function",
    Function: Function{
        Name: "my_new_function",
        Description: "...",
        Parameters: ...
    },
}
```

4. **Добавьте integration тесты**:
```go
// internal/notion/mcp_integration_test.go
t.Run("MyNewFunction", func(t *testing.T) { ... })
```

### Обновление CI/CD

1. **Локальные изменения** в `scripts/ci-local.sh`
2. **GitHub Actions** в `.github/workflows/`
3. **Makefile команды** для удобства разработчиков

## 🐛 Отладка и troubleshooting

### Логи и debug
```bash
# Включить подробные логи
export DEBUG=1

# MCP сервер логи
NOTION_TOKEN=your_token ./notion-mcp-server 2>&1 | tee mcp.log

# Bot логи  
./ai-chatter 2>&1 | tee bot.log
```

### Performance profiling
```bash
# CPU profiling
make profile-cpu
go tool pprof cpu.prof

# Memory profiling
make profile-mem  
go tool pprof mem.prof
```

### Integration тесты
```bash
# Debug конкретного теста
go test ./internal/notion -run "TestMCPIntegration/CreateDialogSummary" -v

# С timeout
go test ./internal/notion -run "TestMCPIntegration" -v -timeout=30s
```

## 📚 Документация

### Добавление документации
- **API изменения** → обновить `docs/`
- **Новые фичи** → добавить в `CHANGELOG.md`
- **Breaking changes** → описать migration guide

### Комментарии в коде
```go
// CreateNotionPage создаёт новую страницу в Notion с указанным содержимым.
// Возвращает ID созданной страницы или ошибку если создание не удалось.
//
// title - заголовок страницы (обязательно)
// content - содержимое в Markdown формате
// parentPageID - ID родительской страницы
func CreateNotionPage(title, content, parentPageID string) (string, error) {
    // ...
}
```

## 🚀 Release процесс

### Подготовка релиза
```bash
# 1. Убедитесь что все тесты проходят
make ci

# 2. Обновите CHANGELOG.md с новыми изменениями
# 3. Создайте release branch
git checkout -b release/v1.x.x

# 4. Создайте tag
git tag -a v1.x.x -m "Release v1.x.x"
git push origin v1.x.x
```

### Что происходит автоматически
- 🧪 Performance тесты запускаются при release tags
- 🔍 Regression анализ
- 📊 Artifacts создаются для релиза

## ❓ Вопросы и помощь

### Где получить помощь
- 📋 **Issues** - для багов и feature requests
- 💬 **Discussions** - для общих вопросов
- 📖 **Docs** - документация в `docs/`

### Сообщение о багах
Пожалуйста, включите:
- 🔍 Шаги для воспроизведения
- 💻 Версия Go и ОС
- 📋 Логи (без секретных данных!)
- 🎯 Ожидаемое vs фактическое поведение

### Feature requests
- 🎯 Описание желаемой функциональности
- 💡 Use cases и примеры
- 🔄 Готовность помочь с реализацией

---

**🙏 Спасибо за ваш вклад в AI Chatter!** Каждое улучшение делает проект лучше для всех.
