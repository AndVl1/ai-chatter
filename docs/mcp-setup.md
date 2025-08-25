# MCP Серверы - Настройка и Использование

## Обзор

AI Chatter использует архитектуру MCP (Model Context Protocol) для интеграции с внешними сервисами. Каждый сервис работает как отдельный MCP сервер, обеспечивая модульность и безопасность.

## Доступные MCP Интеграции

### 📋 Notion MCP
**Функции:** Создание заметок, поиск контента
- **Команды:** `/notion_save`, `/notion_search`
- **Требуется:** `NOTION_TOKEN`, `NOTION_PARENT_PAGE_ID`
- **Бинарный файл:** `./bin/notion-mcp-server`

### 📧 Gmail MCP  
**Функции:** Поиск писем, анализ и обобщение
- **Команды:** `/gmail_summary`
- **Требуется:** `GMAIL_CREDENTIALS_JSON` или `GMAIL_CREDENTIALS_JSON_PATH`
- **Бинарный файл:** `./bin/gmail-mcp-server`

### 📦 GitHub MCP
**Функции:** Работа с релизами, скачивание Android файлов (AAB/APK)
- **Команды:** `/release_rc` (поиск pre-release релизов)
- **Поддерживаемые форматы:** AAB (предпочтительно), APK (fallback)
- **Требуется:** `GITHUB_TOKEN`
- **Бинарный файл:** `./bin/github-mcp-server`

### 🏪 RuStore MCP
**Функции:** Публикация приложений в RuStore
- **Команды:** `/release_rc` (создание черновиков, загрузка AAB/APK)
- **Поддерживаемые форматы:** AAB, APK
- **Требуется:** `RUSTORE_COMPANY_ID`, `RUSTORE_KEY_ID`, `RUSTORE_KEY_SECRET`
- **Бинарный файл:** `./bin/rustore-mcp-server`

### 🔥 VibeCoding MCP
**Функции:** Анализ кода, интерактивная разработка
- **Команды:** Автоматически при загрузке архивов
- **Бинарные файлы:** `./bin/vibecoding-mcp-server`, `./bin/vibecoding-mcp-http-server`

## Быстрый Старт

### 1. Сборка MCP Серверов
```bash
# Отдельный скрипт сборки
./scripts/build-mcp-servers.sh

# Или через Makefile
make mcp-servers

# Или полная сборка
make build
```

### 2. Настройка .env
```bash
# Скопируйте пример конфигурации
cp env.example .env

# Отредактируйте необходимые токены
# Минимум для работы бота:
TELEGRAM_BOT_TOKEN=your_bot_token

# Для GitHub/RuStore интеграции (/release_rc):
GITHUB_TOKEN=ghp_your_github_token
RUSTORE_COMPANY_ID=your_company_id
RUSTORE_KEY_ID=your_key_id  
RUSTORE_KEY_SECRET=your_key_secret

# Для других интеграций:
NOTION_TOKEN=secret_your_notion_token
GMAIL_CREDENTIALS_JSON={"installed":{...}}
```

### 3. Запуск
```bash
# Полная система с автоматической сборкой MCP
./start-ai-chatter.sh

# Только основной бот
./start-ai-chatter.sh basic

# С принудительной пересборкой
./start-ai-chatter.sh full --clean
```

## Диагностика

### Проверка статуса MCP серверов
При запуске `./start-ai-chatter.sh` вы увидите статус каждой интеграции:
```
🔌 MCP Интеграции:
   📋 Notion: ✅ Готов (создание заметок, поиск)
   📧 Gmail: ⚠️ Не настроен
   📦 GitHub: ✅ Готов (релизы, скачивание AAB)
   🏪 RuStore: ✅ Готов (публикация приложений)
```

### Типичные ошибки

#### "no such file or directory"
```
⚠️ Failed to connect to GitHub MCP server: fork/exec ./github-mcp-server: no such file or directory
```
**Решение:** Соберите MCP серверы:
```bash
./scripts/build-mcp-servers.sh
```

#### "authentication failed"
```
❌ GitHub API error 401: Bad credentials
```
**Решение:** Проверьте `GITHUB_TOKEN` в `.env`

#### "permission denied" 
```
Permission denied
```
**Решение:** Сделайте скрипты исполняемыми:
```bash
chmod +x scripts/build-mcp-servers.sh
chmod +x start-ai-chatter.sh
```

## Кастомные Пути

Если нужно использовать кастомные пути к MCP серверам:

```bash
# В .env файле:
GITHUB_MCP_SERVER_PATH=/custom/path/to/github-mcp-server
RUSTORE_MCP_SERVER_PATH=/custom/path/to/rustore-mcp-server
GMAIL_MCP_SERVER_PATH=/custom/path/to/gmail-mcp-server
NOTION_MCP_SERVER_PATH=/custom/path/to/notion-mcp-server
```

## Команды Управления

```bash
# Сборка всех MCP серверов
make mcp-servers

# Очистка собранных серверов
make mcp-clean

# Полная очистка
make clean

# Запуск тестов
make test

# Проверка форматирования
make format
```

## Release Candidate Workflow

### AI-Powered Release Workflow (Рекомендуется) 🤖

Команда `/ai_release` запускает полностью автоматизированный процесс создания релиза с ИИ:

1. **🧠 AI Анализ данных** из GitHub:
   - Автоматический поиск последнего pre-release
   - Анализ коммитов с последнего стабильного релиза
   - ИИ генерирует описание ключевых изменений
   - Умное обнаружение Android файлов (AAB/APK)

2. **📝 Интерактивный сбор данных**:
   - ИИ предлагает варианты описания "Что нового"
   - Пошаговое заполнение RuStore параметров
   - Валидация каждого ответа с переспросом при ошибках
   - Поддержка пропуска опциональных полей

3. **✅ Умная валидация**:
   - Проверка форматов (числа, URL, обязательные поля)
   - Предложения исправлений при ошибках
   - Скрытие секретных данных в логах

4. **🎯 Финализация**:
   - Автоматическое заполнение финального JSON
   - Готовность к публикации в RuStore

**Преимущества AI Release:**
- 🚀 Полная автоматизация сбора данных
- 🧠 ИИ генерирует качественные описания
- 🔄 Умная обработка ошибок
- 📊 Визуальный прогресс

### Manual Release Workflow

Команда `/release_rc` предоставляет ручное управление процессом:

1. **Поиск последнего pre-release** в GitHub (AndVl1/SnakeGame)
2. **Умное обнаружение Android файла** из релиза:
   - 🎯 **AAB** (предпочтительно) - для оптимальной загрузки
   - 📱 **APK** (fallback) - если AAB не найден
3. **Интерактивный запрос** параметров RuStore
4. **Создание черновика** версии в RuStore
5. **Загрузка Android файла** (AAB или APK)
6. **Отправка на модерацию**

**Преимущества fallback:**
- ✅ Поддержка проектов с APK релизами
- ✅ Автоматическое определение типа файла
- ✅ Уведомление пользователя о типе загружаемого файла

Требует настройки как GitHub, так и RuStore интеграций.

## Помощь

- **Общая справка:** `./start-ai-chatter.sh help`
- **Логи системы:** `docker-compose -f docker-compose.full.yml logs -f`
- **Статус сервисов:** `docker-compose -f docker-compose.full.yml ps`
