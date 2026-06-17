package db

import (
	"context"
	"fmt"
)

func (r *Repository) AddAuditLog(ctx context.Context, log AuditLog) error {
	const q = `
		INSERT INTO audit_logs (user_id, action, target, method, path, ip, status_code, duration_ms, client, ts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW());
	`
	if _, err := r.db.ExecContext(
		ctx, q, log.UserID, log.Action, log.Target, log.Method, log.Path, log.IP, log.StatusCode, log.DurationMS, log.Client,
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
		SELECT a.id, a.user_id, COALESCE(u.username,''), a.action, COALESCE(a.target,''),
		       COALESCE(a.method,''), COALESCE(a.path,''), COALESCE(a.ip,''), COALESCE(a.status_code,0),
		       COALESCE(a.duration_ms,0), COALESCE(a.client,''), a.ts
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.ts DESC
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
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.Target, &a.Method, &a.Path, &a.IP, &a.StatusCode, &a.DurationMS, &a.Client, &a.Timestamp); err != nil {
			return nil, fmt.Errorf("scan audit log failed: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
