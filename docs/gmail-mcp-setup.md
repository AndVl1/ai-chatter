# Gmail MCP Integration Setup Guide

## Обзор

Gmail MCP интеграция позволяет боту AI Chatter собирать информацию из Gmail, анализировать её с помощью AI агентов и создавать автоматические саммари в Notion.

## Архитектура

### Компоненты
- **Gmail MCP Server** (`cmd/gmail-mcp-server/`) - сервер для доступа к Gmail API
- **Gmail MCP Client** (`internal/gmail/mcp.go`) - клиент для взаимодействия с Gmail MCP
- **Multi-Agent System** (`internal/agents/agent.go`) - система агентов для обработки данных
- **Telegram Command** (`/gmail_summary`) - команда для запуска анализа

### Workflow
1. **Сбор данных**: Gmail агент ищет письма по запросу пользователя
2. **Валидация**: Второй LLM валидирует собранные данные
3. **Поиск/создание папки**: Notion агент находит или создает страницу "Gmail summaries"
4. **Генерация саммари**: LLM создает структурированный анализ писем
5. **Валидация саммари**: Второй LLM проверяет качество саммари
6. **Создание страницы**: Notion агент создает страницу с результатами

## Требования

### Gmail API Setup
1. **Google Cloud Console**:
   - Создайте проект в [Google Cloud Console](https://console.cloud.google.com/)
   - Включите Gmail API
   - Создайте OAuth 2.0 credentials (Desktop application type)

2. **OAuth 2.0 Flow**:
   ```bash
   # Получите credentials.json из Google Cloud Console
   # Сохраните его как GMAIL_CREDENTIALS_JSON_PATH в .env
   ```

### Переменные окружения
```bash
# Gmail интеграция
# Вариант 1: JSON прямо в переменной (рекомендуется для Docker)
GMAIL_CREDENTIALS_JSON='{"client_id":"your-client-id","client_secret":"your-client-secret","redirect_uris":["urn:ietf:wg:oauth:2.0:oob","http://localhost"]}'

# Вариант 2: Путь к файлу (для локальной разработки)
GMAIL_CREDENTIALS_JSON_PATH=./credentials.json

# Refresh token для автоматической авторизации (обязательно для Docker)
GMAIL_REFRESH_TOKEN=your-refresh-token

# Путь к Gmail MCP серверу (опционально)
GMAIL_MCP_SERVER_PATH=./gmail-mcp-server
```

## Установка

### 1. Настройка Google Cloud
```bash
# 1. Перейдите в Google Cloud Console
# 2. Создайте новый проект или выберите существующий
# 3. Включите Gmail API:
#    APIs & Services -> Enable APIs and Services -> Gmail API -> Enable
# 4. Создайте OAuth 2.0 credentials:
#    APIs & Services -> Credentials -> Create Credentials -> OAuth client ID
#    Application type: Desktop application
```

### 2. Получение credentials.json
```bash
# После создания OAuth credentials:
# 1. Скачайте JSON файл (будет иметь структуру {"installed": {...}})
# 2. Сохраните как credentials.json
# 3. Файл должен выглядеть примерно так:
# {
#   "installed": {
#     "client_id": "your-client-id.googleusercontent.com",
#     "client_secret": "your-client-secret",
#     "redirect_uris": ["urn:ietf:wg:oauth:2.0:oob", "http://localhost"]
#   }
# }
```

### 3. Получение Refresh Token
```bash
# Сборка auth helper утилиты:
./scripts/build-multi-mcp.sh

# Запуск OAuth2 flow:
./gmail-auth-helper credentials.json

# Скопируйте вывод утилиты в .env файл
```

### 4. Обновление .env файла
```bash
# Добавьте в ваш .env файл (вывод gmail-auth-helper):
GMAIL_CREDENTIALS_JSON='{"client_id":"YOUR_CLIENT_ID","client_secret":"YOUR_CLIENT_SECRET","redirect_uris":["urn:ietf:wg:oauth:2.0:oob","http://localhost"]}'
GMAIL_REFRESH_TOKEN='your-refresh-token-from-auth-helper'

# Опционально (по умолчанию ./gmail-mcp-server):
GMAIL_MCP_SERVER_PATH=./gmail-mcp-server
```

### 5. Сборка и запуск
```bash
# Локальная разработка:
go build ./cmd/bot
go build ./cmd/gmail-mcp-server
go build ./cmd/notion-mcp-server
./ai-chatter

# Docker:
docker-compose up -d --build
```

## Использование

### Команда /gmail_summary
```bash
# Базовое использование (только для админа):
/gmail_summary что важного я пропустил за последний день

# Другие примеры:
/gmail_summary непрочитанные письма от коллег
/gmail_summary важные письма за неделю
/gmail_summary письма с темой "проект"
```

### Результат
- Бот создаст страницу в Notion внутри папки "Gmail summaries"
- Вернет ссылку на созданную страницу
- Страница будет содержать структурированный анализ найденных писем

## Конфигурация Gmail Search

### Поддерживаемые параметры поиска
Gmail MCP сервер поддерживает стандартные Gmail search operators:

```bash
# Примеры поисковых запросов:
from:example@gmail.com          # От определенного отправителя
to:me                          # Письма мне
subject:важно                  # По теме письма
has:attachment                 # Письма с вложениями
is:unread                      # Непрочитанные
is:important                   # Важные
newer_than:1d                  # За последний день
older_than:7d                  # Старше недели
```

### Временные фильтры
- `today` - письма за последний день (по умолчанию)
- `week` - письма за последнюю неделю
- `month` - письма за последний месяц

## Безопасность

### OAuth 2.0 Token Management
```bash
# ВАЖНО: Gmail MCP сервер использует OAuth 2.0
# Первый запуск потребует авторизации через браузер
# Токены будут автоматически обновляться

# Для production убедитесь что:
# 1. GMAIL_CREDENTIALS_JSON_PATH содержит только client_id/client_secret
# 2. Токены доступа хранятся безопасно
# 3. Используется HTTPS для redirect URIs в production
```

### Права доступа
```bash
# Gmail MCP запрашивает минимальные права:
# - gmail.readonly: Только чтение писем
# - Не запрашивает права на отправку или изменение
```

## Troubleshooting

### Ошибки подключения
```bash
# Ошибка: "Gmail MCP server failed to connect"
# Проверьте:
# 1. GMAIL_CREDENTIALS_JSON_PATH корректно настроен
# 2. Gmail API включен в Google Cloud Console
# 3. OAuth credentials созданы правильно

# Логи Gmail MCP сервера:
docker-compose logs ai-chatter-bot | grep "Gmail MCP"
```

### Ошибки авторизации
```bash
# Ошибка: "insufficient permissions" или "invalid credentials"
# 1. Проверьте client_id и client_secret
# 2. Убедитесь что Gmail API включен
# 3. Повторите OAuth flow

# Для сброса токенов (если нужно):
# rm -rf ~/.credentials/gmail-mcp-token.json
```

### Ошибки поиска
```bash
# Ошибка: "No emails found"
# 1. Проверьте поисковый запрос
# 2. Убедитесь что есть письма соответствующие критериям
# 3. Проверьте временные фильтры

# Для отладки добавьте более широкий поиск:
/gmail_summary is:unread OR is:important
```

## Лимиты и ограничения

### Gmail API Quotas
```bash
# Google налагает лимиты на использование Gmail API:
# - 1 billion quota units per day
# - 250 quota units per user per second

# Один поиск ≈ 5-10 quota units
# Чтение письма ≈ 5 quota units
# Максимум 50 писем за запрос
```

### Performance
```bash
# Для оптимальной работы:
# - Используйте конкретные поисковые запросы
# - Ограничивайте временные диапазоны
# - Регулярно очищайте старые токены
```

## Логирование

### Gmail MCP Server Logs
```bash
# Логи сервера включают:
# - OAuth авторизацию
# - Поисковые запросы
# - Найденные письма
# - Ошибки API

# Просмотр логов:
docker-compose logs -f ai-chatter-bot | grep "Gmail"
```

### Agent Communication Logs
```bash
# Логи межагентного взаимодействия:
# - Сбор данных Gmail агентом
# - Валидация данных
# - Создание Notion страниц
# - Результаты обработки

# Просмотр в файлах логов:
tail -f logs/bot.jsonl | grep gmail
```

## Разработка

### Добавление новых функций Gmail
```go
// Добавление новых методов в Gmail MCP сервер:
// cmd/gmail-mcp-server/main.go

func (s *GmailMCPServer) NewMethod(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[NewParams]) (*mcp.CallToolResultFor[any], error) {
    // Ваш код
}
```

### Расширение Agent системы
```go
// Добавление новых агентов:
// internal/agents/agent.go

func NewCustomAgent(llmClient llm.Client, customClient *CustomMCPClient) *CustomWorkflow {
    // Ваш код
}
```

## TODO для ручной настройки

> ⚠️ **Важно**: Следующие шаги необходимо выполнить вручную:

1. **Google Cloud Console Setup**:
   - Создать проект в Google Cloud Console
   - Включить Gmail API
   - Создать OAuth 2.0 credentials
   - Скачать credentials.json

2. **OAuth Flow**:
   - При первом запуске пройти авторизацию через браузер
   - Сохранить refresh token для автоматического обновления

3. **Environment Variables**:
   - Добавить `GMAIL_CREDENTIALS_JSON_PATH` в .env файл
   - Настроить `GMAIL_MCP_SERVER_PATH` если необходимо

4. **Testing**:
   - Проверить подключение к Gmail API
   - Протестировать поиск писем
   - Убедиться в работе Notion интеграции

## Поддержка

Для получения помощи:
1. Проверьте логи бота и MCP серверов
2. Убедитесь в правильности настройки Google Cloud Console
3. Проверьте права доступа к Gmail и Notion APIs
4. Создайте issue в репозитории проекта