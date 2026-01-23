package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// GRPCProxy handles proxying HTTP requests to gRPC microservices
type GRPCProxy struct {
	clientPool *ClientPool
}

// NewGRPCProxy creates a new gRPC proxy
func NewGRPCProxy(clientPool *ClientPool) *GRPCProxy {
	return &GRPCProxy{
		clientPool: clientPool,
	}
}

// ProxyRequest proxies an HTTP request to a gRPC microservice
func (p *GRPCProxy) ProxyRequest(
	c *gin.Context,
	serviceAddr string,
	method string,
	requestProto proto.Message,
	responseProto proto.Message,
) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get gRPC connection
	conn, err := p.clientPool.GetConnection(ctx, serviceAddr)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	// Parse request body into proto message
	if c.Request.ContentLength > 0 {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		if err := json.Unmarshal(bodyBytes, requestProto); err != nil {
			return fmt.Errorf("failed to unmarshal request: %w", err)
		}
	}

	// Forward auth headers to microservice
	md := metadata.New(map[string]string{})
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		md.Set("authorization", authHeader)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Invoke gRPC method
	err = conn.Invoke(ctx, method, requestProto, responseProto)
	if err != nil {
		return fmt.Errorf("gRPC call failed: %w", err)
	}

	// Send response
	c.JSON(http.StatusOK, responseProto)
	return nil
}

// ProxyUnary creates a Gin handler that proxies to a gRPC unary method
func (p *GRPCProxy) ProxyUnary(
	serviceAddr string,
	method string,
	newRequest func() proto.Message,
	newResponse func() proto.Message,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := newRequest()
		resp := newResponse()

		if err := p.ProxyRequest(c, serviceAddr, method, req, resp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}
}
