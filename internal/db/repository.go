package db

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const bootstrapSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS devices (
    id BIGSERIAL PRIMARY KEY,
    ip INET NOT NULL UNIQUE,
    brand VARCHAR(32) NOT NULL,
    community VARCHAR(128) NOT NULL,
    snmp_version VARCHAR(8) NOT NULL DEFAULT '2c',
    snmp_port INTEGER NOT NULL DEFAULT 161,
    v3_username VARCHAR(128),
    v3_auth_protocol VARCHAR(16),
    v3_auth_password VARCHAR(256),
    v3_priv_protocol VARCHAR(16),
    v3_priv_password VARCHAR(256),
    v3_security_level VARCHAR(32),
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interfaces (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    "index" INTEGER NOT NULL,
    name VARCHAR(128) NOT NULL,
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (device_id, "index")
);

CREATE TABLE IF NOT EXISTS metrics (
    ts TIMESTAMPTZ NOT NULL,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    interface_id BIGINT REFERENCES interfaces(id) ON DELETE CASCADE,
    cpu_usage NUMERIC(5,2),
    memory_usage NUMERIC(5,2),
    traffic_in_bps BIGINT,
    traffic_out_bps BIGINT
);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'metrics'
    ) THEN
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'traffic_in_bps'
              AND data_type <> 'bigint'
        ) OR EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'traffic_out_bps'
              AND data_type <> 'bigint'
        ) THEN
            IF EXISTS (
                SELECT 1
                FROM pg_matviews
                WHERE schemaname = 'public' AND matviewname = 'metrics_1m'
            ) THEN
                DROP MATERIALIZED VIEW metrics_1m CASCADE;
            END IF;
        END IF;

        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'traffic_in_bps'
              AND data_type <> 'bigint'
        ) THEN
            ALTER TABLE metrics
                ALTER COLUMN traffic_in_bps TYPE BIGINT
                USING COALESCE(traffic_in_bps::BIGINT, 0);
        END IF;
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'traffic_out_bps'
              AND data_type <> 'bigint'
        ) THEN
            ALTER TABLE metrics
                ALTER COLUMN traffic_out_bps TYPE BIGINT
                USING COALESCE(traffic_out_bps::BIGINT, 0);
        END IF;
    END IF;
END $$;

