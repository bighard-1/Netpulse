# 长周期流量趋势压测

在预发布数据库执行，不要对生产库制造模拟原始指标。压测目标是确认两年查询只读取 `traffic_trends_1h`，并且端口与时间范围命中主键 `(interface_id, bucket)`。

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT time_bucket('1 day', bucket),
       SUM(traffic_in_sum) / NULLIF(SUM(in_sample_count), 0)
FROM traffic_trends_1h
WHERE interface_id = 123 AND bucket >= NOW() - INTERVAL '730 days' AND bucket <= NOW()
GROUP BY 1 ORDER BY 1;
```

验收条件：计划应使用 `traffic_trends_1h_pkey` 或等价的端口时间索引；单端口两年范围返回不应扫描 `metrics` 原始表。用 20、100、500 个并发端口分别执行上述查询，记录 p50/p95 延迟、数据库 CPU、活动连接数和回填队列积压。

回填节流参数：`NETPULSE_TRAFFIC_TREND_BACKFILL_WORKERS`（1–4，默认 1）与 `NETPULSE_TRAFFIC_TREND_BACKFILL_CHUNKS_PER_TICK`（1–8，默认 3）。每个分片最多覆盖 7 天；高负载时先将两者降为 1。
