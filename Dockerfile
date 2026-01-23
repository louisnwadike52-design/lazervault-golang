FROM golang:alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server/main.go || \
    CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/main.go || \
    CGO_ENABLED=0 GOOS=linux go build -o main ./main.go || \
    CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/main .

EXPOSE 50051 8081

CMD ["./main"]
