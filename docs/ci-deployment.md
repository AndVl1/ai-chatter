# 🚀 CI/CD Deployment Guide

## Быстрый старт

### 1. Fork репозитория
```bash
# 1. Сделайте fork на GitHub
# 2. Клонируйте свой fork
git clone https://github.com/YOUR_USERNAME/ai-chatter.git
cd ai-chatter
```

### 2. Настройка GitHub Secrets

#### Обязательные secrets для integration тестов:
```bash
# GitHub repository → Settings → Secrets and variables → Actions

NOTION_TOKEN=secret_abc123def456789...
NOTION_TEST_PAGE_ID=12345678-90ab-cdef-1234-567890abcdef
```

#### Получение NOTION_TOKEN:
1. Идите на https://developers.notion.com
2. "My integrations" → "New integration"
3. Название: "AI Chatter CI Tests"
4. Capabilities: Read content, Update content, Insert content
5. Скопируйте "Internal Integration Token"

#### Получение NOTION_TEST_PAGE_ID:
1. Создайте страницу в Notion: "AI Chatter CI Tests"
2. Share → "Connect to integration" → выберите созданную интеграцию
3. Скопируйте ID из URL: `https://notion.so/workspace/Page-Name-{THIS_IS_ID}`

### 3. Проверка CI

```bash
# Первый push активирует CI
git add .
git commit -m "Initial setup"
git push origin main

# Проверьте статус в GitHub Actions
# https://github.com/YOUR_USERNAME/ai-chatter/actions
```

## Структура CI процесса

### Автоматические запуски:

#### 📋 При каждом push/PR:
- ✅ Unit tests (быстро, ~30s)
- 🔨 Build check (все платформы)
- 📊 Code coverage

#### 🌟 При push в main/develop:
- ✅ Unit tests
- 🌐 Integration tests (с реальным Notion API)
- 🔄 Cross-platform builds
- 📈 Coverage upload

#### 🌙 Каждую ночь (02:00 UTC):
- 🧪 Полная test suite
- ⚡ Performance benchmarks
- 📊 Trend analysis
- 🔍 Regression detection

#### 🏷️ При релизных тагах:
- ⚡ Performance profiling
- 🧠 Memory analysis
- 🚀 Regression check

### Workflow файлы:

1. **`.github/workflows/ci.yml`** - основной CI
2. **`.github/workflows/nightly-integration.yml`** - ночные тесты
3. **`.github/workflows/performance.yml`** - performance при релизах

## Локальная разработка

### Быстрая проверка перед push:
```bash
# Полный local CI
./scripts/ci-local.sh

# Или отдельные этапы:
./scripts/ci-local.sh test        # Unit тесты
./scripts/ci-local.sh build       # Сборка
./scripts/ci-local.sh integration # Integration (нужны secrets)
```

### Установка secrets локально:
```bash
# В .env файле или export
export NOTION_TOKEN=secret_your_token
export NOTION_TEST_PAGE_ID=your_page_id

# Проверка integration тестов
./scripts/test-notion-integration.sh
```

## Мониторинг и алерты

### GitHub Checks:
- ✅ **Обязательные**: Unit tests должны пройти для merge
- ⚠️ **Рекомендуемые**: Integration tests (при наличии secrets)
- 📊 **Информационные**: Coverage, performance

### Status badges:
```markdown
# В README.md автоматически показывают:
[![CI](https://github.com/YOUR_USERNAME/ai-chatter/actions/workflows/ci.yml/badge.svg)](...)
[![codecov](https://codecov.io/gh/YOUR_USERNAME/ai-chatter/branch/main/graph/badge.svg)](...)
```

### Что отслеживать:
- 🔴 **Critical**: Сбой CI на main ветке
- 🟠 **Warning**: Coverage ниже 75%
- 🟡 **Info**: Медленные тесты (>5 минут)

## Customization

### Изменение CI настроек:

#### Изменить версии Go:
```yaml
# В .github/workflows/ci.yml
strategy:
  matrix:
    go-version: [ '1.21.x', '1.22.x' ]  # Добавьте нужные версии
```

#### Изменить coverage threshold:
```bash
# В scripts/ci-local.sh
COVERAGE_THRESHOLD=80  # Увеличьте до нужного уровня
```

#### Добавить новые платформы:
```yaml
# В cross-platform job
matrix:
  os: [ubuntu-latest, windows-latest, macos-latest, macos-14]  # ARM64 Mac
```

