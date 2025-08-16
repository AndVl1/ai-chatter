# 🧪 Testing Guide

## Типы тестов

### 1. Unit тесты
Тестируют отдельные компоненты изолированно.

```bash
# Запуск всех unit тестов
go test ./...

# Запуск тестов конкретного пакета
go test ./internal/auth
go test ./internal/history
go test ./internal/storage
```

### 2. Integration тесты 
Тестируют взаимодействие с внешними сервисами (Notion API).

```bash
# Автоматический запуск интеграционных тестов
./scripts/test-notion-integration.sh

# Ручной запуск (требует настройки переменных)
export NOTION_TOKEN=secret_your_token
export NOTION_TEST_PAGE_ID=your-test-page-id
go test ./internal/notion -run "TestMCP" -v
```

## Настройка интеграционных тестов

### Требования
1. **Notion интеграция** - создайте в https://developers.notion.com
2. **Тестовая страница** - страница в Notion для создания подстраниц
3. **Переменные окружения** - NOTION_TOKEN и NOTION_TEST_PAGE_ID

### Пошаговая настройка

#### 1. Создание Notion интеграции
```bash
# 1. Идите на https://developers.notion.com
# 2. "My integrations" → "New integration"
# 3. Название: "AI Chatter Test"
# 4. Скопируйте "Internal Integration Token"
```

#### 2. Создание тестовой страницы
```bash
# 1. Создайте новую страницу в Notion
# 2. Назовите её "AI Chatter Integration Tests"
# 3. Share → "Connect to integration" → выберите вашу интеграцию
# 4. Скопируйте ID из URL страницы
```

#### 3. Настройка переменных
```bash
# В .env файле
NOTION_TOKEN=secret_abc123def456
NOTION_TEST_PAGE_ID=12345678-90ab-cdef-1234-567890abcdef

# Или экспорт для разового запуска
export NOTION_TOKEN=secret_abc123def456
export NOTION_TEST_PAGE_ID=12345678-90ab-cdef-1234-567890abcdef
```

### Что тестируется

#### TestMCPIntegration
- ✅ Подключение к MCP серверу
- ✅ Создание диалога (CreateDialogSummary)
- ✅ Создание произвольной страницы (CreateFreeFormPage)  
- ✅ Поиск в workspace (SearchWorkspace)
- ✅ Обработка ошибок (некорректный parent page ID)

#### TestMCPConnection
- ✅ Базовое подключение/отключение от MCP сервера
- ✅ Создание и закрытие сессии

#### TestRequiredEnvironmentVariables
- ✅ Проверка настроек окружения
- ✅ Документация необходимых переменных

### Структура тестовых данных

Тесты создают реальные страницы в Notion с timestamp суффиксами:

```
📄 AI Chatter Integration Tests (ваша тестовая страница)
  ├── 📝 Integration Test Dialog Test_2024-01-15_14-30-25
  ├── 📄 Integration Test Free Page Test_2024-01-15_14-30-25  
  └── 📄 (другие тестовые страницы)
```

**Безопасность**: Все тестовые страницы помечены и могут быть безопасно удалены.

## Continuous Integration

### GitHub Actions пример
```yaml
name: Tests
on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      - run: go test ./...

  integration-tests:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      - name: Run integration tests
        env:
          NOTION_TOKEN: ${{ secrets.NOTION_TOKEN }}
          NOTION_TEST_PAGE_ID: ${{ secrets.NOTION_TEST_PAGE_ID }}
        run: ./scripts/test-notion-integration.sh
```

### Локальный CI
```bash
#!/bin/bash
# scripts/ci-local.sh

echo "🚀 Running local CI pipeline..."

echo "1️⃣ Unit tests..."
go test ./... || exit 1

echo "2️⃣ Build check..."
go build -o ai-chatter cmd/bot/main.go || exit 1
go build -o notion-mcp-server cmd/notion-mcp-server/main.go || exit 1

echo "3️⃣ Integration tests (if configured)..."
if [ -n "$NOTION_TOKEN" ] && [ -n "$NOTION_TEST_PAGE_ID" ]; then
    ./scripts/test-notion-integration.sh || exit 1
else
    echo "⚠️  Skipping integration tests (env not configured)"
fi

echo "✅ All checks passed!"
```

## Debugging тестов

### Логи MCP сервера
```bash
# Запуск сервера с подробными логами
NOTION_TOKEN=your_token ./notion-mcp-server 2>&1 | tee mcp-server.log

# В другом терминале
go test ./internal/notion -run "TestMCPIntegration" -v
```

### Debugging конкретного теста
```bash
# Запуск одного теста с verbose
go test ./internal/notion -run "TestMCPIntegration/CreateDialogSummary" -v

# С дополнительными флагами
go test ./internal/notion -run "TestMCPIntegration" -v -count=1 -timeout=30s
```

### Распространённые проблемы

#### "MCP session not connected"
```bash
# Проверьте что сервер запущен
ps aux | grep notion-mcp-server

# Пересоберите сервер
go build -o notion-mcp-server cmd/notion-mcp-server/main.go
```

#### "integration does not have access"
```bash
# Проверьте доступ интеграции к тестовой странице
# Share → Connect to integration → выберите вашу интеграцию
```

#### "parent page not found"
```bash
# Проверьте правильность NOTION_TEST_PAGE_ID
echo $NOTION_TEST_PAGE_ID

# Убедитесь что страница существует и доступна
```

## Performance тестирование

### Скорость API вызовов
```bash
# Время создания страницы
time go test ./internal/notion -run "TestMCPIntegration/CreateFreeFormPage" -v

# Параллельные запросы
go test ./internal/notion -run "TestMCP" -v -parallel 3
```

### Memory profiling
```bash
# Профиль памяти
go test ./internal/notion -run "TestMCPIntegration" -memprofile=mem.prof

# Анализ
go tool pprof mem.prof
```

## Полезные команды

```bash
# Очистка тестовых артефактов
rm -f *.prof *.log notion-mcp-server ai-chatter

# Быстрая проверка всего
go test ./... && go build ./...

# Детальная проверка
./scripts/test-notion-integration.sh && echo "✅ All good!"

# Проверка coverage
go test ./... -cover

# Генерация coverage отчёта
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## Best Practices

1. **Изоляция**: Каждый тест должен работать независимо
2. **Cleanup**: Тестовые данные должны быть уникальными 
3. **Timeouts**: Используйте разумные timeout для внешних API
4. **Error handling**: Тестируйте error cases
5. **Documentation**: Документируйте сложные тестовые сценарии
6. **Environment**: Никогда не коммитьте реальные токены
7. **Parallel safe**: Тесты должны работать параллельно

## Мониторинг тестов

### Metrics для отслеживания
- ⏱️ Время выполнения интеграционных тестов
- 📊 Success rate вызовов Notion API
- 🔄 Frequency тестовых прогонов
- 📈 Coverage код базы

### Алерты
- 🚨 Failure интеграционных тестов > 2 раз подряд
- ⚠️ Замедление API responses > 5 секунд
- 📉 Coverage падение > 5%

---

**💡 Совет**: Регулярно запускайте интеграционные тесты перед релизами, чтобы убедиться что интеграция с Notion работает корректно!
