package db

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) GetDeviceLogs(ctx context.Context, deviceID int64) ([]DeviceLog, error) {
	return r.GetDeviceLogsFiltered(ctx, deviceID, "", "all", 100)
}
func (r *Repository) GetDeviceLogsFiltered(ctx context.Context, deviceID int64, level, source string, limit int) ([]DeviceLog, error) {
	level = strings.ToUpper(strings.TrimSpace(level))
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "all"
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var where []string
	args := []any{deviceID}
	where = append(where, "device_id = $1")
	argN := 2
	if level != "" && level != "ALL" {
		where = append(where, fmt.Sprintf("level = $%d", argN))
		args = append(args, level)
		argN++
	}
	switch source {
	case "device":
		where = append(where, "(message LIKE '[SYSLOG] %' OR message LIKE '[TRAP] %')")
	case "system":
		where = append(where, "(message NOT LIKE '[SYSLOG] %' AND message NOT LIKE '[TRAP] %')")
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT id, device_id, level, message, created_at
		FROM device_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d;
	`, strings.Join(where, " AND "), argN)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("get device logs: %w", err)
	}
	defer rows.Close()

	out := make([]DeviceLog, 0, limit)
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
func (r *Repository) AddDeviceLog(ctx context.Context, deviceID int64, level, message string) error {
	const ins = `
		INSERT INTO device_logs (device_id, level, message)
		VALUES ($1, $2, $3);
	`
	if _, err := r.db.ExecContext(ctx, ins, deviceID, level, message); err != nil {
		return fmt.Errorf("add device log: %w", err)
	}
	const trim = `
		DELETE FROM device_logs
		WHERE device_id = $1
		AND id NOT IN (
			SELECT id
			FROM device_logs
			WHERE device_id = $1
			ORDER BY created_at DESC
			LIMIT 100
		);
	`
	if _, err := r.db.ExecContext(ctx, trim, deviceID); err != nil {
		return fmt.Errorf("trim device log: %w", err)
	}
	return nil
}
