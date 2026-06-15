package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type AssetLoadDiagnosticCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion,omitempty"`
}

type AssetLoadDiagnosticReport struct {
	GeneratedAt       time.Time                  `json:"generated_at"`
	OverallStatus     string                     `json:"overall_status"`
	DeviceCount       int                        `json:"device_count"`
	InterfaceCount    int                        `json:"interface_count"`
	RecentMetricCount int                        `json:"recent_metric_count"`
	Checks            []AssetLoadDiagnosticCheck `json:"checks"`
	Suggestions       []string                   `json:"suggestions"`
}

func (r *Repository) DiagnoseAssetLoad(ctx context.Context) (*AssetLoadDiagnosticReport, error) {
	report := &AssetLoadDiagnosticReport{
		GeneratedAt:   time.Now(),
		OverallStatus: "ok",
	}
	suggestions := map[string]struct{}{}
	addSuggestion := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		suggestions[s] = struct{}{}
	}
	addCheck := func(check AssetLoadDiagnosticCheck) {
		if check.Status == "" {
			check.Status = "ok"
		}
		if check.Status == "critical" {
			report.OverallStatus = "critical"
		} else if check.Status == "warning" && report.OverallStatus == "ok" {
			report.OverallStatus = "warning"
		}
		if check.Status != "ok" {
			addSuggestion(check.Suggestion)
		}
		report.Checks = append(report.Checks, check)
	}
	run := func(name string, warnMS, criticalMS int64, fn func(context.Context) (string, string, error)) {
		checkCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
		defer cancel()
		start := time.Now()
		detail, suggestion, err := fn(checkCtx)
		cost := time.Since(start).Milliseconds()
		status := "ok"
		if err != nil {
			status = "critical"
			detail = err.Error()
		} else if criticalMS > 0 && cost >= criticalMS {
			status = "critical"
		} else if warnMS > 0 && cost >= warnMS {
			status = "warning"
		}
		addCheck(AssetLoadDiagnosticCheck{
			Name:       name,
			Status:     status,
			DurationMS: cost,
			Detail:     detail,
			Suggestion: suggestion,
		})
	}

	run("资产规模统计", 800, 3000, func(ctx context.Context) (string, string, error) {
		var devices, interfaces, recentMetrics int
		err := r.db.QueryRowContext(ctx, `
			SELECT
			  (SELECT COUNT(*) FROM devices),
			  (SELECT COUNT(*) FROM interfaces),
			  (SELECT COUNT(*) FROM metrics WHERE ts >= NOW() - INTERVAL '1 hour');
		`).Scan(&devices, &interfaces, &recentMetrics)
		if err != nil {
			return "", "请检查数据库连接和 TimescaleDB 状态。", fmt.Errorf("统计资产规模失败: %w", err)
		}
		report.DeviceCount = devices
		report.InterfaceCount = interfaces
		report.RecentMetricCount = recentMetrics
		suggestion := ""
		if interfaces >= 3000 {
			suggestion = "端口数量较多，建议保持分层轮询并避免多人同时打开大量长周期图表。"
		}
		return fmt.Sprintf("设备 %d 台，端口 %d 个，近1小时指标样本 %d 条", devices, interfaces, recentMetrics), suggestion, nil
	})

	run("关键索引检查", 500, 2000, func(ctx context.Context) (string, string, error) {
		required := []string{
			"idx_metrics_interface_ts",
			"idx_metrics_device_ts",
			"idx_device_logs_device_created_at",
			"idx_interfaces_device_index",
		}
		rows, err := r.db.QueryContext(ctx, `
			SELECT indexname
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND indexname IN ('idx_metrics_interface_ts','idx_metrics_device_ts','idx_device_logs_device_created_at','idx_interfaces_device_index');
		`)
		if err != nil {
			return "", "请确认数据库用户具备读取 pg_indexes 的权限。", fmt.Errorf("检查索引失败: %w", err)
		}
		defer rows.Close()
		exists := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return "", "请检查数据库元数据读取是否正常。", fmt.Errorf("读取索引信息失败: %w", err)
			}
			exists[name] = true
		}
		if err := rows.Err(); err != nil {
			return "", "请检查数据库元数据读取是否正常。", fmt.Errorf("遍历索引信息失败: %w", err)
		}
		missing := make([]string, 0)
		for _, name := range required {
			if !exists[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return "", "请重启 NetPulse 触发自动迁移，或检查启动日志中的 schema migration 错误。", fmt.Errorf("缺失索引: %s", strings.Join(missing, ", "))
		}
		return "关键索引已存在", "", nil
	})

	run("最新端口指标查询", 1500, 8000, func(ctx context.Context) (string, string, error) {
		var count int
		err := r.db.QueryRowContext(ctx, `
			WITH latest_metrics AS (
				SELECT DISTINCT ON (interface_id)
				       interface_id, traffic_in_bps, traffic_out_bps
				FROM metrics
				WHERE interface_id IS NOT NULL
				ORDER BY interface_id, ts DESC
			)
			SELECT COUNT(*) FROM latest_metrics;
		`).Scan(&count)
		if err != nil {
			return "", "这通常会直接拖慢首页资产加载，请检查 metrics 相关索引和 TimescaleDB 状态。", fmt.Errorf("查询最新端口指标失败: %w", err)
		}
		return fmt.Sprintf("可读取 %d 个端口的最新指标", count), "若此项耗时较高，优先检查 idx_metrics_interface_ts 是否存在，并观察数据库 IO/CPU。", nil
	})

	run("完整资产接口模拟", 3000, 12000, func(ctx context.Context) (string, string, error) {
		items, err := r.ListDevicesWithStatus(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "", "完整资产接口已接近或超过前端 20 秒超时阈值，请优先处理索引、端口规模和数据库负载。", fmt.Errorf("完整资产接口模拟超时: %w", ctx.Err())
			}
			return "", "请检查 /api/devices 后端日志中的具体 SQL 或扫描错误。", fmt.Errorf("完整资产接口模拟失败: %w", err)
		}
		ports := 0
		for _, d := range items {
			ports += len(d.Interfaces)
		}
		return fmt.Sprintf("返回 %d 台资产、%d 个端口", len(items), ports), "若此项耗时超过 3 秒，建议观察数据库 CPU/IO，并减少首页自动刷新频率或继续拆分轻量资产摘要接口。", nil
	})

	run("最新指标入库延迟", 500, 2000, func(ctx context.Context) (string, string, error) {
		var last sql.NullTime
		if err := r.db.QueryRowContext(ctx, `SELECT MAX(ts) FROM metrics;`).Scan(&last); err != nil {
			return "", "请确认 metrics 表可读。", fmt.Errorf("查询最新入库时间失败: %w", err)
		}
		if !last.Valid {
			return "暂无指标入库", "若已添加设备，请检查 SNMP 轮询 worker 是否正常启动。", nil
		}
		delay := time.Since(last.Time)
		suggestion := ""
		if delay > 10*time.Minute {
			suggestion = "指标入库延迟较高，请检查轮询失败、SNMP 超时或数据库写入异常。"
		}
		return fmt.Sprintf("最新指标时间 %s，延迟约 %.0f 秒", last.Time.Format(time.RFC3339), delay.Seconds()), suggestion, nil
	})

	for s := range suggestions {
		report.Suggestions = append(report.Suggestions, s)
	}
	if len(report.Suggestions) == 0 {
		report.Suggestions = append(report.Suggestions, "未发现明显异常；若仍偶发超时，请在超时后立即导出本报告，并关注当时数据库和容器资源占用。")
	}
	return report, nil
}