SELECT create_hypertable('metrics', 'ts', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_metrics_device_ts ON metrics (device_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_interface_ts ON metrics (interface_id, ts DESC);

CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', ts) AS bucket,
    device_id,
    interface_id,
    AVG(cpu_usage) AS avg_cpu_usage,
    AVG(memory_usage) AS avg_memory_usage,
    AVG(traffic_in_bps) AS avg_traffic_in_bps,
    AVG(traffic_out_bps) AS avg_traffic_out_bps
FROM metrics
GROUP BY bucket, device_id, interface_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
    'metrics_1m',
    start_offset => INTERVAL '1 day',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists => TRUE
);

SELECT add_retention_policy(
    'metrics',
    drop_after => INTERVAL '180 days',
    if_not_exists => TRUE
);

SELECT add_retention_policy(
    'metrics_1m',
    drop_after => INTERVAL '730 days',
    if_not_exists => TRUE
);

CREATE TABLE IF NOT EXISTS device_logs (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    level VARCHAR(16) NOT NULL DEFAULT 'INFO',
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_device_logs_device_created_at
    ON device_logs (device_id, created_at DESC);

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role VARCHAR(16) NOT NULL CHECK (role IN ('admin','user'))
);

-- Compatibility migration for old users schemas.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'users'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'password_hash'
        ) THEN
            ALTER TABLE users ADD COLUMN password_hash TEXT;
            IF EXISTS (
                SELECT 1
                FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'password'
            ) THEN
                UPDATE users SET password_hash = password WHERE password_hash IS NULL;
            END IF;
            UPDATE users SET password_hash = crypt('changeme123', gen_salt('bf')) WHERE password_hash IS NULL;
            ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'role'
        ) THEN
            ALTER TABLE users ADD COLUMN role VARCHAR(16);
            UPDATE users SET role = 'user' WHERE role IS NULL;
            ALTER TABLE users ALTER COLUMN role SET NOT NULL;
            ALTER TABLE users
                ADD CONSTRAINT users_role_check
                CHECK (role IN ('admin','user'));
        END IF;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    target TEXT,
    method VARCHAR(16),
    path TEXT,
    ip VARCHAR(128),
    status_code INTEGER,
    duration_ms BIGINT,
    client VARCHAR(16),
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS device_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    brand VARCHAR(32) NOT NULL,
    snmp_version VARCHAR(8) NOT NULL DEFAULT '2c',
    snmp_port INTEGER NOT NULL DEFAULT 161,
    community VARCHAR(128),
    v3_username VARCHAR(128),
    v3_auth_protocol VARCHAR(16),
    v3_auth_password VARCHAR(256),
    v3_priv_protocol VARCHAR(16),
    v3_priv_password VARCHAR(256),
    v3_security_level VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS topology_links (
    id BIGSERIAL PRIMARY KEY,
    src_device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    src_if_index INTEGER NOT NULL,
    dst_device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    dst_if_index INTEGER NOT NULL,
    protocol VARCHAR(16) NOT NULL DEFAULT 'LLDP',
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    scope VARCHAR(16) NOT NULL DEFAULT 'global',
    device_id BIGINT REFERENCES devices(id) ON DELETE CASCADE,
    cpu_threshold NUMERIC(6,2),
    mem_threshold NUMERIC(6,2),
    traffic_threshold BIGINT,
    mute_start VARCHAR(8),
    mute_end VARCHAR(8),
    notify_webhook TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_events (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT REFERENCES alert_rules(id) ON DELETE SET NULL,
    device_id BIGINT REFERENCES devices(id) ON DELETE CASCADE,
    level VARCHAR(16) NOT NULL,
    code VARCHAR(64) NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS role_permissions (
    id BIGSERIAL PRIMARY KEY,
    role VARCHAR(32) NOT NULL,
    permission VARCHAR(128) NOT NULL,
    UNIQUE(role, permission)
);

CREATE TABLE IF NOT EXISTS user_permissions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission VARCHAR(128) NOT NULL,
    UNIQUE(user_id, permission)
);

CREATE TABLE IF NOT EXISTS config_snapshots (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    content_hash VARCHAR(128) NOT NULL,
    content TEXT NOT NULL,
    diff TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS discovery_tasks (
    id BIGSERIAL PRIMARY KEY,
    cidr VARCHAR(64) NOT NULL,
    community VARCHAR(128),
    snmp_version VARCHAR(8) NOT NULL DEFAULT '2c',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backup_drill_reports (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(16) NOT NULL,
    message TEXT NOT NULL,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(128) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Compatibility migration for old audit_logs schemas:
-- Some older versions used "timestamp" (or created_at) instead of "ts".
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'audit_logs'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'ts'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN ts TIMESTAMPTZ;

            IF EXISTS (
                SELECT 1
                FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'timestamp'
            ) THEN
                EXECUTE 'UPDATE audit_logs SET ts = "timestamp" WHERE ts IS NULL';
            ELSIF EXISTS (
                SELECT 1
                FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'created_at'
            ) THEN
                UPDATE audit_logs SET ts = created_at WHERE ts IS NULL;
            END IF;

            UPDATE audit_logs SET ts = NOW() WHERE ts IS NULL;
            ALTER TABLE audit_logs ALTER COLUMN ts SET NOT NULL;
            ALTER TABLE audit_logs ALTER COLUMN ts SET DEFAULT NOW();
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'user_id'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN user_id BIGINT;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'action'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN action VARCHAR(64);
            UPDATE audit_logs SET action = 'LEGACY_ACTION' WHERE action IS NULL;
            ALTER TABLE audit_logs ALTER COLUMN action SET NOT NULL;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'target'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN target TEXT;
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'method'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN method VARCHAR(16);
        END IF;
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'path'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN path TEXT;
        END IF;
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'ip'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN ip VARCHAR(128);
        END IF;
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'status_code'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN status_code INTEGER;
        END IF;
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'duration_ms'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN duration_ms BIGINT;
        END IF;
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'client'
        ) THEN
            ALTER TABLE audit_logs ADD COLUMN client VARCHAR(16);
        END IF;
    END IF;
END $$;

-- Compatibility migration for old devices schemas.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'devices'
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='snmp_version'
        ) THEN
            ALTER TABLE devices ADD COLUMN snmp_version VARCHAR(8);
            UPDATE devices SET snmp_version = '2c' WHERE snmp_version IS NULL;
            ALTER TABLE devices ALTER COLUMN snmp_version SET NOT NULL;
            ALTER TABLE devices ALTER COLUMN snmp_version SET DEFAULT '2c';
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='snmp_port'
        ) THEN
            ALTER TABLE devices ADD COLUMN snmp_port INTEGER;
            UPDATE devices SET snmp_port = 161 WHERE snmp_port IS NULL;
            ALTER TABLE devices ALTER COLUMN snmp_port SET NOT NULL;
            ALTER TABLE devices ALTER COLUMN snmp_port SET DEFAULT 161;
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_username'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_username VARCHAR(128);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_auth_protocol'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_auth_protocol VARCHAR(16);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_auth_password'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_auth_password VARCHAR(256);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_priv_protocol'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_priv_protocol VARCHAR(16);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_priv_password'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_priv_password VARCHAR(256);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='v3_security_level'
        ) THEN
            ALTER TABLE devices ADD COLUMN v3_security_level VARCHAR(32);
        END IF;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'audit_logs' AND column_name = 'user_id'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'audit_logs_user_id_fkey'
        ) THEN
            BEGIN
                ALTER TABLE audit_logs
                    ADD CONSTRAINT audit_logs_user_id_fkey
                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
            EXCEPTION
                WHEN duplicate_object THEN
                    NULL;
            END;
        END IF;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_audit_logs_ts ON audit_logs (ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_ts ON audit_logs (user_id, ts DESC);

INSERT INTO users (username, password_hash, role)
VALUES ('admin', crypt('admin123', gen_salt('bf')), 'admin')
ON CONFLICT (username) DO NOTHING;

INSERT INTO role_permissions(role, permission) VALUES
('admin','*'),
('user','device.read'),
('user','metrics.read'),
('user','logs.read')
ON CONFLICT (role, permission) DO NOTHING;
`

type Device struct {
	ID          int64     `json:"id"`
	IP          string    `json:"ip"`
	Brand       string    `json:"brand"`
	Community   string    `json:"-"`
	SNMPVersion string    `json:"snmp_version,omitempty"`
	SNMPPort    int       `json:"snmp_port,omitempty"`
	V3Username  string    `json:"v3_username,omitempty"`
	V3AuthProto string    `json:"v3_auth_protocol,omitempty"`
	V3AuthPass  string    `json:"-"`
	V3PrivProto string    `json:"v3_priv_protocol,omitempty"`
	V3PrivPass  string    `json:"-"`
	V3SecLevel  string    `json:"v3_security_level,omitempty"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
}

type Interface struct {
	ID       int64  `json:"id"`
	DeviceID int64  `json:"device_id,omitempty"`
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Remark   string `json:"remark"`
}

type InterfaceMetric struct {
	IfIndex       int
	IfName        string
	CPUUsage      float64
	MemoryUsage   float64
	TrafficInBps  int64
	TrafficOutBps int64
}

