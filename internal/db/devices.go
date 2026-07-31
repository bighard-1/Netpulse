package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (r *Repository) GetDeviceByID(ctx context.Context, id int64) (*DeviceStatus, error) {
	runtime, _ := r.GetRuntimeSettings(ctx)
	onlineWindow := time.Duration(runtime.StatusOnlineWindowSec) * time.Second
	if onlineWindow <= 0 {
		onlineWindow = 5 * time.Minute
	}
	const q = `
		SELECT d.id, host(d.ip), COALESCE(d.name, host(d.ip)), d.template_id, d.brand, d.community, COALESCE(d.write_community,''), d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.maintenance_mode, FALSE),
		       COALESCE(d.monitoring_paused, FALSE), COALESCE(d.monitoring_pause_reason, ''),
		       COALESCE(NULLIF(d.device_tier,''), 'access'),
		       COALESCE(d.poll_interval_sec,0), COALESCE(d.cpu_threshold,0), COALESCE(d.mem_threshold,0),
		       COALESCE(d.remark, ''), d.created_at, lm.ts AS last_ts, COALESCE(dl.message, ''), COALESCE(lm.uptime_sec, 0)
		FROM devices d
		LEFT JOIN device_latest_metrics lm ON lm.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT message
			FROM device_logs
			WHERE device_id = d.id
			ORDER BY created_at DESC
			LIMIT 1
		) dl ON TRUE
		WHERE d.id = $1 AND d.deleted_at IS NULL;
	`
	var ds DeviceStatus
	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&ds.ID, &ds.IP, &ds.Name, &ds.TemplateID, &ds.Brand, &ds.Community, &ds.WriteCommunity, &ds.SNMPVersion, &ds.SNMPPort,
		&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
		&ds.MaintenanceMode, &ds.MonitoringPaused, &ds.MonitoringPauseReason,
		&ds.DeviceTier,
		&ds.PollIntervalSec, &ds.CPUThreshold, &ds.MemThreshold,
		&ds.Remark, &ds.CreatedAt, &ds.LastMetricAt, &ds.StatusReason, &ds.UptimeSec,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get device by id: %w", err)
	}
	ds.Status = "unknown"
	ds.Community = r.decryptOpt(ds.Community)
	ds.WriteCommunity = r.decryptOpt(ds.WriteCommunity)
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
	ds.Uptime = formatUptime(ds.UptimeSec)

	const iq = `
		SELECT i.id, i.device_id, i."index",
		       COALESCE(NULLIF(i.custom_name,''), i.name) AS display_name,
		       i.name AS raw_name,
		       COALESCE(i.remark, ''),
		       COALESCE(i.speed_mbps, 0),
		       COALESCE(i.oper_status, 0),
		       COALESCE(i.admin_status, 0),
		       COALESCE(m.traffic_in_bps, 0),
		       COALESCE(m.traffic_out_bps, 0)
		FROM interfaces i
		LEFT JOIN interface_latest_metrics m ON m.interface_id = i.id
		WHERE i.device_id = $1
		ORDER BY i."index";
	`
	rows, err := r.db.QueryContext(ctx, iq, id)
	if err != nil {
		return nil, fmt.Errorf("query interfaces by device id: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itf Interface
		if err := rows.Scan(&itf.ID, &itf.DeviceID, &itf.Index, &itf.Name, &itf.RawName, &itf.Remark, &itf.SpeedMbps, &itf.OperStatus, &itf.AdminStatus, &itf.TrafficInBps, &itf.TrafficOutBps); err != nil {
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
	d.WriteCommunity = r.encryptOpt(d.WriteCommunity)
	d.V3AuthPass = r.encryptOpt(d.V3AuthPass)
	d.V3PrivPass = r.encryptOpt(d.V3PrivPass)
	const q = `
		INSERT INTO devices (
			ip, name, template_id, brand, community, write_community, snmp_version, snmp_port,
			v3_username, v3_auth_protocol, v3_auth_password, v3_priv_protocol, v3_priv_password, v3_security_level, maintenance_mode,
			monitoring_paused, monitoring_pause_reason,
			device_tier, poll_interval_sec, cpu_threshold, mem_threshold, remark
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(
		ctx, q, d.IP, d.Name, d.TemplateID, d.Brand, d.Community, d.WriteCommunity, d.SNMPVersion, d.SNMPPort, d.V3Username, d.V3AuthProto,
		d.V3AuthPass, d.V3PrivProto, d.V3PrivPass, d.V3SecLevel, d.MaintenanceMode, d.MonitoringPaused, d.MonitoringPauseReason, d.DeviceTier, d.PollIntervalSec, d.CPUThreshold, d.MemThreshold, d.Remark,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("add device: %w", err)
	}
	return id, nil
}
func (r *Repository) DeleteDevice(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete device: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '3s';`); err != nil {
		return fmt.Errorf("prepare fast device delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL statement_timeout = '15s';`); err != nil {
		return fmt.Errorf("prepare fast device delete: %w", err)
	}
	stmts := []string{
		`DELETE FROM interface_latest_metrics WHERE device_id = $1;`,
		`DELETE FROM device_latest_metrics WHERE device_id = $1;`,
		`DELETE FROM alert_events WHERE device_id = $1;`,
		`DELETE FROM alert_rules WHERE device_id = $1;`,
		`DELETE FROM device_capabilities WHERE device_id = $1;`,
		`DELETE FROM topology_links WHERE src_device_id = $1 OR dst_device_id = $1;`,
		`DELETE FROM topology_nodes WHERE device_id = $1;`,
		`UPDATE devices
		 SET deleted_at = NOW(),
		     monitoring_paused = TRUE,
		     monitoring_pause_reason = '资产已删除，历史数据保留待清理'
		 WHERE id = $1 AND deleted_at IS NULL;`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
			return fmt.Errorf("delete device: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	return nil
}
func (r *Repository) ListDevices(ctx context.Context) ([]Device, error) {
	const q = `
		SELECT id, host(ip), COALESCE(name, host(ip)), template_id, brand, community, COALESCE(write_community,''), snmp_version, snmp_port,
		       COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''),
		       COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''),
		       COALESCE(maintenance_mode,FALSE),
		       COALESCE(monitoring_paused,FALSE), COALESCE(monitoring_pause_reason,''),
		       COALESCE(NULLIF(device_tier,''), 'access'),
		       COALESCE(poll_interval_sec,0), COALESCE(cpu_threshold,0), COALESCE(mem_threshold,0),
		       COALESCE(remark, ''), created_at
		FROM devices
		WHERE deleted_at IS NULL
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
		if err := rows.Scan(&d.ID, &d.IP, &d.Name, &d.TemplateID, &d.Brand, &d.Community, &d.WriteCommunity, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.MaintenanceMode, &d.MonitoringPaused, &d.MonitoringPauseReason, &d.DeviceTier, &d.PollIntervalSec, &d.CPUThreshold, &d.MemThreshold, &d.Remark, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		d.Community = r.decryptOpt(d.Community)
		d.WriteCommunity = r.decryptOpt(d.WriteCommunity)
		d.V3AuthPass = r.decryptOpt(d.V3AuthPass)
		d.V3PrivPass = r.decryptOpt(d.V3PrivPass)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return out, nil
}
func (r *Repository) FindDeviceByIP(ctx context.Context, ip string) (*Device, error) {
	const q = `SELECT id, host(ip), COALESCE(name, host(ip)), template_id, brand, community, COALESCE(write_community,''), snmp_version, snmp_port, COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''), COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''), COALESCE(maintenance_mode,FALSE), COALESCE(monitoring_paused,FALSE), COALESCE(monitoring_pause_reason,''), COALESCE(NULLIF(device_tier,''),'access'), COALESCE(poll_interval_sec,0), COALESCE(cpu_threshold,0), COALESCE(mem_threshold,0), COALESCE(remark,''), created_at FROM devices WHERE ip = $1::inet AND deleted_at IS NULL LIMIT 1;`
	var d Device
	if err := r.db.QueryRowContext(ctx, q, ip).Scan(&d.ID, &d.IP, &d.Name, &d.TemplateID, &d.Brand, &d.Community, &d.WriteCommunity, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.MaintenanceMode, &d.MonitoringPaused, &d.MonitoringPauseReason, &d.DeviceTier, &d.PollIntervalSec, &d.CPUThreshold, &d.MemThreshold, &d.Remark, &d.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d.Community = r.decryptOpt(d.Community)
	d.WriteCommunity = r.decryptOpt(d.WriteCommunity)
	d.V3AuthPass = r.decryptOpt(d.V3AuthPass)
	d.V3PrivPass = r.decryptOpt(d.V3PrivPass)
	return &d, nil
}
func (r *Repository) ListDevicesWithStatus(ctx context.Context) ([]DeviceStatus, error) {
	runtime, _ := r.GetRuntimeSettings(ctx)
	onlineWindow := time.Duration(runtime.StatusOnlineWindowSec) * time.Second
	if onlineWindow <= 0 {
		onlineWindow = 5 * time.Minute
	}
	const q = `
		WITH latest_device_logs AS (
			SELECT DISTINCT ON (device_id)
			       device_id,
			       message
			FROM device_logs
			ORDER BY device_id, created_at DESC
		)
		SELECT d.id, host(d.ip), COALESCE(d.name, host(d.ip)), d.template_id, d.brand, d.community, COALESCE(d.write_community,''), d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.maintenance_mode,FALSE),
		       COALESCE(d.monitoring_paused,FALSE), COALESCE(d.monitoring_pause_reason,''),
		       COALESCE(NULLIF(d.device_tier,''), 'access'),
		       COALESCE(d.poll_interval_sec,0), COALESCE(d.cpu_threshold,0), COALESCE(d.mem_threshold,0),
		       COALESCE(d.remark, ''), d.created_at, lm.ts AS last_ts, COALESCE(dl.message, ''),
		       COALESCE(lm.storage_usage, 0), COALESCE(lm.storage_total, 0), COALESCE(lm.storage_free, 0), COALESCE(lm.uptime_sec, 0)
		FROM devices d
		LEFT JOIN device_latest_metrics lm ON lm.device_id = d.id
		LEFT JOIN latest_device_logs dl ON dl.device_id = d.id
		WHERE d.deleted_at IS NULL
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
			&ds.ID, &ds.IP, &ds.Name, &ds.TemplateID, &ds.Brand, &ds.Community, &ds.WriteCommunity, &ds.SNMPVersion, &ds.SNMPPort,
			&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
			&ds.MaintenanceMode, &ds.MonitoringPaused, &ds.MonitoringPauseReason,
			&ds.DeviceTier,
			&ds.PollIntervalSec, &ds.CPUThreshold, &ds.MemThreshold,
			&ds.Remark, &ds.CreatedAt, &ds.LastMetricAt, &ds.StatusReason,
			&ds.StorageUsage, &ds.StorageTotal, &ds.StorageFree, &ds.UptimeSec,
		); err != nil {
			return nil, fmt.Errorf("scan device status: %w", err)
		}
		ds.Status = "unknown"
		ds.Community = r.decryptOpt(ds.Community)
		ds.WriteCommunity = r.decryptOpt(ds.WriteCommunity)
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
		ds.Uptime = formatUptime(ds.UptimeSec)
		out = append(out, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device status: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}

	const iq = `
		SELECT i.id, i.device_id, i."index",
		       COALESCE(NULLIF(i.custom_name,''), i.name) AS display_name,
		       i.name AS raw_name,
		       COALESCE(i.remark, ''),
		       COALESCE(i.speed_mbps, 0),
		       COALESCE(i.oper_status, 0),
		       COALESCE(i.admin_status, 0),
		       COALESCE(m.traffic_in_bps, 0),
		       COALESCE(m.traffic_out_bps, 0)
		FROM interfaces i
		LEFT JOIN interface_latest_metrics m ON m.interface_id = i.id
		ORDER BY i.device_id, i."index";
	`
	iRows, err := r.db.QueryContext(ctx, iq)
	if err != nil {
		return nil, fmt.Errorf("query interfaces for devices: %w", err)
	}
	defer iRows.Close()

	byDevice := make(map[int64][]Interface)
	for iRows.Next() {
		var itf Interface
		if err := iRows.Scan(&itf.ID, &itf.DeviceID, &itf.Index, &itf.Name, &itf.RawName, &itf.Remark, &itf.SpeedMbps, &itf.OperStatus, &itf.AdminStatus, &itf.TrafficInBps, &itf.TrafficOutBps); err != nil {
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
func (r *Repository) UpdateDevice(ctx context.Context, d Device) error {
	const q = `
		UPDATE devices
		SET ip = $2::inet,
		    name = $3,
		    brand = $4,
		    remark = $5,
		    maintenance_mode = $6,
		    monitoring_paused = $7,
		    monitoring_pause_reason = $8,
		    device_tier = $9,
		    poll_interval_sec = $10,
		    cpu_threshold = $11,
		    mem_threshold = $12
		WHERE id = $1 AND deleted_at IS NULL;
	`
	if _, err := r.db.ExecContext(ctx, q, d.ID, strings.TrimSpace(d.IP), strings.TrimSpace(d.Name), strings.TrimSpace(d.Brand), d.Remark, d.MaintenanceMode, d.MonitoringPaused, strings.TrimSpace(d.MonitoringPauseReason), strings.TrimSpace(d.DeviceTier), d.PollIntervalSec, d.CPUThreshold, d.MemThreshold); err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return nil
}

func formatUptime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%d天 %02d:%02d", d, h, m)
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}
