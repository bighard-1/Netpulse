package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (r *Repository) EnsureRuntimeSettings(ctx context.Context, defaults map[string]string) error {
	if len(defaults) == 0 {
		return nil
	}
	const q = `
		INSERT INTO system_settings(key, value, updated_at)
		VALUES($1, $2, NOW())
		ON CONFLICT (key) DO NOTHING;
	`
	for k, v := range defaults {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, q, k, strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("ensure runtime setting %s: %w", k, err)
		}
	}
	return nil
}
func (r *Repository) GetSystemSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM system_settings ORDER BY key;`)
	if err != nil {
		return nil, fmt.Errorf("query system settings: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan system settings: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system settings: %w", err)
	}
	return out, nil
}
func (r *Repository) UpsertSystemSettings(ctx context.Context, kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	const q = `
		INSERT INTO system_settings(key, value, updated_at)
		VALUES($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
	`
	for k, v := range kv {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, q, k, strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("upsert system setting %s: %w", k, err)
		}
	}
	return nil
}
func parseIntSetting(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func clampIntSetting(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func parseFloatSetting(raw string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
func (r *Repository) GetRuntimeSettings(ctx context.Context) (RuntimeSettings, error) {
	kv, err := r.GetSystemSettings(ctx)
	if err != nil {
		return RuntimeSettings{}, err
	}
	out := RuntimeSettings{
		SNMPPollIntervalSec:   parseIntSetting(kv["snmp_poll_interval_sec"], 60),
		PollIntervalCoreSec:   parseIntSetting(kv["poll_interval_core_sec"], 60),
		PollIntervalAggSec:    parseIntSetting(kv["poll_interval_agg_sec"], 90),
		PollIntervalAccessSec: parseIntSetting(kv["poll_interval_access_sec"], 120),
		SNMPDeviceTimeoutSec:  parseIntSetting(kv["snmp_device_timeout_sec"], 15),
		StatusOnlineWindowSec: parseIntSetting(kv["status_online_window_sec"], 300),
		WebIdleTimeoutMin:     parseIntSetting(kv["web_idle_timeout_min"], 180),
		AlertCPUThreshold:     parseFloatSetting(kv["alert_cpu_threshold"], 90),
		AlertMemThreshold:     parseFloatSetting(kv["alert_mem_threshold"], 90),
		AlertWebhookURL:       kv["alert_webhook_url"],
		SNMPCalibrationMap:    strings.TrimSpace(kv["snmp_calibration_map"]),
	}
	out.SNMPPollIntervalSec = clampIntSetting(out.SNMPPollIntervalSec, 5, 3600)
	if out.PollIntervalCoreSec < 5 {
		out.PollIntervalCoreSec = out.SNMPPollIntervalSec
	}
	out.PollIntervalCoreSec = clampIntSetting(out.PollIntervalCoreSec, 5, 3600)
	if out.PollIntervalAggSec < 5 {
		out.PollIntervalAggSec = out.SNMPPollIntervalSec
	}
	out.PollIntervalAggSec = clampIntSetting(out.PollIntervalAggSec, 5, 3600)
	if out.PollIntervalAccessSec < 5 {
		out.PollIntervalAccessSec = out.SNMPPollIntervalSec
	}
	out.PollIntervalAccessSec = clampIntSetting(out.PollIntervalAccessSec, 5, 3600)
	out.SNMPDeviceTimeoutSec = clampIntSetting(out.SNMPDeviceTimeoutSec, 2, 120)
	out.StatusOnlineWindowSec = clampIntSetting(out.StatusOnlineWindowSec, 30, 3600)
	out.WebIdleTimeoutMin = clampIntSetting(out.WebIdleTimeoutMin, 5, 1440)
	if out.AlertCPUThreshold > 100 {
		out.AlertCPUThreshold = 100
	}
	if out.AlertMemThreshold > 100 {
		out.AlertMemThreshold = 100
	}
	return out, nil
}
