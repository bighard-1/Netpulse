package db

import (
	"context"
)

func (r *Repository) SaveConfigSnapshot(ctx context.Context, deviceID int64, hash, content, diff string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO config_snapshots(device_id,content_hash,content,diff) VALUES($1,$2,$3,$4);`, deviceID, hash, content, diff)
	return err
}
func (r *Repository) SaveBackupDrillReport(ctx context.Context, status, message, detail string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO backup_drill_reports(status,message,detail) VALUES($1,$2,$3::jsonb);`, status, message, detail)
	return err
}
func (r *Repository) ListBackupDrillReports(ctx context.Context, limit int) ([]BackupDrillReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,status,message,detail::text,created_at FROM backup_drill_reports ORDER BY id DESC LIMIT $1;`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupDrillReport{}
	for rows.Next() {
		var b BackupDrillReport
		if err := rows.Scan(&b.ID, &b.Status, &b.Message, &b.Detail, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
