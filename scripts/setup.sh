#!/bin/bash

# Create necessary directories if they don't exist
# mkdir -p proto
mkdir -p pb
# mkdir -p grpc
# mkdir -p api
# mkdir -p models
# mkdir -p validators
# mkdir -p configs
# mkdir -p database
# mkdir -p utils
# mkdir -p token
# mkdir -p onboarding
# mkdir -p mail
# mkdir -p services

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "protoc is not installed. Installing..."
    brew install protobuf
fi

# Check if protoc-gen-go and protoc-gen-go-grpc are installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Update PATH to include Go binaries
export PATH="$PATH:$(go env GOPATH)/bin"

# Generate proto files
protoc --proto_path=proto \
    --go_out=pb --go_opt=paths=source_relative \
    --go-grpc_out=pb --go-grpc_opt=paths=source_relative \
    proto/*.proto

echo "Setup completed successfully!" 