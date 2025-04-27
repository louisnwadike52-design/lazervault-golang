#!/bin/bash

# Create directories if they don't exist
mkdir -p proto/google/api
mkdir -p proto/protoc-gen-openapiv2/options
mkdir -p proto/validate
mkdir -p swagger

# Download Google APIs
curl -o proto/google/api/annotations.proto https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/annotations.proto
curl -o proto/google/api/http.proto https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/http.proto
curl -o proto/google/api/field_behavior.proto https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/field_behavior.proto

# Download gRPC-Gateway protos
curl -o proto/protoc-gen-openapiv2/options/annotations.proto https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/annotations.proto
curl -o proto/protoc-gen-openapiv2/options/openapiv2.proto https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/openapiv2.proto

# Make the script executable
chmod +x scripts/proto-deps.sh