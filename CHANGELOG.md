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

## [Day 5.6 - Custom MCP Server Solution]

### Added
- **🏗️ Кастомный MCP сервер**: Собственная реализация на Go с официальным MCP SDK
  - Команда: `go build -o notion-mcp-server cmd/notion-mcp-server/main.go`
  - Использует `github.com/modelcontextprotocol/go-sdk@v0.2.0`
  - Работает через stdio transport (subprocess)
  - Регистрирует 3 инструмента: `create_page`, `search`, `save_dialog_to_notion`
- **Обновлённый MCP клиент**: Полностью переписан для работы с официальным SDK
  - Использует `mcp.NewClient()` и `mcp.NewCommandTransport()`
  - Автоматически запускает сервер как подпроцесс
  - Передаёт `NOTION_TOKEN` через переменные окружения
- **Комплексное тестирование**: `cmd/test-custom-mcp/main.go`
  - Тестирует сохранение диалогов
  - Тестирует создание произвольных страниц  
  - Тестирует поиск по workspace
- **Автоматизированные скрипты**: `scripts/test-custom-mcp.sh`

### Technical Architecture
- **Сервер**: MCP сервер работает как stdio subprocess
- **Клиент**: Использует `CommandTransport` для подключения
- **Протокол**: Полностью совместим с MCP spec 2024-11-05
- **Транспорт**: JSON-RPC 2.0 через stdin/stdout
- **API**: Прямые вызовы Notion REST API v1

### Implementation Details
- **Структуры параметров**: `CreatePageParams`, `SearchParams`, `SaveDialogParams`
- **Типизированные инструменты**: Используют MCP SDK generics
- **Graceful error handling**: Детальные сообщения об ошибках
- **Metadata support**: Возврат `page_id`, `success`, и других метаданных
- **Reusable logic**: Переиспользует существующий `notion.MCPClient`

### Benefits over Docker approach
- ✅ **Нативная Go интеграция** vs Docker overhead
- ✅ **Полная кастомизация** vs готовый образ
- ✅ **Официальный MCP SDK** vs HTTP клиент
- ✅ **Type safety** vs JSON мапы
- ✅ **Простая отладка** vs контейнер
- ✅ **Прямая компиляция** vs Docker build

## [Day 5.7 - Custom MCP Server Fixes]

### Fixed
- **🐛 Panic fix**: Исправлена проблема с `nil pointer dereference` в stdio transport
  - Заменён `&mcp.StdioTransport{}` на `mcp.NewStdioTransport()`
  - Убрано использование `NewLoggingTransport` для упрощения
- **🔄 Рекурсивный вызов**: Устранена проблема с рекурсивным вызовом MCP клиента внутри сервера
  - Реализован прямой `NotionAPIClient` вместо `notion.MCPClient`
  - Добавлены методы `createPage`, `searchPages` для прямой работы с Notion API
  - Убрана зависимость от `ai-chatter/internal/notion` в сервере
- **🔧 Правильная parent structure**: Исправлена структура создания страниц
  - Изменён parent с `page_id` на `workspace` для корневых страниц
  - Добавлена обработка ошибок Notion API

### Added
- **🐛 Debug script**: `scripts/debug-mcp-server.sh` для отладки MCP сервера
- **📝 Улучшенное логирование**: Детальные логи всех операций сервера
- **⚡ Прямой API**: `NotionAPIClient` с методами `doNotionRequest`, `createPage`, `searchPages`

### Technical Details
- **Transport fix**: Использование `mcp.NewStdioTransport()` вместо struct literal
- **No circular deps**: Убрана зависимость сервера от основного MCP клиента
- **Direct HTTP**: Прямые HTTP запросы к `https://api.notion.com/v1`
- **Error handling**: Корректная обработка ошибок на всех уровнях

### Benefits
- ✅ **Стабильность**: Нет больше panic при запуске
- ✅ **Производительность**: Прямые API вызовы без лишних слоёв
- ✅ **Отладка**: Простая диагностика проблем
- ✅ **Надёжность**: Proper error handling

## [Day 5.8 - Mandatory Parent Page ID]

### Changed
- **🔧 Mandatory parent_page_id**: Следуем официальному Notion API требованию
  - `CreatePageParams.ParentPageID` теперь обязательный параметр
  - `SaveDialogParams.ParentPageID` теперь обязательный параметр
  - Все методы проверяют наличие parent_page_id
