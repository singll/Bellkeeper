.PHONY: build build-backend build-frontend run test clean docker-build docker-up docker-down migrate dev dev-frontend

# Variables
BINARY_NAME=bellkeeper
MAIN_PATH=./cmd/bellkeeper
DOCKER_COMPOSE=docker compose -f docker/docker-compose.yml

# Build all
build: build-backend build-frontend

# Build backend
build-backend:
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

# Build frontend
build-frontend:
	cd web && pnpm install && pnpm build

# Run locally
run:
	go run $(MAIN_PATH) serve

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air

# Run frontend dev server
dev-frontend:
	cd web && pnpm dev

# Test
test:
	go test -v ./...

# Test with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Format code
fmt:
	go fmt ./...

# Lint (requires golangci-lint)
lint:
	golangci-lint run

# Generate swagger docs (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)
swagger:
	swag init -g cmd/bellkeeper/main.go -o api/docs

# Docker
docker-build:
	docker build -t bellkeeper:latest -f docker/Dockerfile .

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f

# Database
migrate:
	go run $(MAIN_PATH) migrate

# Install dependencies
deps:
	go mod download
	go mod tidy

# All in one: format, lint, test, build
all: fmt lint test build
