package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (r *Repository) SaveSystemHealthSnapshot(ctx context.Context, ts time.Time, score float64, activeAlerts int, availability float64) error {
	const q = `
		INSERT INTO system_health(ts, score, active_alerts, availability)
		VALUES($1, $2, $3, $4);
	`
	_, err := r.db.ExecContext(ctx, q, ts, score, activeAlerts, availability)
	if err != nil {
		return fmt.Errorf("save system health snapshot: %w", err)
	}
	return nil
}
func (r *Repository) GetSystemHealthTrend(ctx context.Context, limit int) ([]SystemHealthPoint, error) {
	if limit <= 0 || limit > 2000 {
		limit = 30
	}
	const q = `
		SELECT ts, score, active_alerts, availability
		FROM system_health
		ORDER BY ts DESC
		LIMIT $1;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("get system health trend: %w", err)
	}
	defer rows.Close()

	tmp := make([]SystemHealthPoint, 0, limit)
	for rows.Next() {
		var p SystemHealthPoint
		if err := rows.Scan(&p.Timestamp, &p.Score, &p.ActiveAlerts, &p.Availability); err != nil {
			return nil, fmt.Errorf("scan system health trend: %w", err)
		}
		tmp = append(tmp, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system health trend: %w", err)
	}
	// Return ascending order for charting.
	for i, j := 0, len(tmp)-1; i < j; i, j = i+1, j-1 {
		tmp[i], tmp[j] = tmp[j], tmp[i]
	}
	return tmp, nil
}
func (r *Repository) GetMetricsIngestStatus(ctx context.Context) (time.Time, int64, error) {
	var last sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(ts) FROM metrics;`).Scan(&last); err != nil {
		return time.Time{}, 0, fmt.Errorf("get metrics ingest status: %w", err)
	}
	if !last.Valid {
		return time.Time{}, 0, nil
	}
	delay := time.Since(last.Time).Seconds()
	if delay < 0 {
		delay = 0
	}
	return last.Time, int64(delay), nil
}
func (r *Repository) GetOpsPollSummary(ctx context.Context, window time.Duration) (OpsPollSummary, error) {
	if window <= 0 {
		window = time.Hour
	}
	since := time.Now().Add(-window)
	out := OpsPollSummary{WindowMinutes: int(window.Minutes())}
	const q = `
		SELECT COUNT(*) AS poll_errors,
		       COUNT(DISTINCT device_id) AS failed_devices,
		       COUNT(*) FILTER (
		         WHERE message LIKE '[TIMEOUT]%'
		            OR message LIKE '[CONNECT_FAILED]%'
		            OR message LIKE '[HOST_UNREACHABLE]%'
		            OR message LIKE '[TCP161_BLOCKED]%'
		       ) AS timeout_count,
		       MAX(created_at) AS last_error_at
		FROM device_logs
		WHERE created_at >= $1
		  AND (
		    UPPER(level) IN ('ERROR','WARNING','CRITICAL')
		    OR message LIKE '[DB_WRITE_FAILED]%'
		    OR message LIKE '[SYNC_FAILED]%'
		    OR message LIKE '[POLL_FAILED]%'
		    OR message LIKE '[TIMEOUT]%'
		    OR message LIKE '[CONNECT_FAILED]%'
		    OR message LIKE '[HOST_UNREACHABLE]%'
		    OR message LIKE '[TCP161_BLOCKED]%'
		  );
	`
	var last sql.NullTime
	if err := r.db.QueryRowContext(ctx, q, since).Scan(&out.PollErrorCount, &out.FailedDevices, &out.TimeoutCount, &last); err != nil {
		return out, fmt.Errorf("get ops poll summary: %w", err)
	}
	if last.Valid {
		out.LastPollErrorAt = last.Time
	}
	return out, nil
}
func (r *Repository) GetTrafficSampleSummary(ctx context.Context, window time.Duration) (TrafficSampleSummary, error) {
	if window <= 0 {
		window = time.Hour
	}
	since := time.Now().Add(-window)
	out := TrafficSampleSummary{WindowMinutes: int(window.Minutes())}
	const q = `
		WITH samples AS (
		  SELECT unnest(ARRAY[
		           NULLIF(traffic_in_status, ''),
		           NULLIF(traffic_out_status, '')
		         ]) AS status
		  FROM metrics
		  WHERE ts >= $1
		    AND (traffic_in_status IS NOT NULL OR traffic_out_status IS NOT NULL)
		)
		SELECT COUNT(*) AS total_samples,
		       COUNT(*) FILTER (WHERE status = 'VALID' OR status LIKE 'DIR_%' OR status = 'CACHE_AVG') AS valid_samples,
		       COUNT(*) FILTER (WHERE status IN ('WINDOW_GAP','CACHE_WAIT','TIME_ERROR')) AS gap_samples,
		       COUNT(*) FILTER (WHERE status IN ('COUNTER_RESET','DEVICE_REBOOT','COUNTER_SOURCE_SWITCH') OR status LIKE '%ANOMALY%') AS anomaly_samples,
		       COUNT(*) FILTER (WHERE status = 'INITIALIZING') AS initializing_samples
		FROM samples
		WHERE status IS NOT NULL;
	`
	if err := r.db.QueryRowContext(ctx, q, since).Scan(&out.TotalSamples, &out.ValidSamples, &out.GapSamples, &out.AnomalySamples, &out.InitializingSamples); err != nil {
		return out, fmt.Errorf("get traffic sample summary: %w", err)
	}
	return out, nil
}