type DeviceStatus struct {
	Device
	LastMetricAt *time.Time  `json:"last_metric_at"`
	Status       string      `json:"status"`
	StatusReason string      `json:"status_reason,omitempty"`
	Interfaces   []Interface `json:"interfaces"`
}

type DeviceHistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	CPUUsage  float64   `json:"cpu_usage"`
	MemUsage  float64   `json:"mem_usage"`
}

type InterfaceHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	TrafficInBps  float64   `json:"traffic_in_bps"`
	TrafficOutBps float64   `json:"traffic_out_bps"`
}

type DeviceLog struct {
	ID        int64     `json:"id"`
	DeviceID  int64     `json:"device_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
}

type AuditLog struct {
	ID         int64     `json:"id"`
	UserID     *int64    `json:"user_id"`
	Username   string    `json:"username,omitempty"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Method     string    `json:"method,omitempty"`
	Path       string    `json:"path,omitempty"`
	IP         string    `json:"ip,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	Client     string    `json:"client,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Repository struct {
	db      *sql.DB
	credKey []byte
}

type DeviceTemplate struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Brand           string    `json:"brand"`
	SNMPVersion     string    `json:"snmp_version"`
	SNMPPort        int       `json:"snmp_port"`
	Community       string    `json:"community,omitempty"`
	V3Username      string    `json:"v3_username,omitempty"`
	V3AuthProtocol  string    `json:"v3_auth_protocol,omitempty"`
	V3AuthPassword  string    `json:"-"`
	V3PrivProtocol  string    `json:"v3_priv_protocol,omitempty"`
	V3PrivPassword  string    `json:"-"`
	V3SecurityLevel string    `json:"v3_security_level,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type TopologyLink struct {
	ID          int64     `json:"id"`
	SrcDeviceID int64     `json:"src_device_id"`
	SrcIfIndex  int       `json:"src_if_index"`
	DstDeviceID int64     `json:"dst_device_id"`
	DstIfIndex  int       `json:"dst_if_index"`
	Protocol    string    `json:"protocol"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
}

type AlertRule struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Scope            string    `json:"scope"`
	DeviceID         *int64    `json:"device_id,omitempty"`
	CPUThreshold     *float64  `json:"cpu_threshold,omitempty"`
	MemThreshold     *float64  `json:"mem_threshold,omitempty"`
	TrafficThreshold *int64    `json:"traffic_threshold,omitempty"`
	MuteStart        string    `json:"mute_start,omitempty"`
	MuteEnd          string    `json:"mute_end,omitempty"`
	NotifyWebhook    string    `json:"notify_webhook,omitempty"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
}

type BackupDrillReport struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type RuntimeSettings struct {
	SNMPPollIntervalSec   int     `json:"snmp_poll_interval_sec"`
	SNMPDeviceTimeoutSec  int     `json:"snmp_device_timeout_sec"`
	StatusOnlineWindowSec int     `json:"status_online_window_sec"`
	AlertCPUThreshold     float64 `json:"alert_cpu_threshold"`
	AlertMemThreshold     float64 `json:"alert_mem_threshold"`
	AlertWebhookURL       string  `json:"alert_webhook_url"`
}

func NewRepository(db *sql.DB) *Repository {
	key := []byte(os.Getenv("NETPULSE_CRED_KEY"))
	if len(key) != 32 {
		key = nil
	}
	return &Repository{db: db, credKey: key}
}

// EnsureSchema auto-bootstraps database objects for a blank database.
func (r *Repository) EnsureSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := r.db.ExecContext(ctx, bootstrapSchemaSQL); err != nil {
		return fmt.Errorf("schema bootstrap failed: %w", err)
	}
	return nil
}

func (r *Repository) EnsureRuntimeSettings(ctx context.Context, defaults map[string]string) error {
	if len(defaults) == 0 {
		return nil
	}
	const q = `
		INSERT INTO system_settings(key, value, updated_at)
		VALUES($1, $2, NOW())
		ON CONFLICT (key) DO NOTHING;
	`
	for k, v := range defaults {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, q, k, strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("ensure runtime setting %s: %w", k, err)
		}
	}
	return nil
}

func (r *Repository) GetSystemSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM system_settings ORDER BY key;`)
	if err != nil {
		return nil, fmt.Errorf("query system settings: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan system settings: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system settings: %w", err)
	}
	return out, nil
}

func (r *Repository) UpsertSystemSettings(ctx context.Context, kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	const q = `
		INSERT INTO system_settings(key, value, updated_at)
		VALUES($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
	`
	for k, v := range kv {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, q, k, strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("upsert system setting %s: %w", k, err)
		}
	}
	return nil
}

