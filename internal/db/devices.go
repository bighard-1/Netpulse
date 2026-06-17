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
		SELECT d.id, host(d.ip), COALESCE(d.name, host(d.ip)), d.template_id, d.brand, d.community, d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.maintenance_mode, FALSE),
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
		WHERE d.id = $1;
	`
	var ds DeviceStatus
	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&ds.ID, &ds.IP, &ds.Name, &ds.TemplateID, &ds.Brand, &ds.Community, &ds.SNMPVersion, &ds.SNMPPort,
		&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
		&ds.MaintenanceMode,
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
	d.V3AuthPass = r.encryptOpt(d.V3AuthPass)
	d.V3PrivPass = r.encryptOpt(d.V3PrivPass)
	const q = `
		INSERT INTO devices (
			ip, name, template_id, brand, community, snmp_version, snmp_port,
			v3_username, v3_auth_protocol, v3_auth_password, v3_priv_protocol, v3_priv_password, v3_security_level, maintenance_mode,
			device_tier, poll_interval_sec, cpu_threshold, mem_threshold, remark
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id;
	`
	var id int64
	if err := r.db.QueryRowContext(
		ctx, q, d.IP, d.Name, d.TemplateID, d.Brand, d.Community, d.SNMPVersion, d.SNMPPort, d.V3Username, d.V3AuthProto,
		d.V3AuthPass, d.V3PrivProto, d.V3PrivPass, d.V3SecLevel, d.MaintenanceMode, d.DeviceTier, d.PollIntervalSec, d.CPUThreshold, d.MemThreshold, d.Remark,
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
		SELECT id, host(ip), COALESCE(name, host(ip)), template_id, brand, community, snmp_version, snmp_port,
		       COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''),
		       COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''),
		       COALESCE(maintenance_mode,FALSE),
		       COALESCE(NULLIF(device_tier,''), 'access'),
		       COALESCE(poll_interval_sec,0), COALESCE(cpu_threshold,0), COALESCE(mem_threshold,0),
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
		if err := rows.Scan(&d.ID, &d.IP, &d.Name, &d.TemplateID, &d.Brand, &d.Community, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.MaintenanceMode, &d.DeviceTier, &d.PollIntervalSec, &d.CPUThreshold, &d.MemThreshold, &d.Remark, &d.CreatedAt); err != nil {
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
func (r *Repository) FindDeviceByIP(ctx context.Context, ip string) (*Device, error) {
	const q = `SELECT id, host(ip), COALESCE(name, host(ip)), template_id, brand, community, snmp_version, snmp_port, COALESCE(v3_username,''), COALESCE(v3_auth_protocol,''), COALESCE(v3_auth_password,''), COALESCE(v3_priv_protocol,''), COALESCE(v3_priv_password,''), COALESCE(v3_security_level,''), COALESCE(maintenance_mode,FALSE), COALESCE(NULLIF(device_tier,''),'access'), COALESCE(poll_interval_sec,0), COALESCE(cpu_threshold,0), COALESCE(mem_threshold,0), COALESCE(remark,''), created_at FROM devices WHERE ip = $1::inet LIMIT 1;`
	var d Device
	if err := r.db.QueryRowContext(ctx, q, ip).Scan(&d.ID, &d.IP, &d.Name, &d.TemplateID, &d.Brand, &d.Community, &d.SNMPVersion, &d.SNMPPort, &d.V3Username, &d.V3AuthProto, &d.V3AuthPass, &d.V3PrivProto, &d.V3PrivPass, &d.V3SecLevel, &d.MaintenanceMode, &d.DeviceTier, &d.PollIntervalSec, &d.CPUThreshold, &d.MemThreshold, &d.Remark, &d.CreatedAt); err != nil {
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
		SELECT d.id, host(d.ip), COALESCE(d.name, host(d.ip)), d.template_id, d.brand, d.community, d.snmp_version, d.snmp_port,
		       COALESCE(d.v3_username,''), COALESCE(d.v3_auth_protocol,''), COALESCE(d.v3_auth_password,''),
		       COALESCE(d.v3_priv_protocol,''), COALESCE(d.v3_priv_password,''), COALESCE(d.v3_security_level,''),
		       COALESCE(d.maintenance_mode,FALSE),
		       COALESCE(NULLIF(d.device_tier,''), 'access'),
		       COALESCE(d.poll_interval_sec,0), COALESCE(d.cpu_threshold,0), COALESCE(d.mem_threshold,0),
		       COALESCE(d.remark, ''), d.created_at, lm.ts AS last_ts, COALESCE(dl.message, ''),
		       COALESCE(lm.storage_usage, 0), COALESCE(lm.storage_total, 0), COALESCE(lm.storage_free, 0), COALESCE(lm.uptime_sec, 0)
		FROM devices d
		LEFT JOIN device_latest_metrics lm ON lm.device_id = d.id
		LEFT JOIN latest_device_logs dl ON dl.device_id = d.id
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
			&ds.ID, &ds.IP, &ds.Name, &ds.TemplateID, &ds.Brand, &ds.Community, &ds.SNMPVersion, &ds.SNMPPort,
			&ds.V3Username, &ds.V3AuthProto, &ds.V3AuthPass, &ds.V3PrivProto, &ds.V3PrivPass, &ds.V3SecLevel,
			&ds.MaintenanceMode,
			&ds.DeviceTier,
			&ds.PollIntervalSec, &ds.CPUThreshold, &ds.MemThreshold,
			&ds.Remark, &ds.CreatedAt, &ds.LastMetricAt, &ds.StatusReason,
			&ds.StorageUsage, &ds.StorageTotal, &ds.StorageFree, &ds.UptimeSec,
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
		SET name = $2,
		    brand = $3,
		    remark = $4,
		    maintenance_mode = $5,
		    device_tier = $6,
		    poll_interval_sec = $7,
		    cpu_threshold = $8,
		    mem_threshold = $9
		WHERE id = $1;
	`
	if _, err := r.db.ExecContext(ctx, q, d.ID, strings.TrimSpace(d.Name), strings.TrimSpace(d.Brand), d.Remark, d.MaintenanceMode, strings.TrimSpace(d.DeviceTier), d.PollIntervalSec, d.CPUThreshold, d.MemThreshold); err != nil {
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
