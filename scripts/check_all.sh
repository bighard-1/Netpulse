#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "[1/5] go test"
cd "$ROOT_DIR"
go test ./...

echo "[2/5] go vet"
go vet ./...

echo "[3/5] go build"
go build ./...

echo "[4/5] web lint"
npm --prefix "$ROOT_DIR/web" run lint

echo "[5/5] web build"
npm --prefix "$ROOT_DIR/web" run build

echo "All checks passed."
