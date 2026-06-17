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
	"strings"
	"time"
)

const (
	MaxTrafficHistoryRange = 730 * 24 * time.Hour
	trafficRollup5mRange   = 90 * 24 * time.Hour
	trafficRollup1hRange   = MaxTrafficHistoryRange
)

type Device struct {
	ID              int64     `json:"id"`
	IP              string    `json:"ip"`
	Name            string    `json:"name"`
	TemplateID      *int64    `json:"template_id,omitempty"`
	Brand           string    `json:"brand"`
	Community       string    `json:"-"`
	SNMPVersion     string    `json:"snmp_version,omitempty"`
	SNMPPort        int       `json:"snmp_port,omitempty"`
	V3Username      string    `json:"v3_username,omitempty"`
	V3AuthProto     string    `json:"v3_auth_protocol,omitempty"`
	V3AuthPass      string    `json:"-"`
	V3PrivProto     string    `json:"v3_priv_protocol,omitempty"`
	V3PrivPass      string    `json:"-"`
	V3SecLevel      string    `json:"v3_security_level,omitempty"`
	MaintenanceMode bool      `json:"maintenance_mode"`
	DeviceTier      string    `json:"device_tier,omitempty"`
	PollIntervalSec int       `json:"poll_interval_sec,omitempty"`
	CPUThreshold    float64   `json:"cpu_threshold,omitempty"`
	MemThreshold    float64   `json:"mem_threshold,omitempty"`
	StorageUsage    float64   `json:"storage_usage,omitempty"`
	StorageTotal    float64   `json:"storage_total,omitempty"`
	StorageFree     float64   `json:"storage_free,omitempty"`
	Remark          string    `json:"remark"`
	CreatedAt       time.Time `json:"created_at"`
	Uptime          string    `json:"uptime,omitempty"`
	UptimeSec       int64     `json:"uptime_sec,omitempty"`
}

type Interface struct {
	ID            int64  `json:"id"`
	DeviceID      int64  `json:"device_id,omitempty"`
	DeviceIP      string `json:"device_ip,omitempty"`
	DeviceName    string `json:"device_name,omitempty"`
	Index         int    `json:"index"`
	Name          string `json:"name"`
	RawName       string `json:"raw_name,omitempty"`
	Remark        string `json:"remark"`
	SpeedMbps     int    `json:"speed_mbps,omitempty"`
	OperStatus    int    `json:"oper_status,omitempty"`
	AdminStatus   int    `json:"admin_status,omitempty"`
	TrafficInBps  int64  `json:"traffic_in_bps,omitempty"`
	TrafficOutBps int64  `json:"traffic_out_bps,omitempty"`
}

type InterfaceMetric struct {
	IfIndex        int
	IfName         string
	CPUUsage       float64
	MemoryUsage    float64
	StorageUsage   float64
	StorageTotal   float64
	StorageFree    float64
	UptimeSec      int64
	SpeedMbps      int
	OperStatus     int
	AdminStatus    int
	TrafficInBps   *int64
	TrafficOutBps  *int64
	TrafficInStat  string
	TrafficOutStat string
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

type DeviceStorageHistoryPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	StorageUsage float64   `json:"storage_usage"`
	StorageTotal float64   `json:"storage_total"`
	StorageFree  float64   `json:"storage_free"`
}

type InterfaceHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	TrafficInBps  *float64  `json:"traffic_in_bps"`
	TrafficOutBps *float64  `json:"traffic_out_bps"`
}

