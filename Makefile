# Variables
BINARY_LINUX := bin/app-linux-amd64
BINARY_DARWIN := bin/app-darwin-arm64
APP_NAME := app
APP_PORT := 8081
DOCKER_COMPOSE := docker-compose

.PHONY: build build-linux build-darwin run up down clean logs

# Статическая сборка для Linux (для Docker)
build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-w -s" \
		-o $(BINARY_LINUX) main.go
	@chmod +x $(BINARY_LINUX)
	@echo "Built: $(BINARY_LINUX)"
	@file $(BINARY_LINUX) || true

# Сборка для macOS
build-darwin:
	@echo "Building for macOS (arm64)..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-ldflags="-w -s" \
		-o $(BINARY_DARWIN) main.go
	@chmod +x $(BINARY_DARWIN)
	@echo "Built: $(BINARY_DARWIN)"

# Сборка для обеих платформ
build: clean build-linux build-darwin

rebuild: down
	@echo "Removing old images..."
	docker-compose down --rmi all --volumes --remove-orphans 2>/dev/null || true
	@echo "Cleaning..."
	make clean
	@echo "Building without cache..."
	docker-compose build --no-cache app
	@echo "Starting services..."
	docker-compose up -d
	@echo "Rebuild complete. Use 'make logs' to see logs."

rebuild-app:
	@echo "Stopping app..."
	docker-compose stop app
	docker-compose rm -f app 2>/dev/null || true
	@echo "Building app without cache..."
	docker-compose build --no-cache app
	@echo "Starting app..."
	docker-compose up -d app
	@echo "App rebuilt. Use 'make logs' to see logs."

# Локальный запуск
run:
	@echo "Starting server locally..."
	go run main.go

# Запуск в Docker с пересборкой
up: build-linux
	@echo "Starting Docker Compose..."
	$(DOCKER_COMPOSE) up -d
	@echo "Services started. Use 'make logs' to see logs."

# Запуск без сборки (если уже собрано)
up-no-build:
	@echo "Starting Docker Compose without build..."
	$(DOCKER_COMPOSE) up -d

# Остановка всех сервисов
down:
	@echo "Stopping Docker Compose..."
	$(DOCKER_COMPOSE) down --remove-orphans

# Полная остановка с удалением volumes
down-clean:
	@echo "Stopping Docker Compose and removing volumes..."
	$(DOCKER_COMPOSE) down --remove-orphans --volumes

# Просмотр логов приложения
logs:
	@echo "Tailing logs for $(APP_NAME)..."
	$(DOCKER_COMPOSE) logs -f $(APP_NAME)

# Просмотр логов всех сервисов
logs-all:
	@echo "Tailing logs for all services..."
	$(DOCKER_COMPOSE) logs -f

# Проверка статуса контейнеров
status:
	$(DOCKER_COMPOSE) ps

# Перезапуск только приложения
restart-app:
	@echo "Restarting application..."
	$(DOCKER_COMPOSE) restart $(APP_NAME)

# Сборка Docker образа
docker-build:
	@echo "Building Docker image..."
	docker build -t $(APP_NAME):latest .

# Очистка бинарников
clean:
	@echo "Cleaning binaries..."
	rm -rf ./bin

# Полная очистка
clean-all: down-clean clean
	@echo "Cleaning Docker resources..."
	docker system prune -f --volumes

# Проверка зависимостей
deps:
	@echo "Checking dependencies..."
	go mod tidy
	go mod verify

# Проверка подключения к БД
check-db:
	@echo "Checking database connection..."
	$(DOCKER_COMPOSE) exec postgresql pg_isready -U postgres

# Просмотр метрик
metrics:
	@echo "Prometheus metrics available at: http://localhost:9090"
	@echo "Grafana available at: http://localhost:3000 (admin/admin)"
	@echo "Application metrics at: http://localhost:$(APP_PORT)/metrics"

# Быстрый тест API
test-api:
	@echo "Testing API endpoints..."
	@echo "1. Checking health/metrics..."
	curl -f http://localhost:$(APP_PORT)/metrics || echo "Failed to connect"
	@echo ""
	@echo "Use 'curl -X POST http://localhost:$(APP_PORT)/api/deposit' for deposit"
	@echo "Use 'curl -X POST http://localhost:$(APP_PORT)/api/withdraw' for withdraw"

# ============ Redis Mode Management ============

redis-on:
	@echo "Enabling Redis mode..."
	@sed -i 's/REDIS_ENABLED: .*/REDIS_ENABLED: "true"/' docker-compose.yaml
	@echo "Redis ENABLED"