func parseIntSetting(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseFloatSetting(raw string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func (r *Repository) GetRuntimeSettings(ctx context.Context) (RuntimeSettings, error) {
	kv, err := r.GetSystemSettings(ctx)
	if err != nil {
		return RuntimeSettings{}, err
	}
	out := RuntimeSettings{
		SNMPPollIntervalSec:   parseIntSetting(kv["snmp_poll_interval_sec"], 60),
		SNMPDeviceTimeoutSec:  parseIntSetting(kv["snmp_device_timeout_sec"], 15),
		StatusOnlineWindowSec: parseIntSetting(kv["status_online_window_sec"], 300),
		AlertCPUThreshold:     parseFloatSetting(kv["alert_cpu_threshold"], 90),
		AlertMemThreshold:     parseFloatSetting(kv["alert_mem_threshold"], 90),
		AlertWebhookURL:       kv["alert_webhook_url"],
	}
	if out.SNMPPollIntervalSec < 5 {
		out.SNMPPollIntervalSec = 5
	}
	if out.SNMPPollIntervalSec > 3600 {
		out.SNMPPollIntervalSec = 3600
	}
	if out.SNMPDeviceTimeoutSec < 2 {
		out.SNMPDeviceTimeoutSec = 2
	}
	if out.SNMPDeviceTimeoutSec > 120 {
		out.SNMPDeviceTimeoutSec = 120
	}
	if out.StatusOnlineWindowSec < 30 {
		out.StatusOnlineWindowSec = 30
	}
	if out.StatusOnlineWindowSec > 3600 {
		out.StatusOnlineWindowSec = 3600
	}
	if out.AlertCPUThreshold > 100 {
		out.AlertCPUThreshold = 100
	}
	if out.AlertMemThreshold > 100 {
		out.AlertMemThreshold = 100
	}
	return out, nil
}

func (r *Repository) UpsertAdmin(username, passwordHash string) error {
	const q = `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    role = 'admin';
	`
	if _, err := r.db.Exec(q, username, passwordHash); err != nil {
		return fmt.Errorf("upsert admin failed: %w", err)
	}
	return nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	const q = `
		SELECT id, username, password_hash, role
		FROM users
		WHERE username = $1;
	`
	var u User
	if err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user failed: %w", err)
	}
	return &u, nil
}

func (r *Repository) AddAuditLog(ctx context.Context, log AuditLog) error {
	const q = `
		INSERT INTO audit_logs (user_id, action, target, method, path, ip, status_code, duration_ms, client, ts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW());
	`
	if _, err := r.db.ExecContext(
		ctx, q, log.UserID, log.Action, log.Target, log.Method, log.Path, log.IP, log.StatusCode, log.DurationMS, log.Client,
	); err != nil {
		return fmt.Errorf("insert audit log failed: %w", err)
	}
	return nil
}

func (r *Repository) ListAuditLogs(ctx context.Context, limit int) ([]AuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	const q = `
		SELECT a.id, a.user_id, COALESCE(u.username,''), a.action, COALESCE(a.target,''),
		       COALESCE(a.method,''), COALESCE(a.path,''), COALESCE(a.ip,''), COALESCE(a.status_code,0),
		       COALESCE(a.duration_ms,0), COALESCE(a.client,''), a.ts
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.ts DESC
		LIMIT $1;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs failed: %w", err)
	}
	defer rows.Close()
	out := make([]AuditLog, 0)
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.Target, &a.Method, &a.Path, &a.IP, &a.StatusCode, &a.DurationMS, &a.Client, &a.Timestamp); err != nil {
			return nil, fmt.Errorf("scan audit log failed: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) CreateUser(ctx context.Context, username, passwordHash, role string) error {
	const q = `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3);
	`
	if _, err := r.db.ExecContext(ctx, q, username, passwordHash, role); err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}
	return nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	const q = `
		SELECT id, username, password_hash, role
		FROM users
		ORDER BY id;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list users failed: %w", err)
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
			return nil, fmt.Errorf("scan users failed: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) GetDeviceByID(ctx context.Context, id int64) (*DeviceStatus, error) {
	runtime, _ := r.GetRuntimeSettings(ctx)
	onlineWindow := time.Duration(runtime.StatusOnlineWindowSec) * time.Second
	if onlineWindow <= 0 {
		onlineWindow = 5 * time.Minute
	}
	const q = `
		SELECT d.id, host(d.ip), d.brand, d.community, d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.remark, ''), d.created_at, lm.last_ts, COALESCE(dl.message, '')
		FROM devices d
		LEFT JOIN (
			SELECT device_id, MAX(ts) AS last_ts
			FROM metrics
			GROUP BY device_id
		) lm ON lm.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT message
			FROM device_logs
			WHERE device_id = d.id
			ORDER BY created_at DESC
			LIMIT 1
		) dl ON TRUE
		WHERE d.id = $1;
	`
	var ds DeviceStatus
	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&ds.ID, &ds.IP, &ds.Brand, &ds.Community, &ds.SNMPVersion, &ds.SNMPPort,
		&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
		&ds.Remark, &ds.CreatedAt, &ds.LastMetricAt, &ds.StatusReason,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get device by id: %w", err)
	}
	ds.Status = "unknown"
	ds.Community = r.decryptOpt(ds.Community)
	ds.V3AuthPass = r.decryptOpt(ds.V3AuthPass)
	ds.V3PrivPass = r.decryptOpt(ds.V3PrivPass)
	if ds.LastMetricAt != nil {
		if time.Since(*ds.LastMetricAt) <= onlineWindow {
			ds.Status = "online"
			ds.StatusReason = ""
		} else {
			ds.Status = "offline"
		}
	}

	const iq = `
		SELECT id, device_id, "index", name, COALESCE(remark, '')
		FROM interfaces
		WHERE device_id = $1
		ORDER BY "index";
	`
	rows, err := r.db.QueryContext(ctx, iq, id)
	if err != nil {
		return nil, fmt.Errorf("query interfaces by device id: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itf Interface
		if err := rows.Scan(&itf.ID, &itf.DeviceID, &itf.Index, &itf.Name, &itf.Remark); err != nil {
			return nil, fmt.Errorf("scan interface by device id: %w", err)
		}
		ds.Interfaces = append(ds.Interfaces, itf)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interfaces by device id: %w", err)
	}
	return &ds, nil
}

func (r *Repository) AddDevice(ctx context.Context, d Device) (int64, error) {
	d.Community = r.encryptOpt(d.Community)
	d.V3AuthPass = r.encryptOpt(d.V3AuthPass)
	d.V3PrivPass = r.encryptOpt(d.V3PrivPass)
	const q = `
		INSERT INTO devices (
			ip, brand, community, snmp_version, snmp_port,
			v3_username, v3_auth_protocol, v3_auth_password, v3_priv_protocol, v3_priv_password, v3_security_level,
			remark
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(
		ctx, q, d.IP, d.Brand, d.Community, d.SNMPVersion, d.SNMPPort, d.V3Username, d.V3AuthProto,
		d.V3AuthPass, d.V3PrivProto, d.V3PrivPass, d.V3SecLevel, d.Remark,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("add device: %w", err)
	}
	return id, nil
}

func (r *Repository) DeleteDevice(ctx context.Context, id int64) error {
	const q = `DELETE FROM devices WHERE id = $1;`
	if _, err := r.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	return nil
}

func (r *Repository) ListDevices(ctx context.Context) ([]Device, error) {
	const q = `
		SELECT id, host(ip), brand, community, snmp_version, snmp_port,
		       COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''),
		       COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''),
		       COALESCE(remark, ''), created_at
		FROM devices
		ORDER BY id;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	out := make([]Device, 0)
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.IP, &d.Brand, &d.Community, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.Remark, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		d.Community = r.decryptOpt(d.Community)
		d.V3AuthPass = r.decryptOpt(d.V3AuthPass)
		d.V3PrivPass = r.decryptOpt(d.V3PrivPass)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return out, nil
}