#### Настроить Slack уведомления:
```yaml
# Добавьте в конец любого job:
- name: Notify Slack
  if: failure()
  env:
    SLACK_WEBHOOK: ${{ secrets.SLACK_WEBHOOK }}
  run: |
    curl -X POST -H 'Content-type: application/json' \
      --data '{"text":"❌ CI failed for ai-chatter"}' \
      $SLACK_WEBHOOK
```

### Кастомные тесты:

#### Добавить benchmark тесты:
```go
// internal/notion/benchmark_test.go
func BenchmarkMCPConnection(b *testing.B) {
    token := os.Getenv("NOTION_TOKEN")
    if token == "" {
        b.Skip("NOTION_TOKEN not set")
    }
    
    for i := 0; i < b.N; i++ {
        client := NewMCPClient(token)
        client.Connect(context.Background(), token)
        client.Close()
    }
}
```

#### Добавить end-to-end тесты:
```go
// e2e/telegram_test.go (новый пакет)
func TestTelegramBot(t *testing.T) {
    // Тестирование полного flow через Telegram API
}
```

## Troubleshooting

### Частые проблемы:

#### 1. "NOTION_TOKEN not set"
```bash
# Проверьте secrets в GitHub:
# Settings → Secrets and variables → Actions
# Убедитесь что secret называется точно NOTION_TOKEN
```

#### 2. Integration тесты skip
```bash
# Это нормально если secrets не настроены
# CI будет проходить с warning:
# "⚠️ Skipping integration tests - environment not configured"
```

#### 3. Coverage fails
```bash
# Если coverage ниже threshold, CI проходит но с warning
# Добавьте тесты или снизьте threshold в ci-local.sh
```

#### 4. Cross-platform build fails
```bash
# Обычно из-за platform-specific кода
# Используйте build tags: // +build linux
# Или проверки: if runtime.GOOS == "linux"
```

#### 5. MCP server не запускается
```bash
# В GitHub Actions логах ищите:
# "❌ Failed to start MCP server"
# Проверьте зависимости и сборку notion-mcp-server
```

### Debug CI:

#### Локальная имитация GitHub Actions:
```bash
# Установите act (https://github.com/nektos/act)
brew install act

# Запустите GitHub Actions локально
act -j unit-tests
act -j integration-tests --secret NOTION_TOKEN=your_token
```

#### Добавить debug логи в CI:
```yaml
- name: Debug environment
  run: |
    echo "Go version: $(go version)"
    echo "Working directory: $(pwd)"
    echo "Environment variables:"
    env | grep -E "(NOTION|GO)" | sort
```

## Production Deployment

### Release process:
```bash
# 1. Убедитесь что все тесты проходят
./scripts/ci-local.sh

# 2. Создайте release tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# 3. GitHub Actions автоматически:
#    - Запустит performance тесты
#    - Создаст release artifacts
#    - Проверит regression
```

### Автоматический deployment:
```yaml
# Добавьте в .github/workflows/ci.yml
deploy:
  runs-on: ubuntu-latest
  needs: [unit-tests, integration-tests]
  if: github.ref == 'refs/heads/main'
  
  steps:
    - name: Deploy to production
      run: |
        # Ваша логика deployment
        echo "Deploying to production..."
```

## Best Practices

### 1. **Безопасность**
- ✅ Используйте GitHub secrets для токенов
- ✅ Никогда не логируйте secret values
- ✅ Ограничивайте permissions workflows
- ✅ Регулярно ротируйте токены

### 2. **Performance**
- ✅ Кэшируйте Go modules
- ✅ Параллелизируйте независимые jobs
- ✅ Используйте matrix для multiple versions
- ✅ Fail-fast для экономии ресурсов

### 3. **Reliability**
- ✅ Graceful handling отсутствующих secrets
- ✅ Timeout для long-running тестов
- ✅ Retry для flaky external API calls
- ✅ Comprehensive error reporting

### 4. **Maintainability**
- ✅ Документируйте custom workflows
- ✅ Версионируйте GitHub Actions
- ✅ Модульная структура CI скриптов
- ✅ Regular updates зависимостей

---

**🎯 Результат**: После настройки у вас будет полностью автоматизированный CI/CD процесс, который гарантирует качество кода и надёжность интеграции с Notion!
