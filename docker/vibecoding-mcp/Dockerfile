# Dockerfile для VibeCoding MCP сервера
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем VibeCoding MCP сервер
RUN go build -o vibecoding-mcp-server ./cmd/vibecoding-mcp-server/

# Финальный образ
FROM alpine:latest

# Устанавливаем необходимые пакеты
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Копируем исполняемый файл
COPY --from=builder /app/vibecoding-mcp-server .

# Создаем директорию для сокетов
RUN mkdir -p /tmp/vibecoding-mcp

# Открываем порт для MCP сервера
EXPOSE 8090

# Запускаем VibeCoding MCP сервер
CMD ["./vibecoding-mcp-server"]