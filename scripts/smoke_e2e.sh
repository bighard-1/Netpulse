#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080/api}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
DEVICE_IP="${DEVICE_IP:-172.24.134.45}"
DEVICE_BRAND="${DEVICE_BRAND:-H3C}"
DEVICE_COMMUNITY="${DEVICE_COMMUNITY:-public}"

request() {
  local method="$1" url="$2" data="${3:-}" auth="${4:-}"
  if [[ -n "$data" ]]; then
    curl -sS -X "$method" "$url" \
      -H 'Content-Type: application/json' \
      ${auth:+-H "Authorization: Bearer ${auth}"} \
      -d "$data" -w "\n%{http_code}"
  else
    curl -sS -X "$method" "$url" \
      ${auth:+-H "Authorization: Bearer ${auth}"} \
      -w "\n%{http_code}"
  fi
}

echo "[1/6] Login as admin"
RESP="$(request POST "${BASE_URL}/auth/login" "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}")"
BODY="$(echo "$RESP" | sed '$d')"
CODE="$(echo "$RESP" | tail -n1)"
if [[ "$CODE" != "200" ]]; then
  echo "Login failed with HTTP $CODE: $BODY"
  exit 1
fi
TOKEN="$(echo "$BODY" | sed -n 's/.*\"token\":\"\\([^\"]*\\)\".*/\\1/p')"
if [[ -z "${TOKEN}" ]]; then
  echo "Login token missing"
  exit 1
fi

echo "[2/6] Add device"
RESP="$(request POST "${BASE_URL}/devices" "{
  \"ip\":\"${DEVICE_IP}\",
  \"brand\":\"${DEVICE_BRAND}\",
  \"community\":\"${DEVICE_COMMUNITY}\",
  \"snmp_version\":\"2c\",
  \"snmp_port\":161,
  \"remark\":\"smoke-test\"
}" "${TOKEN}")"
BODY="$(echo "$RESP" | sed '$d')"
CODE="$(echo "$RESP" | tail -n1)"
if [[ "$CODE" != "201" ]]; then
  echo "Add device failed with HTTP $CODE: $BODY"
  exit 1
fi
DEVICE_ID="$(echo "${BODY}" | sed -n 's/.*\"id\":\\([0-9]*\\).*/\\1/p')"
if [[ -z "${DEVICE_ID}" ]]; then
  echo "Add device response missing id: ${BODY}"
  exit 1
fi

echo "[3/6] Get device detail"
RESP="$(request GET "${BASE_URL}/devices/${DEVICE_ID}" "" "${TOKEN}")"
BODY="$(echo "$RESP" | sed '$d')"
CODE="$(echo "$RESP" | tail -n1)"
if [[ "$CODE" != "200" ]]; then
  echo "Get device detail failed HTTP $CODE: $BODY"
  exit 1
fi
echo "$BODY" | grep -q "\"id\":${DEVICE_ID}" || { echo "Device detail assertion failed"; exit 1; }

echo "[4/6] Get device list and find first interface"
RESP="$(request GET "${BASE_URL}/devices" "" "${TOKEN}")"
BODY="$(echo "$RESP" | sed '$d')"
CODE="$(echo "$RESP" | tail -n1)"
if [[ "$CODE" != "200" ]]; then
  echo "List devices failed HTTP $CODE: $BODY"
  exit 1
fi
echo "$BODY" | grep -q "\"status\"" || { echo "Device list missing status"; exit 1; }
IF_ID="$(echo "${BODY}" | sed -n 's/.*\"interfaces\":\\[\\{\"id\":\\([0-9]*\\).*/\\1/p' | head -n1)"

if [[ -n "${IF_ID}" ]]; then
  echo "[5/6] Update first interface remark"
  RESP="$(request PUT "${BASE_URL}/interfaces/${IF_ID}/remark" '{"remark":"smoke-port"}' "${TOKEN}")"
  BODY="$(echo "$RESP" | sed '$d')"
  CODE="$(echo "$RESP" | tail -n1)"
  if [[ "$CODE" != "200" ]]; then
    echo "Update interface remark failed HTTP $CODE: $BODY"
    exit 1
  fi
else
  echo "[5/6] Skip interface remark update (no interface yet)"
fi

echo "[6/6] Query device logs"
RESP="$(request GET "${BASE_URL}/devices/${DEVICE_ID}/logs" "" "${TOKEN}")"
BODY="$(echo "$RESP" | sed '$d')"
CODE="$(echo "$RESP" | tail -n1)"
if [[ "$CODE" != "200" ]]; then
  echo "Get logs failed HTTP $CODE: $BODY"
  exit 1
fi
echo "$BODY" | grep -q "\"message\"" || { echo "Device logs assertion failed"; exit 1; }

echo "Smoke E2E passed with assertions"
