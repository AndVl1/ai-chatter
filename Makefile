# AI Chatter - Makefile
.PHONY: help build test clean ci format integration cross coverage mcp-servers mcp-clean

# Переменные
GO_VERSION := $(shell go version | cut -d' ' -f3)
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT := $(shell git rev-parse --short HEAD)

# Цвета для вывода
BLUE := \033[34m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
NC := \033[0m # No Color

help: ## Показать эту справку
	@echo "$(BLUE)AI Chatter - Available commands:$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "$(GREEN)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Build info:$(NC)"
	@echo "  Go version: $(GO_VERSION)"
	@echo "  Git commit: $(GIT_COMMIT)"
	@echo "  Build time: $(BUILD_TIME)"

build: ## Собрать все приложения
	@echo "$(BLUE)🔨 Building applications...$(NC)"
	@go build -o ai-chatter cmd/bot/main.go
	@make mcp-servers
	@go build -o test-custom-mcp cmd/test-custom-mcp/main.go
	@echo "$(GREEN)✅ Build completed$(NC)"

test: ## Запустить unit тесты
	@echo "$(BLUE)🧪 Running unit tests...$(NC)"
	@go test -race -v ./...

coverage: ## Запустить тесты с coverage
	@echo "$(BLUE)📊 Running tests with coverage...$(NC)"
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✅ Coverage report: coverage.html$(NC)"

mcp-servers: ## Собрать все MCP серверы
	@echo "$(BLUE)🔧 Building MCP servers...$(NC)"
	@mkdir -p bin
	@go build -o bin/notion-mcp-server cmd/notion-mcp-server/main.go
	@go build -o bin/gmail-mcp-server cmd/gmail-mcp-server/main.go
	@go build -o bin/github-mcp-server cmd/github-mcp-server/main.go
	@go build -o bin/rustore-mcp-server cmd/rustore-mcp-server/main.go
	@go build -o bin/vibecoding-mcp-server cmd/vibecoding-mcp-server/main.go
	@go build -o bin/vibecoding-mcp-http-server cmd/vibecoding-mcp-http-server/main.go
	@echo "$(GREEN)✅ MCP servers built in bin/ directory$(NC)"

mcp-clean: ## Очистить собранные MCP серверы
	@echo "$(BLUE)🧹 Cleaning MCP servers...$(NC)"
	@rm -rf bin/
	@echo "$(GREEN)✅ MCP servers cleaned$(NC)"

integration: ## Запустить интеграционные тесты
	@echo "$(BLUE)🌐 Running integration tests...$(NC)"
	@./scripts/test-notion-integration.sh

format: ## Проверить и исправить форматирование кода
	@echo "$(BLUE)🎨 Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✅ Code formatted$(NC)"

clean: ## Очистить артефакты сборки
	@echo "$(BLUE)🧹 Cleaning up...$(NC)"
	@rm -f ai-chatter test-custom-mcp
	@make mcp-clean
	@rm -f coverage.out coverage.html *.prof *.log
	@echo "$(GREEN)✅ Cleanup completed$(NC)"

ci: ## Запустить полный CI pipeline локально
	@echo "$(BLUE)🚀 Running full CI pipeline...$(NC)"
	@./scripts/ci-local.sh

ci-fast: format build test ## Быстрая проверка (форматирование + сборка + unit тесты)
	@echo "$(GREEN)✅ Fast CI check completed$(NC)"

cross: ## Проверить cross-platform сборку
	@echo "$(BLUE)🌍 Cross-platform build check...$(NC)"
	@GOOS=linux GOARCH=amd64 go build -o /dev/null cmd/bot/main.go
	@GOOS=darwin GOARCH=amd64 go build -o /dev/null cmd/bot/main.go  
	@GOOS=windows GOARCH=amd64 go build -o /dev/null cmd/bot/main.go
	@echo "$(GREEN)✅ Cross-platform builds OK$(NC)"

deps: ## Обновить зависимости
	@echo "$(BLUE)📦 Updating dependencies...$(NC)"
	@go mod download
	@go mod tidy
	@echo "$(GREEN)✅ Dependencies updated$(NC)"

lint: ## Запустить линтер (если установлен)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(BLUE)🔍 Running linter...$(NC)"; \
		golangci-lint run ./...; \
		echo "$(GREEN)✅ Linting completed$(NC)"; \
	else \
		echo "$(YELLOW)⚠️ golangci-lint not installed$(NC)"; \
		echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

dev: build ## Запустить в режиме разработки
	@echo "$(BLUE)🚀 Starting development server...$(NC)"
	@echo "$(YELLOW)Loading .env file...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./ai-chatter

mcp-server: build ## Запустить MCP сервер (Notion)
	@echo "$(BLUE)🔌 Starting Notion MCP server...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./notion-mcp-server

vibe-mcp: build ## Запустить VibeCoding MCP сервер (stdio)
	@echo "$(BLUE)🎯 Starting VibeCoding MCP server (stdio)...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./vibecoding-mcp-server

vibe-http: build ## Запустить VibeCoding HTTP SSE MCP сервер
	@echo "$(BLUE)🌐 Starting VibeCoding HTTP SSE MCP server...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./vibecoding-mcp-http-server

install: build ## Установить в GOPATH/bin
	@echo "$(BLUE)📥 Installing to GOPATH/bin...$(NC)"
	@go install cmd/bot/main.go
	@go install cmd/notion-mcp-server/main.go
	@go install cmd/vibecoding-mcp-server/main.go
	@go install cmd/vibecoding-mcp-http-server/main.go
	@echo "$(GREEN)✅ Installation completed$(NC)"

docker-single: ## Собрать один Docker образ
	@echo "$(BLUE)🐳 Building single Docker image...$(NC)"
	@docker build -t ai-chatter:$(GIT_COMMIT) .
	@docker tag ai-chatter:$(GIT_COMMIT) ai-chatter:latest
	@echo "$(GREEN)✅ Docker image built$(NC)"

release: clean format build test cross ## Подготовить релиз
	@echo "$(BLUE)🏷️ Preparing release...$(NC)"
	@echo "$(YELLOW)Version: $(GIT_COMMIT)$(NC)"
	@echo "$(YELLOW)Build time: $(BUILD_TIME)$(NC)"
	@echo "$(GREEN)✅ Release ready$(NC)"

benchmark: ## Запустить бенчмарки
	@echo "$(BLUE)⚡ Running benchmarks...$(NC)"
	@go test -bench=. -benchmem ./... | tee benchmark-results.txt
	@echo "$(GREEN)✅ Benchmarks completed$(NC)"

profile-cpu: ## CPU профилирование
	@echo "$(BLUE)🔥 CPU profiling...$(NC)"
	@go test -cpuprofile=cpu.prof -run=TestMCPConnection ./internal/notion
	@if [ -f cpu.prof ]; then \
		echo "$(GREEN)📊 CPU profile: cpu.prof$(NC)"; \
		echo "View with: go tool pprof cpu.prof"; \
	fi

profile-mem: ## Memory профилирование  
	@echo "$(BLUE)🧠 Memory profiling...$(NC)"
	@go test -memprofile=mem.prof -run=TestMCPConnection ./internal/notion
	@if [ -f mem.prof ]; then \
		echo "$(GREEN)📊 Memory profile: mem.prof$(NC)"; \
		echo "View with: go tool pprof mem.prof"; \
	fi

# Docker команды
docker-build: ## Собрать все Docker образы
	@echo "$(BLUE)🐳 Building Docker images...$(NC)"
	@docker-compose -f docker-compose.full.yml build
	@echo "$(GREEN)✅ Docker images built$(NC)"

start: ## Запустить всю систему (полная конфигурация)
	@echo "$(BLUE)🚀 Starting full AI Chatter system...$(NC)"
	@./start-ai-chatter.sh

start-basic: ## Запустить только основной бот
	@echo "$(BLUE)🤖 Starting basic AI Chatter bot...$(NC)"
	@./start-ai-chatter.sh basic

start-vibe: ## Запустить с VibeCoding
	@echo "$(BLUE)🔥 Starting AI Chatter with VibeCoding...$(NC)"
	@./start-ai-chatter.sh vibecoding

stop: ## Остановить все Docker контейнеры
	@echo "$(BLUE)🛑 Stopping AI Chatter system...$(NC)"
	@docker-compose -f docker-compose.full.yml down 2>/dev/null || true
	@docker-compose -f docker-compose.vibecoding.yml down 2>/dev/null || true
	@docker-compose -f docker-compose.yml down 2>/dev/null || true
	@echo "$(GREEN)✅ System stopped$(NC)"

logs: ## Показать логи всех сервисов
	@echo "$(BLUE)📋 Showing logs...$(NC)"
	@docker-compose -f docker-compose.full.yml logs -f

status: ## Показать статус всех контейнеров
	@echo "$(BLUE)📊 System status:$(NC)"
	@docker-compose -f docker-compose.full.yml ps 2>/dev/null || echo "$(YELLOW)No containers running$(NC)"

restart: stop start ## Перезапустить систему

clean-docker: ## Очистить Docker данные
	@echo "$(BLUE)🧹 Cleaning Docker data...$(NC)"
	@docker system prune -f
	@docker volume prune -f
	@echo "$(GREEN)✅ Docker cleanup completed$(NC)"

# Aliases для удобства
all: ci ## Alias для 'ci'
check: ci-fast ## Alias для 'ci-fast'
fmt: format ## Alias для 'format'
run: start ## Alias для 'start'
