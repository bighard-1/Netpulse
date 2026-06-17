package db

import (
	"context"
	"strings"
)

func (r *Repository) GlobalSearch(ctx context.Context, q string, limit int, ctxDeviceID int64) ([]GlobalSearchResult, error) {
	kw := "%" + strings.TrimSpace(q) + "%"
	if strings.TrimSpace(q) == "" {
		return []GlobalSearchResult{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	const sqlq = `
		WITH dev AS (
			SELECT
				'device'::text AS category,
				d.id::bigint AS id,
				COALESCE(d.name, host(d.ip)) AS title,
				('IP='||host(d.ip)||' 品牌='||d.brand||' 备注='||COALESCE(d.remark,'')) AS sub,
				'device'::text AS type,
				d.id::bigint AS device_id,
				0::bigint AS interface_id,
				COALESCE(d.name, host(d.ip)) AS device_name,
				host(d.ip) AS device_ip,
				''::text AS interface_name,
				''::text AS interface_custom_name,
				''::text AS interface_remark,
				CASE
					WHEN COALESCE(d.name,'') ILIKE $1 THEN 'device_name'
					WHEN host(d.ip) ILIKE $1 THEN 'device_ip'
					WHEN COALESCE(d.remark,'') ILIKE $1 THEN 'device_remark'
					ELSE 'device_brand'
				END AS match_field,
				COALESCE(d.remark,'') AS snippet,
				CASE WHEN $3 > 0 AND d.id = $3 THEN 0 ELSE 1 END AS priority_scope,
				similarity(COALESCE(d.name, host(d.ip)), $4) AS sim
			FROM devices d
			WHERE host(d.ip) ILIKE $1 OR COALESCE(d.name,'') ILIKE $1 OR d.brand ILIKE $1 OR COALESCE(d.remark,'') ILIKE $1
		),
		ports AS (
			SELECT
				'interface'::text AS category,
				i.id::bigint AS id,
				COALESCE(NULLIF(i.custom_name,''), i.name) AS title,
				('设备='||COALESCE(d.name,host(d.ip))||' ifIndex='||i."index"||' 备注='||COALESCE(i.remark,'')) AS sub,
				'port'::text AS type,
				d.id::bigint AS device_id,
				i.id::bigint AS interface_id,
				COALESCE(d.name, host(d.ip)) AS device_name,
				host(d.ip) AS device_ip,
				i.name AS interface_name,
				COALESCE(i.custom_name,'') AS interface_custom_name,
				COALESCE(i.remark,'') AS interface_remark,
				CASE
					WHEN COALESCE(i.custom_name,'') ILIKE $1 THEN 'port_custom_name'
					WHEN i.name ILIKE $1 THEN 'port_name'
					WHEN COALESCE(i.remark,'') ILIKE $1 THEN 'port_remark'
					WHEN COALESCE(d.name,'') ILIKE $1 THEN 'device_name'
					ELSE 'device_ip'
				END AS match_field,
				COALESCE(i.remark,'') AS snippet,
				CASE WHEN $3 > 0 AND d.id = $3 THEN 0 ELSE 1 END AS priority_scope,
				GREATEST(
					similarity(i.name, $4),
					similarity(COALESCE(i.custom_name,''), $4),
					similarity(COALESCE(d.name,''), $4),
					similarity(host(d.ip), $4)
				) AS sim
			FROM interfaces i
			JOIN devices d ON d.id=i.device_id
			WHERE i.name ILIKE $1
			   OR COALESCE(i.custom_name,'') ILIKE $1
			   OR COALESCE(i.remark,'') ILIKE $1
			   OR host(d.ip) ILIKE $1
			   OR COALESCE(d.name,'') ILIKE $1
		),
		all_hits AS (
			SELECT * FROM dev
			UNION ALL
			SELECT * FROM ports
		)
		SELECT
			category, id, title, sub, type, device_id, interface_id, device_name, device_ip,
			interface_name, interface_custom_name, interface_remark, match_field, snippet
		FROM all_hits
		ORDER BY priority_scope ASC, device_name ASC, category ASC, sim DESC, id DESC
		LIMIT $2;
	`
	rows, err := r.db.QueryContext(ctx, sqlq, kw, limit, ctxDeviceID, strings.TrimSpace(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GlobalSearchResult{}
	for rows.Next() {
		var x GlobalSearchResult
		if err := rows.Scan(
			&x.Category, &x.ID, &x.Title, &x.Sub, &x.Type, &x.DeviceID, &x.InterfaceID,
			&x.DeviceName, &x.DeviceIP, &x.InterfaceName, &x.InterfaceCustomName, &x.InterfaceRemark,
			&x.MatchField, &x.Snippet,
		); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
