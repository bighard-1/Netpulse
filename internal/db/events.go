package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (r *Repository) GetRecentEvents(ctx context.Context, limit int) ([]RecentEvent, error) {
	return r.QueryRecentEvents(ctx, EventFilter{Limit: limit})
}

func (r *Repository) QueryRecentEvents(ctx context.Context, f EventFilter) ([]RecentEvent, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 30
	}
	eventType := strings.TrimSpace(strings.ToLower(f.EventType))
	if eventType == "all" {
		eventType = ""
	}
	deviceName := strings.TrimSpace(strings.ToLower(f.DeviceName))
	var startArg any
	if f.Start != nil {
		startArg = *f.Start
	}
	var endArg any
	if f.End != nil {
		endArg = *f.End
	}
	const q = `
		SELECT id, device_id, device_ip, device_name, interface_id, interface_name, interface_raw_name, interface_remark, interface_index, level, event_type, code, source, message, created_at
		FROM (
			SELECT l.id AS id,
			       l.device_id AS device_id,
			       host(d.ip) AS device_ip,
			       COALESCE(d.name, host(d.ip)) AS device_name,
			       i.id AS interface_id,
			       COALESCE(NULLIF(i.custom_name,''), i.name, '') AS interface_name,
			       COALESCE(i.name, '') AS interface_raw_name,
			       COALESCE(i.remark, '') AS interface_remark,
			       COALESCE(i."index", 0) AS interface_index,
			       l.level AS level,
			       CASE
			         WHEN l.message LIKE '[DEVICE_%' THEN 'device_status'
			         WHEN l.message LIKE '[PORT_%' THEN 'port_status'
			         WHEN l.message LIKE '[POLL_%' OR l.message LIKE '[TCP161_%' OR l.message LIKE '[HOST_%' THEN 'polling'
			         WHEN l.message LIKE '[SYSLOG]%' OR l.message LIKE '[TRAP]%' THEN 'log'
			         WHEN UPPER(l.level) IN ('ERROR','CRITICAL','WARNING') THEN 'log'
			         ELSE 'log'
			       END AS event_type,
			       COALESCE(NULLIF(substring(l.message from '^\[([^\]]+)\]'), ''), 'DEVICE_LOG') AS code,
			       'device_log' AS source,
			       l.message AS message,
			       l.created_at AS created_at
			FROM (
				SELECT id, device_id, level, message, created_at
				FROM device_logs
				WHERE NOT (
					UPPER(level)='INFO'
					AND (
						message LIKE '[OK]%'
						OR message LIKE '[ALERT_MUTED]%'
						OR message LIKE '[POLL_OK]%'
						OR message LIKE '[TCP161_OK]%'
						OR message LIKE '[HOST_OK]%'
					)
				)
				ORDER BY created_at DESC
				LIMIT GREATEST($1 * 5, 300)
			) l
			JOIN devices d ON d.id = l.device_id
			LEFT JOIN interfaces i
			  ON i.device_id = l.device_id
			 AND i.index = NULLIF(substring(l.message from 'ifIndex=([0-9]+)'), '')::integer
			UNION ALL
			SELECT (ae.id + 1000000000) AS id,
			       ae.device_id AS device_id,
			       host(d.ip) AS device_ip,
			       COALESCE(d.name, host(d.ip)) AS device_name,
			       NULL::bigint AS interface_id,
			       ''::text AS interface_name,
			       ''::text AS interface_raw_name,
			       ''::text AS interface_remark,
			       0::integer AS interface_index,
			       UPPER(ae.level) AS level,
			       'alert' AS event_type,
			       COALESCE(ae.code, 'ALERT') AS code,
			       'alert_event' AS source,
			       ('[' || COALESCE(ae.code, 'ALERT') || '] ' || ae.message) AS message,
			       ae.created_at AS created_at
			FROM alert_events ae
			JOIN devices d ON d.id = ae.device_id
		) e
		WHERE ($2::bigint = 0 OR e.device_id = $2)
		  AND ($3 = '' OR lower(e.device_name) LIKE '%' || $3 || '%' OR lower(e.device_ip) LIKE '%' || $3 || '%')
		  AND ($4 = '' OR e.event_type = $4)
		  AND ($5::timestamptz IS NULL OR e.created_at >= $5)
		  AND ($6::timestamptz IS NULL OR e.created_at <= $6)
		ORDER BY created_at DESC
		LIMIT $1;
	`
	rows, err := r.db.QueryContext(ctx, q, limit, f.DeviceID, deviceName, eventType, startArg, endArg)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}
	defer rows.Close()

	out := make([]RecentEvent, 0, limit)
	for rows.Next() {
		var e RecentEvent
		var interfaceID sql.NullInt64
		if err := rows.Scan(
			&e.ID,
			&e.DeviceID,
			&e.DeviceIP,
			&e.DeviceName,
			&interfaceID,
			&e.InterfaceName,
			&e.InterfaceRawName,
			&e.InterfaceRemark,
			&e.InterfaceIndex,
			&e.Level,
			&e.Type,
			&e.Code,
			&e.Source,
			&e.Message,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recent event: %w", err)
		}
		if interfaceID.Valid {
			v := interfaceID.Int64
			e.InterfaceID = &v
		}
		enrichRecentPortEvent(&e)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent events: %w", err)
	}
	return out, nil
}

func enrichRecentPortEvent(e *RecentEvent) {
	if e == nil || e.Type != "port_status" {
		return
	}
	rawName := strings.TrimSpace(e.InterfaceRawName)
	displayName := strings.TrimSpace(e.InterfaceName)
	if rawName == "" && displayName == "" && e.InterfaceIndex <= 0 {
		return
	}
	if displayName != "" && strings.Contains(e.Message, displayName) {
		return
	}
	label := rawName
	if label == "" && e.InterfaceIndex > 0 {
		label = fmt.Sprintf("ifIndex=%d", e.InterfaceIndex)
	}
	if displayName != "" {
		if label == "" || displayName == label {
			label = displayName
		} else {
			label = fmt.Sprintf("%s / %s", label, displayName)
		}
	}
	if label == "" || strings.Contains(e.Message, label) {
		return
	}
	e.Message = fmt.Sprintf("%s · %s", label, e.Message)
}
