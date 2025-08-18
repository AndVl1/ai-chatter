# Dockerfile для AI Chatter Bot
FROM golang:1.24-alpine AS builder

# Устанавливаем зависимости для сборки
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Копируем модули и загружаем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарные файлы
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ai-chatter cmd/bot/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o notion-mcp-server cmd/notion-mcp-server/main.go

# Минимальный образ для production
FROM alpine:latest

# Устанавливаем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Копируем бинарные файлы из builder
COPY --from=builder /app/ai-chatter .
COPY --from=builder /app/notion-mcp-server .

# Копируем необходимые файлы
COPY --from=builder /app/prompts ./prompts

# Создаем директории для данных и логов
RUN mkdir -p /app/data /app/logs

# Устанавливаем права на выполнение
RUN chmod +x ./ai-chatter ./notion-mcp-server

# Создаем скрипт запуска
RUN echo '#!/bin/sh' > /app/start.sh && \
    echo 'export NOTION_MCP_SERVER_PATH="/app/notion-mcp-server"' >> /app/start.sh && \
    echo 'exec ./ai-chatter "$@"' >> /app/start.sh && \
    chmod +x /app/start.sh

# Запускаем бот через скрипт
CMD ["/app/start.sh"]
