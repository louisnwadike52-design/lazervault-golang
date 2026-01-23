//go:build ignore
// +build ignore

package main

import (
	accountspb "accounts-service/proto"
	authpb "auth-service/proto"
	"context"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// This file tests that proto imports are configured correctly
func main() {
	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{}

	// These function calls verify that the proto packages are importable
	// and that the Register functions exist
	_ = authpb.RegisterAuthServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", opts)
	_ = accountspb.RegisterAccountsServiceHandlerFromEndpoint(ctx, mux, "localhost:50052", opts)
}
