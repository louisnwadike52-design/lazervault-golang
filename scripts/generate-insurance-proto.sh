#!/bin/bash

# Generate insurance protobuf files
echo "Generating insurance protobuf files..."

# Generate Go files from proto
protoc --proto_path=proto \
       --go_out=. \
       --go_opt=paths=source_relative \
       --go-grpc_out=. \
       --go-grpc_opt=paths=source_relative \
       proto/insurance.proto

# Generate gRPC-Gateway files
protoc --proto_path=proto \
       --grpc-gateway_out=. \
       --grpc-gateway_opt=paths=source_relative \
       proto/insurance.proto

# Generate OpenAPI/Swagger files
protoc --proto_path=proto \
       --openapiv2_out=swagger \
       --openapiv2_opt=logtostderr=true \
       proto/insurance.proto

echo "Insurance protobuf files generated successfully!" 