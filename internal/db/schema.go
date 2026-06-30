package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

const bootstrapSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devices (
    id BIGSERIAL PRIMARY KEY,
    ip INET NOT NULL UNIQUE,
    name VARCHAR(128),
    template_id BIGINT,
    brand VARCHAR(32) NOT NULL,
    community VARCHAR(128) NOT NULL,
    write_community VARCHAR(128),
    snmp_version VARCHAR(8) NOT NULL DEFAULT '2c',
    snmp_port INTEGER NOT NULL DEFAULT 161,
    v3_username VARCHAR(128),
    v3_auth_protocol VARCHAR(16),
    v3_auth_password VARCHAR(256),
    v3_priv_protocol VARCHAR(16),
    v3_priv_password VARCHAR(256),
    v3_security_level VARCHAR(32),
    maintenance_mode BOOLEAN NOT NULL DEFAULT FALSE,
    monitoring_paused BOOLEAN NOT NULL DEFAULT FALSE,
    monitoring_pause_reason TEXT NOT NULL DEFAULT '',
    device_tier VARCHAR(16) NOT NULL DEFAULT 'access',
    poll_interval_sec INTEGER NOT NULL DEFAULT 0,
    cpu_threshold NUMERIC(6,2) NOT NULL DEFAULT 0,
    mem_threshold NUMERIC(6,2) NOT NULL DEFAULT 0,
    remark TEXT,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE devices ADD COLUMN IF NOT EXISTS template_id BIGINT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS write_community VARCHAR(128);
ALTER TABLE devices ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS poll_interval_sec INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS cpu_threshold NUMERIC(6,2) NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS mem_threshold NUMERIC(6,2) NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_tier VARCHAR(16) NOT NULL DEFAULT 'access';
ALTER TABLE devices ADD COLUMN IF NOT EXISTS monitoring_paused BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS monitoring_pause_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS interfaces (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    "index" INTEGER NOT NULL,
    name VARCHAR(128) NOT NULL,
    custom_name VARCHAR(128),
    speed_mbps INTEGER NOT NULL DEFAULT 0,
    oper_status SMALLINT,
    admin_status SMALLINT,
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (device_id, "index")
);
ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS custom_name VARCHAR(128);
ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS speed_mbps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS oper_status SMALLINT;
ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS admin_status SMALLINT;

CREATE INDEX IF NOT EXISTS idx_devices_name_trgm ON devices USING GIN (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_devices_ip_trgm ON devices USING GIN ((host(ip)) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_devices_remark_trgm ON devices USING GIN (remark gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_interfaces_name_trgm ON interfaces USING GIN (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_interfaces_custom_name_trgm ON interfaces USING GIN (custom_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_interfaces_remark_trgm ON interfaces USING GIN (remark gin_trgm_ops);

CREATE TABLE IF NOT EXISTS metrics (
    ts TIMESTAMPTZ NOT NULL,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    interface_id BIGINT REFERENCES interfaces(id) ON DELETE CASCADE,
    cpu_usage NUMERIC(5,2),
    memory_usage NUMERIC(5,2),
    storage_usage NUMERIC(5,2),
    storage_total NUMERIC(20,2),
    storage_free NUMERIC(20,2),
    uptime_sec BIGINT,
    traffic_in_bps BIGINT,
    traffic_out_bps BIGINT,
    traffic_in_status VARCHAR(32),
    traffic_out_status VARCHAR(32)
);

ALTER TABLE metrics ADD COLUMN IF NOT EXISTS storage_usage NUMERIC(5,2);
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS storage_total NUMERIC(20,2);
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS storage_free NUMERIC(20,2);
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS uptime_sec BIGINT;
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS traffic_in_status VARCHAR(32);
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS traffic_out_status VARCHAR(32);

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

        -- storage_* historical compatibility: old schemas may use varchar/text.
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'storage_total'
              AND data_type NOT IN ('numeric','double precision','real','bigint','integer')
        ) THEN
            ALTER TABLE metrics
                ALTER COLUMN storage_total TYPE NUMERIC(20,2)
                USING NULLIF(regexp_replace(COALESCE(storage_total::text, ''), '[^0-9\\.-]', '', 'g'), '')::NUMERIC(20,2);
        END IF;
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'storage_free'
              AND data_type NOT IN ('numeric','double precision','real','bigint','integer')
        ) THEN
            ALTER TABLE metrics
                ALTER COLUMN storage_free TYPE NUMERIC(20,2)
                USING NULLIF(regexp_replace(COALESCE(storage_free::text, ''), '[^0-9\\.-]', '', 'g'), '')::NUMERIC(20,2);
        END IF;
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'metrics' AND column_name = 'storage_usage'
              AND data_type NOT IN ('numeric','double precision','real','bigint','integer')
        ) THEN
            ALTER TABLE metrics
                ALTER COLUMN storage_usage TYPE NUMERIC(5,2)
                USING NULLIF(regexp_replace(COALESCE(storage_usage::text, ''), '[^0-9\\.-]', '', 'g'), '')::NUMERIC(5,2);
        END IF;
    END IF;
END $$;

SELECT create_hypertable('metrics', 'ts', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_metrics_device_ts ON metrics (device_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_interface_ts ON metrics (interface_id, ts DESC);

CREATE TABLE IF NOT EXISTS traffic_5m (
    bucket TIMESTAMPTZ NOT NULL,
    interface_id BIGINT NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    samples INTEGER NOT NULL DEFAULT 0,
    avg_traffic_in_bps NUMERIC(20,2),
    avg_traffic_out_bps NUMERIC(20,2),
    max_traffic_in_bps BIGINT,
    max_traffic_out_bps BIGINT,
    port_down_samples INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (interface_id, bucket)
);
SELECT create_hypertable('traffic_5m', 'bucket', if_not_exists => TRUE);
ALTER TABLE traffic_5m ADD COLUMN IF NOT EXISTS port_down_samples INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_traffic_5m_device_bucket ON traffic_5m(device_id, bucket DESC);

CREATE TABLE IF NOT EXISTS traffic_1h (
    bucket TIMESTAMPTZ NOT NULL,
    interface_id BIGINT NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    samples INTEGER NOT NULL DEFAULT 0,
    avg_traffic_in_bps NUMERIC(20,2),
    avg_traffic_out_bps NUMERIC(20,2),
    max_traffic_in_bps BIGINT,
    max_traffic_out_bps BIGINT,
    port_down_samples INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (interface_id, bucket)
);
SELECT create_hypertable('traffic_1h', 'bucket', if_not_exists => TRUE);
ALTER TABLE traffic_1h ADD COLUMN IF NOT EXISTS port_down_samples INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_traffic_1h_device_bucket ON traffic_1h(device_id, bucket DESC);

CREATE TABLE IF NOT EXISTS traffic_rollup_state (
    grain VARCHAR(16) PRIMARY KEY,
    last_bucket TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_duration_ms BIGINT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

SELECT remove_continuous_aggregate_policy(
    'metrics_1m',
    if_exists => TRUE
);

DO $$
DECLARE
    j RECORD;
BEGIN
    -- Defensive cleanup for old/duplicated refresh policies to prevent overlap errors.
    FOR j IN
        SELECT job_id
        FROM timescaledb_information.jobs
        WHERE hypertable_name = 'metrics_1m'
          AND proc_name = 'policy_refresh_continuous_aggregate'
    LOOP
        BEGIN
            PERFORM delete_job(j.job_id);
        EXCEPTION
            WHEN OTHERS THEN
                NULL;
        END;
    END LOOP;
END $$;

SELECT add_continuous_aggregate_policy(
    'metrics_1m',
    start_offset => INTERVAL '30 days',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '5 minutes',
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

CREATE TABLE IF NOT EXISTS device_capabilities (
    device_id BIGINT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    snmp_version VARCHAR(8),
    supports_cpu BOOLEAN NOT NULL DEFAULT FALSE,
    supports_memory BOOLEAN NOT NULL DEFAULT FALSE,
    supports_if_traffic BOOLEAN NOT NULL DEFAULT FALSE,
    interface_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS device_capability_history (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    snmp_version VARCHAR(8),
    supports_cpu BOOLEAN NOT NULL DEFAULT FALSE,
    supports_memory BOOLEAN NOT NULL DEFAULT FALSE,
    supports_if_traffic BOOLEAN NOT NULL DEFAULT FALSE,
    interface_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_device_capability_history_device_time
    ON device_capability_history(device_id, created_at DESC);

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
    description TEXT,
    match_sysobjectid TEXT,
    match_sysdescr TEXT,
    priority INTEGER NOT NULL DEFAULT 100,
    auto_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    snmp_version VARCHAR(8) NOT NULL DEFAULT '2c',
    snmp_port INTEGER NOT NULL DEFAULT 161,
    community VARCHAR(128),
    v3_username VARCHAR(128),
    v3_auth_protocol VARCHAR(16),
    v3_auth_password VARCHAR(256),
    v3_priv_protocol VARCHAR(16),
    v3_priv_password VARCHAR(256),
    v3_security_level VARCHAR(32),
    cpu_oid TEXT,
    mem_oid TEXT,
    if_in_oid TEXT,
    if_out_oid TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS match_sysobjectid TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS match_sysdescr TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 100;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS auto_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS cpu_oid TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS mem_oid TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS if_in_oid TEXT;
ALTER TABLE device_templates ADD COLUMN IF NOT EXISTS if_out_oid TEXT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema='public' AND table_name='devices' AND column_name='template_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname='devices_template_id_fkey'
    ) THEN
        BEGIN
            ALTER TABLE devices
                ADD CONSTRAINT devices_template_id_fkey
                FOREIGN KEY (template_id) REFERENCES device_templates(id) ON DELETE SET NULL;
        EXCEPTION
            WHEN duplicate_object THEN NULL;
        END;
    END IF;
END $$;

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

CREATE TABLE IF NOT EXISTS topology_nodes (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
    label VARCHAR(128),
    x NUMERIC(12,2) NOT NULL DEFAULT 0,
    y NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS topology_edges (
    id BIGSERIAL PRIMARY KEY,
    source_node_id BIGINT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    target_node_id BIGINT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    label VARCHAR(128),
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (source_node_id <> target_node_id),
    UNIQUE (source_node_id, target_node_id)
);
CREATE INDEX IF NOT EXISTS idx_topology_edges_source ON topology_edges(source_node_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_target ON topology_edges(target_node_id);

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
    status VARCHAR(16) NOT NULL DEFAULT 'open',
    assignee VARCHAR(64),
    note TEXT,
    silenced_until TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'open';
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS assignee VARCHAR(64);
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS note TEXT;
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS silenced_until TIMESTAMPTZ;
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

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

CREATE TABLE IF NOT EXISTS system_health (
    ts TIMESTAMPTZ NOT NULL,
    score NUMERIC(6,2) NOT NULL,
    active_alerts INTEGER NOT NULL,
    availability NUMERIC(6,2) NOT NULL
);

SELECT create_hypertable('system_health', 'ts', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_system_health_ts ON system_health(ts DESC);

CREATE TABLE IF NOT EXISTS webhook_configs (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    endpoint TEXT NOT NULL,
    secret TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
            WHERE table_schema='public' AND table_name='devices' AND column_name='name'
        ) THEN
            ALTER TABLE devices ADD COLUMN name VARCHAR(128);
            UPDATE devices SET name = host(ip) WHERE name IS NULL OR name = '';
        END IF;
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
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='write_community'
        ) THEN
            ALTER TABLE devices ADD COLUMN write_community VARCHAR(128);
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='deleted_at'
        ) THEN
            ALTER TABLE devices ADD COLUMN deleted_at TIMESTAMPTZ;
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema='public' AND table_name='devices' AND column_name='maintenance_mode'
        ) THEN
            ALTER TABLE devices ADD COLUMN maintenance_mode BOOLEAN NOT NULL DEFAULT FALSE;
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

func (r *Repository) EnsureSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), schemaBootstrapTimeout())
	defer cancel()

	started := time.Now()
	if _, err := r.db.ExecContext(ctx, bootstrapSchemaSQL); err != nil {
		return fmt.Errorf("schema bootstrap failed: %w", err)
	}
	log.Printf("schema bootstrap completed in %s", time.Since(started).Round(time.Millisecond))
	if err := r.ensureSchemaVersion(ctx); err != nil {
		return err
	}
	return nil
}

func schemaBootstrapTimeout() time.Duration {
	const (
		defaultSeconds = 180
		minSeconds     = 30
		maxSeconds     = 1800
	)
	raw := os.Getenv("NETPULSE_SCHEMA_TIMEOUT_SEC")
	if raw == "" {
		return defaultSeconds * time.Second
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return defaultSeconds * time.Second
	}
	if seconds < minSeconds {
		seconds = minSeconds
	}
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (r *Repository) ensureSchemaVersion(ctx context.Context) error {
	const ensureBase = `
		INSERT INTO schema_migrations(version, description)
		VALUES (1, 'bootstrap base schema')
		ON CONFLICT (version) DO NOTHING;
	`
	if _, err := r.db.ExecContext(ctx, ensureBase); err != nil {
		return fmt.Errorf("ensure migration baseline: %w", err)
	}

	// v2: interfaces custom_name support for human-friendly alias.
	const mig2 = `
		ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS custom_name VARCHAR(128);
		INSERT INTO schema_migrations(version, description)
		VALUES (2, 'add interfaces.custom_name')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 2, mig2); err != nil {
		return fmt.Errorf("apply migration v2 failed: %w", err)
	}

	// v3: indexes for latest-metric join paths.
	const mig3 = `
		CREATE INDEX IF NOT EXISTS idx_metrics_interface_ts ON metrics (interface_id, ts DESC);
		CREATE INDEX IF NOT EXISTS idx_metrics_device_ts ON metrics (device_id, ts DESC);
		INSERT INTO schema_migrations(version, description)
		VALUES (3, 'ensure metrics latest-point indexes')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 3, mig3); err != nil {
		return fmt.Errorf("apply migration v3 failed: %w", err)
	}
	// v4: interface speed/status and metrics uptime support.
	const mig4 = `
		ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS speed_mbps INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS oper_status SMALLINT;
		ALTER TABLE interfaces ADD COLUMN IF NOT EXISTS admin_status SMALLINT;
		ALTER TABLE metrics ADD COLUMN IF NOT EXISTS uptime_sec BIGINT;
		INSERT INTO schema_migrations(version, description)
		VALUES (4, 'add interfaces.speed_mbps/interfaces.oper_status/interfaces.admin_status and metrics.uptime_sec')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 4, mig4); err != nil {
		return fmt.Errorf("apply migration v4 failed: %w", err)
	}

	// v5: lightweight observability indexes for operation status and event drill-down pages.
	const mig5 = `
		CREATE INDEX IF NOT EXISTS idx_metrics_ts_status ON metrics (ts DESC, traffic_in_status, traffic_out_status);
		CREATE INDEX IF NOT EXISTS idx_device_logs_created_at ON device_logs (created_at DESC);
		INSERT INTO schema_migrations(version, description)
		VALUES (5, 'add observability indexes for ops and traffic diagnostics')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 5, mig5); err != nil {
		return fmt.Errorf("apply migration v5 failed: %w", err)
	}
	// v6: manually managed topology graph. Independent tables only; avoid touching metrics/views.
	const mig6 = `
		CREATE TABLE IF NOT EXISTS topology_nodes (
		    id BIGSERIAL PRIMARY KEY,
		    device_id BIGINT NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
		    label VARCHAR(128),
		    x NUMERIC(12,2) NOT NULL DEFAULT 0,
		    y NUMERIC(12,2) NOT NULL DEFAULT 0,
		    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS topology_edges (
		    id BIGSERIAL PRIMARY KEY,
		    source_node_id BIGINT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
		    target_node_id BIGINT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
		    label VARCHAR(128),
		    remark TEXT,
		    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    CHECK (source_node_id <> target_node_id),
		    UNIQUE (source_node_id, target_node_id)
		);
		CREATE INDEX IF NOT EXISTS idx_topology_edges_source ON topology_edges(source_node_id);
		CREATE INDEX IF NOT EXISTS idx_topology_edges_target ON topology_edges(target_node_id);
		INSERT INTO schema_migrations(version, description)
		VALUES (6, 'add manual topology graph tables')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 6, mig6); err != nil {
		return fmt.Errorf("apply migration v6 failed: %w", err)
	}
	// v7: faster dashboard asset loading. These indexes are additive only and
	// keep existing query semantics unchanged.
	const mig7 = `
		CREATE INDEX IF NOT EXISTS idx_device_logs_device_created_at
			ON device_logs (device_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_interfaces_device_index
			ON interfaces (device_id, "index");
		INSERT INTO schema_migrations(version, description)
		VALUES (7, 'add lightweight asset dashboard loading indexes')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 7, mig7); err != nil {
		return fmt.Errorf("apply migration v7 failed: %w", err)
	}
	// v8: latest metric cache tables. Dashboard and device summary pages should
	// not scan the historical Timescale hypertable to find one latest point per
	// interface on every refresh.
	const mig8 = `
		CREATE TABLE IF NOT EXISTS device_latest_metrics (
		    device_id BIGINT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
		    ts TIMESTAMPTZ NOT NULL,
		    cpu_usage NUMERIC(5,2),
		    memory_usage NUMERIC(5,2),
		    storage_usage NUMERIC(5,2),
		    storage_total NUMERIC(20,2),
		    storage_free NUMERIC(20,2),
		    uptime_sec BIGINT,
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS interface_latest_metrics (
		    interface_id BIGINT PRIMARY KEY REFERENCES interfaces(id) ON DELETE CASCADE,
		    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
		    ts TIMESTAMPTZ NOT NULL,
		    traffic_in_bps BIGINT,
		    traffic_out_bps BIGINT,
		    traffic_in_status VARCHAR(32),
		    traffic_out_status VARCHAR(32),
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_interface_latest_metrics_device
			ON interface_latest_metrics(device_id);
		CREATE INDEX IF NOT EXISTS idx_interface_latest_metrics_ts
			ON interface_latest_metrics(ts DESC);
		CREATE INDEX IF NOT EXISTS idx_device_latest_metrics_ts
			ON device_latest_metrics(ts DESC);
		INSERT INTO schema_migrations(version, description)
		VALUES (8, 'add latest metric cache tables')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 8, mig8); err != nil {
		return fmt.Errorf("apply migration v8 failed: %w", err)
	}
	// v9: reserved marker for long-range chart hardening. Heavy indexes on
	// existing Timescale continuous aggregates must not run in the blocking
	// startup migration path; keep this version lightweight so service startup
	// remains reliable on large production databases.
	const mig9 = `
		INSERT INTO schema_migrations(version, description)
		VALUES (9, 'reserve non-blocking long-range chart hardening marker')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 9, mig9); err != nil {
		return fmt.Errorf("apply migration v9 failed: %w", err)
	}
	// v10: per-device monitoring pause switch. Additive only; lets operators
	// stop known-offline assets from generating repetitive polling events.
	const mig10 = `
		ALTER TABLE devices ADD COLUMN IF NOT EXISTS monitoring_paused BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE devices ADD COLUMN IF NOT EXISTS monitoring_pause_reason TEXT NOT NULL DEFAULT '';
		INSERT INTO schema_migrations(version, description)
		VALUES (10, 'add per-device monitoring pause switch')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 10, mig10); err != nil {
		return fmt.Errorf("apply migration v10 failed: %w", err)
	}
	// v11: keep explicit port-down samples in traffic rollups so long-range
	// charts can show down periods as blank gaps without breaking normal up
	// traffic into dotted-looking fragments.
	const mig11 = `
		ALTER TABLE traffic_5m ADD COLUMN IF NOT EXISTS port_down_samples INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE traffic_1h ADD COLUMN IF NOT EXISTS port_down_samples INTEGER NOT NULL DEFAULT 0;
		INSERT INTO schema_migrations(version, description)
		VALUES (11, 'add port down sample counters to traffic rollups')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 11, mig11); err != nil {
		return fmt.Errorf("apply migration v11 failed: %w", err)
	}
	// v12: distinguish SNMP read/write community strings on stored assets.
	// Existing read-only collection keeps using community; write_community is
	// optional and reserved for operations that need write access.
	const mig12 = `
		ALTER TABLE devices ADD COLUMN IF NOT EXISTS write_community VARCHAR(128);
		INSERT INTO schema_migrations(version, description)
		VALUES (12, 'add optional devices.write_community')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 12, mig12); err != nil {
		return fmt.Errorf("apply migration v12 failed: %w", err)
	}
	// v13: archive-delete devices quickly without deleting heavy historical
	// metrics in the request path. Active devices keep IP uniqueness, while
	// archived devices remain available for historical cleanup/forensics.
	const mig13 = `
		ALTER TABLE devices ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
		DO $$
		DECLARE
			c RECORD;
		BEGIN
			FOR c IN
				SELECT conname
				FROM pg_constraint
				WHERE conrelid = 'public.devices'::regclass
				  AND contype = 'u'
				  AND pg_get_constraintdef(oid) = 'UNIQUE (ip)'
			LOOP
				EXECUTE format('ALTER TABLE devices DROP CONSTRAINT %I', c.conname);
			END LOOP;
		END $$;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_active_ip_unique
			ON devices(ip)
			WHERE deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_devices_deleted_at
			ON devices(deleted_at)
			WHERE deleted_at IS NOT NULL;
		INSERT INTO schema_migrations(version, description)
		VALUES (13, 'add archive delete support for devices')
		ON CONFLICT (version) DO NOTHING;
	`
	if err := r.applySchemaMigration(ctx, 13, mig13); err != nil {
		return fmt.Errorf("apply migration v13 failed: %w", err)
	}
	return nil
}

func (r *Repository) applySchemaMigration(ctx context.Context, version int64, query string) error {
	applied, err := r.schemaMigrationApplied(ctx, version)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	started := time.Now()
	if _, err := r.db.ExecContext(ctx, query); err != nil {
		return err
	}
	log.Printf("schema migration v%d applied in %s", version, time.Since(started).Round(time.Millisecond))
	return nil
}

func (r *Repository) schemaMigrationApplied(ctx context.Context, version int64) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM schema_migrations
			WHERE version = $1
		);
	`, version).Scan(&exists); err != nil {
		return false, fmt.Errorf("check migration v%d status: %w", version, err)
	}
	return exists, nil
}