- **📋 Правильная структура Notion API**: Используем официальный формат
  ```json
  {
    "parent": {
      "type": "page_id", 
      "page_id": "12345678-90ab-cdef-1234-567890abcdef"
    }
  }
  ```

### Added
- **⚙️ NOTION_PARENT_PAGE_ID**: Новая обязательная переменная окружения
- **📖 Детальная документация**: `docs/notion-parent-page-setup.md`
  - Как получить parent page ID из URL
  - Как найти через API
  - Как создать новую родительскую страницу
  - Решение проблем и ошибок
- **🛡️ Валидация**: Проверка parent_page_id на всех уровнях
  - MCP сервер валидирует параметры
  - Telegram handlers проверяют настройки
  - Понятные сообщения об ошибках

### Technical Details
- **API compliance**: Полное соответствие Notion API v1
- **Error handling**: Детальные ошибки для каждого случая
- **Graceful fallback**: Использование default parent page для create_notion_page
- **Config integration**: NOTION_PARENT_PAGE_ID в config.Config

### Benefits
- ✅ **API соответствие**: Правильная работа с официальным Notion API
- ✅ **Организация**: Все страницы создаются в указанной родительской странице  
- ✅ **Отладка**: Понятные ошибки при неправильной настройке
- ✅ **Безопасность**: Интеграция работает только с доступными страницами

### Breaking Changes
- **🔴 Обязательный NOTION_PARENT_PAGE_ID**: Нужно добавить в .env
- **🔴 Изменён API**: CreateDialogSummary теперь принимает parentPageID параметр
- **🔴 Изменён API**: CreateFreeFormPage требует валидный parentPageID

### Migration Guide
1. Получите parent page ID из Notion (см. документацию)
2. Добавьте в .env: `NOTION_PARENT_PAGE_ID=ваш-page-id`
3. Убедитесь что интеграция имеет доступ к родительской странице
4. Перезапустите бота

## [Day 5.9 - Integration Tests & Improved LLM Feedback]

### Added
- **🧪 Полноценные интеграционные тесты**: `internal/notion/mcp_integration_test.go`
  - Тестирование создания диалогов через MCP
  - Тестирование создания произвольных страниц  
  - Тестирование поиска в workspace
  - Обработка ошибок и edge cases
  - Автоматическая очистка тестовых данных
- **🎯 NOTION_TEST_PAGE_ID**: Переменная для тестовой страницы
- **📜 Скрипт интеграционных тестов**: `scripts/test-notion-integration.sh`
  - Автоматический запуск MCP сервера
  - Проверка переменных окружения
  - Понятные сообщения об ошибках
  - Graceful cleanup

### Changed
- **💬 Улучшенный feedback от LLM**: Теперь при выполнении MCP действий:
  - 💾 "Сохраняю диалог в Notion..." - уведомление о начале операции
  - 🔍 "Ищу в Notion..." - feedback при поиске
  - 📝 "Создаю страницу в Notion..." - уведомление о создании
  - ✅ LLM формулирует краткий итоговый ответ на основе результатов
  - ❌ Корректная обработка ошибок без дублирования технических деталей

### Technical Details
- **Tool Call Pipeline**: Новая архитектура обработки LLM tool calls
  1. LLM вызывает функцию (save_dialog_to_notion, search_notion, create_notion_page)
  2. Бот отправляет уведомление пользователю о начале операции
  3. Выполняется MCP вызов
  4. Результат отправляется обратно в LLM
  5. LLM формулирует понятный ответ пользователю
- **ToolCallResult структура**: Новый тип для передачи результатов tool calls
- **Enhanced Message**: Добавлен ToolCallID для tool response сообщений
- **Continuous conversation**: `continueConversationWithToolResults()` метод

### Integration Testing
- **Environment setup**: Проверка NOTION_TOKEN и NOTION_TEST_PAGE_ID
- **Real API testing**: Создание реальных страниц в тестовой Notion
- **Graceful skipping**: Автоматический skip если переменные не установлены
- **Timestamped test data**: Уникальные суффиксы для избежания конфликтов

