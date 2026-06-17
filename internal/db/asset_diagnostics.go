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
			"idx_interface_latest_metrics_device",
			"idx_interface_latest_metrics_ts",
			"idx_device_latest_metrics_ts",
			"idx_traffic_5m_device_bucket",
			"idx_traffic_1h_device_bucket",
		}
		rows, err := r.db.QueryContext(ctx, `
			SELECT indexname
			FROM pg_indexes
			WHERE schemaname = 'public'
				  AND indexname IN (
				    'idx_metrics_interface_ts',
				    'idx_metrics_device_ts',
				    'idx_device_logs_device_created_at',
				    'idx_interfaces_device_index',
				    'idx_interface_latest_metrics_device',
				    'idx_interface_latest_metrics_ts',
				    'idx_device_latest_metrics_ts',
				    'idx_traffic_5m_device_bucket',
				    'idx_traffic_1h_device_bucket'
				  );
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

	run("流量预聚合状态", 800, 3000, func(ctx context.Context) (string, string, error) {
		var c5m, c1h int
		err := r.db.QueryRowContext(ctx, `
				SELECT
				  (SELECT COUNT(*) FROM traffic_5m WHERE bucket >= NOW() - INTERVAL '7 days'),
				  (SELECT COUNT(*) FROM traffic_1h WHERE bucket >= NOW() - INTERVAL '30 days');
			`).Scan(&c5m, &c1h)
		if err != nil {
			return "", "请确认新版本已完成启动迁移，并观察 traffic_rollup_state 是否有错误。", fmt.Errorf("检查流量预聚合失败: %w", err)
		}
		if c5m == 0 {
			return fmt.Sprintf("近7天5分钟聚合点 %d 条，近30天1小时聚合点 %d 条", c5m, c1h), "流量预聚合正在回填或未运行，请等待后台任务完成；长周期图表会优先依赖预聚合数据。", nil
		}
		return fmt.Sprintf("近7天5分钟聚合点 %d 条，近30天1小时聚合点 %d 条", c5m, c1h), "", nil
	})

	run("Timescale连续聚合检查", 800, 3000, func(ctx context.Context) (string, string, error) {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `
			SELECT to_regclass('public.metrics_1m') IS NOT NULL;
		`).Scan(&exists); err != nil {
			return "", "请确认数据库用户具备读取系统目录的权限。", fmt.Errorf("检查 metrics_1m 失败: %w", err)
		}
		if !exists {
			return "", "请重启 NetPulse 触发 schema migration，或检查启动日志中的 TimescaleDB 连续聚合创建错误。", fmt.Errorf("metrics_1m 连续聚合不存在")
		}
		var recentBuckets int
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM metrics_1m
			WHERE bucket >= NOW() - INTERVAL '2 hours';
		`).Scan(&recentBuckets); err != nil {
			return "", "metrics_1m 存在但不可查询，请检查 TimescaleDB 连续聚合刷新策略。", fmt.Errorf("查询 metrics_1m 失败: %w", err)
		}
		suggestion := ""
		if report.RecentMetricCount > 0 && recentBuckets == 0 {
			suggestion = "近2小时连续聚合暂无数据，长周期图表可能退化或变慢；请检查 TimescaleDB policy 是否正常刷新。"
		}
		meta := "连续聚合元数据可读"
		var isContinuous bool
		if err := r.db.QueryRowContext(ctx, `
			SELECT EXISTS (
			  SELECT 1
			  FROM timescaledb_information.continuous_aggregates
			  WHERE view_schema = 'public'
			    AND view_name = 'metrics_1m'
			);
		`).Scan(&isContinuous); err != nil {
			meta = "连续聚合元数据不可读，已按 metrics_1m 可查询状态判断"
		} else if !isContinuous {
			meta = "metrics_1m 可查询，但未在 continuous_aggregates 元数据中识别"
			if suggestion == "" {
				suggestion = "若长周期图表持续变慢，请检查 metrics_1m 是否为 Timescale 连续聚合。"
			}
		}
		return fmt.Sprintf("metrics_1m 已存在且可查询，近2小时聚合桶 %d 个，%s", recentBuckets, meta), suggestion, nil
	})

	run("最新端口指标查询", 1500, 8000, func(ctx context.Context) (string, string, error) {
		var total, recent int
		err := r.db.QueryRowContext(ctx, `
			SELECT
			  COUNT(*),
			  COUNT(*) FILTER (WHERE ts >= NOW() - INTERVAL '10 minutes')
			FROM interface_latest_metrics;
		`).Scan(&total, &recent)
		if err != nil {
			return "", "请确认自动迁移已创建 interface_latest_metrics，或重启 NetPulse 触发 schema migration。", fmt.Errorf("查询最新端口缓存失败: %w", err)
		}
		if report.InterfaceCount > 0 && total == 0 {
			return "", "最新指标缓存需要至少等待一个轮询周期写入；若长时间为空，请检查 SNMP worker 是否正常。", fmt.Errorf("最新端口指标缓存为空")
		}
		return fmt.Sprintf("最新指标缓存 %d 个端口，其中近10分钟 %d 个端口", total, recent), "若缓存数量明显少于端口数量，请等待对应设备层级的轮询周期，或检查轮询失败日志。", nil
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
