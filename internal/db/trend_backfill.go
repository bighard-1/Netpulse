package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const trafficTrendBackfillChunk = 7 * 24 * time.Hour

type TrafficTrendBackfillStatus struct {
	State            string     `json:"state"`
	CompletedThrough *time.Time `json:"completed_through,omitempty"`
	Target           *time.Time `json:"target,omitempty"`
	ProgressPercent  int        `json:"progress_percent"`
	LastError        string     `json:"last_error,omitempty"`
}

type TrafficTrendBackfillOverview struct {
	Pending           int        `json:"pending"`
	Failed            int        `json:"failed"`
	Completed         int        `json:"completed"`
	OldestRequestedAt *time.Time `json:"oldest_requested_at,omitempty"`
}

func (r *Repository) GetTrafficTrendBackfillOverview(ctx context.Context) (TrafficTrendBackfillOverview, error) {
	var out TrafficTrendBackfillOverview
	var oldest sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE next_bucket < target_bucket AND last_error = ''),
		COUNT(*) FILTER (WHERE last_error <> ''),
		COUNT(*) FILTER (WHERE next_bucket >= target_bucket AND last_error = ''),
		MIN(requested_at) FILTER (WHERE next_bucket < target_bucket)
		FROM traffic_trend_backfill_state`).Scan(&out.Pending, &out.Failed, &out.Completed, &oldest)
	if oldest.Valid {
		out.OldestRequestedAt = &oldest.Time
	}
	return out, err
}

func (r *Repository) GetTrafficTrendBackfillStatus(ctx context.Context, interfaceID int64, start, end time.Time) (TrafficTrendBackfillStatus, error) {
	var next, target time.Time
	var lastErr string
	err := r.db.QueryRowContext(ctx, `SELECT next_bucket, target_bucket, last_error FROM traffic_trend_backfill_state WHERE interface_id = $1`, interfaceID).Scan(&next, &target, &lastErr)
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficTrendBackfillStatus{State: "not_started"}, nil
	}
	if err != nil {
		return TrafficTrendBackfillStatus{}, err
	}
	status := TrafficTrendBackfillStatus{CompletedThrough: &next, Target: &target, LastError: lastErr}
	requestedEnd := end.UTC().Truncate(time.Hour)
	if requestedEnd.After(target) {
		requestedEnd = target
	}
	if strings.TrimSpace(lastErr) != "" {
		status.State = "failed"
		return status, nil
	}
	if !next.Before(requestedEnd) {
		status.State = "ready"
		status.ProgressPercent = 100
		return status, nil
	}
	status.State = "backfilling"
	span := requestedEnd.Sub(start.UTC().Truncate(time.Hour))
	if span > 0 {
		status.ProgressPercent = int(next.Sub(start.UTC().Truncate(time.Hour)) * 100 / span)
	}
	if status.ProgressPercent < 0 {
		status.ProgressPercent = 0
	}
	if status.ProgressPercent > 99 {
		status.ProgressPercent = 99
	}
	return status, nil
}

func (r *Repository) RetryTrafficTrendBackfill(ctx context.Context, interfaceID int64) error {
	if err := r.EnsureTrafficTrendBackfill(ctx, interfaceID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE traffic_trend_backfill_state SET last_error = '', updated_at = NOW() WHERE interface_id = $1`, interfaceID)
	return err
}

// EnsureTrafficTrendBackfill queues only the requested port. This makes old
// installations converge on the data users actually open, without a global
// table scan at startup.
func (r *Repository) EnsureTrafficTrendBackfill(ctx context.Context, interfaceID int64) error {
	qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(qctx, `
		INSERT INTO traffic_trend_backfill_state(interface_id, next_bucket, target_bucket, priority, requested_at, updated_at)
		SELECT $1,
		       GREATEST(date_trunc('hour', MIN(ts)), date_trunc('hour', NOW() - INTERVAL '730 days')),
		       date_trunc('hour', NOW()), 100, NOW(), NOW()
		FROM metrics
		WHERE interface_id = $1
		HAVING MIN(ts) IS NOT NULL
		ON CONFLICT (interface_id) DO UPDATE SET
		  target_bucket = GREATEST(traffic_trend_backfill_state.target_bucket, EXCLUDED.target_bucket), priority = 100, requested_at = NOW(),
		  updated_at = NOW();
	`, interfaceID)
	return err
}