### Benefits
- ✅ **Лучший UX**: Пользователь видит что происходит во время выполнения действий
- ✅ **Надёжность**: Полное интеграционное тестирование с реальным API
- ✅ **Отладка**: Простая диагностика проблем интеграции
- ✅ **Автоматизация**: Скрипты для CI/CD процессов
- ✅ **Conversation flow**: LLM может продолжить диалог после выполнения действий

### Usage Examples

**Пользователь:** "Сохрани наш диалог"
1. 💾 "Сохраняю диалог в Notion..."
2. [MCP создаёт страницу]
3. ✅ "Диалог успешно сохранён в Notion под названием 'Обсуждение проекта'. Теперь вы можете найти его позже через поиск!"

**Пользователь:** "Найди информацию о прошлых проектах"
1. 🔍 "Ищу в Notion..."
2. [MCP выполняет поиск]
3. 📋 "Нашёл 3 диалога о проектах: проект А (15 янв), проект Б (22 янв), проект В (1 фев). Какой именно вас интересует?"

## [Day 5.10 - CI/CD Integration Tests]

### Added
- **🚀 Полноценный CI/CD pipeline**: GitHub Actions workflows
  - **ci.yml**: Unit tests + Integration tests + Cross-platform builds
  - **nightly-integration.yml**: Ночные тесты с детальной отчётностью  
  - **performance.yml**: Performance тесты при релизах
- **📋 Локальный CI скрипт**: `scripts/ci-local.sh`
  - Имитация полного CI процесса локально
  - Проверка форматирования, сборки, тестов
  - Cross-platform build checks
  - Coverage анализ с threshold проверкой
- **📖 CI Documentation**: `docs/ci-setup.md`
  - Настройка GitHub Secrets
  - Отладка CI проблем
  - Performance monitoring
  - Безопасность и best practices

### Changed
- **🔄 Обновлённый CI workflow**: Разделение на этапы
  - Unit tests - быстрые тесты без внешних зависимостей
  - Integration tests - только на main/develop с реальным Notion API
  - Cross-platform - проверка сборки на Windows/macOS/Linux
- **📊 Coverage reporting**: Автоматическая отправка в Codecov
- **🎯 Smart integration testing**: Graceful skip если secrets не настроены

### Technical Details
- **GitHub Actions matrix**: Тестирование на нескольких версиях Go
- **Conditional execution**: Integration тесты только при наличии secrets
- **Artifact management**: Сохранение отчётов, profiles, logs
- **Performance monitoring**: Memory/CPU profiling при релизах
- **Trend analysis**: Отслеживание качества тестов во времени

### CI Pipeline Structure
```yaml
🔄 Push/PR → Unit Tests → ✅ Pass
                       ↓
🌟 main/develop → Integration Tests → 📊 Coverage
                                   ↓  
🌙 Nightly → Full Test Suite → 📈 Trend Analysis
          ↓
🏷️ Release Tag → Performance Tests → 🔍 Regression Check
```

### Local CI Usage
```bash
# Полный pipeline
./scripts/ci-local.sh

# Отдельные этапы  
./scripts/ci-local.sh test        # Unit тесты
./scripts/ci-local.sh build       # Сборка
./scripts/ci-local.sh integration # Integration тесты
./scripts/ci-local.sh cross       # Cross-platform
```

### GitHub Secrets Setup
```yaml
Required for integration tests:
  NOTION_TOKEN: secret_abc123...
  NOTION_TEST_PAGE_ID: 12345678-90ab-cdef-...

Optional:
  CODECOV_TOKEN: для улучшенной coverage интеграции
```

### Benefits
- ✅ **Автоматизация**: Полный CI/CD без ручных проверок
- ✅ **Качество**: Обязательные тесты перед merge
- ✅ **Надёжность**: Integration тесты с реальным Notion API
- ✅ **Performance**: Отслеживание регрессий производительности
- ✅ **Cross-platform**: Гарантия работы на всех ОС
- ✅ **Visibility**: Детальные отчёты и метрики
- ✅ **Local development**: Возможность имитации CI локально

### Monitoring & Alerts
- 🔴 **Critical**: Сбой integration tests на main
- 🟠 **Warning**: Coverage ниже 75%
- 🟡 **Info**: Медленные тесты (>5 минут)
- 📊 **Metrics**: Build time, test duration, API latency

## [Day 5.12 - List Available Pages & Smart Parent Selection]

