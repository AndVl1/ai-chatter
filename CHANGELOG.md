# Changelog

All notable changes to this project will be documented in this file.

## [Day 5 - MCP Integration]

### Added
- **Notion MCP Integration**: Полная интеграция с Notion через MCP (Model Control Protocol)
  - Новый модуль `internal/notion/mcp.go` с клиентом для работы с Notion API
  - Поддержка создания страниц, поиска и работы с workspace
  - Конфигурация через переменную окружения `NOTION_TOKEN`
- **Новые команды бота**:
  - `/notion_save <название>` - сохранение текущего диалога в Notion
  - `/notion_search <запрос>` - поиск сохранённых диалогов в Notion
- **Расширенная архитектура бота**:
  - Интеграция MCPClient в структуру Bot
  - Обновлены конструкторы и инициализация
- **Обновлённый системный промпт**: добавлена информация о командах Notion
- **Документация**: обновлены примеры конфигурации в `env.example`

### Technical Details
- MCPClient работает напрямую с Notion REST API v1
- Поддержка создания страниц под родительскими страницами или в workspace
- Graceful degradation при отсутствии Notion токена
- Логирование всех операций с Notion
- Структурированные типы для результатов MCP операций

## [Day 5.2 - Function Calling Integration]

### Added
- **Автоматическое Function Calling**: LLM теперь может сама определять когда нужно работать с Notion
  - Расширен интерфейс `llm.Client` с методом `GenerateWithTools()`
  - Добавлены структуры для поддержки OpenAI function calling
  - Новые типы: `ToolCall`, `FunctionCall`, `Function`, `Tool`
- **Автоматические функции Notion для LLM**:
  - `save_dialog_to_notion` - автоматическое сохранение диалогов
  - `search_notion` - поиск в ранее сохранённых беседах  
  - `create_notion_page` - создание произвольных страниц
- **Умная обработка ответов**: бот автоматически выполняет function calls

## [Day 5.3 - Production-Ready Notion Integration]

### Improved
- **Улучшенная Notion интеграция** вместо нестабильных MCP SDK:
  - Прямая работа с Notion REST API v1 (более надёжно)
  - Улучшенное создание блоков из markdown содержимого
  - Поддержка заголовков (h1, h2, h3), параграфов и разделителей
  - Автоматический поиск родительских страниц
  - Более детальное логирование операций
- **Структурированные результаты поиска**: форматированный вывод с URL и заголовками
- **Graceful degradation**: работа без токена или при ошибках API
- **HTTP timeout**: таймаут 30 секунд для запросов к Notion
- **Расширенные метаданные**: сохранение информации о пользователе, времени создания и типе контента

### Removed
- Попытки интеграции с нестабильными MCP SDK (modelcontextprotocol/go-sdk, llmcontext/gomcp)
- Экспериментальный MCP сервер (пока API не стабилизируются)

### Notes
- **Почему не MCP**: Официальный MCP SDK ещё в разработке (v0.2.0, unstable), неофициальные SDK имеют несовместимые API
- **Когда MCP**: Когда SDK стабилизируются (планируется август 2025), можно будет легко мигрировать благодаря абстракции MCPClient

## [Day 5.4 - Official Notion MCP Integration]

### Added
- **Официальный Notion MCP клиент**: Подключение к серверу `https://mcp.notion.com/mcp`
  - HTTP клиент с поддержкой JSON-RPC 2.0 протокола
  - Автоматическая инициализация MCP сессии
  - Вызов инструментов: `create_page`, `search`, `tools/list`
  - Поддержка версии протокола 2024-11-05
- **OAuth интеграция**: Официальная авторизация через Notion (безопаснее токенов)
- **Полная документация**: `docs/notion-mcp-setup.md` с инструкциями по настройке
- **Логирование MCP**: Детальные логи всех JSON-RPC запросов и ответов
- **Graceful fallback**: Работа через REST API если MCP недоступен

### Improved
- **Лучшая безопасность**: OAuth авторизация вместо raw токенов
- **Официальная поддержка**: Использование сервера от команды Notion
- **Автоматические обновления**: Сервер обновляется командой Notion
- **Упрощённая настройка**: Подключение через Notion app одним кликом

