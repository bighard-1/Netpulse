package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const bootstrapSchemaSQL = `
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
    schedule_interval => INTERVAL '1 minute',
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
    role VARCHAR(16) NOT NULL CHECK (role IN ('admin','user')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL,
    action VARCHAR(64) NOT NULL,
    method VARCHAR(16) NOT NULL,
    path TEXT NOT NULL,
    ip VARCHAR(64),
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_username_created_at ON audit_logs (username, created_at DESC);
`

type Device struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip"`
	Brand     string    `json:"brand"`
	Community string    `json:"community,omitempty"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"created_at"`
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
	CPUUsage      float64
	MemoryUsage   float64
	TrafficInBps  int64
	TrafficOutBps int64
}

type DeviceStatus struct {
	Device
	LastMetricAt *time.Time  `json:"last_metric_at"`
	Status       string      `json:"status"`
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
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditLog struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	IP        string    `json:"ip"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// EnsureSchema auto-bootstraps database objects for a blank database.
func (r *Repository) EnsureSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var regclass sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT to_regclass('public.devices')`).Scan(&regclass); err != nil {
		return fmt.Errorf("check schema existence failed: %w", err)
	}
	if regclass.Valid && regclass.String != "" {
		return nil
	}

	if _, err := r.db.ExecContext(ctx, bootstrapSchemaSQL); err != nil {
		return fmt.Errorf("schema bootstrap failed: %w", err)
	}
	return nil
}

func (r *Repository) UpsertAdmin(username, passwordHash string) error {
	const q = `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES ($1, $2, 'admin', TRUE)
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    role = 'admin',
		    enabled = TRUE;
	`
	if _, err := r.db.Exec(q, username, passwordHash); err != nil {
		return fmt.Errorf("upsert admin failed: %w", err)
	}
	return nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	const q = `
		SELECT id, username, password_hash, role, enabled, created_at
		FROM users
		WHERE username = $1;
	`
	var u User
	if err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt,
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
		INSERT INTO audit_logs (username, action, method, path, ip, detail)
		VALUES ($1, $2, $3, $4, $5, $6);
	`
	if _, err := r.db.ExecContext(
		ctx, q, log.Username, log.Action, log.Method, log.Path, log.IP, log.Detail,
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
		SELECT id, username, action, method, path, COALESCE(ip,''), COALESCE(detail,''), created_at
		FROM audit_logs
		ORDER BY created_at DESC
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
		if err := rows.Scan(&a.ID, &a.Username, &a.Action, &a.Method, &a.Path, &a.IP, &a.Detail, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log failed: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) CreateUser(ctx context.Context, username, passwordHash, role string) error {
	const q = `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES ($1, $2, $3, TRUE);
	`
	if _, err := r.db.ExecContext(ctx, q, username, passwordHash, role); err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}
	return nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	const q = `
		SELECT id, username, password_hash, role, enabled, created_at
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
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan users failed: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) AddDevice(ctx context.Context, d Device) (int64, error) {
	const q = `
		INSERT INTO devices (ip, brand, community, remark)
		VALUES ($1, $2, $3, $4)
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, d.IP, d.Brand, d.Community, d.Remark).Scan(&id); err != nil {
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
		SELECT id, ip::text, brand, community, COALESCE(remark, ''), created_at
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
		if err := rows.Scan(&d.ID, &d.IP, &d.Brand, &d.Community, &d.Remark, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return out, nil
}

func (r *Repository) ListDevicesWithStatus(ctx context.Context) ([]DeviceStatus, error) {
	const q = `
		SELECT d.id, d.ip::text, d.brand, d.community, COALESCE(d.remark, ''), d.created_at, lm.last_ts
		FROM devices d
		LEFT JOIN (
			SELECT device_id, MAX(ts) AS last_ts
			FROM metrics
			GROUP BY device_id
		) lm ON lm.device_id = d.id
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
			&ds.ID, &ds.IP, &ds.Brand, &ds.Community, &ds.Remark, &ds.CreatedAt, &ds.LastMetricAt,
		); err != nil {
			return nil, fmt.Errorf("scan device status: %w", err)
		}
		ds.Status = "offline"
		if ds.LastMetricAt != nil && now.Sub(*ds.LastMetricAt) <= 2*time.Minute {
			ds.Status = "online"
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

// SyncInterfaces replaces the current interface snapshot for one device.
// Call this right after device onboarding SNMP discovery.
func (r *Repository) SyncInterfaces(ctx context.Context, deviceID int64, interfaces []Interface) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync interfaces tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM interfaces WHERE device_id = $1;`, deviceID); err != nil {
		return fmt.Errorf("clear interfaces: %w", err)
	}

	const q = `
		INSERT INTO interfaces (device_id, "index", name, remark)
		VALUES ($1, $2, $3, $4);
	`
	for _, itf := range interfaces {
		if _, err := tx.ExecContext(ctx, q, deviceID, itf.Index, itf.Name, itf.Remark); err != nil {
			return fmt.Errorf("insert interface index=%d: %w", itf.Index, err)
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
		INSERT INTO metrics (
			ts, device_id, interface_id, cpu_usage, memory_usage, traffic_in_bps, traffic_out_bps
		)
		VALUES ($1, $2, (
			SELECT i.id FROM interfaces i WHERE i.device_id = $2 AND i."index" = $3
		), $4, $5, $6, $7);
	`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save metrics tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, m := range metrics {
		if _, err := tx.ExecContext(
			ctx, q, ts, deviceID, m.IfIndex, m.CPUUsage, m.MemoryUsage, m.TrafficInBps, m.TrafficOutBps,
		); err != nil {
			return fmt.Errorf("insert metric ifIndex=%d: %w", m.IfIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save metrics: %w", err)
	}
	return nil
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
