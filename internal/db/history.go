package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

func (r *Repository) GetInterfaceHistory(
	ctx context.Context, interfaceID int64, start, end time.Time, interval string, maxPoints int,
) ([]InterfaceHistoryPoint, error) {
	span := end.Sub(start)
	if span > MaxTrafficHistoryRange {
		return nil, fmt.Errorf("traffic history range exceeds 730 days")
	}
	interval = strings.TrimSpace(strings.ToLower(interval))
	if span > 31*24*time.Hour {
		items, err := r.queryInterfaceTrafficRollup(ctx, "traffic_1h", interfaceID, start, end, interval, maxPoints)
		if (err == nil && len(items) > 0) || span > trafficRollup5mRange {
			return items, err
		}
		fallback, fallbackErr := r.queryInterfaceTraffic1m(ctx, interfaceID, start, end, interval, maxPoints)
		if fallbackErr == nil {
			return fallback, nil
		}
		if err != nil {
			return nil, err
		}
		log.Printf("metrics_1m traffic fallback skipped interface=%d: %v", interfaceID, fallbackErr)
		return items, nil
	}
	if span > 24*time.Hour {
		items, err := r.queryInterfaceTrafficRollup(ctx, "traffic_5m", interfaceID, start, end, interval, maxPoints)
		if err == nil && len(items) > 0 {
			return items, nil
		}
		fallback, fallbackErr := r.queryInterfaceTraffic1m(ctx, interfaceID, start, end, interval, maxPoints)
		if fallbackErr == nil {
			return fallback, nil
		}
		if err != nil {
			return nil, err
		}
		log.Printf("metrics_1m traffic fallback skipped interface=%d: %v", interfaceID, fallbackErr)
		return items, nil
	}

	bucketInterval := resolveHistoryBucketInterval(span, interval, maxPoints, false)
	q := `
		SELECT ts, traffic_in_bps, traffic_out_bps
		FROM metrics
		WHERE interface_id = $1
		  AND ts >= $2 AND ts <= $3
		  AND (traffic_in_bps IS NOT NULL OR traffic_out_bps IS NOT NULL)
		ORDER BY ts;
	`
	if bucketInterval != "" {
		q = fmt.Sprintf(`
			SELECT time_bucket('%[1]s', ts) AS ts,
			       AVG(traffic_in_bps) AS traffic_in_bps,
			       AVG(traffic_out_bps) AS traffic_out_bps
			FROM metrics
			WHERE interface_id = $1
			  AND ts >= $2 AND ts <= $3
			  AND (traffic_in_bps IS NOT NULL OR traffic_out_bps IS NOT NULL)
			GROUP BY 1
			ORDER BY 1;
		`, bucketInterval)
	}
	return r.scanInterfaceHistory(ctx, q, interfaceID, start, end, span, maxPoints)
}

func (r *Repository) queryInterfaceTraffic1m(
	ctx context.Context, interfaceID int64, start, end time.Time, interval string, maxPoints int,
) ([]InterfaceHistoryPoint, error) {
	span := end.Sub(start)
	bucketInterval := resolveHistoryBucketInterval(span, interval, maxPoints, true)
	if bucketInterval == "" {
		bucketInterval = "5 minutes"
	}
	if d, ok := parseSQLInterval(bucketInterval); !ok || d < time.Minute {
		bucketInterval = "1 minute"
	}
	q := fmt.Sprintf(`
		SELECT time_bucket('%[1]s', bucket) AS ts,
		       AVG(avg_traffic_in_bps) AS traffic_in_bps,
		       AVG(avg_traffic_out_bps) AS traffic_out_bps
		FROM metrics_1m
		WHERE interface_id = $1
		  AND bucket >= $2 AND bucket <= $3
		  AND (avg_traffic_in_bps IS NOT NULL OR avg_traffic_out_bps IS NOT NULL)
		GROUP BY 1
		ORDER BY 1;
	`, bucketInterval)
	out, err := r.scanInterfaceHistory(ctx, q, interfaceID, start, end, span, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("get interface history from metrics_1m fallback: %w", err)
	}
	return out, nil
}

