package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

// LoadClientTLSCredentials loads mTLS client credentials for gateway
// Returns insecure credentials if mTLS is disabled
func LoadClientTLSCredentials(enableMTLS bool, certDir string) (credentials.TransportCredentials, error) {
	if !enableMTLS {
		return nil, nil // Will use insecure credentials
	}

	// Load CA certificate
	caCert, err := os.ReadFile(fmt.Sprintf("%s/ca.crt", certDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Determine gateway name from working directory
	gatewayName := os.Getenv("GATEWAY_NAME")
	if gatewayName == "" {
		gatewayName = "core-gateway" // default
	}

	// Load client certificate and private key
	clientCert, err := tls.LoadX509KeyPair(
		fmt.Sprintf("%s/%s-client.crt", certDir, gatewayName),
		fmt.Sprintf("%s/%s-client.key", certDir, gatewayName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Create TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13, // Enforce TLS 1.3 for security
	}

	return credentials.NewTLS(config), nil
}

// GetDialOptions returns appropriate gRPC dial options based on mTLS configuration
func GetDialOptions(enableMTLS bool, certDir string) ([]interface{}, error) {
	creds, err := LoadClientTLSCredentials(enableMTLS, certDir)
	if err != nil {
		return nil, err
	}

	if creds != nil {
		return []interface{}{creds}, nil
	}

	// Return empty slice for insecure mode
	return []interface{}{}, nil
}
