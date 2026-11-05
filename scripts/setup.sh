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

# --- pinned plugin toolchain -------------------------------------------------
# NOT `go install @latest`. The probe it replaced ("if ! command -v
# protoc-gen-go") always failed, because the plugins live in GOPATH/bin which
# is not on PATH until the export two lines down — so every run installed
# whatever @latest happened to be, into a repo that is NOT on one version.
# The damage, measured 2026-11-03: regenerating one service's protos at
# another's plugin version silently rewrote 13,909 lines (banking.proto) and
# ~5,000 (auth.proto).
#
# `make proto` below emits every pb.go in this gateway from ONE protoc run, so
# one plugin version has to serve them all — this pins the version the
# directory is already predominantly on, and prints what it found.
_TC_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
if [ -f "$_TC_ROOT/scripts/protoc_toolchain.sh" ]; then
    # shellcheck source=/dev/null
    . "$_TC_ROOT/scripts/protoc_toolchain.sh"
    protoc_use_majority_go_plugin "$(cd "$(dirname "$0")/.." && pwd)"
    protoc_require_plugin protoc-gen-go-grpc \
        google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    protoc_require_plugin protoc-gen-grpc-gateway \
        github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
    protoc_require_plugin protoc-gen-openapiv2 \
        github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
else
    echo "WARNING: scripts/protoc_toolchain.sh not found — refusing to install" >&2
    echo "  protoc plugins at @latest, which can rewrite generated code at the" >&2
    echo "  wrong version. Run this from a full checkout." >&2
    exit 1
fi
# -----------------------------------------------------------------------------

# Update PATH to include Go binaries
export PATH="$(go env GOPATH)/bin:$PATH"

# Generate proto files
make proto

echo "Setup completed successfully!" 