type DeviceCapability struct {
	DeviceID          int64     `json:"device_id"`
	SNMPVersion       string    `json:"snmp_version"`
	SupportsCPU       bool      `json:"supports_cpu"`
	SupportsMemory    bool      `json:"supports_memory"`
	SupportsIfTraffic bool      `json:"supports_if_traffic"`
	InterfaceCount    int       `json:"interface_count"`
	UpdatedAt         time.Time `json:"updated_at"`
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
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Brand            string    `json:"brand"`
	Description      string    `json:"description,omitempty"`
	MatchSysObjectID string    `json:"match_sysobjectid,omitempty"`
	MatchSysDescr    string    `json:"match_sysdescr,omitempty"`
	Priority         int       `json:"priority"`
	AutoEnabled      bool      `json:"auto_enabled"`
	SNMPVersion      string    `json:"snmp_version"`
	SNMPPort         int       `json:"snmp_port"`
	Community        string    `json:"community,omitempty"`
	V3Username       string    `json:"v3_username,omitempty"`
	V3AuthProtocol   string    `json:"v3_auth_protocol,omitempty"`
	V3AuthPassword   string    `json:"-"`
	V3PrivProtocol   string    `json:"v3_priv_protocol,omitempty"`
	V3PrivPassword   string    `json:"-"`
	V3SecurityLevel  string    `json:"v3_security_level,omitempty"`
	CPUOID           string    `json:"cpu_oid,omitempty"`
	MemOID           string    `json:"mem_oid,omitempty"`
	IfInOID          string    `json:"if_in_oid,omitempty"`
	IfOutOID         string    `json:"if_out_oid,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
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

type TopologyNode struct {
	ID           int64     `json:"id"`
	DeviceID     int64     `json:"device_id"`
	Label        string    `json:"label"`
	X            float64   `json:"x"`
	Y            float64   `json:"y"`
	DeviceName   string    `json:"device_name"`
	DeviceIP     string    `json:"device_ip"`
	DeviceBrand  string    `json:"device_brand"`
	DeviceTier   string    `json:"device_tier"`
	DeviceStatus string    `json:"device_status"`
	StatusReason string    `json:"status_reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TopologyEdge struct {
	ID           int64     `json:"id"`
	SourceNodeID int64     `json:"source_node_id"`
	TargetNodeID int64     `json:"target_node_id"`
	Label        string    `json:"label"`
	Remark       string    `json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
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

type GlobalSearchResult struct {
	Category            string `json:"category"`
	ID                  int64  `json:"id"`
	Title               string `json:"title"`
	Sub                 string `json:"sub"`
	Type                string `json:"type"`
	DeviceID            int64  `json:"device_id,omitempty"`
	InterfaceID         int64  `json:"interface_id,omitempty"`
	DeviceName          string `json:"device_name,omitempty"`
	DeviceIP            string `json:"device_ip,omitempty"`
	InterfaceName       string `json:"interface_name,omitempty"`
	InterfaceCustomName string `json:"interface_custom_name,omitempty"`
	InterfaceRemark     string `json:"interface_remark,omitempty"`
	MatchField          string `json:"match_field,omitempty"`
	Snippet             string `json:"snippet,omitempty"`
}

type RuntimeSettings struct {
	SNMPPollIntervalSec   int     `json:"snmp_poll_interval_sec"`
	PollIntervalCoreSec   int     `json:"poll_interval_core_sec"`
	PollIntervalAggSec    int     `json:"poll_interval_agg_sec"`
	PollIntervalAccessSec int     `json:"poll_interval_access_sec"`
	SNMPDeviceTimeoutSec  int     `json:"snmp_device_timeout_sec"`
	StatusOnlineWindowSec int     `json:"status_online_window_sec"`
	AlertCPUThreshold     float64 `json:"alert_cpu_threshold"`
	AlertMemThreshold     float64 `json:"alert_mem_threshold"`
	AlertWebhookURL       string  `json:"alert_webhook_url"`
	SNMPCalibrationMap    string  `json:"snmp_calibration_map"`
}

type SystemHealthPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	Score        float64   `json:"score"`
	ActiveAlerts int       `json:"active_alerts"`
	Availability float64   `json:"availability"`
}

type OpsPollSummary struct {
	PollErrorCount  int       `json:"poll_error_count"`
	FailedDevices   int       `json:"failed_devices"`
	TimeoutCount    int       `json:"timeout_count"`
	LastPollErrorAt time.Time `json:"last_poll_error_at,omitempty"`
	WindowMinutes   int       `json:"window_minutes"`
}

type TrafficSampleSummary struct {
	TotalSamples        int `json:"total_samples"`
	ValidSamples        int `json:"valid_samples"`
	GapSamples          int `json:"gap_samples"`
	AnomalySamples      int `json:"anomaly_samples"`
	InitializingSamples int `json:"initializing_samples"`
	WindowMinutes       int `json:"window_minutes"`
}

type TrafficRollupStatus struct {
	Grain          string     `json:"grain"`
	LastBucket     *time.Time `json:"last_bucket,omitempty"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastDurationMS int64      `json:"last_duration_ms"`
	LastError      string     `json:"last_error,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type RecentEvent struct {
	ID               int64     `json:"id"`
	DeviceID         int64     `json:"device_id"`
	DeviceIP         string    `json:"device_ip"`
	DeviceName       string    `json:"device_name"`
	InterfaceID      *int64    `json:"interface_id,omitempty"`
	InterfaceName    string    `json:"interface_name,omitempty"`
	InterfaceRawName string    `json:"interface_raw_name,omitempty"`
	InterfaceRemark  string    `json:"interface_remark,omitempty"`
	InterfaceIndex   int       `json:"interface_index,omitempty"`
	Level            string    `json:"level"`
	Type             string    `json:"type"`
	Code             string    `json:"code,omitempty"`
	Source           string    `json:"source"`
	Message          string    `json:"message"`
	CreatedAt        time.Time `json:"created_at"`
}

type EventFilter struct {
	Limit      int
	DeviceID   int64
	DeviceName string
	EventType  string
	Start      *time.Time
	End        *time.Time
}

type AlertEvent struct {
	ID             int64      `json:"id"`
	RuleID         *int64     `json:"rule_id,omitempty"`
	DeviceID       int64      `json:"device_id"`
	DeviceIP       string     `json:"device_ip"`
	DeviceName     string     `json:"device_name"`
	Level          string     `json:"level"`
	Code           string     `json:"code"`
	Message        string     `json:"message"`
	Status         string     `json:"status"`
	Assignee       string     `json:"assignee,omitempty"`
	Note           string     `json:"note,omitempty"`
	SilencedUntil  *time.Time `json:"silenced_until,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func NewRepository(db *sql.DB) *Repository {
	key := []byte(os.Getenv("NETPULSE_CRED_KEY"))
	if len(key) != 32 {
		key = nil
	}
	return &Repository{db: db, credKey: key}
}

