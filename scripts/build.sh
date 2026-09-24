#!/usr/bin/env bash
# Build all Go binaries into ./bin (go build ./... is also run to verify every package).
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin
go vet ./...
go build ./...
go build -o bin/app ./cmd/app
go build -o bin/nm-host ./cmd/nm-host
echo "built: $(ls bin | tr '\n' ' ')"