### Added
- **📋 Новая функция list_available_pages**: Получение списка доступных страниц для создания подстраниц
  - **MCP Server**: `ListAvailablePages` функция в `cmd/notion-mcp-server/main.go`
    - Возврат всех доступных страниц в workspace с информацией о возможности быть родителем
    - Фильтрация по типу страницы (`page_type`) и только родительские (`parent_only`)
    - Ограничение количества результатов (`limit`, max 100, default 20)
    - Структурированный возврат с ID, названием, URL, типом и флагом `can_be_parent`
    - Автоматическое определение возможности быть родителем (все страницы в Notion могут)
  - **MCP Client**: `ListAvailablePages` метод в `internal/notion/mcp.go`
    - Парсинг метаданных с полной информацией о страницах
    - Новые типы: `MCPAvailablePagesResult`, `MCPAvailablePageResult`
    - Поддержка всех параметров фильтрации
  - **LLM Tool**: `list_available_pages` в `internal/llm/tools.go`
    - Описание: "Получает список доступных страниц в Notion workspace, которые могут использоваться как родительские страницы"
    - Параметры: `limit` (int), `page_type` (string), `parent_only` (bool)
    - Использование: для выбора подходящей родительской страницы при создании новых страниц
  - **Bot Integration**: Обработка в `internal/telegram/process.go`
    - Уведомление пользователя "📋 Получаю список доступных страниц..."
    - Парсинг всех параметров из function call
    - Форматированный вывод с номерацией, ID, типами и флагами родительства
    - Отображение URL для быстрого доступа

### Enhanced Create Page Function
- **🔄 Обновлён create_notion_page**: Улучшена поддержка выбора родительской страницы
  - **New Parameter**: `parent_page_id` - прямое указание ID родительской страницы
  - **Backward Compatibility**: Поддержка старого параметра `parent_page`
  - **Priority Logic**: `parent_page_id` имеет приоритет над `parent_page`
  - **Smart Default**: Fallback на `NOTION_PARENT_PAGE_ID` если ничего не указано
  - **LLM Guidance**: Обновлено описание с указанием использовать `list_available_pages` для выбора
  - **Process Flow**: LLM может сначала получить список страниц, выбрать подходящую, затем создать подстраницу

### New Data Types
- **ListPagesParams**: Параметры получения списка (Limit, PageType, ParentOnly)
- **AvailablePageResult**: Информация о доступной странице (ID, Title, URL, CanBeParent, Type)
- **MCPAvailablePagesResult**: Результат через MCP (Success, Message, Pages[], TotalFound)
- **MCPAvailablePageResult**: Информация о доступной странице через MCP

### Testing & Validation
- **Integration Tests**: Новый тест `ListAvailablePages` в `internal/notion/mcp_integration_test.go`
  - Проверка получения списка доступных страниц с валидацией данных
  - Тест создания подстраницы под найденной родительской страницей
  - Проверка фильтра `parent_only` и корректности флага `CanBeParent`
  - Автоматическое тестирование подстраниц при наличии доступных родителей
- **Custom MCP Test**: Обновлён `cmd/test-custom-mcp/main.go`
  - Тестирование новой функции получения списка доступных страниц
  - Отображение результатов с ID, названиями и флагами родительства
  - Проверка работы с лимитами и фильтрами

### Smart Workflow
- **🤖 LLM Integration**: Интеллектуальный выбор родительской страницы
  ```
  User: "Создай страницу про API документацию"
  LLM: 1. Calls list_available_pages()
       2. Analyzes available pages
       3. Selects appropriate parent (e.g., "Development" or "Documentation")
       4. Calls create_notion_page(parent_page_id="selected-id")
  Bot: Creates subpage under the most suitable parent
  ```

### Use Cases
- **🏗️ Smart Page Organization**: Автоматический выбор подходящей родительской страницы
- **📂 Workspace Navigation**: Просмотр доступных страниц для организации контента
- **🔗 Parent Page Discovery**: Поиск страниц которые могут быть родителями
- **📋 Content Structuring**: Создание иерархической структуры страниц

### Benefits
- ✅ **Smart Parent Selection**: LLM автоматически выбирает подходящую родительскую страницу
- ✅ **Workspace Awareness**: Полная видимость доступных страниц для организации
- ✅ **Flexible Filtering**: Фильтрация по типу и возможности быть родителем
- ✅ **Hierarchical Organization**: Создание структурированной иерархии страниц
- ✅ **Backward Compatibility**: Поддержка старого API с новыми возможностями
- ✅ **User Experience**: Автоматическая организация без необходимости знать ID