func (r *Repository) StartTrafficTrendBackfillWorker(ctx context.Context) {
	workers := trafficTrendBackfillWorkers()
	for i := 0; i < workers; i++ {
		go r.runTrafficTrendBackfillWorker(ctx)
	}
}
func (r *Repository) runTrafficTrendBackfillWorker(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < trafficTrendBackfillChunksPerTick() && ctx.Err() == nil; i++ {
			worked, err := r.backfillOneTrafficTrendChunk(ctx)
			if err != nil {
				log.Printf("traffic trend backfill paused: %v", err)
				break
			}
			if !worked {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Repository) backfillOneTrafficTrendChunk(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var interfaceID int64
	var next, target time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT interface_id, next_bucket, target_bucket
		FROM traffic_trend_backfill_state
		WHERE next_bucket < target_bucket
		ORDER BY priority DESC, requested_at, next_bucket
		LIMIT 1 FOR UPDATE SKIP LOCKED;
	`).Scan(&interfaceID, &next, &target)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	end := next.Add(trafficTrendBackfillChunk)
	if end.After(target) {
		end = target
	}
	qctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	_, err = tx.ExecContext(qctx, `
		INSERT INTO traffic_trends_1h (
		  bucket, interface_id, device_id, sample_count, in_sample_count, out_sample_count,
		  traffic_in_sum, traffic_out_sum, min_traffic_in_bps, max_traffic_in_bps,
		  min_traffic_out_bps, max_traffic_out_bps, port_down_samples, updated_at
		)
		SELECT time_bucket('1 hour', ts), interface_id, MAX(device_id), COUNT(*)::INTEGER,
		  COUNT(traffic_in_bps)::INTEGER, COUNT(traffic_out_bps)::INTEGER,
		  COALESCE(SUM(traffic_in_bps), 0)::NUMERIC(28,2), COALESCE(SUM(traffic_out_bps), 0)::NUMERIC(28,2),
		  MIN(traffic_in_bps), MAX(traffic_in_bps), MIN(traffic_out_bps), MAX(traffic_out_bps),
		  COUNT(*) FILTER (WHERE traffic_in_status = 'PORT_DOWN' OR traffic_out_status = 'PORT_DOWN')::INTEGER, NOW()
		FROM metrics WHERE interface_id = $1 AND ts >= $2 AND ts < $3
		GROUP BY 1, interface_id
		ON CONFLICT (interface_id, bucket) DO UPDATE SET
		  device_id = EXCLUDED.device_id, sample_count = EXCLUDED.sample_count,
		  in_sample_count = EXCLUDED.in_sample_count, out_sample_count = EXCLUDED.out_sample_count,
		  traffic_in_sum = EXCLUDED.traffic_in_sum, traffic_out_sum = EXCLUDED.traffic_out_sum,
		  min_traffic_in_bps = EXCLUDED.min_traffic_in_bps, max_traffic_in_bps = EXCLUDED.max_traffic_in_bps,
		  min_traffic_out_bps = EXCLUDED.min_traffic_out_bps, max_traffic_out_bps = EXCLUDED.max_traffic_out_bps,
		  port_down_samples = EXCLUDED.port_down_samples, updated_at = NOW();
	`, interfaceID, next, end)
	if err != nil {
		_ = tx.Rollback()
		_, _ = r.db.ExecContext(ctx, `UPDATE traffic_trend_backfill_state SET last_error = $2, updated_at = NOW() WHERE interface_id = $1`, interfaceID, err.Error())
		return false, fmt.Errorf("backfill port %d %s..%s: %w", interfaceID, next.Format(time.RFC3339), end.Format(time.RFC3339), err)
	}
	// Preserve the completed cursor. The next chart request only extends its
	// target bucket, so a port is never needlessly rebuilt from day one.
	_, err = tx.ExecContext(ctx, `UPDATE traffic_trend_backfill_state SET next_bucket = $2, last_error = '', priority = 0, attempts = attempts + 1, updated_at = NOW() WHERE interface_id = $1`, interfaceID, end)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func trafficTrendBackfillWorkers() int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("NETPULSE_TRAFFIC_TREND_BACKFILL_WORKERS")))
	if v < 1 {
		return 1
	}
	if v > 4 {
		return 4
	}
	return v
}

func trafficTrendBackfillChunksPerTick() int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("NETPULSE_TRAFFIC_TREND_BACKFILL_CHUNKS_PER_TICK")))
	if v < 1 {
		return 3
	}
	if v > 8 {
		return 8
	}
	return v
}
