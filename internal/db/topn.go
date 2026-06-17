package db

import (
	"context"
	"fmt"
)

func (r *Repository) GetInterfaceTopBySpeedClass(ctx context.Context, speedMin, speedMax int, limit int) ([]Interface, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	const q = `
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
		WHERE i.speed_mbps >= $1 AND ($2 = 0 OR i.speed_mbps < $2)
		ORDER BY (COALESCE(m.traffic_in_bps, 0) + COALESCE(m.traffic_out_bps, 0)) DESC
		LIMIT $3;
	`
	rows, err := r.db.QueryContext(ctx, q, speedMin, speedMax, limit)
	if err != nil {
		return nil, fmt.Errorf("get interface top by speed class: %w", err)
	}
	defer rows.Close()
	out := make([]Interface, 0, limit)
	for rows.Next() {
		var it Interface
		if err := rows.Scan(&it.ID, &it.DeviceID, &it.Index, &it.Name, &it.RawName, &it.Remark, &it.SpeedMbps, &it.OperStatus, &it.AdminStatus, &it.TrafficInBps, &it.TrafficOutBps); err != nil {
			return nil, fmt.Errorf("scan interface top by speed class: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface top by speed class: %w", err)
	}
	return out, nil
}
