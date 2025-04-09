# Variables
BINARY_NAME=lazervaultGo
DB_NAME=lazervault_db

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Main commands
.PHONY: all build clean run watch test

all: clean build

build:
	@echo "Building..."
	@go build -o $(GOBIN)/$(BINARY_NAME) main.go

clean:
	@echo "Cleaning..."
	@rm -rf $(GOBIN)
	@go clean

run:
	@go run main.go

# Watch mode using air
watch:
	@echo "Starting in watch mode..."
	@air

# Database commands
.PHONY: db-create db-drop db-reset

db-create:
	@echo "Creating database..."
	@createdb -U postgres $(DB_NAME)

db-drop:
	@echo "Dropping database..."
	@dropdb -U postgres $(DB_NAME) --if-exists

db-reset: db-drop db-create

# Development setup
.PHONY: setup install-air

setup:
	@echo "Setting up development environment..."
	@./scripts/setup.sh

install-air:
	@echo "Installing air..."
	@go install github.com/air-verse/air@latest

migrate:
	@echo "Migrating..."
	@go run main.go -migrate

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make build      - Build the application"
	@echo "  make clean      - Clean build files"
	@echo "  make run        - Run the application"
	@echo "  make watch      - Run in watch mode using air"
	@echo "  make db-create  - Create database"
	@echo "  make db-drop    - Drop database"
	@echo "  make db-reset   - Reset database"
	@echo "  make setup      - Setup development environment"

.PHONY: proto
proto:
	protoc --proto_path=proto \
		--go_out=pb --go_opt=paths=source_relative \
		--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
		proto/*.proto

.PHONY: evans
evans:
	evans --host localhost --port 50051 -r repl

.PHONY: dev
dev:
	nodemon
