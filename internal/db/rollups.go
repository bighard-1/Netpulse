package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func (r *Repository) StartTrafficRollupWorker(ctx context.Context) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("NETPULSE_ENABLE_TRAFFIC_ROLLUPS")), "false") {
		log.Printf("traffic rollup worker disabled by NETPULSE_ENABLE_TRAFFIC_ROLLUPS=false")
		return
	}
	go func() {
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			r.runTrafficRollupOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (r *Repository) runTrafficRollupOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	for i := 0; i < trafficRollupChunksPerRun("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", 4); i++ {
		if !r.aggregateTraffic5m(ctx) || ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil {
		return
	}
	for i := 0; i < trafficRollupChunksPerRun("NETPULSE_TRAFFIC_ROLLUP_1H_CHUNKS_PER_RUN", 4); i++ {
		if !r.aggregateTraffic1h(ctx) || ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil {
		return
	}
	r.cleanupTrafficRollups(ctx)
}

func trafficRollupChunksPerRun(envKey string, fallback int) int {
	if s := strings.TrimSpace(os.Getenv(envKey)); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			if v > 24 {
				return 24
			}
			return v
		}
	}
	return fallback
}

func trafficRollupBackfillEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("NETPULSE_TRAFFIC_ROLLUP_BACKFILL")), "true")
}

// Normal collection must never spend all of its database capacity rebuilding
// months of derived data. Historical rebuilds are an explicit maintenance
// action; the normal worker only keeps a short recent window current.
func capTrafficRollupStart(start, targetEnd time.Time, recentWindow time.Duration) time.Time {
	if trafficRollupBackfillEnabled() {
		return start
	}
	minimum := targetEnd.Add(-recentWindow)
	if start.Before(minimum) {
		return minimum
	}
	return start
}