redis-off:
	@echo "Disabling Redis mode..."
	@sed -i 's/REDIS_ENABLED: .*/REDIS_ENABLED: "false"/' docker-compose.yaml
	@echo "Redis DISABLED"

redis-toggle:
	@current=$$(grep "REDIS_ENABLED:" docker-compose.yaml | grep -o '"true"\|"false"'); \
	if [ "$$current" = '"true"' ]; then \
		$(MAKE) redis-off; \
	else \
		$(MAKE) redis-on; \
	fi

redis-status:
	@echo "Current Redis mode:"
	@grep "REDIS_ENABLED:" docker-compose.yaml

# Быстрый рестарт с новым режимом
rebuild-with-redis-on: redis-on rebuild-app
rebuild-with-redis-off: redis-off rebuild-app

# ============ RedLock Testing ============
# Переменные для тестирования
TEST_URL ?= http://localhost:$(APP_PORT)
TEST_USER_ID ?= 1
TEST_PARALLEL ?= 5
TEST_AMOUNT ?= 100.00

# Тест: 5 параллельных deposits
test-redlock-deposit:
	@echo "=== Testing Redlock: $(TEST_PARALLEL) Parallel Deposits ==="
	@echo "User ID: $(TEST_USER_ID), Amount: $(TEST_AMOUNT)"
	@echo ""
	@for i in $$(seq 1 $(TEST_PARALLEL)); do \
		curl -s -X POST $(TEST_URL)/api/deposit \
		-H "Content-Type: application/json" \
		-d "{\"user_id\": $(TEST_USER_ID), \"amount\": $(TEST_AMOUNT)}" \
		-w "Request $$i - Status: %{http_code} - Time: %{time_total}s\n" & \
	done; \
	wait; \
	echo ""; \
	echo "All $(TEST_PARALLEL) deposit requests completed"

# Тест: 5 параллельных withdraws
test-redlock-withdraw:
	@echo "=== Testing Redlock: $(TEST_PARALLEL) Parallel Withdraws ==="
	@echo "User ID: $(TEST_USER_ID), Amount: $(TEST_AMOUNT)"
	@echo ""
	@for i in $$(seq 1 $(TEST_PARALLEL)); do \
		curl -s -X POST $(TEST_URL)/api/withdraw \
		-H "Content-Type: application/json" \
		-d "{\"user_id\": $(TEST_USER_ID), \"amount\": $(TEST_AMOUNT)}" \
		-w "Request $$i - Status: %{http_code} - Time: %{time_total}s\n" & \
	done; \
	wait; \
	echo ""; \
	echo "All $(TEST_PARALLEL) withdraw requests completed"

# Быстрый одноразовый тест (по умолчанию 5 запросов)
test-redlock:
	@echo "=== Quick Redlock Test (default: 5 parallel deposits) ==="
	@echo "Usage: make test-redlock TEST_PARALLEL=10 TEST_USER_ID=5 TEST_AMOUNT=50.00"
	@echo ""
	@$(MAKE) test-redlock-deposit

# Help
help:
	@echo "Available commands:"
	@echo ""
	@echo "=== BUILD & RUN ==="
	@echo " make build - Build for Linux and macOS"
	@echo " make build-linux - Build only for Linux (Docker)"
	@echo " make rebuild - Rebuild all Docker services"
	@echo " make rebuild-app - Rebuild only app"
	@echo " make run - Run locally"
	@echo " make up - Build and start Docker services"
	@echo " make down - Stop Docker services"
	@echo ""
	@echo "=== LOGS & STATUS ==="
	@echo " make logs - View application logs"
	@echo " make logs-all - View all service logs"
	@echo " make status - Check container status"
	@echo ""
	@echo "=== REDIS MODE ==="
	@echo " make redis-on - Enable Redis Redlock mode"
	@echo " make redis-off - Disable Redis Redlock mode"
	@echo " make redis-toggle - Toggle Redis mode"
	@echo " make redis-status - Show current Redis mode"
	@echo ""
	@echo "=== REDLOCK TESTING ==="
	@echo " make test-redlock - Quick test (5 parallel deposits)"
	@echo " make test-redlock-deposit - Test parallel deposits"
	@echo " make test-redlock-withdraw - Test parallel withdraws"
	@echo ""
	@echo "=== OTHER ==="
	@echo " make test-api - Test API endpoints"
	@echo " make metrics - Show monitoring URLs"
	@echo " make clean - Remove binaries"
	@echo " make help - Show this help"

# По умолчанию показываем help
.DEFAULT_GOAL := help
