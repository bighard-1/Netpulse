#!/usr/bin/env bash
set -euo pipefail

# Static guardrail audit for NetPulse API routes.
# This script is read-only. It helps reviewers spot risky route changes before
# release, especially admin/write endpoints that accidentally lose protection.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ROUTES_FILE="${ROOT_DIR}/internal/api/handlers.go"

if [[ ! -f "${ROUTES_FILE}" ]]; then
  echo "handlers.go not found: ${ROUTES_FILE}" >&2
  exit 1
fi

echo "NetPulse API Guardrail Audit"
echo "Source: ${ROUTES_FILE}"
echo

route_lines="$(grep -nE 'pr\.(Get|Post|Put|Delete)\(|pr\.With' "${ROUTES_FILE}" || true)"
if [[ -z "${route_lines}" ]]; then
  echo "No protected route registrations found."
  exit 1
fi

echo "[1/4] Mutating routes without explicit audit middleware"
mutating_without_audit="$(printf '%s\n' "${route_lines}" | grep -E '\.(Post|Put|Delete)\(' | grep -v 'auditMiddleware' || true)"
if [[ -n "${mutating_without_audit}" ]]; then
  printf '%s\n' "${mutating_without_audit}" | sed 's/^/WARN /'
else
  echo "OK all mutating routes include audit middleware"
fi
echo

echo "[2/4] Admin-sensitive routes without adminOnly"
admin_sensitive="$(printf '%s\n' "${route_lines}" | grep -E '/api/(system|users|admin|templates|alerts/rules|discovery|audit|reports|diagnostics)' | grep -v 'adminOnly' || true)"
if [[ -n "${admin_sensitive}" ]]; then
  printf '%s\n' "${admin_sensitive}" | sed 's/^/WARN /'
else
  echo "OK admin-sensitive route patterns use adminOnly"
fi
echo

echo "[3/4] Write routes using permission-based protection"
write_permissions="$(printf '%s\n' "${route_lines}" | grep -E '\.(Post|Put|Delete)\(' | grep 'requirePermission' || true)"
if [[ -n "${write_permissions}" ]]; then
  printf '%s\n' "${write_permissions}" | sed 's/^/OK   /'
else
  echo "WARN no permission-protected write routes found"
fi
echo

echo "[4/4] Explicit read routes that intentionally allow all logged-in users"
logged_in_reads="$(
  {
    printf '%s\n' "${route_lines}" | grep 'pr.Get("/api/events/recent"' || true
    printf '%s\n' "${route_lines}" | grep 'pr.Get("/api/system/health"' || true
    printf '%s\n' "${route_lines}" | grep 'pr.Get("/api/devices/{id}"' || true
    printf '%s\n' "${route_lines}" | grep 'pr.Get("/api/devices/{id}/capabilities"' || true
  } | sed '/^$/d'
)"
if [[ -n "${logged_in_reads}" ]]; then
  printf '%s\n' "${logged_in_reads}" | sed 's/^/INFO /'
else
  echo "INFO no broad read routes matched"
fi
echo

if [[ -n "${mutating_without_audit}${admin_sensitive}" ]]; then
  cat <<'EOF'
Result: REVIEW NEEDED
This script is intentionally conservative. WARN items are not always bugs, but
they must be reviewed before release so compatibility exceptions remain explicit.
EOF
else
  echo "Result: OK"
fi