### Technical Implementation
```yaml
MCP Server:
  - Tool: list_available_pages
  - Function: ListAvailablePages()
  - Validation: limit ≤ 100, type filtering
  - Output: Structured result with full page info

MCP Client:  
  - Method: ListAvailablePages(limit, pageType, parentOnly)
  - Returns: MCPAvailablePagesResult
  - Metadata: Full page information with capabilities

LLM Tools:
  - create_notion_page: Enhanced with parent_page_id support
  - list_available_pages: New tool for page discovery
  - Smart workflow: list → analyze → create with parent

Bot Handler:
  - Case: "list_available_pages" 
  - Enhanced: "create_notion_page" with parent_page_id
  - Smart: Automatic parent selection workflow
```

### Example Usage
```bash
User: "Создай страницу с планом проекта"
Bot: "📋 Получаю список доступных страниц..."
LLM: Uses list_available_pages tool
Bot: "📋 Найдено 10 доступных страниц:
     1. **Projects** (ID: abc123) ✅ Can be parent
     2. **Team Docs** (ID: def456) ✅ Can be parent
     ..."
LLM: Analyzes and selects "Projects" as appropriate parent
     Uses create_notion_page(title="План проекта", content="...", parent_page_id="abc123")
Bot: "📝 Создаю страницу в Notion..."
Bot: "✅ Страница 'План проекта' создана под 'Projects'"
```

### Intelligent Page Hierarchy
- **Context-Aware**: LLM анализирует содержание и выбирает подходящую родительскую страницу
- **Automatic Organization**: Без необходимости пользователю знать структуру workspace
- **Flexible Structure**: Поддержка любой иерархии страниц в Notion
- **Dynamic Discovery**: Получение актуального списка доступных страниц в реальном времени

### Updated System Prompt
- **📝 Enhanced Notion Instructions**: Обновлён `prompts/system_prompt.txt`
  - Подробное описание всех доступных Notion функций
  - Инструкции по smart page organization
  - Workflow для создания страниц с автоматическим выбором родителя
  - Best practices для использования search и list функций
  - Примеры контекстного размещения (API docs → Development, meeting notes → Team)
  - Руководство по использованию exact_match в поиске

### Fixed
- **🐛 Response Length Optimization**: Исправлена ошибка "Provider returned error (400)" из-за слишком длинных ответов
  - **MCP Server**: Компактный формат ответов в `SearchPagesWithID` и `ListAvailablePages`
    - Убраны лишние разметка (**bold**) и детали URL из основного текста
    - Краткий формат: "1. Page Title (ID: abc123)" вместо многострочного
    - Снижены лимиты по умолчанию: search 5→20 (было 10→50), list 10→25 (было 20→100)
  - **Bot Responses**: Оптимизированы ответы в `internal/telegram/process.go`
    - Убраны дублирующиеся детали (URL, подробные типы)
    - Компактное отображение с emoji индикаторами (✅) вместо текста
    - Значительно сокращена длина передаваемого LLM контента
  - **LLM Tools**: Обновлены описания лимитов в `internal/llm/tools.go`
    - search_pages_with_id: "по умолчанию 5, максимум 20"  
    - list_available_pages: "по умолчанию 10, максимум 25"
    - Предотвращение превышения token limits у LLM провайдеров
  - **LLM Call Fix**: Исправлен вызов LLM для tool call результатов
    - Заменён `Generate` на `GenerateWithTools` (как в обычных запросах)
    - Передаются tools для корректной обработки LLM провайдером
    - Убраны искусственные ограничения длины ответов
    - Восстановлен полный функционал отображения результатов

## [Day 5.11 - Search Pages with ID Function]

