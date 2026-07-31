# NetPulse 容量与稳态加固说明

本说明用于评估“百台以内、数千端口、分层轮询”场景下的运行体验。它不改变任何业务配置，只提供排障入口和安全基线。

## 低风险加固点

- 端口流量历史查询加入短 TTL 内存缓存，降低多人重复打开同一图表时的数据库压力。
- 拓扑图读取加入短 TTL 内存缓存，编辑拓扑后自动失效，避免仪表盘反复拉取完整拓扑。
- 运行观测页展示图表缓存、拓扑缓存和最近慢请求，方便定位“页面加载慢”到底来自接口、数据库还是前端。
- `/api/metrics/history` 响应会返回周期、数据源、缓存命中、点数和查询耗时，方便定位 7 日/30 日图表慢在缓存、预聚合还是数据库查询。
- 资产/数据库诊断增加 TimescaleDB 连续聚合检查，便于判断长周期图表断点或加载慢是否与聚合视图有关。

## 建议规模

- 100 台以内、几千端口、核心/汇聚/接入分层轮询：可作为当前推荐规模。
- 多人同时打开大量长周期图表时，应关注 `metrics/history` 的 P95 延迟。
- 如果未来超过 300 台或上万端口，建议再引入独立 Poller 队列、缓存层和更细的时序聚合策略。

## 只读容量探针

运行方式：

```bash
BASE_URL=http://127.0.0.1:8080/api USERNAME=admin PASSWORD=admin123 ./scripts/capacity_probe.sh
```

可选参数：

```bash
CONCURRENCY=8 ROUNDS=5 DEVICE_ID=1 PORT_ID=10 ./scripts/capacity_probe.sh
```

判定建议：

- `dashboard/topology/events` P95 小于 `800ms`：通常较顺畅。
- `metrics/history` P95 小于 `1500ms`：单端口图表通常可接受。
- P95 超过 `3000ms` 或出现 `000/5xx`：优先查看“设置 -> 运行观测 -> 最近服务端慢请求”，并执行“告警与日志 -> 资产/数据库诊断”。

## 排障顺序

1. 先看“运行观测”的最近慢请求，确认慢的是哪个接口。
2. 如果慢接口是资产或图表查询，执行“资产/数据库诊断”。
3. 如果最近指标入库延迟正常但图表慢，优先看慢请求里的 `周期/数据源/缓存/点数`，再检查 TimescaleDB 索引、连续聚合和查询跨度。
4. 如果慢接口集中在拓扑或事件流，优先检查是否有大量重复刷新或异常客户端持续轮询。
# Long-range traffic query safeguards

For a single interface, charts up to 31 days read `metrics` directly through
the `(interface_id, ts)` index and aggregate only that interface's points. This
does not wait for a global rollup job.

The traffic rollup worker normally refreshes only the recent window (6 hours
for 5-minute rows and 24 hours for hourly rows). To deliberately rebuild old
derived data, set `NETPULSE_TRAFFIC_ROLLUP_BACKFILL=true` temporarily and
monitor database load; do not enable it during normal peak operations.
