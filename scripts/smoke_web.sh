#!/usr/bin/env bash
set -euo pipefail

# NetPulse web/api smoke test
# Usage:
#   BASE_URL=http://127.0.0.1:8080/api USERNAME=admin PASSWORD=admin123 ./scripts/smoke_web.sh
# Optional:
#   DEVICE_ID=7 PORT_ID=12 ./scripts/smoke_web.sh

BASE_URL="${BASE_URL:-http://127.0.0.1:8080/api}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
DEVICE_ID="${DEVICE_ID:-}"
PORT_ID="${PORT_ID:-}"

echo "[1/6] login: ${BASE_URL}"
LOGIN_JSON="$(curl -fsS -X POST "${BASE_URL}/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(printf '%s' "${LOGIN_JSON}" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
if [[ -z "${TOKEN}" ]]; then
  echo "login failed: token missing"
  exit 1
fi
AUTH=(-H "Authorization: Bearer ${TOKEN}")
echo "ok: token acquired"

echo "[2/6] list devices"
DEVICES_JSON="$(curl -fsS "${BASE_URL}/devices" "${AUTH[@]}")"
echo "ok"

if [[ -z "${DEVICE_ID}" ]]; then
  DEVICE_ID="$(printf '%s' "${DEVICES_JSON}" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1 || true)"
fi

echo "[3/6] list recent events"
curl -fsS "${BASE_URL}/events/recent?limit=5" "${AUTH[@]}" >/dev/null
echo "ok"

echo "[4/6] health trend"
END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
START="$(date -u -v-30d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)"
curl -fsS "${BASE_URL}/system/health?start=${START}&end=${END}" "${AUTH[@]}" >/dev/null
echo "ok"

echo "[5/6] list templates"
curl -fsS "${BASE_URL}/templates" "${AUTH[@]}" >/dev/null
echo "ok"

echo "[6/6] read runtime settings"
curl -fsS "${BASE_URL}/settings/runtime" "${AUTH[@]}" >/dev/null
echo "ok"

if [[ -n "${DEVICE_ID}" ]]; then
  echo "[extra] device detail: ${DEVICE_ID}"
  DETAIL_JSON="$(curl -fsS "${BASE_URL}/devices/${DEVICE_ID}" "${AUTH[@]}")"
  echo "ok"

  if [[ -z "${PORT_ID}" ]]; then
    PORT_ID="$(printf '%s' "${DETAIL_JSON}" | sed -n 's/.*"interfaces":[[]\([^]]*\)[]].*/\1/p' | sed -n 's/.*"id":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1 || true)"
  fi

  if [[ -n "${PORT_ID}" ]]; then
    echo "[extra] port history: ${PORT_ID}"
    curl -fsS "${BASE_URL}/metrics/history?type=traffic&id=${PORT_ID}&start=${START}&end=${END}" "${AUTH[@]}" >/dev/null
    echo "ok"
  fi
fi

echo "smoke passed"
