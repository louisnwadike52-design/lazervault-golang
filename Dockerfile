# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies: git for go modules, make for build scripts, protobuf-dev for protoc
RUN apk update && apk add --no-cache make git protobuf-dev

# Install protoc Go plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest

# Copy go.mod and go.sum first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod tidy && go mod verify

# Copy the rest of the application source code
COPY . .

# Generate protobuf files and build the application
# Ensure your Makefile has 'proto' and 'build' targets
RUN make proto
# Build the Go application statically within Docker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/bin/lazervaultGo main.go

# Stage 2: Create the final lightweight image
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies: ca-certificates for HTTPS
RUN apk update && apk add --no-cache ca-certificates

# Create destination directories for swagger and pb explicitly
RUN mkdir -p /app/swagger /app/pb

# Copy necessary artifacts from the builder stage
COPY --from=builder /app/bin/lazervaultGo /app/lazervaultGo
COPY .env /app/.env 
COPY --from=builder /app/swagger/ /app/swagger/ 
COPY --from=builder /app/pb/ /app/pb/         

# Make the Go application binary executable
RUN chmod +x /app/lazervaultGo

# Expose the ports your application will listen on inside the container.
# These should correspond to GRPC_SERVER_PORT and HTTP_SERVER_PORT from your bundled .env file.
EXPOSE 7007 8080

# Command to run your Go application binary (or arguments to entrypoint)
CMD ["/app/lazervaultGo"]