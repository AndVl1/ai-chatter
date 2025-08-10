# ai-chatter — Telegram-бот для работы с LLM (OpenAI/YandexGPT)

[![CI](https://github.com/AndVl1/ai-chatter/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/AndVl1/ai-chatter/actions/workflows/ci.yml)

Небольшой телеграм-бот на Go, который отправляет сообщения в OpenAI-совместимые API или YaGPT и возвращает ответы модели. Доступ ограничен белым списком пользователей.

## Возможности
- Поддержка провайдеров: OpenAI и YandexGPT (YaGPT)
- Переключение провайдера через переменные окружения
- Кастомный системный промпт из файла
- Логирование входящих сообщений и ответов LLM (модель и токены)
- Ответ неавторизованным пользователям: «запрос отправлен на проверку»

## Требования
- Go 1.18+

## Быстрый старт
1) Клонируйте репозиторий и перейдите в каталог проекта.
2) Создайте файл `.env` (не коммитится) на основе примера ниже.
3) При необходимости отредактируйте `prompts/system_prompt.txt`.
4) Запустите бота:
```bash
go run cmd/bot/main.go
```

## Переменные окружения
Создайте файл `.env` в корне проекта. Пример:
```dotenv
# Выбор провайдера: openai | yandex
LLM_PROVIDER=openai

# Телеграм-бот
TELEGRAM_BOT_TOKEN=xxx
# Список разрешённых пользователей (ID через двоеточие)
ALLOWED_USERS=123456789:987654321
ADMIN_USER=000000000
ALLOWLIST_FILE_PATH=data/allowlist.json
PENDING_FILE_PATH=data/pending.json

# OpenAI (или совместимый API)
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=
OPENAI_MODEL=gpt-3.5-turbo

# YandexGPT (YaGPT)
YANDEX_OAUTH_TOKEN=ya_oauth_...
YANDEX_FOLDER_ID=b1g...id

# Системный промпт
SYSTEM_PROMPT_PATH=prompts/system_prompt.txt

# Логи JSONL
LOG_FILE_PATH=logs/log.jsonl

# Форматирование сообщений
MESSAGE_PARSE_MODE=Markdown
```

Замечания:
- Для OpenAI-совместимых сервисов можно указать `OPENAI_BASE_URL` и `OPENAI_MODEL`.
- Для YaGPT используются `YANDEX_OAUTH_TOKEN` (чтобы получить IAM-токен) и `YANDEX_FOLDER_ID`.

## Поведение бота
- Если пользователь не в белом списке `ALLOWED_USERS`, бот ответит: «запрос отправлен на проверку», а в лог попадут его ID и username.
- В ответе бота первой строкой выводится мета-информация:
  `[model=..., tokens: prompt=..., completion=..., total=...]`
- В логи пишутся входящие сообщения и ответы модели с токенами.

## Структура проекта (основное)
- `cmd/bot/main.go` — точка входа
- `internal/config` — конфигурация из окружения
- `internal/auth` — проверка доступа (белый список)
- `internal/llm` — абстракция и клиенты LLM (OpenAI/YaGPT)
- `internal/telegram` — адаптер к Telegram Bot API
- `prompts/system_prompt.txt` — системный промпт

## Советы по безопасности
- Не коммитьте `.env`. Публикуйте только шаблон `.env.example` без значений.
- При утечке секретов немедленно ротируйте ключи/токены.

## Лицензия
Проект распространяется «как есть». Используйте на свой страх и риск.
