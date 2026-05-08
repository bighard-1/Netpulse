#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "[1/4] go test"
cd "$ROOT_DIR"
go test ./...

echo "[2/4] go build"
go build ./...

echo "[3/4] web lint"
cd "$ROOT_DIR/web"
pnpm lint

echo "[4/4] web build"
pnpm build

echo "All checks passed."
