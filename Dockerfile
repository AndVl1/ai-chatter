# Dockerfile для AI Chatter Bot
FROM golang:1.21-alpine AS builder

# Устанавливаем зависимости для сборки
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Копируем модули и загружаем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарный файл
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ai-chatter cmd/bot/main.go

# Минимальный образ для production
FROM alpine:latest

# Устанавливаем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Копируем бинарный файл из builder
COPY --from=builder /app/ai-chatter .

# Копируем необходимые файлы
COPY --from=builder /app/prompts ./prompts

# Создаем директории для данных и логов
RUN mkdir -p /app/data /app/logs

# Устанавливаем права на выполнение
RUN chmod +x ./ai-chatter

# Запускаем бот
CMD ["./ai-chatter"]