### Technical Details
- **Протокол**: JSON-RPC 2.0 over HTTPS
- **Эндпоинт**: `https://mcp.notion.com/mcp` (Streamable HTTP)
- **Альтернативы**: SSE (`/sse`) и STDIO прокси
- **Версия MCP**: 2024-11-05 (актуальная)
- **Таймаут**: 30 секунд для HTTP запросов
- **Структуры**: `MCPRequest`, `MCPResponse`, `MCPError` для типизации

### Benefits over direct API
- ✅ **OAuth авторизация** vs ручные токены
- ✅ **Официальная поддержка** vs самостоятельная реализация  
- ✅ **Автоматические обновления** vs ручное сопровождение
- ✅ **Стандартизированный протокол** vs custom решения
- ✅ **Лучшая безопасность** vs управление токенами

## [Day 5.5 - Local Docker MCP Solution]

### Added
- **🐳 Docker Notion MCP сервер**: Локальный запуск официального `mcp/notion:latest`
  - Простая команда: `./scripts/start-notion-mcp.sh`
  - Docker Compose конфигурация для production
  - Автоматическое определение URL через `NOTION_MCP_URL`
  - Поддержка `http://localhost:3000/mcp` по умолчанию
- **Упрощённая аутентификация**: Прямое использование Notion Integration Token
- **Полная Docker инфраструктура**: 
  - `docker-compose.yml` для всей системы
  - `Dockerfile` для AI Chatter бота
  - `scripts/start-notion-mcp.sh` для быстрого запуска
- **Обновлённая документация**: `docs/docker-mcp-setup.md`

### Improved
- **Убрана OAuth сложность**: Нет необходимости в сложной авторизации
- **Лучший контроль**: Локальный сервер под полным управлением
- **Простая отладка**: Доступ к логам Docker контейнера
- **Offline работа**: Не зависит от внешних сервисов
- **Быстрая настройка**: Один токен + Docker команда

### Technical Details
- **Docker образ**: `mcp/notion:latest` (официальный)
- **Порт**: 3000 (настраиваемый)
- **Переменные**: `NOTION_TOKEN`, `NOTION_MCP_URL`
- **Сеть**: Docker bridge для изоляции
- **Volumes**: Персистентное хранение данных и логов

### Benefits over cloud MCP
- ✅ **Простая настройка** vs OAuth flow
- ✅ **Полный контроль** vs внешний сервис
- ✅ **Offline работа** vs зависимость от интернета
- ✅ **Debugging возможности** vs чёрный ящик
- ✅ **Прямые токены** vs сложная авторизация
- ✅ **Настраиваемость** vs фиксированная конфигурация
- **Обновлённый системный промпт**: LLM знает о доступных функциях

### How it works
- Пользователь: "Сохрани эту беседу"
- LLM автоматически вызывает `save_dialog_to_notion` с подходящим названием
- Бот выполняет функцию и сохраняет диалог в Notion
- Никаких ручных команд не требуется!

### Compatibility
- **OpenAI/OpenRouter**: Полная поддержка function calling
- **YandexGPT**: Graceful fallback без function calling
- Работает только в обычном режиме (не в режиме ТЗ)

## [Day 1-2]

