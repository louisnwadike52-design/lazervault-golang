package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ClientPool manages gRPC client connections to microservices
type ClientPool struct {
	connections map[string]*grpc.ClientConn
	mu          sync.RWMutex
}

// NewClientPool creates a new client pool
func NewClientPool() *ClientPool {
	return &ClientPool{
		connections: make(map[string]*grpc.ClientConn),
	}
}

// GetConnection returns a gRPC connection to the specified address
func (p *ClientPool) GetConnection(ctx context.Context, address string) (*grpc.ClientConn, error) {
	p.mu.RLock()
	conn, exists := p.connections[address]
	p.mu.RUnlock()

	if exists && conn.GetState().String() != "SHUTDOWN" {
		return conn, nil
	}

	// Create new connection
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := p.connections[address]; exists && conn.GetState().String() != "SHUTDOWN" {
		return conn, nil
	}

	// Create connection with production-ready options
	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
			grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
		),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", address, err)
	}

	p.connections[address] = conn
	fmt.Printf("✅ Established gRPC connection to %s\n", address)

	return conn, nil
}

// Close closes all connections in the pool
func (p *ClientPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for addr, conn := range p.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing connection to %s: %w", addr, err))
		} else {
			fmt.Printf("🔌 Closed connection to %s\n", addr)
		}
	}

	p.connections = make(map[string]*grpc.ClientConn)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}
