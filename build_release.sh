#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

IMAGE_TAG="${IMAGE_TAG:-netpulse:v1.0.0-pro}"

echo "[1/7] Build web frontend"
cd web
npm ci
npm run build
cd "$ROOT_DIR"

echo "[2/7] Sync frontend dist for go:embed"
mkdir -p cmd/netpulse/web
rm -rf cmd/netpulse/web/dist
cp -R web/dist cmd/netpulse/web/

echo "[3/7] Build Go binary check"
go build ./cmd/netpulse

echo "[4/7] Verify static embed entrypoint"
grep -q '//go:embed all:web/dist' cmd/netpulse/main.go
grep -q 'fs.Sub(embeddedWebFS, "web/dist")' cmd/netpulse/main.go
grep -q 'rootMux.Handle("/api/", handler.Router())' cmd/netpulse/main.go

echo "[5/7] Verify alert suppression wiring"
grep -q 'ShouldSuppress' internal/snmp/alert_manager.go
grep -q 'emitAlert' internal/snmp/worker.go
grep -q 'ALERT_SUPPRESSED' internal/snmp/worker.go

echo "[6/7] Build Docker image ${IMAGE_TAG}"
docker build -t "${IMAGE_TAG}" .

echo "[7/7] Done"
echo "Image ready: ${IMAGE_TAG}"
echo "Note: runtime image includes tzdata + postgresql-client (for backup/restore)."