func (r *Repository) aggregateTraffic5m(ctx context.Context) bool {
	targetEnd := time.Now().UTC().Truncate(5 * time.Minute).Add(-5 * time.Minute)
	if targetEnd.IsZero() {
		return false
	}
	start := r.nextTrafficRollupStart(ctx, "5m", targetEnd.Add(-trafficRollup5mRange))
	if start.Before(targetEnd.Add(-trafficRollup5mRange)) {
		start = targetEnd.Add(-trafficRollup5mRange)
	}
	start = capTrafficRollupStart(start, targetEnd, 6*time.Hour)
	if !start.Before(targetEnd) {
		r.recordTrafficRollupState(ctx, "5m", targetEnd, time.Now(), 0, "")
		return false
	}
	chunkEnd := start.Add(6 * time.Hour)
	if chunkEnd.After(targetEnd) {
		chunkEnd = targetEnd
	}
	started := time.Now()
	qctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(qctx, `
		INSERT INTO traffic_5m (
		    bucket, interface_id, device_id, samples,
		    avg_traffic_in_bps, avg_traffic_out_bps,
		    max_traffic_in_bps, max_traffic_out_bps, port_down_samples, updated_at
		)
		SELECT
		    time_bucket('5 minutes', ts) AS bucket,
		    interface_id,
		    device_id,
		    COUNT(*) FILTER (WHERE traffic_in_bps IS NOT NULL OR traffic_out_bps IS NOT NULL)::INTEGER AS samples,
		    AVG(traffic_in_bps)::NUMERIC(20,2) AS avg_traffic_in_bps,
		    AVG(traffic_out_bps)::NUMERIC(20,2) AS avg_traffic_out_bps,
		    MAX(traffic_in_bps)::BIGINT AS max_traffic_in_bps,
		    MAX(traffic_out_bps)::BIGINT AS max_traffic_out_bps,
		    COUNT(*) FILTER (WHERE traffic_in_status = 'PORT_DOWN' OR traffic_out_status = 'PORT_DOWN')::INTEGER AS port_down_samples,
		    NOW()
		FROM metrics
		WHERE ts >= $1 AND ts < $2
		  AND interface_id IS NOT NULL
		  AND (
		    traffic_in_bps IS NOT NULL OR traffic_out_bps IS NOT NULL OR
		    traffic_in_status = 'PORT_DOWN' OR traffic_out_status = 'PORT_DOWN'
		  )
		GROUP BY 1, interface_id, device_id
		ON CONFLICT (interface_id, bucket) DO UPDATE SET
		    device_id = EXCLUDED.device_id,
		    samples = EXCLUDED.samples,
		    avg_traffic_in_bps = EXCLUDED.avg_traffic_in_bps,
		    avg_traffic_out_bps = EXCLUDED.avg_traffic_out_bps,
		    max_traffic_in_bps = EXCLUDED.max_traffic_in_bps,
		    max_traffic_out_bps = EXCLUDED.max_traffic_out_bps,
		    port_down_samples = EXCLUDED.port_down_samples,
		    updated_at = NOW();
	`, start, chunkEnd)
	r.recordTrafficRollupState(ctx, "5m", chunkEnd, started, time.Since(started).Milliseconds(), errString(err))
	if err != nil && ctx.Err() == nil {
		log.Printf("traffic 5m rollup skipped %s..%s: %v", start.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
	}
	return err == nil && chunkEnd.Before(targetEnd)
}
func (r *Repository) aggregateTraffic1h(ctx context.Context) bool {
	targetEnd := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	start := r.nextTrafficRollupStart(ctx, "1h", targetEnd.Add(-trafficRollup1hRange))
	if start.Before(targetEnd.Add(-trafficRollup1hRange)) {
		start = targetEnd.Add(-trafficRollup1hRange)
	}
	start = capTrafficRollupStart(start, targetEnd, 24*time.Hour)
	if !start.Before(targetEnd) {
		r.recordTrafficRollupState(ctx, "1h", targetEnd, time.Now(), 0, "")
		return false
	}
	chunkEnd := start.Add(7 * 24 * time.Hour)
	if chunkEnd.After(targetEnd) {
		chunkEnd = targetEnd
	}
	started := time.Now()
	qctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(qctx, `
		INSERT INTO traffic_1h (
		    bucket, interface_id, device_id, samples,
		    avg_traffic_in_bps, avg_traffic_out_bps,
		    max_traffic_in_bps, max_traffic_out_bps, port_down_samples, updated_at
		)
		SELECT
		    time_bucket('1 hour', bucket) AS bucket,
		    interface_id,
		    device_id,
		    SUM(samples)::INTEGER AS samples,
		    CASE WHEN SUM(CASE WHEN avg_traffic_in_bps IS NOT NULL THEN samples ELSE 0 END) > 0 THEN
		      SUM(COALESCE(avg_traffic_in_bps, 0) * samples)::NUMERIC(20,2) /
		      SUM(CASE WHEN avg_traffic_in_bps IS NOT NULL THEN samples ELSE 0 END)
		    END AS avg_traffic_in_bps,
		    CASE WHEN SUM(CASE WHEN avg_traffic_out_bps IS NOT NULL THEN samples ELSE 0 END) > 0 THEN
		      SUM(COALESCE(avg_traffic_out_bps, 0) * samples)::NUMERIC(20,2) /
		      SUM(CASE WHEN avg_traffic_out_bps IS NOT NULL THEN samples ELSE 0 END)
		    END AS avg_traffic_out_bps,
		    MAX(avg_traffic_in_bps)::BIGINT AS max_traffic_in_bps,
		    MAX(avg_traffic_out_bps)::BIGINT AS max_traffic_out_bps,
		    SUM(port_down_samples)::INTEGER AS port_down_samples,
		    NOW()
		FROM traffic_5m
		WHERE bucket >= $1 AND bucket < $2
		  AND interface_id IS NOT NULL
		  AND (samples > 0 OR port_down_samples > 0)
		GROUP BY 1, interface_id, device_id
		ON CONFLICT (interface_id, bucket) DO UPDATE SET
		    device_id = EXCLUDED.device_id,
		    samples = EXCLUDED.samples,
		    avg_traffic_in_bps = EXCLUDED.avg_traffic_in_bps,
		    avg_traffic_out_bps = EXCLUDED.avg_traffic_out_bps,
		    max_traffic_in_bps = EXCLUDED.max_traffic_in_bps,
		    max_traffic_out_bps = EXCLUDED.max_traffic_out_bps,
		    port_down_samples = EXCLUDED.port_down_samples,
		    updated_at = NOW();
	`, start, chunkEnd)
	r.recordTrafficRollupState(ctx, "1h", chunkEnd, started, time.Since(started).Milliseconds(), errString(err))
	if err != nil && ctx.Err() == nil {
		log.Printf("traffic 1h rollup skipped %s..%s: %v", start.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
	}
	return err == nil && chunkEnd.Before(targetEnd)
}
func (r *Repository) cleanupTrafficRollups(ctx context.Context) {
	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, _ = r.db.ExecContext(qctx, `DELETE FROM traffic_5m WHERE bucket < NOW() - INTERVAL '90 days';`)
	_, _ = r.db.ExecContext(qctx, `DELETE FROM traffic_1h WHERE bucket < NOW() - INTERVAL '730 days';`)
}
func (r *Repository) nextTrafficRollupStart(ctx context.Context, grain string, fallback time.Time) time.Time {
	qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var last sql.NullTime
	err := r.db.QueryRowContext(qctx, `SELECT last_bucket FROM traffic_rollup_state WHERE grain = $1;`, grain).Scan(&last)
	if err == nil && last.Valid {
		return last.Time
	}
	return fallback
}
func (r *Repository) recordTrafficRollupState(ctx context.Context, grain string, lastBucket, started time.Time, durationMS int64, lastErr string) {
	qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = r.db.ExecContext(qctx, `
		INSERT INTO traffic_rollup_state(grain, last_bucket, last_run_at, last_duration_ms, last_error, updated_at)
		VALUES($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (grain) DO UPDATE SET
		    last_bucket = EXCLUDED.last_bucket,
		    last_run_at = EXCLUDED.last_run_at,
		    last_duration_ms = EXCLUDED.last_duration_ms,
		    last_error = EXCLUDED.last_error,
		    updated_at = NOW();
	`, grain, lastBucket, started, durationMS, lastErr)
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func (r *Repository) GetTrafficRollupStatuses(ctx context.Context) ([]TrafficRollupStatus, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT grain, last_bucket, last_run_at, last_duration_ms, last_error, updated_at
		FROM traffic_rollup_state
		ORDER BY grain;
	`)
	if err != nil {
		return nil, fmt.Errorf("query traffic rollup state: %w", err)
	}
	defer rows.Close()
	out := make([]TrafficRollupStatus, 0, 2)
	for rows.Next() {
		var s TrafficRollupStatus
		var lastBucket, lastRun sql.NullTime
		if err := rows.Scan(&s.Grain, &lastBucket, &lastRun, &s.LastDurationMS, &s.LastError, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan traffic rollup state: %w", err)
		}
		if lastBucket.Valid {
			t := lastBucket.Time
			s.LastBucket = &t
		}
		if lastRun.Valid {
			t := lastRun.Time
			s.LastRunAt = &t
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic rollup state: %w", err)
	}
	return out, nil
}
