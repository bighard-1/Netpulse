#!/usr/bin/env bash
set -euo pipefail

# NetPulse read-only capacity probe.
# It never creates, updates, or deletes data. Use it to measure API latency under
# light concurrent dashboard/chart access.
#
# Usage:
#   BASE_URL=http://127.0.0.1:8080/api USERNAME=admin PASSWORD=admin123 ./scripts/capacity_probe.sh
#
# Optional:
#   CONCURRENCY=8 ROUNDS=5 DEVICE_ID=1 PORT_ID=10 ./scripts/capacity_probe.sh

BASE_URL="${BASE_URL:-http://127.0.0.1:8080/api}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
CONCURRENCY="${CONCURRENCY:-6}"
ROUNDS="${ROUNDS:-4}"
DEVICE_ID="${DEVICE_ID:-}"
PORT_ID="${PORT_ID:-}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd python3

echo "[1/5] login ${BASE_URL}"
LOGIN_JSON="$(curl -fsS -X POST "${BASE_URL}/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(printf '%s' "${LOGIN_JSON}" | python3 -c 'import json, sys; print(json.load(sys.stdin).get("token", ""))')"
if [[ -z "${TOKEN}" ]]; then
  echo "login failed: token missing" >&2
  exit 1
fi

AUTH=(-H "Authorization: Bearer ${TOKEN}")
END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
START_DAY="$(date -u -v-1d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 day ago' +%Y-%m-%dT%H:%M:%SZ)"

echo "[2/5] discover sample device/port"
DEVICES_JSON="$(curl -fsS "${BASE_URL}/devices" "${AUTH[@]}")"
if [[ -z "${DEVICE_ID}" ]]; then
  DEVICE_ID="$(printf '%s' "${DEVICES_JSON}" | python3 -c 'import json, sys
data=json.load(sys.stdin)
if isinstance(data, dict): data=data.get("devices", data.get("items", []))
for item in data or []:
    if item.get("id"):
        print(item["id"]); break')"
fi

if [[ -n "${DEVICE_ID}" && -z "${PORT_ID}" ]]; then
  DETAIL_JSON="$(curl -fsS "${BASE_URL}/devices/${DEVICE_ID}" "${AUTH[@]}")"
  PORT_ID="$(printf '%s' "${DETAIL_JSON}" | python3 -c 'import json, sys
data=json.load(sys.stdin)
for item in data.get("interfaces", []) or []:
    if item.get("id"):
        print(item["id"]); break')"
fi

echo "sample device=${DEVICE_ID:-none} port=${PORT_ID:-none}"

declare -a URLS=(
  "${BASE_URL}/devices"
  "${BASE_URL}/topology"
  "${BASE_URL}/events/recent?limit=20"
  "${BASE_URL}/system/health?limit=30"
)
if [[ -n "${DEVICE_ID}" ]]; then
  URLS+=("${BASE_URL}/devices/${DEVICE_ID}")
fi
if [[ -n "${PORT_ID}" ]]; then
  URLS+=("${BASE_URL}/metrics/history?type=traffic&id=${PORT_ID}&start=${START_DAY}&end=${END}&max_points=900")
fi

run_one() {
  local url="$1" out="$2"
  curl -sS -o /dev/null -w "%{http_code} %{time_total} ${url}\n" "${url}" "${AUTH[@]}" >>"${out}" || \
    printf "000 999 %s\n" "${url}" >>"${out}"
}

echo "[3/5] run read-only probe: concurrency=${CONCURRENCY}, rounds=${ROUNDS}"
OUT="${TMP_DIR}/latency.log"
: >"${OUT}"
for ((round=1; round<=ROUNDS; round++)); do
  pids=()
  for ((i=1; i<=CONCURRENCY; i++)); do
    for url in "${URLS[@]}"; do
      run_one "${url}" "${OUT}" &
      pids+=("$!")
    done
  done
  for pid in "${pids[@]}"; do
    wait "${pid}"
  done
done

echo "[4/5] summary"
python3 - "${OUT}" <<'PY'
import statistics
import sys
from collections import defaultdict

path = sys.argv[1]
items = []
by_url = defaultdict(list)
errors = []
with open(path, "r", encoding="utf-8") as f:
    for line in f:
        parts = line.rstrip("\n").split(" ", 2)
        if len(parts) != 3:
            continue
        code, cost, url = parts
        ms = float(cost) * 1000
        items.append(ms)
        by_url[url].append(ms)
        if not code.startswith("2"):
            errors.append((code, url))

def p95(values):
    values = sorted(values)
    if not values:
        return 0
    idx = min(len(values) - 1, int(round((len(values) - 1) * 0.95)))
    return values[idx]

print(f"requests={len(items)} errors={len(errors)} avg_ms={statistics.mean(items):.1f} p95_ms={p95(items):.1f} max_ms={max(items):.1f}")
for url, values in sorted(by_url.items()):
    print(f"- {url}")
    print(f"  count={len(values)} avg_ms={statistics.mean(values):.1f} p95_ms={p95(values):.1f} max_ms={max(values):.1f}")
if errors:
    print("errors:")
    for code, url in errors[:20]:
        print(f"  {code} {url}")
PY

echo "[5/5] guidance"
cat <<'EOF'
建议判定:
- dashboard/topology/events P95 < 800ms: 体验通常较顺畅。
- traffic history P95 < 1500ms: 单端口图表通常可接受。
- P95 > 3000ms 或出现 000/5xx: 优先查看 设置 -> 运行观测 -> 最近服务端慢请求，并执行 资产/数据库诊断。
EOF