func (r *Repository) queryInterfaceTrafficRollup(
	ctx context.Context, table string, interfaceID int64, start, end time.Time, interval string, maxPoints int,
) ([]InterfaceHistoryPoint, error) {
	span := end.Sub(start)
	minBucket := 5 * time.Minute
	sourceLabel := "5m"
	switch table {
	case "traffic_5m":
	case "traffic_1h":
		minBucket = time.Hour
		sourceLabel = "1h"
	default:
		return nil, fmt.Errorf("unsupported traffic rollup table")
	}
	bucketInterval := resolveHistoryBucketInterval(span, interval, maxPoints, true)
	if bucketInterval == "" {
		bucketInterval = durationToSQLInterval(minBucket)
	}
	if d, ok := parseSQLInterval(bucketInterval); !ok || d < minBucket {
		bucketInterval = durationToSQLInterval(minBucket)
	}
	q := fmt.Sprintf(`
		SELECT time_bucket('%[1]s', bucket) AS ts,
		       CASE WHEN SUM(CASE WHEN avg_traffic_in_bps IS NOT NULL THEN samples ELSE 0 END) > 0 THEN
		         SUM(COALESCE(avg_traffic_in_bps, 0) * samples)::DOUBLE PRECISION /
		         SUM(CASE WHEN avg_traffic_in_bps IS NOT NULL THEN samples ELSE 0 END)
		       END AS traffic_in_bps,
		       CASE WHEN SUM(CASE WHEN avg_traffic_out_bps IS NOT NULL THEN samples ELSE 0 END) > 0 THEN
		         SUM(COALESCE(avg_traffic_out_bps, 0) * samples)::DOUBLE PRECISION /
		         SUM(CASE WHEN avg_traffic_out_bps IS NOT NULL THEN samples ELSE 0 END)
		       END AS traffic_out_bps
		FROM %[2]s
		WHERE interface_id = $1
		  AND bucket >= $2 AND bucket <= $3
		  AND (avg_traffic_in_bps IS NOT NULL OR avg_traffic_out_bps IS NOT NULL)
		GROUP BY 1
		ORDER BY 1;
	`, bucketInterval, table)
	out, err := r.scanInterfaceHistory(ctx, q, interfaceID, start, end, span, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("get interface history from %s rollup: %w", sourceLabel, err)
	}
	return out, nil
}

