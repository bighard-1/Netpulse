CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS devices (
    id BIGSERIAL PRIMARY KEY,
    ip INET NOT NULL UNIQUE,
    brand VARCHAR(32) NOT NULL,
    community VARCHAR(128) NOT NULL,
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
    schedule_interval => INTERVAL '1 minute'
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

