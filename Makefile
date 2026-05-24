.PHONY: build run run-local dev infra infra-down clean test

APP_NAME := epay
BUILD_DIR := ./bin
LOCAL_ENV := DATABASE_HOST=localhost DATABASE_PORT=5432 DATABASE_USER=epay DATABASE_PASSWORD=epay_pass DATABASE_DBNAME=epay DATABASE_SSLMODE=disable REDIS_ADDR=localhost:6379 JWT_SECRET=$${JWT_SECRET:-your-jwt-secret-at-least-32-chars-long}

build:
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

infra:
	@echo "Starting local dependencies (PostgreSQL + Redis)..."
	docker compose up -d postgres redis

infra-down:
	@echo "Stopping local dependencies..."
	docker compose stop postgres redis

run:
	@echo "Starting $(APP_NAME)..."
	@go run ./cmd/server

run-local: infra
	@echo "Starting $(APP_NAME) locally against Docker dependencies..."
	@$(LOCAL_ENV) go run ./cmd/server

dev: infra
	@if command -v air > /dev/null 2>&1; then \
		echo "Starting with air (hot reload)..."; \
		$(LOCAL_ENV) air; \
	else \
		echo "air not found, installing..."; \
		go install github.com/air-verse/air@latest; \
		echo "Starting with air (hot reload)..."; \
		$(LOCAL_ENV) air; \
	fi

test:
	@echo "Running tests..."
	@go test ./... -v -count=1

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"