### Added
- **Project Structure**: Initialized a Go project with a modular structure (`cmd`, `internal`).
- **Configuration Management**: Implemented configuration loading from environment variables using `godotenv` and `caarlos0/env`.
- **Telegram Bot Integration**: Added a basic Telegram bot using `go-telegram-bot-api`.
- **LLM Client Abstraction**: Created a common `llm.Client` interface to support multiple LLM providers.
- **OpenAI Client**: Implemented a client for OpenAI-compatible APIs.
- **YandexGPT Client**: Implemented a client for YandexGPT using `Morwran/yagpt`.
- **LLM Provider Selection**: Added the ability to choose the LLM provider (`openai` or `yandex`) via the `LLM_PROVIDER` environment variable.
- **User Authorization**: Created a service to restrict bot access to a list of allowed user IDs.
- **Flexible API Endpoint**: Added the ability to specify a custom `BaseURL` for the LLM API via the `OPENAI_BASE_URL` environment variable.
- **Flexible Model Selection**: Added the ability to specify the LLM model name via the `OPENAI_MODEL` environment variable, with `gpt-3.5-turbo` as the default.
- **Enhanced Unauthorized User Handling**: The bot now replies to unauthorized users with a "request sent for review" message and logs their user ID and username.
- **.env Loading**: Improved `.env` loading to search multiple common locations (`.env`, `../.env`, `cmd/bot/.env`).
- **System Prompt**: Added `SYSTEM_PROMPT_PATH` and support for a system prompt file; passed to both OpenAI and YaGPT clients.
- **Logging**: Added logging of incoming user messages and LLM responses (model name and token usage).
- **Response Meta Line**: Bot prepends each answer with `[model=..., tokens: prompt=..., completion=..., total=...]`.
- **Per-user Conversation History**: Implemented a thread-safe history manager; the context is isolated per user and included in LLM requests.
- **Reset Context Button**: Added an inline button "Сбросить контекст" in Telegram; clears only the requesting user's history.
- **LLM Context Refactor**: Refactored `llm.Client` interface to `Generate(ctx, []llm.Message)` and updated OpenAI/YaGPT clients to accept full message history.
- **History Summary**: Added an inline button "История" to request a summary of the user's conversation with the assistant; the summary is logged, sent to the user (with meta line), and appended back to the user's history.
- **Storage Abstraction**: Introduced `storage.Recorder` and `storage.Event` for pluggable persistence.
- **File Logger (JSONL)**: Implemented file-based recorder writing one JSON per line to `LOG_FILE_PATH` (default `logs/log.jsonl`).
- **History Restore**: On startup, the bot preloads events from the recorder and reconstructs per-user history.
- **Config**: Added `LOG_FILE_PATH` env var to configure the path for JSONL log file.
- **Admin Approval Flow**: Added `ADMIN_USER` env var. Unauthorized user requests are sent to the admin with inline buttons "разрешить"/"запретить".
- **Allowlist Storage**: Introduced `auth.Repository` abstraction and file-based JSON allowlist (`ALLOWLIST_FILE_PATH`, default `data/allowlist.json`) storing `{id, username, first_name, last_name}`; approvals/denials update file and in-memory state.
- **/start Improvements**: Added welcome message with hints about inline buttons; auto-sends access request to admin and informs the user.
- **Pending Storage**: Added file-based pending repository (`PENDING_FILE_PATH`, default `data/pending.json`) to persist pending access requests across restarts.
- **Admin Pending Commands**: Added `/pending` to list pending users and `/approve <user_id>`, `/deny <user_id>` to allow/deny; updates pending file and allowlist on the fly.
- **Pending UX**: If a user has already requested access, bot no longer spams admin and informs the user to wait for approval.
- **Markdown Formatting**: Added `MESSAGE_PARSE_MODE` env var (now default `HTML`). All outgoing messages support HTML/Markdown/MarkdownV2.
- **CI**: Added GitHub Actions workflow to build and run tests on pushes/PRs to `main` and `develop`.
- **Unit Tests**: Added tests for history, storage, auth, pending, and telegram logic (including JSON parsing of LLM responses).
- **OpenRouter Support**: Added optional OpenRouter headers (`OPENROUTER_REFERRER`, `OPENROUTER_TITLE`) and README instructions; set `OPENAI_BASE_URL=https://openrouter.ai/api/v1` and supply OpenRouter model names.
- **Admin Provider/Model Hot-Reload**: Added `/provider <openai|yandex>` and `/model <openai/gpt-5-nano|openai/gpt-oss-20b:free|qwen/qwen3-coder>`; selections persisted in `data/provider.txt` and `data/model.txt` and applied without restart.
- **Startup Notice**: On bot start, logs "Bot started" and sends admin a message with current provider and model.
- **JSON Output Contract**: System prompt now enforces a JSON response structure `{title, answer, meta}` without markdown fences; bot parses it, sends only title+answer to the user, and stores `meta` for context.
- **Flexible `meta` Parsing**: `meta` can be a string or a JSON object/array; objects are compacted to a single-line JSON string for storage/context.
- **Context Flags**: History entries now track `isUsedInContext`. Reset marks all user entries as unused (kept in history).
- **Persistent `can_use`**: JSONL events include optional `can_use` flag; on reset the bot rewrites the log setting `can_use=false` for the user, so context state survives restarts.

