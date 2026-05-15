.PHONY: build run dev clean test

APP_NAME := epay
BUILD_DIR := ./bin

build:
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

run:
	@echo "Starting $(APP_NAME)..."
	@go run ./cmd/server

dev:
	@if command -v air > /dev/null 2>&1; then \
		echo "Starting with air (hot reload)..."; \
		air; \
	else \
		echo "air not found, installing..."; \
		go install github.com/air-verse/air@latest; \
		echo "Starting with air (hot reload)..."; \
		air; \
	fi

test:
	@echo "Running tests..."
	@go test ./... -v -count=1

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"
