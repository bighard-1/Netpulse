package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

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
func (r *Repository) UpdateDeviceRemark(ctx context.Context, id int64, remark string) error {
	const q = `UPDATE devices SET remark = $2 WHERE id = $1 AND deleted_at IS NULL;`
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
func (r *Repository) GetInterfaceByID(ctx context.Context, id int64) (*Interface, error) {
	const q = `
		SELECT i.id,
		       i.device_id,
		       host(d.ip) AS device_ip,
		       COALESCE(d.name, host(d.ip)) AS device_name,
		       i."index",
		       COALESCE(NULLIF(i.custom_name,''), i.name) AS display_name,
		       i.name AS raw_name,
		       COALESCE(i.remark, ''),
		       COALESCE(i.speed_mbps, 0),
		       COALESCE(i.oper_status, 0),
		       COALESCE(i.admin_status, 0),
		       COALESCE(m.traffic_in_bps, 0),
		       COALESCE(m.traffic_out_bps, 0)
		FROM interfaces i
		JOIN devices d ON d.id = i.device_id
		LEFT JOIN interface_latest_metrics m ON m.interface_id = i.id
		WHERE i.id = $1 AND d.deleted_at IS NULL
		LIMIT 1;
	`
	var itf Interface
	if err := r.db.QueryRowContext(ctx, q, id).Scan(
		&itf.ID,
		&itf.DeviceID,
		&itf.DeviceIP,
		&itf.DeviceName,
		&itf.Index,
		&itf.Name,
		&itf.RawName,
		&itf.Remark,
		&itf.SpeedMbps,
		&itf.OperStatus,
		&itf.AdminStatus,
		&itf.TrafficInBps,
		&itf.TrafficOutBps,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get interface by id: %w", err)
	}
	return &itf, nil
}
func (r *Repository) GetInterfaceByDeviceIndex(ctx context.Context, deviceID int64, ifIndex int) (*Interface, error) {
	const q = `
		SELECT id,
		       device_id,
		       "index",
		       COALESCE(NULLIF(custom_name,''), name) AS display_name,
		       name AS raw_name,
		       COALESCE(remark, '')
		FROM interfaces
		WHERE device_id = $1 AND "index" = $2
		LIMIT 1;
	`
	var itf Interface
	if err := r.db.QueryRowContext(ctx, q, deviceID, ifIndex).Scan(
		&itf.ID,
		&itf.DeviceID,
		&itf.Index,
		&itf.Name,
		&itf.RawName,
		&itf.Remark,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get interface by device index: %w", err)
	}
	return &itf, nil
}
func (r *Repository) UpdateInterfaceProfile(ctx context.Context, id int64, name, remark *string) error {
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

	nameTrim := ""
	hasName := name != nil
	if hasName {
		nameTrim = strings.TrimSpace(*name)
	}
	if hasName && nameTrim != "" {
		const cq = `
			SELECT EXISTS(
				SELECT 1 FROM interfaces
				WHERE device_id = $1
				  AND lower(COALESCE(NULLIF(custom_name,''), name)) = lower($2)
				  AND id <> $3
			);
		`
		var exists bool
		if err := r.db.QueryRowContext(ctx, cq, deviceID, nameTrim, id).Scan(&exists); err != nil {
			return fmt.Errorf("check interface name conflict: %w", err)
		}
		if exists {
			return fmt.Errorf("interface name conflict in this device")
		}
	}

	hasRemark := remark != nil
	remarkVal := ""
	if hasRemark {
		remarkVal = *remark
	}
	clearName := hasName && nameTrim == ""
	setName := hasName && nameTrim != ""

	const q = `
		UPDATE interfaces
		SET custom_name = CASE
				WHEN $2 THEN NULL
				WHEN $3 THEN NULLIF($4, '')
				ELSE custom_name
			END,
			remark = CASE
				WHEN $5 THEN $6
				ELSE remark
			END
		WHERE id = $1;
	`
	if _, err := r.db.ExecContext(ctx, q, id, clearName, setName, nameTrim, hasRemark, remarkVal); err != nil {
		return fmt.Errorf("update interface profile: %w", err)
	}
	return nil
}
