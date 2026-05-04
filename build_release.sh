#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

echo "[1/5] Build web frontend"
cd web
npm ci
npm run build
cd "$ROOT_DIR"

echo "[2/5] Sync frontend dist for go:embed"
mkdir -p cmd/netpulse/web
rm -rf cmd/netpulse/web/dist
cp -R web/dist cmd/netpulse/web/

echo "[3/5] Build Go binary check"
go build ./cmd/netpulse

echo "[4/5] Build Docker image netpulse:latest"
docker build -t netpulse:latest .

echo "[5/5] Done"
echo "Image ready: netpulse:latest"
echo "Note: runtime image includes tzdata + postgresql-client (for backup/restore)."
