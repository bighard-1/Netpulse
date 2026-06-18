# NetPulse 项目维护总览

本文档是 NetPulse 的维护入口。目标是让后续开发、修复、构建镜像前都能快速判断：应该改哪里、不应该碰哪里、改完要测什么。

## 1. 核心模块职责

| 模块 | 负责什么 | 不应该负责什么 |
| --- | --- | --- |
| `internal/api/` | HTTP 路由、鉴权、参数校验、调用 DB/SNMP 服务 | 不直接写复杂 SQL，不做 SNMP 差分计算 |
| `internal/db/` | TimescaleDB/PostgreSQL 表结构、查询、聚合、备份相关数据访问 | 不处理前端展示细节，不直接发 SNMP |
| `internal/snmp/` | SNMP 采集、端口状态、流量计数器状态机、告警事件生成 | 不处理 Web 页面布局，不直接拼 SQL 页面响应 |
| `internal/snmp/trafficcalc/` | 端口流量 Counter64/Counter32 差分、异常值、状态码等纯计算 | 不访问数据库，不发网络请求 |
| `web/src/views/` | 页面组装、用户交互、调用 service/composable/util | 不堆积复杂计算，不重复解析 API 错误结构 |
| `web/src/utils/` | 可测试的纯前端工具函数，如图表刻度、Top N、错误解析 | 不访问浏览器页面状态，不发真实 HTTP |
| `web/src/services/api.js` | Axios 实例、API 封装、兼容旧错误入口 | 不写页面提示逻辑，不处理业务 UI |
| `mobile/` | iOS/Android 客户端 | 不影响 Web 端构建和后端采集逻辑 |

## 2. 稳定功能保护线

以下功能已经稳定，后续修改前必须先确认真实需求，禁止顺手重构：

- SNMP 流量差分与高速端口状态机。
- `GET /api/metrics/history` 长周期图表查询。
- `traffic_5m`、`traffic_1h` rollup 机制。
- Web 端端口详情页当日、7 日、30 日、自定义周期图表。
- 拓扑图展示与管理权限。
- 后台备份/恢复任务模式。
- iOS 图表读数、缩放、保存相册等已稳定体验。

## 3. 每次改动前先选改动类型

### 只改前端展示

建议检查：

```bash
npm --prefix web test
npm --prefix web run build
```

禁止顺手修改：

- `internal/snmp/`
- `internal/db/history.go`
- 数据库迁移 SQL

### 只改后端查询或采集

建议检查：

```bash
go test ./...
go vet ./...
npm --prefix web test
```

如果涉及流量图，还必须人工验证：

- 当日流量
- 近 7 日流量
- 近 30 日流量
- 自定义周期

### 涉及数据库 schema 或迁移

必须额外确认：

- 启动日志没有 `ensure schema failed`
- 大索引、回填、连续聚合刷新不能阻塞启动
- 生产环境升级前先备份数据库

## 4. 发布前最小检查命令

建议每次提交或构建镜像前执行：

```bash
npm --prefix web test
npm --prefix web run build
go test ./...
go vet ./...
git diff --check
```

如果涉及 Shell 脚本：

```bash
bash -n scripts/*.sh
```

## 5. 重点回归路径

发布前至少抽测以下路径：

1. 登录 Web。
2. 仪表盘加载成功。
3. 全局搜索设备和端口。
4. 拓扑图显示在线/离线节点。
5. 进入资产中心。
6. 进入任意设备详情页。
7. 打开端口详情页。
8. 查看当日、7 日、30 日流量图。
9. 查看实时事件流。
10. 打开设置页运行观测。

## 6. 常见风险与排查入口

| 现象 | 优先查看 | 常见原因 |
| --- | --- | --- |
| 首页资产加载超时 | 告警与日志 -> 资产/数据库诊断 | 端口规模大、索引缺失、DB 压力高 |
| 30 日流量图慢 | 运行观测、rollup 状态、`metrics/history` 慢请求中的周期/数据源/缓存/点数 | rollup 未完成、查询落回原始表、缓存未命中 |
| 图表断线 | 端口指标状态码、采集日志 | SNMP 超时、计数器重置、设备重启、缺数据 |
| 拓扑图加载慢 | 运行观测中的拓扑缓存命中 | 拓扑缓存失效或节点/边过多 |
| 备份演练失败 | 系统设置 -> 后台任务状态 | pg_dump 被杀、数据库体积过大、权限异常 |

## 7. 当前测试护栏

后端：

- `internal/snmp/trafficcalc/calc_test.go`：SNMP 流量核心计算。
- `internal/snmp/worker_test.go`：采集状态机关键场景。
- `internal/db/history_test.go`：长周期历史查询、分桶、降采样。
- `internal/db/schema_test.go`：启动迁移超时配置。

前端：

- `web/scripts/smoke-utils.mjs`：API 错误解析、仪表盘搜索/Top N、端口图表工具。

## 8. 推荐后续优化顺序

1. 继续把页面里的复杂计算抽到 `web/src/utils/`。
2. 给后端高风险纯函数补测试，避免依赖真实数据库才能发现问题。
3. 对容量探针结果建立简单基线，例如 7 日图表、30 日图表 P95。
4. 继续按页面把复杂计算抽到可测试工具函数，减少视图文件膨胀。
5. 移动端保持“只读”边界，避免与 Web 管理能力混杂。