## [Day 3]
- TS flow: reintroduced JSON field `status` with values `continue|final`. When `status=final` and user is in `/tz` mode, the bot decorates the answer with a "ТЗ Готово" marker and exits TZ mode.
- LLM responses: schema simplified to `{title, answer, compressed_context, status}`; `compressed_context` is appended into per-user system prompt and disables previous history for context.
- Logging: restored detailed logs for LLM interactions — outbound messages (purpose, roles, sizes, truncated contents) and inbound responses (model, token usage, raw content).
- System prompt: updated to describe the new schema including `status` and the 80% context fullness rule; clarified that the model must not use formatting in its `answer`.
- TZ mode cap: limited the clarification phase to at most 15 assistant messages. Upon reaching the cap, the bot forces finalization (requests a final TS) and returns the result with the "ТЗ Готово" marker.

## [Day 4]
- TS flow: reintroduced JSON field `status` with values `continue|final`. When `status=final` and user is in `/tz` mode, the bot decorates the answer with a "ТЗ Готово" marker and exits TZ mode.
- LLM responses: schema simplified to `{title, answer, compressed_context, status}`; `compressed_context` is appended into per-user system prompt and disables previous history for context.
- Logging: restored detailed logs for LLM interactions — outbound messages (purpose, roles, sizes, truncated contents) and inbound responses (model, token usage, raw content).
- System prompt: updated to describe the new schema including `status` and the 80% context fullness rule; clarified that the model must not use formatting in its `answer`.
- TZ mode cap: limited the clarification phase to at most 15 assistant messages. Upon reaching the cap, the bot forces finalization (requests a final TS) and returns the result with the "ТЗ Готово" marker.
- Refactor: split Telegram logic into `bot.go`, `handlers.go`, `process.go`; unified finalization path via a single `sendFinalTS` function.
- Numbered questions: enforced numbered list of clarifying questions (1., 2., ...) each on a new line; auto-enforced before sending when needed.
- Context reset on `/tz`: previous user history is marked as not used (and persisted via `can_use=false`) before starting a new TZ session.
- Secondary model (model2):
  - Added admin command `/model2 <model>` with persistence to `data/model2.txt`; lazy initialization of a second LLM client.
  - After sending final TS, the bot announces preparation and generates a user instruction (recipe/implementation plan) with the second model, then sends it.
  - During TZ, after each primary model response, the second model acts as a checker: receives only `answer` and `status`, returns JSON `{ "status": "ok|fail", "msg": "..." }`. On `fail`, the bot auto-corrects the primary response using the first model with the provided `msg` and sends the corrected answer to the user.
- Logging of checker/correction: persisted `[tz_check]` responses and `[tz_correct_req]` correction intents to the JSONL log (not used in context).
- Tests: updated and added unit tests for finalization flow, forced finalization at cap, numbered formatting, model2 usage (`/model2`), and checker-based correction.

## [Refactoring - 2025-01-27]

### Refactored
- **LLM Factory Pattern**: Создана фабрика `llm.Factory` для централизованного создания LLM клиентов, устранено дублирование кода в `main.go`, `bot.go`
- **Configuration Fix**: Исправлено дублирование env переменной `MODEL_FILE_PATH` для `Model2FilePath`, теперь используется `MODEL2_FILE_PATH`
- **Bot Structure Cleanup**: Удалены избыточные поля из структуры `Bot` (openaiAPIKey, openaiBaseURL, etc), теперь используется `llmFactory`
- **Dynamic Model Lists**: Заменен хардкод списка моделей в административных командах на динамическое получение из `llm.AllowedModels`
- **Improved Error Handling**: Улучшена обработка ошибок при создании LLM клиентов с fallback механизмами

### Technical Improvements
- Уменьшено количество полей в Bot struct с ~20 до ~15
- Устранено дублирование логики создания LLM клиентов в 3 местах
- Централизована конфигурация разрешенных моделей
- Упрощена поддержка новых LLM провайдеров

### Added
- **TZ Test Mode**: Автоматический тест-режим для проверки генерации ТЗ (`/tz test <тема>`)
- **Dual Model Architecture**: Model1 (TZ generator) + Model2 (auto-responder) для реалистичного тестирования
- **Response Validation**: Проверка формата ответов модели (отсутствие ```json блоков, валидация схемы)
- **Auto-failure Handling**: Автоматическое завершение при ошибках с очисткой контекста
- **Test Coverage**: Unit-тесты для валидации и автогенерации ответов