func (r *Repository) encryptOpt(v string) string {
	if v == "" || len(r.credKey) != 32 {
		return v
	}
	block, err := aes.NewCipher(r.credKey)
	if err != nil {
		return v
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return v
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return v
	}
	out := gcm.Seal(nonce, nonce, []byte(v), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(out)
}

func (r *Repository) decryptOpt(v string) string {
	if v == "" || len(r.credKey) != 32 || len(v) < 5 || v[:4] != "enc:" {
		return v
	}
	raw, err := base64.StdEncoding.DecodeString(v[4:])
	if err != nil {
		return v
	}
	block, err := aes.NewCipher(r.credKey)
	if err != nil {
		return v
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return v
	}
	if len(raw) < gcm.NonceSize() {
		return v
	}
	nonce, cipherText := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return v
	}
	return string(plain)
}

func (r *Repository) HasPermission(ctx context.Context, userID int64, role, permission string) (bool, error) {
	if role == "admin" {
		return true, nil
	}
	if userID > 0 {
		const uq = `SELECT EXISTS(SELECT 1 FROM user_permissions WHERE user_id=$1 AND permission=$2);`
		var ok bool
		if err := r.db.QueryRowContext(ctx, uq, userID, permission).Scan(&ok); err == nil && ok {
			return true, nil
		}
	}
	const q = `SELECT EXISTS(SELECT 1 FROM role_permissions WHERE role=$1 AND (permission=$2 OR permission='*'));`
	var ok bool
	if err := r.db.QueryRowContext(ctx, q, role, permission).Scan(&ok); err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return ok, nil
}

func (r *Repository) UpdateUser(ctx context.Context, id int64, username, role string, passwordHash *string) error {
	if passwordHash != nil && *passwordHash != "" {
		_, err := r.db.ExecContext(ctx, `UPDATE users SET username=$2, role=$3, password_hash=$4 WHERE id=$1;`, id, username, role, *passwordHash)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE users SET username=$2, role=$3 WHERE id=$1;`, id, username, role)
	return err
}

func (r *Repository) DeleteUser(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id=$1;`, id)
	return err
}

