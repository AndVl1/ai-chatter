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
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gmail-mcp-server cmd/gmail-mcp-server/main.go

# Минимальный образ для production с Docker поддержкой
FROM docker:24-dind

# Устанавливаем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Копируем бинарные файлы из builder
COPY --from=builder /app/ai-chatter .
COPY --from=builder /app/notion-mcp-server .
COPY --from=builder /app/gmail-mcp-server .

# Копируем необходимые файлы
COPY --from=builder /app/prompts ./prompts

# Создаем директории для данных и логов
RUN mkdir -p /app/data /app/logs

# Устанавливаем права на выполнение
RUN chmod +x ./ai-chatter ./notion-mcp-server ./gmail-mcp-server

# Создаем скрипт запуска с Docker daemon
RUN echo '#!/bin/sh' > /app/start.sh && \
    echo 'set +e' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Запускаем Docker daemon в фоне' >> /app/start.sh && \
    echo 'echo "🐳 Starting Docker daemon..."' >> /app/start.sh && \
    echo 'dockerd --host=unix:///var/run/docker.sock --iptables=false --bridge=none &' >> /app/start.sh && \
    echo 'DOCKER_PID=$!' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Ждем пока Docker daemon запустится' >> /app/start.sh && \
    echo 'echo "⏳ Waiting for Docker daemon to start..."' >> /app/start.sh && \
    echo 'timeout=30' >> /app/start.sh && \
    echo 'while [ $timeout -gt 0 ]; do' >> /app/start.sh && \
    echo '  if [ -S /var/run/docker.sock ] && docker info >/dev/null 2>&1; then' >> /app/start.sh && \
    echo '    echo "✅ Docker daemon is ready"' >> /app/start.sh && \
    echo '    break' >> /app/start.sh && \
    echo '  fi' >> /app/start.sh && \
    echo '  sleep 1' >> /app/start.sh && \
    echo '  timeout=$((timeout - 1))' >> /app/start.sh && \
    echo 'done' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo 'if [ $timeout -eq 0 ]; then' >> /app/start.sh && \
    echo '  echo "❌ Docker daemon failed to start within 30 seconds"' >> /app/start.sh && \
    echo '  echo "🔧 Falling back to mock mode for code validation"' >> /app/start.sh && \
    echo '  kill $DOCKER_PID 2>/dev/null || true' >> /app/start.sh && \
    echo 'fi' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Устанавливаем переменные окружения для MCP серверов' >> /app/start.sh && \
    echo 'export NOTION_MCP_SERVER_PATH="/app/notion-mcp-server"' >> /app/start.sh && \
    echo 'export GMAIL_MCP_SERVER_PATH="/app/gmail-mcp-server"' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Функция graceful shutdown' >> /app/start.sh && \
    echo 'cleanup() {' >> /app/start.sh && \
    echo '  echo "🛑 Shutting down services..."' >> /app/start.sh && \
    echo '  kill $AI_CHATTER_PID 2>/dev/null || true' >> /app/start.sh && \
    echo '  kill $DOCKER_PID 2>/dev/null || true' >> /app/start.sh && \
    echo '  exit 0' >> /app/start.sh && \
    echo '}' >> /app/start.sh && \
    echo 'trap cleanup SIGTERM SIGINT' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Запускаем AI Chatter бот' >> /app/start.sh && \
    echo 'echo "🤖 Starting AI Chatter bot with Docker support..."' >> /app/start.sh && \
    echo './ai-chatter "$@" &' >> /app/start.sh && \
    echo 'AI_CHATTER_PID=$!' >> /app/start.sh && \
    echo '' >> /app/start.sh && \
    echo '# Ждем завершения бота' >> /app/start.sh && \
    echo 'wait $AI_CHATTER_PID' >> /app/start.sh && \
    chmod +x /app/start.sh

# Открываем порт для Docker daemon (если нужен внешний доступ)
EXPOSE 2375 2376

# Запускаем бот через скрипт с Docker поддержкой
CMD ["/app/start.sh"]
