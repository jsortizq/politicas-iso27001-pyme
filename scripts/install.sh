#!/usr/bin/env bash
# install.sh — Build and install the iso-cli tool
# Usage: ./scripts/install.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BINARY_NAME="iso-cli"

echo "╔══════════════════════════════════════════════════════════╗"
echo "║        iso-cli — ISO 27001:2022 Compliance Tool         ║"
echo "║                    Build & Install                       ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# Check Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go 1.21+ from https://go.dev"
    exit 1
fi

GO_VERSION=$(go version | grep -oP '\d+\.\d+')
echo "  Go version: $GO_VERSION"
echo "  Building $BINARY_NAME..."

cd "$REPO_ROOT"
go build -o "$BINARY_NAME" ./cmd/iso-cli/

echo "  Build successful: ./$BINARY_NAME"
echo ""

# Optional: install to GOPATH/bin
read -p "  Install to \$GOPATH/bin? [y/N] " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    go install ./cmd/iso-cli/
    echo "  Installed to $(go env GOPATH)/bin/$BINARY_NAME"
fi

echo ""
echo "  Run './iso-cli help' to get started."