func (r *Repository) ListUserPermissions(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT permission FROM user_permissions WHERE user_id=$1 ORDER BY permission;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceUserPermissions(ctx context.Context, userID int64, permissions []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id=$1;`, userID); err != nil {
		return err
	}
	for _, p := range permissions {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_permissions(user_id,permission) VALUES($1,$2) ON CONFLICT (user_id,permission) DO NOTHING;`, userID, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) CreateTemplate(ctx context.Context, t DeviceTemplate) (int64, error) {
	t.Community = r.encryptOpt(t.Community)
	t.V3AuthPassword = r.encryptOpt(t.V3AuthPassword)
	t.V3PrivPassword = r.encryptOpt(t.V3PrivPassword)
	const q = `INSERT INTO device_templates(name,brand,snmp_version,snmp_port,community,v3_username,v3_auth_protocol,v3_auth_password,v3_priv_protocol,v3_priv_password,v3_security_level) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id;`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, t.Name, t.Brand, t.SNMPVersion, t.SNMPPort, t.Community, t.V3Username, t.V3AuthProtocol, t.V3AuthPassword, t.V3PrivProtocol, t.V3PrivPassword, t.V3SecurityLevel).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) ListTemplates(ctx context.Context) ([]DeviceTemplate, error) {
	const q = `SELECT id,name,brand,snmp_version,snmp_port,COALESCE(community,''),COALESCE(v3_username,''),COALESCE(v3_auth_protocol,''),COALESCE(v3_auth_password,''),COALESCE(v3_priv_protocol,''),COALESCE(v3_priv_password,''),COALESCE(v3_security_level,''),created_at FROM device_templates ORDER BY id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceTemplate{}
	for rows.Next() {
		var t DeviceTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Brand, &t.SNMPVersion, &t.SNMPPort, &t.Community, &t.V3Username, &t.V3AuthProtocol, &t.V3AuthPassword, &t.V3PrivProtocol, &t.V3PrivPassword, &t.V3SecurityLevel, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Community = r.decryptOpt(t.Community)
		t.V3AuthPassword = r.decryptOpt(t.V3AuthPassword)
		t.V3PrivPassword = r.decryptOpt(t.V3PrivPassword)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveAlertEvent(ctx context.Context, ruleID *int64, deviceID int64, level, code, message string) error {
	const q = `INSERT INTO alert_events(rule_id,device_id,level,code,message) VALUES($1,$2,$3,$4,$5);`
	_, err := r.db.ExecContext(ctx, q, ruleID, deviceID, level, code, message)
	return err
}

func (r *Repository) UpsertAlertRule(ctx context.Context, ar AlertRule) (int64, error) {
	if ar.ID == 0 {
		const ins = `INSERT INTO alert_rules(name,scope,device_id,cpu_threshold,mem_threshold,traffic_threshold,mute_start,mute_end,notify_webhook,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id;`
		var id int64
		if err := r.db.QueryRowContext(ctx, ins, ar.Name, ar.Scope, ar.DeviceID, ar.CPUThreshold, ar.MemThreshold, ar.TrafficThreshold, ar.MuteStart, ar.MuteEnd, ar.NotifyWebhook, ar.Enabled).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	const up = `UPDATE alert_rules SET name=$2,scope=$3,device_id=$4,cpu_threshold=$5,mem_threshold=$6,traffic_threshold=$7,mute_start=$8,mute_end=$9,notify_webhook=$10,enabled=$11 WHERE id=$1;`
	_, err := r.db.ExecContext(ctx, up, ar.ID, ar.Name, ar.Scope, ar.DeviceID, ar.CPUThreshold, ar.MemThreshold, ar.TrafficThreshold, ar.MuteStart, ar.MuteEnd, ar.NotifyWebhook, ar.Enabled)
	return ar.ID, err
}

func (r *Repository) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	const q = `SELECT id,name,scope,device_id,cpu_threshold,mem_threshold,traffic_threshold,COALESCE(mute_start,''),COALESCE(mute_end,''),COALESCE(notify_webhook,''),enabled,created_at FROM alert_rules ORDER BY id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRule{}
	for rows.Next() {
		var a AlertRule
		if err := rows.Scan(&a.ID, &a.Name, &a.Scope, &a.DeviceID, &a.CPUThreshold, &a.MemThreshold, &a.TrafficThreshold, &a.MuteStart, &a.MuteEnd, &a.NotifyWebhook, &a.Enabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertTopologyLink(ctx context.Context, l TopologyLink) (int64, error) {
	if l.ID == 0 {
		const ins = `INSERT INTO topology_links(src_device_id,src_if_index,dst_device_id,dst_if_index,protocol,remark) VALUES($1,$2,$3,$4,$5,$6) RETURNING id;`
		var id int64
		if err := r.db.QueryRowContext(ctx, ins, l.SrcDeviceID, l.SrcIfIndex, l.DstDeviceID, l.DstIfIndex, l.Protocol, l.Remark).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	const up = `UPDATE topology_links SET src_device_id=$2,src_if_index=$3,dst_device_id=$4,dst_if_index=$5,protocol=$6,remark=$7 WHERE id=$1;`
	_, err := r.db.ExecContext(ctx, up, l.ID, l.SrcDeviceID, l.SrcIfIndex, l.DstDeviceID, l.DstIfIndex, l.Protocol, l.Remark)
	return l.ID, err
}

func (r *Repository) ListTopologyLinks(ctx context.Context) ([]TopologyLink, error) {
	const q = `SELECT id,src_device_id,src_if_index,dst_device_id,dst_if_index,protocol,COALESCE(remark,''),created_at FROM topology_links ORDER BY id DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopologyLink{}
	for rows.Next() {
		var t TopologyLink
		if err := rows.Scan(&t.ID, &t.SrcDeviceID, &t.SrcIfIndex, &t.DstDeviceID, &t.DstIfIndex, &t.Protocol, &t.Remark, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteTopologyLink(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM topology_links WHERE id=$1;`, id)
	return err
}

func (r *Repository) FindDeviceByIP(ctx context.Context, ip string) (*Device, error) {
	const q = `SELECT id, host(ip), brand, community, snmp_version, snmp_port, COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''), COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''), COALESCE(remark,''), created_at FROM devices WHERE ip = $1::inet LIMIT 1;`
	var d Device
	if err := r.db.QueryRowContext(ctx, q, ip).Scan(&d.ID, &d.IP, &d.Brand, &d.Community, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.Remark, &d.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d.Community = r.decryptOpt(d.Community)
	d.V3AuthPass = r.decryptOpt(d.V3AuthPass)
	d.V3PrivPass = r.decryptOpt(d.V3PrivPass)
	return &d, nil
}

func (r *Repository) SaveConfigSnapshot(ctx context.Context, deviceID int64, hash, content, diff string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO config_snapshots(device_id,content_hash,content,diff) VALUES($1,$2,$3,$4);`, deviceID, hash, content, diff)
	return err
}

func (r *Repository) SaveBackupDrillReport(ctx context.Context, status, message, detail string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO backup_drill_reports(status,message,detail) VALUES($1,$2,$3::jsonb);`, status, message, detail)
	return err
}

func (r *Repository) ListBackupDrillReports(ctx context.Context, limit int) ([]BackupDrillReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,status,message,detail::text,created_at FROM backup_drill_reports ORDER BY id DESC LIMIT $1;`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupDrillReport{}
	for rows.Next() {
		var b BackupDrillReport
		if err := rows.Scan(&b.ID, &b.Status, &b.Message, &b.Detail, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) ListDevicesWithStatus(ctx context.Context) ([]DeviceStatus, error) {
	runtime, _ := r.GetRuntimeSettings(ctx)
	onlineWindow := time.Duration(runtime.StatusOnlineWindowSec) * time.Second
	if onlineWindow <= 0 {
		onlineWindow = 5 * time.Minute
	}
	const q = `
		SELECT d.id, host(d.ip), d.brand, d.community, d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.remark, ''), d.created_at, lm.last_ts, COALESCE(dl.message, '')
		FROM devices d
		LEFT JOIN (
			SELECT device_id, MAX(ts) AS last_ts
			FROM metrics
			GROUP BY device_id
		) lm ON lm.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT message
			FROM device_logs
			WHERE device_id = d.id
			ORDER BY created_at DESC
			LIMIT 1
		) dl ON TRUE
		ORDER BY d.id;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list devices with status: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	out := make([]DeviceStatus, 0)
	for rows.Next() {
		var ds DeviceStatus
		if err := rows.Scan(
			&ds.ID, &ds.IP, &ds.Brand, &ds.Community, &ds.SNMPVersion, &ds.SNMPPort,
			&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
			&ds.Remark, &ds.CreatedAt, &ds.LastMetricAt, &ds.StatusReason,
		); err != nil {
			return nil, fmt.Errorf("scan device status: %w", err)
		}
		ds.Status = "unknown"
		ds.Community = r.decryptOpt(ds.Community)
		ds.V3AuthPass = r.decryptOpt(ds.V3AuthPass)
		ds.V3PrivPass = r.decryptOpt(ds.V3PrivPass)
		if ds.LastMetricAt != nil {
			if now.Sub(*ds.LastMetricAt) <= onlineWindow {
				ds.Status = "online"
				ds.StatusReason = ""
			} else {
				ds.Status = "offline"
			}
		}
		out = append(out, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device status: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}

	const iq = `
		SELECT id, device_id, "index", name, COALESCE(remark, '')
		FROM interfaces
		ORDER BY device_id, "index";
	`
	iRows, err := r.db.QueryContext(ctx, iq)
	if err != nil {
		return nil, fmt.Errorf("query interfaces for devices: %w", err)
	}
	defer iRows.Close()

	byDevice := make(map[int64][]Interface)
	for iRows.Next() {
		var itf Interface
		if err := iRows.Scan(&itf.ID, &itf.DeviceID, &itf.Index, &itf.Name, &itf.Remark); err != nil {
			return nil, fmt.Errorf("scan interface: %w", err)
		}
		byDevice[itf.DeviceID] = append(byDevice[itf.DeviceID], itf)
	}
	if err := iRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interfaces: %w", err)
	}

	for i := range out {
		out[i].Interfaces = byDevice[out[i].ID]
	}

	return out, nil
}

// SyncInterfaces upserts interface snapshot for one device and preserves existing remarks.
// It also removes stale interfaces that no longer exist on device.
func (r *Repository) SyncInterfaces(ctx context.Context, deviceID int64, interfaces []Interface) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync interfaces tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const q = `
		INSERT INTO interfaces (device_id, "index", name, remark)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (device_id, "index")
		DO UPDATE SET
			name = EXCLUDED.name,
			remark = COALESCE(interfaces.remark, EXCLUDED.remark);
	`
	seen := make(map[int]struct{}, len(interfaces))
	for _, itf := range interfaces {
		seen[itf.Index] = struct{}{}
		if _, err := tx.ExecContext(ctx, q, deviceID, itf.Index, itf.Name, itf.Remark); err != nil {
			return fmt.Errorf("insert interface index=%d: %w", itf.Index, err)
		}
	}
	if len(seen) > 0 {
		indexes := make([]int, 0, len(seen))
		for idx := range seen {
			indexes = append(indexes, idx)
		}
		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM interfaces WHERE device_id = $1 AND NOT ("index" = ANY($2));`,
			deviceID,
			indexes,
		); err != nil {
			return fmt.Errorf("delete stale interfaces: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync interfaces: %w", err)
	}
	return nil
}

func (r *Repository) GetInterfaceIDMap(ctx context.Context, deviceID int64) (map[int]int64, error) {
	const q = `SELECT id, "index" FROM interfaces WHERE device_id = $1;`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query interface id map: %w", err)
	}
	defer rows.Close()

	out := make(map[int]int64)
	for rows.Next() {
		var id int64
		var idx int
		if err := rows.Scan(&id, &idx); err != nil {
			return nil, fmt.Errorf("scan interface id map: %w", err)
		}
		out[idx] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface id map: %w", err)
	}
	return out, nil
}

func (r *Repository) SaveMetrics(
	ctx context.Context,
	deviceID int64,
	ts time.Time,
	metrics []InterfaceMetric,
) error {
	const q = `
		WITH upsert_if AS (
			INSERT INTO interfaces (device_id, "index", name, remark)
			VALUES ($2, $3, $8, '')
			ON CONFLICT (device_id, "index")
			DO UPDATE SET
				name = CASE
					WHEN EXCLUDED.name <> '' THEN EXCLUDED.name
					ELSE interfaces.name
				END
			RETURNING id
		)
		INSERT INTO metrics (
			ts, device_id, interface_id, cpu_usage, memory_usage, traffic_in_bps, traffic_out_bps
		)
		VALUES ($1, $2, (SELECT id FROM upsert_if LIMIT 1), $4, $5, $6, $7);
	`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save metrics tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, m := range metrics {
		cpu := clampPercent(m.CPUUsage)
		mem := clampPercent(m.MemoryUsage)
		inBps := clampTrafficBps(m.TrafficInBps)
		outBps := clampTrafficBps(m.TrafficOutBps)

		if _, err := tx.ExecContext(
			ctx, q, ts, deviceID, m.IfIndex, cpu, mem, inBps, outBps, m.IfName,
		); err != nil {
			return fmt.Errorf("insert metric ifIndex=%d: %w", m.IfIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save metrics: %w", err)
	}
	return nil
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func clampTrafficBps(v int64) int64 {
	if v < 0 {
		return 0
	}
	const maxReasonableBps int64 = 9_000_000_000_000_000
	if v > maxReasonableBps {
		return 0
	}
	return v
}

func (r *Repository) UpdateDeviceRemark(ctx context.Context, id int64, remark string) error {
	const q = `UPDATE devices SET remark = $2 WHERE id = $1;`
	if _, err := r.db.ExecContext(ctx, q, id, remark); err != nil {
		return fmt.Errorf("update device remark: %w", err)
	}
	return nil
}

func (r *Repository) UpdateInterfaceRemark(ctx context.Context, id int64, remark string) error {
	const q = `UPDATE interfaces SET remark = $2 WHERE id = $1;`
	if _, err := r.db.ExecContext(ctx, q, id, remark); err != nil {
		return fmt.Errorf("update interface remark: %w", err)
	}
	return nil
}

func (r *Repository) UpdateInterfaceProfile(ctx context.Context, id int64, name, remark string) error {
	const uq = `
		SELECT i.device_id
		FROM interfaces i
		WHERE i.id = $1;
	`
	var deviceID int64
	if err := r.db.QueryRowContext(ctx, uq, id).Scan(&deviceID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("interface not found")
		}
		return fmt.Errorf("query interface device: %w", err)
	}

	if strings.TrimSpace(name) != "" {
		const cq = `
			SELECT EXISTS(
				SELECT 1 FROM interfaces
				WHERE device_id = $1 AND lower(name) = lower($2) AND id <> $3
			);
		`
		var exists bool
		if err := r.db.QueryRowContext(ctx, cq, deviceID, strings.TrimSpace(name), id).Scan(&exists); err != nil {
			return fmt.Errorf("check interface name conflict: %w", err)
		}
		if exists {
			return fmt.Errorf("interface name conflict in this device")
		}
	}

	const q = `
		UPDATE interfaces
		SET name = CASE WHEN $2 <> '' THEN $2 ELSE name END,
			remark = $3
		WHERE id = $1;
	`
	if _, err := r.db.ExecContext(ctx, q, id, strings.TrimSpace(name), remark); err != nil {
		return fmt.Errorf("update interface profile: %w", err)
	}
	return nil
}

func (r *Repository) GetDeviceHistory(
	ctx context.Context, deviceID int64, start, end time.Time,
) ([]DeviceHistoryPoint, error) {
	useAgg := end.Sub(start) > 7*24*time.Hour
	q := `
		SELECT ts, COALESCE(cpu_usage, 0), COALESCE(memory_usage, 0)
		FROM metrics
		WHERE device_id = $1 AND ts >= $2 AND ts <= $3
		ORDER BY ts;
	`
	if useAgg {
		q = `
			SELECT bucket AS ts, COALESCE(avg_cpu_usage, 0), COALESCE(avg_memory_usage, 0)
			FROM metrics_1m
			WHERE device_id = $1 AND bucket >= $2 AND bucket <= $3
			ORDER BY bucket;
		`
	}

	rows, err := r.db.QueryContext(ctx, q, deviceID, start, end)
	if err != nil {
		return nil, fmt.Errorf("get device history: %w", err)
	}
	defer rows.Close()

	out := make([]DeviceHistoryPoint, 0)
	for rows.Next() {
		var p DeviceHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.CPUUsage, &p.MemUsage); err != nil {
			return nil, fmt.Errorf("scan device history: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device history: %w", err)
	}
	return out, nil
}

func (r *Repository) GetInterfaceHistory(
	ctx context.Context, interfaceID int64, start, end time.Time,
) ([]InterfaceHistoryPoint, error) {
	useAgg := end.Sub(start) > 7*24*time.Hour
	q := `
		SELECT ts, COALESCE(traffic_in_bps, 0), COALESCE(traffic_out_bps, 0)
		FROM metrics
		WHERE interface_id = $1 AND ts >= $2 AND ts <= $3
		ORDER BY ts;
	`
	if useAgg {
		q = `
			SELECT bucket AS ts, COALESCE(avg_traffic_in_bps, 0), COALESCE(avg_traffic_out_bps, 0)
			FROM metrics_1m
			WHERE interface_id = $1 AND bucket >= $2 AND bucket <= $3
			ORDER BY bucket;
		`
	}

	rows, err := r.db.QueryContext(ctx, q, interfaceID, start, end)
	if err != nil {
		return nil, fmt.Errorf("get interface history: %w", err)
	}
	defer rows.Close()

	out := make([]InterfaceHistoryPoint, 0)
	for rows.Next() {
		var p InterfaceHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.TrafficInBps, &p.TrafficOutBps); err != nil {
			return nil, fmt.Errorf("scan interface history: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface history: %w", err)
	}
	return out, nil
}

func (r *Repository) GetDeviceLogs(ctx context.Context, deviceID int64) ([]DeviceLog, error) {
	const q = `
		SELECT id, device_id, level, message, created_at
		FROM device_logs
		WHERE device_id = $1
		ORDER BY created_at DESC
		LIMIT 100;
	`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, fmt.Errorf("get device logs: %w", err)
	}
	defer rows.Close()

	out := make([]DeviceLog, 0, 100)
	for rows.Next() {
		var l DeviceLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device log: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device logs: %w", err)
	}
	return out, nil
}

func (r *Repository) AddDeviceLog(ctx context.Context, deviceID int64, level, message string) error {
	const ins = `
		INSERT INTO device_logs (device_id, level, message)
		VALUES ($1, $2, $3);
	`
	if _, err := r.db.ExecContext(ctx, ins, deviceID, level, message); err != nil {
		return fmt.Errorf("add device log: %w", err)
	}
	const trim = `
		DELETE FROM device_logs
		WHERE device_id = $1
		AND id NOT IN (
			SELECT id
			FROM device_logs
			WHERE device_id = $1
			ORDER BY created_at DESC
			LIMIT 100
		);
	`
	if _, err := r.db.ExecContext(ctx, trim, deviceID); err != nil {
		return fmt.Errorf("trim device log: %w", err)
	}
	return nil
}