// EnsureSchema auto-bootstraps database objects for a blank database.
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

// SyncInterfaces upserts interface snapshot for one device and preserves existing remarks.
// It also removes stale interfaces that no longer exist on device.
func (r *Repository) SaveMetrics(
	ctx context.Context,
	deviceID int64,
	ts time.Time,
	metrics []InterfaceMetric,
) error {
	const q = `
		WITH upsert_if AS (
			INSERT INTO interfaces (device_id, "index", name, remark, speed_mbps, oper_status, admin_status)
			VALUES ($2, $3, $4, '', GREATEST($13, 0), NULLIF($14, 0), NULLIF($17, 0))
			ON CONFLICT (device_id, "index")
			DO UPDATE SET
				name = CASE
					WHEN EXCLUDED.name <> '' THEN EXCLUDED.name
					ELSE interfaces.name
				END,
				speed_mbps = CASE
					WHEN $13 > 0 THEN $13
					ELSE interfaces.speed_mbps
				END,
				oper_status = CASE
					WHEN $14 BETWEEN 1 AND 7 THEN $14
					ELSE interfaces.oper_status
				END,
				admin_status = CASE
					WHEN $17 BETWEEN 1 AND 3 THEN $17
					ELSE interfaces.admin_status
				END
			RETURNING id
		),
		inserted_metric AS (
			INSERT INTO metrics (
				ts, device_id, interface_id, cpu_usage, memory_usage, storage_usage, storage_total, storage_free, uptime_sec, traffic_in_bps, traffic_out_bps, traffic_in_status, traffic_out_status
			)
			VALUES ($1, $2, (SELECT id FROM upsert_if LIMIT 1), $5, $6, $7, $8, $9, $10, $11, $12, $15, $16)
			RETURNING interface_id
		),
		latest_upsert AS (
			INSERT INTO interface_latest_metrics (
				interface_id, device_id, ts, traffic_in_bps, traffic_out_bps, traffic_in_status, traffic_out_status, updated_at
			)
			SELECT interface_id, $2, $1, $11, $12, $15, $16, NOW()
			FROM inserted_metric
			ON CONFLICT (interface_id)
			DO UPDATE SET
				device_id = EXCLUDED.device_id,
				ts = EXCLUDED.ts,
				traffic_in_bps = EXCLUDED.traffic_in_bps,
				traffic_out_bps = EXCLUDED.traffic_out_bps,
				traffic_in_status = EXCLUDED.traffic_in_status,
				traffic_out_status = EXCLUDED.traffic_out_status,
				updated_at = NOW()
			WHERE interface_latest_metrics.ts <= EXCLUDED.ts
			RETURNING interface_id
		)
		SELECT interface_id FROM inserted_metric
		UNION ALL
		SELECT interface_id FROM latest_upsert
		LIMIT 1;
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
		inBps := clampTrafficBpsNullable(m.TrafficInBps)
		outBps := clampTrafficBpsNullable(m.TrafficOutBps)

		var interfaceID int64
		if err := tx.QueryRowContext(
			ctx, q, ts, deviceID, m.IfIndex, m.IfName, cpu, mem, clampPercent(m.StorageUsage), clampNonNegative(m.StorageTotal), clampNonNegative(m.StorageFree), m.UptimeSec, inBps, outBps, m.SpeedMbps, m.OperStatus, strings.TrimSpace(m.TrafficInStat), strings.TrimSpace(m.TrafficOutStat), m.AdminStatus,
		).Scan(&interfaceID); err != nil {
			return fmt.Errorf("insert metric ifIndex=%d: %w", m.IfIndex, err)
		}
	}
	if len(metrics) > 0 {
		m := metrics[0]
		const latestDeviceQ = `
			INSERT INTO device_latest_metrics (
				device_id, ts, cpu_usage, memory_usage, storage_usage, storage_total, storage_free, uptime_sec, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
			ON CONFLICT (device_id)
			DO UPDATE SET
				ts = EXCLUDED.ts,
				cpu_usage = EXCLUDED.cpu_usage,
				memory_usage = EXCLUDED.memory_usage,
				storage_usage = EXCLUDED.storage_usage,
				storage_total = EXCLUDED.storage_total,
				storage_free = EXCLUDED.storage_free,
				uptime_sec = EXCLUDED.uptime_sec,
				updated_at = NOW()
			WHERE device_latest_metrics.ts <= EXCLUDED.ts;
		`
		if _, err := tx.ExecContext(
			ctx,
			latestDeviceQ,
			deviceID,
			ts,
			clampPercent(m.CPUUsage),
			clampPercent(m.MemoryUsage),
			clampPercent(m.StorageUsage),
			clampNonNegative(m.StorageTotal),
			clampNonNegative(m.StorageFree),
			m.UptimeSec,
		); err != nil {
			return fmt.Errorf("upsert device latest metric: %w", err)
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

func clampTrafficBpsNullable(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{Valid: false}
	}
	n := *v
	if n < 0 {
		return sql.NullInt64{Valid: false}
	}
	const maxReasonableBps int64 = 9_000_000_000_000_000
	if n > maxReasonableBps {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: n, Valid: true}
}

func clampNonNegative(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	return v
}

func (r *Repository) GetDeviceHistory(
	ctx context.Context, deviceID int64, start, end time.Time, interval string, maxPoints int,
) ([]DeviceHistoryPoint, error) {
	useAgg := end.Sub(start) > 7*24*time.Hour
	interval = strings.TrimSpace(strings.ToLower(interval))
	bucketInterval := ""
	if interval != "" || useAgg {
		bucketInterval = resolveHistoryBucketInterval(end.Sub(start), interval, maxPoints, useAgg)
	}
	q := `
		SELECT ts,
		       AVG(COALESCE(cpu_usage, 0)) AS cpu_usage,
		       AVG(COALESCE(memory_usage, 0)) AS memory_usage
		FROM metrics
		WHERE device_id = $1 AND ts >= $2 AND ts <= $3
		GROUP BY ts
		ORDER BY ts;
	`
	if useAgg {
		q = `
			SELECT bucket AS ts,
			       AVG(COALESCE(avg_cpu_usage, 0)) AS avg_cpu_usage,
			       AVG(COALESCE(avg_memory_usage, 0)) AS avg_memory_usage
			FROM metrics_1m
			WHERE device_id = $1 AND bucket >= $2 AND bucket <= $3
			GROUP BY bucket
			ORDER BY bucket;
		`
	}
	if bucketInterval != "" {
		q = fmt.Sprintf(`
			SELECT time_bucket('%s', bucket) AS ts,
			       AVG(COALESCE(avg_cpu_usage, 0)) AS cpu_usage,
			       AVG(COALESCE(avg_memory_usage, 0)) AS memory_usage
			FROM metrics_1m
			WHERE device_id = $1 AND bucket >= $2 AND bucket <= $3
			GROUP BY 1
			ORDER BY 1;
		`, bucketInterval)
		if !useAgg {
			q = fmt.Sprintf(`
				SELECT time_bucket('%s', ts) AS ts,
				       AVG(COALESCE(cpu_usage, 0)) AS cpu_usage,
				       AVG(COALESCE(memory_usage, 0)) AS memory_usage
				FROM metrics
				WHERE device_id = $1 AND ts >= $2 AND ts <= $3
				GROUP BY 1
				ORDER BY 1;
			`, bucketInterval)
		}
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
	return decimateDeviceHistory(out, end.Sub(start), maxPoints), nil
}

func (r *Repository) GetDeviceStorageHistory(
	ctx context.Context, deviceID int64, start, end time.Time, interval string, maxPoints int,
) ([]DeviceStorageHistoryPoint, error) {
	interval = strings.TrimSpace(strings.ToLower(interval))
	bucketInterval := resolveHistoryBucketInterval(end.Sub(start), interval, maxPoints, false)
	q := `
		SELECT ts,
		       COALESCE(storage_usage, 0) AS storage_usage,
		       COALESCE(storage_total, 0) AS storage_total,
		       COALESCE(storage_free, 0) AS storage_free
		FROM metrics
		WHERE device_id = $1 AND ts >= $2 AND ts <= $3
		ORDER BY ts;
	`
	if bucketInterval != "" {
		q = fmt.Sprintf(`
			SELECT time_bucket('%s', ts) AS ts,
			       AVG(COALESCE(storage_usage, 0)) AS storage_usage,
			       AVG(COALESCE(storage_total, 0)) AS storage_total,
			       AVG(COALESCE(storage_free, 0)) AS storage_free
			FROM metrics
			WHERE device_id = $1 AND ts >= $2 AND ts <= $3
			GROUP BY 1
			ORDER BY 1;
		`, bucketInterval)
	}
	rows, err := r.db.QueryContext(ctx, q, deviceID, start, end)
	if err != nil {
		return nil, fmt.Errorf("get device storage history: %w", err)
	}
	defer rows.Close()
	out := make([]DeviceStorageHistoryPoint, 0)
	for rows.Next() {
		var p DeviceStorageHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.StorageUsage, &p.StorageTotal, &p.StorageFree); err != nil {
			return nil, fmt.Errorf("scan device storage history: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device storage history: %w", err)
	}
	target := 2500
	if maxPoints > 0 {
		target = maxPoints
	}
	if len(out) <= target {
		return out, nil
	}
	step := int(math.Ceil(float64(len(out)) / float64(target)))
	if step < 1 {
		step = 1
	}
	cut := make([]DeviceStorageHistoryPoint, 0, target)
	for i := 0; i < len(out); i += step {
		cut = append(cut, out[i])
	}
	if cut[len(cut)-1].Timestamp != out[len(out)-1].Timestamp {
		cut = append(cut, out[len(out)-1])
	}
	return cut, nil
}