### Added
- **🔍 Новая функция search_pages_with_id**: Поиск страниц в Notion с возвратом ID, заголовка и URL
  - **MCP Server**: `SearchPagesWithID` функция в `cmd/notion-mcp-server/main.go`
    - Поддержка точного и приблизительного поиска (`exact_match`)
    - Ограничение количества результатов (`limit`, max 50)
    - Возврат структурированных данных с ID, названием и URL
    - Фильтрация пустых результатов и валидация входных данных
  - **MCP Client**: `SearchPagesWithID` метод в `internal/notion/mcp.go`
    - Парсинг метаданных с результатами поиска
    - Структурированный возврат `MCPPageSearchResult` с массивом страниц
    - Обработка ошибок и пустых результатов
  - **LLM Tool**: `search_pages_with_id` в `internal/llm/tools.go`
    - Описание: "Ищет страницы в Notion по названию и возвращает их ID, заголовок и URL"
    - Параметры: `query` (required), `exact_match` (bool), `limit` (int)
    - Использование: когда LLM нужно найти страницу по названию для получения ID
  - **Bot Integration**: Обработка в `internal/telegram/process.go`
    - Уведомление пользователя "🔍 Ищу страницы в Notion..."
    - Парсинг параметров из function call
    - Форматированный вывод результатов с номерацией
    - Отображение ID, названий и URL найденных страниц

### New Data Types
- **PageSearchResult**: Результат поиска одной страницы (ID, Title, URL)
- **SearchPagesParams**: Параметры поиска (Query, Limit, ExactMatch)  
- **MCPPageSearchResult**: Результат поиска через MCP (Success, Message, Pages[], TotalFound)
- **MCPPageResult**: Информация о найденной странице (ID, Title, URL)

### Testing
- **Integration Tests**: Новый тест `SearchPagesWithID` в `internal/notion/mcp_integration_test.go`
  - Проверка поиска страниц с валидацией ID и названий
  - Тест точного совпадения с созданными страницами
  - Обработка случаев когда индексация ещё не завершилась
- **Custom MCP Test**: Обновлён `cmd/test-custom-mcp/main.go`
  - Тестирование новой функции поиска страниц с ID
  - Отображение найденных результатов с ID и названиями
  - Проверка работы с лимитами результатов

### Use Cases
- **🔍 Поиск по названию**: "Найди страницу 'Проект Alpha'" → возврат ID для дальнейшего использования
- **📋 Браузинг страниц**: "Покажи все страницы со словом 'отчёт'" → список с ID и ссылками
- **🔗 Получение ссылок**: Автоматическое получение URL страниц для шаринга
- **🤖 LLM интеграция**: Когда LLM знает только название страницы, но нужен ID

### Benefits
- ✅ **Structured Output**: Возврат ID, названия и URL в структурированном виде
- ✅ **Flexible Search**: Поддержка точного и приблизительного поиска
- ✅ **Performance**: Ограничение результатов для быстрого ответа
- ✅ **User Experience**: Информативные уведомления и форматированный вывод
- ✅ **LLM Ready**: Интеграция в tool calling для автоматического использования
- ✅ **Error Handling**: Graceful обработка ошибок и пустых результатов

### Technical Implementation
```yaml
MCP Server:
  - Tool: search_pages_with_id
  - Function: SearchPagesWithID()
  - Validation: limit ≤ 50, non-empty query
  - Output: Structured result with meta

MCP Client:  
  - Method: SearchPagesWithID(query, limit, exactMatch)
  - Returns: MCPPageSearchResult
  - Metadata: Parsing structured results from MCP

Bot Handler:
  - Case: "search_pages_with_id"
  - Notification: "🔍 Ищу страницы в Notion..."
  - Response: Formatted list with numbering
```

### Example Usage
```bash
User: "Найди все страницы про тестирование"
Bot: "🔍 Ищу страницы в Notion..."
LLM: Uses search_pages_with_id tool
Bot: "🔍 Найдено 3 страницы:
      1. **Testing Guide** 
         ID: 123abc-456def
         URL: https://notion.so/...
      2. **Unit Tests** 
         ID: 789ghi-012jkl
         URL: https://notion.so/..."
```

### Fixed
- **🔧 test-custom-mcp validation**: Добавлена обязательная проверка `NOTION_TEST_PAGE_ID`
  - `cmd/test-custom-mcp/main.go`: Проверка переменной с понятной ошибкой
  - `scripts/test-custom-mcp.sh`: Проверка и передача всех необходимых переменных
  - `scripts/test-notion-integration.sh`: Улучшенные инструкции по настройке
  - Graceful handling отсутствующих environment variables
  - Подробные инструкции по получению Notion page ID
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
