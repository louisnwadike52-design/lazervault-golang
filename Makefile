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

.PHONY: proto proto-deps clean install-tools

# Install all required protoc plugins #install these manually one by one
install-tools: install-redis
	go install \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest \
		google.golang.org/protobuf/cmd/protoc-gen-go@latest \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest \
		github.com/hibiken/asynq/tools/asynq@latest \
		github.com/go-redis/redis/v8@latest

# Install redis
install-redis:
	brew install redis


# Download proto dependencies
proto-deps:
	./scripts/proto-deps.sh

# Generate proto files
proto: create-dirs
	protoc \
		--proto_path=proto \
		--proto_path=proto/google/api \
		--proto_path=proto/protoc-gen-openapiv2/options \
		--go_out=./pb --go_opt=paths=source_relative \
		--go-grpc_out=./pb --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=./pb --grpc-gateway_opt=paths=source_relative \
		--grpc-gateway_opt=allow_repeated_fields_in_body=true \
		--openapiv2_out=swagger \
		--openapiv2_opt=allow_merge=true,merge_file_name=api \
		proto/*.proto

# Clean generated files
clean:
	rm -rf pb/*.go
	rm -rf swagger/*.json

.PHONY: evans
evans:
	evans --host localhost --port 50051 -r repl

.PHONY: dev
dev:
	nodemon

.PHONY: create-dirs
create-dirs:
	mkdir -p pb
	mkdir -p swagger
