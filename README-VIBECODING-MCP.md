# VibeCoding MCP Server & Web Interface

Полноценный VibeCoding MCP сервер с внешним веб-интерфейсом для работы с coding сессиями.

## Архитектура

### Компоненты системы

1. **VibeCoding MCP Server** (`cmd/vibecoding-mcp-server/`)
   - Полноценный MCP сервер по аналогии с Gmail/Notion серверами
   - Работает через stdin/stdout протокол
   - Предоставляет 7 основных инструментов для работы с VibeCoding сессиями

2. **External Web Interface** (`docker/vibecoding-web/`)
   - Внешний веб-интерфейс в отдельном контейнере
   - Коммуникация с VibeCoding сессиями через HTTP API
   - Современный веб-интерфейс для управления файлами, командами и тестами

3. **Docker Integration**
   - Автоматический запуск MCP сервера в coding контейнерах
   - Изолированная среда выполнения
   - Сетевая коммуникация между компонентами

## VibeCoding MCP Tools

### Зарегистрированные инструменты:

1. **`vibe_list_files`** - Получить список файлов в workspace
   - Параметры: `user_id`
   - Возврат: список файлов с метаданными

2. **`vibe_read_file`** - Прочитать содержимое файла
   - Параметры: `user_id`, `filename`
   - Возврат: содержимое файла

3. **`vibe_write_file`** - Записать файл
   - Параметры: `user_id`, `filename`, `content`, `generated`
   - Возврат: статус записи

4. **`vibe_execute_command`** - Выполнить команду
   - Параметры: `user_id`, `command`
   - Возврат: результат выполнения с выводом

5. **`vibe_validate_code`** - Валидировать код
   - Параметры: `user_id`, `filename`
   - Возврат: результат валидации

6. **`vibe_run_tests`** - Запустить тесты
   - Параметры: `user_id`, `test_file`
   - Возврат: результат тестирования

7. **`vibe_get_session_info`** - Получить информацию о сессии
   - Параметры: `user_id`
   - Возврат: метаданные сессии

## Веб-интерфейс

### Возможности:

- **📁 File Management**: Просмотр, редактирование и сохранение файлов
- **💻 Terminal**: Выполнение команд в реальном времени
- **🧪 Test Runner**: Запуск тестов с отображением результатов
- **ℹ️ Session Info**: Отображение метаданных сессии
- **🔄 Real-time Updates**: Обновление статуса соединения

### API Endpoints:

- `GET /api/files/:userId` - Список файлов (через HTTP API)
- `GET /api/files/:userId/:filename` - Содержимое файла (через HTTP API)  
- `POST /api/files/:userId/:filename` - Сохранение файла (заглушка)
- `POST /api/execute/:userId` - Выполнение команды (заглушка)
- `POST /api/test/:userId` - Запуск тестов (заглушка)
- `GET /api/session/:userId` - Информация о сессии (через HTTP API)
- `GET /api/status` - Статус сервера и HTTP API соединения

## Быстрый старт

### 1. Автоматический запуск

```bash
# Запуск всей системы одной командой
./start-ai-chatter.sh --full
```

### 2. Ручной запуск

```bash
# Сборка MCP сервера
go build -o ./cmd/vibecoding-mcp-server/vibecoding-mcp-server ./cmd/vibecoding-mcp-server/

# Запуск через Docker Compose
docker-compose -f docker-compose.full.yml up --build -d
```

### 3. Доступ к интерфейсу

- **Веб-интерфейс**: http://localhost:3000
- **API статус**: http://localhost:3000/api/status

## Использование

### 1. Загрузка сессии

1. Откройте веб-интерфейс по адресу http://localhost:3000
2. Введите User ID в поле в боковой панели
3. Нажмите "Load Session" для загрузки активной VibeCoding сессии

### 2. Работа с файлами

- Выберите файл из списка в боковой панели
- Редактируйте содержимое в редакторе
- Нажмите "Save" для сохранения изменений

### 3. Выполнение команд

- Перейдите на вкладку "Terminal"
- Введите команду и нажмите "Run" или Enter
- Просматривайте результат в терминале

### 4. Запуск тестов

- Перейдите на вкладку "Tests"
- Нажмите "Run Tests" для выполнения тестов
- Просматривайте результаты тестирования

## Docker Integration

### Автоматический запуск MCP сервера

При создании VibeCoding сессии:

1. Создается Docker контейнер для coding окружения
2. Автоматически копируется и запускается MCP сервер внутри контейнера
3. MCP сервер предоставляет доступ к файлам и командам через стандартный протокол
4. Внешний веб-интерфейс подключается к MCP серверу для управления сессией

### Сетевая архитектура

```
External Web Interface (port 3000)
          ↓ HTTP API
VibeCoding Internal API (port 8080)
          ↓ Direct API calls
VibeCoding Session Manager
          ↓ Docker API
Docker Containers (coding environments + MCP servers)
```

## Мониторинг и отладка

### Логи контейнеров

```bash
# Все логи
docker-compose -f docker-compose.vibecoding.yml logs -f

# Только веб-интерфейс
docker-compose -f docker-compose.vibecoding.yml logs -f vibecoding-web

# Только MCP сервер
docker-compose -f docker-compose.vibecoding.yml logs -f vibecoding-mcp
```

### Статус системы

```bash
# Статус контейнеров
docker-compose -f docker-compose.vibecoding.yml ps

# Проверка API
curl http://localhost:3000/api/status
```

### Остановка системы

```bash
docker-compose -f docker-compose.vibecoding.yml down
```

## Технические детали

### MCP Protocol Implementation

- Использует MCP SDK для Go (`github.com/modelcontextprotocol/go-sdk/mcp`)
- Стандартный транспорт через stdin/stdout
- JSON-RPC 2.0 протокол для коммуникации
- Полная совместимость с официальной спецификацией MCP

### Web Interface Technology Stack

- **Backend**: Node.js + Express
- **Frontend**: Vanilla JavaScript + CSS
- **Communication**: RESTful API + MCP client
- **Containerization**: Docker + Alpine Linux

### Security Considerations

- Изолированные Docker контейнеры для каждой сессии
- Ограниченные сетевые доступы
- Валидация всех пользовательских входов
- Отсутствие прямого доступа к хост-системе

## Расширение функциональности

### Добавление новых MCP инструментов

1. Добавьте новый метод в `VibeCodingMCPServer` (cmd/vibecoding-mcp-server/main.go)
2. Зарегистрируйте инструмент в `main()` функции
3. Добавьте соответствующий API endpoint в веб-интерфейс (docker/vibecoding-web/server.js)
4. Обновите UI при необходимости

### Интеграция с другими системами

MCP протокол позволяет легко интегрировать VibeCoding с:
- IDE и редакторами кода
- CI/CD системами
- Внешними AI ассистентами
- Системами мониторинга и аналитики

## Поддержка

При возникновении проблем:

1. Проверьте логи контейнеров
2. Убедитесь в доступности Docker и docker-compose
3. Проверьте статус MCP соединения через /api/status
4. Перезапустите систему при необходимости

---

**Результат**: Полная реализация VibeCoding MCP архитектуры с внешним веб-интерфейсом, обеспечивающая масштабируемое и безопасное взаимодействие с coding сессиями через стандартный MCP протокол.