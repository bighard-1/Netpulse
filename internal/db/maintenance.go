package db

import (
	"context"
	"log"
	"os"
	"strings"
	"time"
)

func (r *Repository) StartBackgroundMaintenance(ctx context.Context) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("NETPULSE_ENABLE_OPTIONAL_INDEX_MAINTENANCE"))) != "true" {
		log.Printf("optional index maintenance disabled; set NETPULSE_ENABLE_OPTIONAL_INDEX_MAINTENANCE=true to run it during a maintenance window")
		return
	}
	go r.ensureOptionalIndexes(ctx)
}
func (r *Repository) ensureOptionalIndexes(ctx context.Context) {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	indexes := []struct {
		name        string
		concurrent  string
		nonBlocking string
	}{
		{
			name:        "idx_metrics_1m_interface_bucket",
			concurrent:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_metrics_1m_interface_bucket ON metrics_1m (interface_id, bucket DESC);`,
			nonBlocking: `CREATE INDEX IF NOT EXISTS idx_metrics_1m_interface_bucket ON metrics_1m (interface_id, bucket DESC);`,
		},
		{
			name:        "idx_metrics_1m_device_bucket",
			concurrent:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_metrics_1m_device_bucket ON metrics_1m (device_id, bucket DESC);`,
			nonBlocking: `CREATE INDEX IF NOT EXISTS idx_metrics_1m_device_bucket ON metrics_1m (device_id, bucket DESC);`,
		},
	}

	for attempt := 1; attempt <= 3; attempt++ {
		failed := 0
		for _, idx := range indexes {
			if ctx.Err() != nil {
				return
			}
			if r.indexExists(ctx, idx.name) {
				continue
			}
			if err := r.createOptionalIndex(ctx, idx.concurrent, idx.nonBlocking); err != nil {
				failed++
				log.Printf("optional index %s attempt %d skipped: %v", idx.name, attempt, err)
				continue
			}
			log.Printf("optional index %s ready", idx.name)
		}
		if failed == 0 {
			return
		}
		if attempt == 3 {
			return
		}
		retry := time.NewTimer(5 * time.Minute)
		select {
		case <-ctx.Done():
			retry.Stop()
			return
		case <-retry.C:
		}
	}
}
func (r *Repository) createOptionalIndex(ctx context.Context, concurrentQuery, fallbackQuery string) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SET lock_timeout = '2s'`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `SET statement_timeout = '10min'`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, concurrentQuery); err != nil {
		// TimescaleDB hypertables and continuous aggregates do not support
		// CREATE INDEX CONCURRENTLY. Fall back only inside this background task;
		// startup migrations remain fast and safe.
		if strings.Contains(err.Error(), "hypertables do not support concurrent index creation") ||
			strings.Contains(err.Error(), "SQLSTATE 0A000") {
			_, fallbackErr := conn.ExecContext(ctx, fallbackQuery)
			if fallbackErr != nil {
				return fallbackErr
			}
			return nil
		}
		return err
	}
	return nil
}
func (r *Repository) indexExists(ctx context.Context, name string) bool {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND indexname = $1
		);
	`, name).Scan(&exists)
	return err == nil && exists
}
