# NetPulse V1.0.0-pro

NetPulse is a modern Network Operations & Maintenance (O&M) platform built for Huawei/H3C and standard SNMP devices, with a production-ready Go backend, TimescaleDB time-series core, Vue 3 NOC console, and iOS mobile client.

## Project Vision

Traditional NMS stacks such as Zabbix/Nagios are strong at generic host/service checks, but many teams now need:

- Faster network-centric onboarding (SNMP-focused device workflows)
- Better long-range trend analysis without external TSDB plumbing
- Built-in mobile-first operations
- Unified incident context for "alert -> root cause -> action"

NetPulse targets that gap with:

- Timescale-native 3-year history handling
- NOC-style dashboard and incident feed
- Quick-Peek workflow on Web + Mobile
- Production O&M controls (backup/restore, audit, suppression, maintenance mode)

## Key Features

### 1. NOC Dashboard (Web + Mobile V2.0)
- Global Health Score (0-100)
- Device availability and active incident overview
- Traffic hotspot ranking
- Critical incident feed with severity coding

### 2. SNMP Monitoring Engine
- SNMP v1/v2c/v3
- CPU / Memory / Interface traffic polling
- Interface sync with ifIndex/ifName mapping
- Syslog + SNMP Trap listeners
- SNMP v3 precheck before add (`/api/devices/precheck`)
- Per-device calibration map (runtime configurable)

### 3. 3-Year History & Performance
- `metrics` hypertable for raw telemetry
- `metrics_1m` continuous aggregate for long-range queries
- Optimized retention and aggregation policies for historical analysis
- Adaptive query interval support (`1m / 5m / 1h`)
- Server-side decimation for large ranges

### 4. Alert Suppression (V2.0)
- Upstream-aware suppression logic
- Downstream alerts are marked related/suppressed when upstream is already down
- Maintenance mode aware: collection continues, alerts muted

### 5. Security & Governance
- JWT auth for API protection
- RBAC + per-user permissions
- Audit logs for login and operation tracking
- Unified error payload (`code + message + hint`)
- Mobile secure token storage:
  - iOS Keychain

### 6. O&M Operations
- One-click backup / restore
- Backup drill report loop
- Runtime settings center (polling interval, timeout, thresholds)
- Runtime SNMP calibration center (JSON + row editor)
- Device self-diagnosis export
- Device capability matrix API/UI

---

## Architecture

- Backend: Go (`cmd/netpulse`, `internal/api`, `internal/db`, `internal/snmp`)
- Database: PostgreSQL + TimescaleDB (`latest-pg15` recommended)
- Frontend: Vue 3 + Element Plus + ECharts
- Mobile:
  - iOS: SwiftUI + Charts + Keychain + Face ID/Touch ID

---

## Production Build & Image

Use the release script:

```bash
./build_release.sh
```

Release script guarantees:

1. Build Vue dist
2. Sync `web/dist` into Go embed path
3. Build Go binary check
4. Validate static serving wiring in `main.go`
5. Validate alert suppression wiring (`AlertManager` + worker)
6. Build Docker image with production tag:

```text
netpulse:v1.0.0-pro
```

You can override tag:

```bash
IMAGE_TAG=netpulse:v1.0.1-pro ./build_release.sh
```

## Local Packaging Output

After local build/package, artifacts are stored in:

```text
package/
```

Recommended naming:
- `NetPulse_v1.0.0-pro_linux_amd64.tar`
- `NetPulse_v1.0.0-pro_ios_unsigned.ipa`

## GHCR Push

Production image publish target:

```text
ghcr.io/bighard-1/netpulse
```

Recommended tags:
- `v1.0.0-pro`
- `latest`

---

## 1Panel Quick Start (Exact Variables)

### A. Deploy TimescaleDB first
Image:

```text
timescale/timescaledb:latest-pg15
```

Mandatory DB variables:
- `POSTGRES_DB=netpulse`
- `POSTGRES_USER=postgres`
- `POSTGRES_PASSWORD=<strong-password>`

Volume:
- `/var/lib/postgresql/data`

### B. Deploy NetPulse container
Image:

```text
ghcr.io/bighard-1/netpulse:v1.0.0-pro
```

Mandatory NetPulse variables:
- `DB_HOST=<timescaledb-container-name-or-ip>`
- `DB_PORT=5432`
- `DB_USER=postgres`
- `DB_PASSWORD=<same-as-db>`
- `DB_NAME=netpulse`
- `DB_SSLMODE=disable`
- `JWT_SECRET=<long-random-secret>`
- `ADMIN_USERNAME=admin`
- `ADMIN_PASSWORD=<strong-admin-password>`

Recommended variables:
- `NETPULSE_CRED_KEY=<32-byte-key-for-snmp-secrets>`
- `SNMP_POLL_INTERVAL_SEC=60`
- `SNMP_DEVICE_TIMEOUT_SEC=15`
- `STATUS_ONLINE_WINDOW_SEC=300`
- `ALERT_CPU_THRESHOLD=90`
- `ALERT_MEM_THRESHOLD=90`
- `ALERT_WEBHOOK_URL=`
- `SNMP_CALIBRATION_MAP={}`
- `SYSLOG_ADDR=:514`
- `SNMP_TRAP_ADDR=:9162`
- `BACKUP_DRILL_EVERY_HOURS=168`

Port mapping:
- `8080/tcp` (Web/API)
- `514/udp` (Syslog, optional)
- `9162/udp` (SNMP Trap, optional)

---

## Core API Additions (Recent)

- `POST /api/devices/precheck`: SNMP connectivity/auth precheck before add.
- `GET /api/devices/{id}/capabilities`: current capability matrix for a device.
- `GET /api/metrics/history?interval=1m|5m|1h`: interval-aware history query.

---

## Security Specs

- JWT bearer token for protected API routes
- Login throttling / temporary lock controls
- Role-based and permission-based authorization
- Full audit records: user, action, target, method, path, source IP, status, latency
- Sensitive SNMP secrets can be encrypted at rest via `NETPULSE_CRED_KEY`
- Mobile secure sessions:
  - iOS token in Keychain
  
---

## Golden Shimmer UI Standard

All skeleton loaders now follow production shimmer spec:

- Cycle: `1.5s`
- Base: `#1E293B`
- Highlight: `#0F172A`

Applied on:
- Web (`el-skeleton` global style override)
- iOS (`ShimmerRect`)

---

## NetPulse Master Demo Script

See: [NOC Demo Scenario](./docs/NOC_DEMO_SCENARIO.md)

---

## Repo Structure

```text
cmd/netpulse            # server entry, static embed
internal/api            # REST APIs, auth, middleware
internal/db             # schema bootstrap, repository
internal/snmp           # collector, worker, alert manager
web                     # Vue 3 NOC console
mobile/ios              # iOS app
deploy                  # docker/1panel materials
package                 # packaged outputs
```
