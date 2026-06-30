package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *Repository) GetStorageOverview(ctx context.Context) ([]StorageOverviewItem, error) {
	const q = `
		WITH wanted(name) AS (
			VALUES
				('devices'), ('interfaces'), ('metrics'), ('traffic_5m'), ('traffic_1h'),
				('device_logs'), ('alert_events'), ('audit_logs'), ('system_health'),
				('device_capability_history'), ('config_snapshots')
		)
		SELECT w.name,
		       COALESCE(pg_total_relation_size(format('public.%I', w.name)::regclass), 0) AS total_bytes,
		       pg_size_pretty(COALESCE(pg_total_relation_size(format('public.%I', w.name)::regclass), 0)) AS total_size,
		       GREATEST(COALESCE(s.n_live_tup, c.reltuples, 0), 0)::BIGINT AS estimated_rows
		FROM wanted w
		LEFT JOIN pg_class c ON c.oid = format('public.%I', w.name)::regclass
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		ORDER BY total_bytes DESC, w.name;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("get storage overview: %w", err)
	}
	defer rows.Close()

	out := make([]StorageOverviewItem, 0, 12)
	for rows.Next() {
		var item StorageOverviewItem
		if err := rows.Scan(&item.TableName, &item.TotalBytes, &item.TotalSize, &item.EstimatedRows); err != nil {
			return nil, fmt.Errorf("scan storage overview: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate storage overview: %w", err)
	}
	return out, nil
}

func (r *Repository) CleanupOperationalData(ctx context.Context, retention OperationalDataRetention, dryRun bool) (OperationalCleanupResult, error) {
	retention = normalizeOperationalRetention(retention)
	items := []struct {
		target string
		days   int
		where  string
	}{
		{target: "audit_logs", days: retention.AuditLogDays, where: "ts < $1"},
		{target: "device_logs", days: retention.DeviceLogDays, where: "created_at < $1"},
		{target: "alert_events", days: retention.ResolvedAlertDays, where: "created_at < $1 AND status <> 'open'"},
		{target: "system_health", days: retention.SystemHealthDays, where: "ts < $1"},
		{target: "backup_drill_reports", days: retention.BackupDrillDays, where: "created_at < $1"},
		{target: "device_capability_history", days: retention.CapabilityHistoryDays, where: "created_at < $1"},
	}

	result := OperationalCleanupResult{DryRun: dryRun, Items: make([]OperationalCleanupItem, 0, len(items))}
	for _, item := range items {
		cutoff := time.Now().AddDate(0, 0, -item.days)
		count, err := r.countCleanupRows(ctx, item.target, item.where, cutoff)
		if err != nil {
			return result, err
		}
		cleanupItem := OperationalCleanupItem{Target: item.target, RetentionDays: item.days, MatchedRows: count}
		if !dryRun && count > 0 {
			deleted, err := r.deleteCleanupRows(ctx, item.target, item.where, cutoff)
			if err != nil {
				return result, err
			}
			cleanupItem.DeletedRows = deleted
		}
		result.Items = append(result.Items, cleanupItem)
	}
	return result, nil
}

func normalizeOperationalRetention(in OperationalDataRetention) OperationalDataRetention {
	clamp := func(v, fallback, min, max int) int {
		if v <= 0 {
			v = fallback
		}
		if v < min {
			return min
		}
		if v > max {
			return max
		}
		return v
	}
	return OperationalDataRetention{
		AuditLogDays:          clamp(in.AuditLogDays, 180, 30, 3650),
		DeviceLogDays:         clamp(in.DeviceLogDays, 90, 30, 3650),
		ResolvedAlertDays:     clamp(in.ResolvedAlertDays, 180, 30, 3650),
		SystemHealthDays:      clamp(in.SystemHealthDays, 730, 90, 3650),
		BackupDrillDays:       clamp(in.BackupDrillDays, 365, 90, 3650),
		CapabilityHistoryDays: clamp(in.CapabilityHistoryDays, 180, 30, 3650),
	}
}

func (r *Repository) countCleanupRows(ctx context.Context, table, where string, cutoff time.Time) (int64, error) {
	var count sql.NullInt64
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s;", table, where)
	if err := r.db.QueryRowContext(ctx, q, cutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("count cleanup rows for %s: %w", table, err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

func (r *Repository) deleteCleanupRows(ctx context.Context, table, where string, cutoff time.Time) (int64, error) {
	q := fmt.Sprintf("DELETE FROM %s WHERE %s;", table, where)
	res, err := r.db.ExecContext(ctx, q, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete cleanup rows for %s: %w", table, err)
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}