func (r *Repository) scanInterfaceHistory(
	ctx context.Context, q string, interfaceID int64, start, end time.Time, span time.Duration, maxPoints int,
) ([]InterfaceHistoryPoint, error) {
	rows, err := r.db.QueryContext(ctx, q, interfaceID, start, end)
	if err != nil {
		return nil, fmt.Errorf("get interface history: %w", err)
	}
	defer rows.Close()

	out := make([]InterfaceHistoryPoint, 0)
	for rows.Next() {
		var p InterfaceHistoryPoint
		var in, outB sql.NullFloat64
		if err := rows.Scan(&p.Timestamp, &in, &outB); err != nil {
			return nil, fmt.Errorf("scan interface history: %w", err)
		}
		if in.Valid {
			v := in.Float64
			p.TrafficInBps = &v
		}
		if outB.Valid {
			v := outB.Float64
			p.TrafficOutBps = &v
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface history: %w", err)
	}
	return decimateInterfaceHistory(out, span, maxPoints), nil
}

func decimateDeviceHistory(in []DeviceHistoryPoint, span time.Duration, maxPointsOverride int) []DeviceHistoryPoint {
	if len(in) <= 0 {
		return in
	}
	maxPoints := 4000
	if maxPointsOverride > 0 {
		maxPoints = maxPointsOverride
	}
	if span > 180*24*time.Hour {
		maxPoints = 1800
	} else if span > 30*24*time.Hour {
		maxPoints = 2500
	}
	if maxPointsOverride > 0 {
		maxPoints = maxPointsOverride
	}
	if len(in) <= maxPoints {
		return in
	}
	step := int(math.Ceil(float64(len(in)) / float64(maxPoints)))
	if step < 1 {
		step = 1
	}
	out := make([]DeviceHistoryPoint, 0, maxPoints+2)
	for i := 0; i < len(in); i += step {
		out = append(out, in[i])
	}
	if out[len(out)-1].Timestamp != in[len(in)-1].Timestamp {
		out = append(out, in[len(in)-1])
	}
	return out
}

func decimateInterfaceHistory(in []InterfaceHistoryPoint, span time.Duration, maxPointsOverride int) []InterfaceHistoryPoint {
	if len(in) <= 0 {
		return in
	}
	maxPoints := 4000
	if maxPointsOverride > 0 {
		maxPoints = maxPointsOverride
	}
	if span > 180*24*time.Hour {
		maxPoints = 1800
	} else if span > 30*24*time.Hour {
		maxPoints = 2500
	}
	if maxPointsOverride > 0 {
		maxPoints = maxPointsOverride
	}
	if len(in) <= maxPoints {
		return in
	}
	step := int(math.Ceil(float64(len(in)) / float64(maxPoints)))
	if step < 1 {
		step = 1
	}
	out := make([]InterfaceHistoryPoint, 0, maxPoints+2)
	for i := 0; i < len(in); i += step {
		out = append(out, in[i])
	}
	if out[len(out)-1].Timestamp != in[len(in)-1].Timestamp {
		out = append(out, in[len(in)-1])
	}
	return out
}

func resolveHistoryBucketInterval(span time.Duration, requested string, maxPoints int, useAgg bool) string {
	minSec := 0
	switch {
	case span > 31*24*time.Hour:
		minSec = 3600
	case span > 7*24*time.Hour:
		minSec = 1800
	case span > 24*time.Hour:
		minSec = 300
	}
	if useAgg && minSec < 60 {
		minSec = 60
	}
	formatSec := func(sec int) string {
		if sec < minSec {
			sec = minSec
		}
		if useAgg && sec < 60 {
			sec = 60
		}
		switch {
		case sec >= 3600:
			h := int(math.Ceil(float64(sec) / 3600.0))
			return fmt.Sprintf("%d hours", h)
		case sec >= 60:
			m := int(math.Ceil(float64(sec) / 60.0))
			return fmt.Sprintf("%d minutes", m)
		default:
			return fmt.Sprintf("%d seconds", sec)
		}
	}
	if strings.HasSuffix(requested, "s") {
		if v, err := strconv.Atoi(strings.TrimSuffix(requested, "s")); err == nil && v > 0 {
			return formatSec(v)
		}
	}
	if strings.HasSuffix(requested, "m") {
		if v, err := strconv.Atoi(strings.TrimSuffix(requested, "m")); err == nil && v > 0 {
			return formatSec(v * 60)
		}
	}
	if strings.HasSuffix(requested, "h") {
		if v, err := strconv.Atoi(strings.TrimSuffix(requested, "h")); err == nil && v > 0 {
			return formatSec(v * 3600)
		}
	}
	switch requested {
	case "1m":
		return formatSec(60)
	case "2m":
		return formatSec(120)
	case "5m":
		return formatSec(300)
	case "10m":
		return formatSec(600)
	case "30m":
		return formatSec(1800)
	case "1h":
		return formatSec(3600)
	}
	if maxPoints <= 0 || span <= 0 {
		if minSec > 0 {
			return formatSec(minSec)
		}
		return ""
	}
	sec := int(math.Ceil(span.Seconds() / float64(maxPoints)))
	if sec < 1 {
		sec = 1
	}
	return formatSec(sec)
}

func durationToSQLInterval(d time.Duration) string {
	if d >= time.Hour {
		h := int(math.Ceil(d.Hours()))
		return fmt.Sprintf("%d hours", h)
	}
	if d >= time.Minute {
		m := int(math.Ceil(d.Minutes()))
		return fmt.Sprintf("%d minutes", m)
	}
	sec := int(math.Ceil(d.Seconds()))
	if sec < 1 {
		sec = 1
	}
	return fmt.Sprintf("%d seconds", sec)
}

func parseSQLInterval(v string) (time.Duration, bool) {
	fields := strings.Fields(strings.TrimSpace(strings.ToLower(v)))
	if len(fields) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 {
		return 0, false
	}
	switch strings.TrimSuffix(fields[1], "s") {
	case "hour":
		return time.Duration(n) * time.Hour, true
	case "minute":
		return time.Duration(n) * time.Minute, true
	case "second":
		return time.Duration(n) * time.Second, true
	default:
		return 0, false
	}
}
