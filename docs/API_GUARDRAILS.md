# NetPulse API 契约护栏

本文档用于维护 NetPulse Web/API 的权限边界、错误处理和兼容入口。它不是完整 OpenAPI 文档，而是发布前必须关注的接口契约摘要。

## 通用约定

- 所有 `/api/*` 保护接口均要求 JWT。
- 移动端 Token 仅用于只读查询；写权限会被后端拦截。
- 写入类接口应具备审计日志，除非明确是临时兼容入口。
- 错误响应应优先保持统一结构：

```json
{
  "code": "ERR_INTERNAL",
  "error": "short error",
  "message": "human readable message",
  "hint": "operator hint"
}
```

## 只读高频接口

这些接口直接影响仪表盘、拓扑、图表和移动端体验，改动前必须做构建和回归：

| 接口 | 说明 | 权限 |
| --- | --- | --- |
| `GET /api/devices` | 资产列表、状态、端口摘要 | `device.read` |
| `GET /api/devices/{id}` | 设备详情与端口列表 | 登录用户 |
| `GET /api/interfaces/{id}` | 端口详情上下文 | `device.read` |
| `GET /api/metrics/history` | CPU/内存/端口流量历史 | `metrics.read` |
| `GET /api/topology` | 拓扑图读取 | `device.read` |
| `GET /api/events/recent` | 实时事件流 | 登录用户 |
| `GET /api/search` | 全局搜索 | `device.read` |

## 管理员接口

这些接口应保持 `adminOnly`：

| 接口范围 | 说明 |
| --- | --- |
| `/api/topology/nodes*` / `/api/topology/edges*` | 拓扑编辑 |
| `/api/system/ops` | 运行观测 |
| `/api/system/backup/jobs*` | 后台备份任务 |
| `/api/system/restore/jobs` | 后台恢复任务 |
| `/api/system/jobs*` | 后台任务状态 |
| `/api/audit*` | 审计日志 |
| `/api/users*` / `/api/admin/users*` | 用户管理 |
| `/api/templates*` | 监控模板 |
| `/api/alerts/rules*` | 告警规则 |
| `/api/settings/runtime` | 运行参数 |
| `/api/diagnostics/asset-load` | 资产/数据库诊断 |

## 兼容入口

历史同步入口仍保留，便于旧前端或外部脚本兼容：

- `GET /api/system/backup`
- `POST /api/system/restore`
- `POST /api/system/backup/drill`

推荐新功能优先调用后台任务接口：

- `POST /api/system/backup/jobs`
- `GET /api/system/jobs/{id}`
- `GET /api/system/backup/jobs/{id}/download`
- `POST /api/system/restore/jobs`
- `POST /api/system/backup/drill/jobs`

## 发布前 API 审计

运行静态护栏审计：

```bash
./scripts/api_guardrail_audit.sh
```

该脚本会标记：

- 写接口缺少审计中间件。
- 管理敏感接口缺少 `adminOnly`。
- 允许所有登录用户读取的兼容/公开只读接口。

`WARN` 不一定代表缺陷，但必须在发布前人工确认。

