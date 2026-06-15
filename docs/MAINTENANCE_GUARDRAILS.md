# NetPulse 维护护栏

本文档记录当前稳定功能的维护边界。后续修复或优化时，应优先遵守这些护栏，避免“修一个点、伤一片”。

## 权限边界

- 移动端定位为只读工作台，后端会阻止移动端执行写权限接口。
- Web 端普通用户默认只允许查看、搜索、查询图表和查看事件。
- Web 端管理员仍需要进入编辑模式后再执行增删改操作。
- 拓扑编辑、备份恢复、用户管理、模板、告警规则、运行参数等功能仅管理员可用。

## 高风险接口

以下接口改动前必须做后端测试和前端构建，并确认不会影响现有稳定功能：

- `/api/devices`
- `/api/devices/{id}`
- `/api/interfaces/{id}`
- `/api/metrics/history`
- `/api/topology`
- `/api/events/recent`
- `/api/system/backup/jobs`
- `/api/system/restore/jobs`
- `/api/system/ops`
- `/api/diagnostics/asset-load`

## 图表与采集护栏

- 不要在前端刷新时直接触发 SNMP 查询。
- 不要把缺失数据补 0；缺失点应让图表断开。
- 不要轻易修改 Counter64/Counter32 选择、delta 计算、采样窗口状态机。
- 修改长周期图表查询前，必须确认当日、近7日、近30日、自定义周期都仍能显示。
- S12700E 等框式设备的图表稳定性依赖后端采样状态机，不建议在前端做“强行平滑”替代真实计算。

## 加载体验护栏

- 首页、拓扑、图表、事件流应遵循“先保留旧数据/骨架，再异步刷新”的策略。
- 短暂 API 超时不应清空已有事件流或拓扑图。
- 可重复访问的重型只读接口可使用短 TTL 缓存，但编辑后必须主动失效。

## 发布前检查

```bash
go test ./...
npm --prefix web run build
bash -n scripts/capacity_probe.sh
git diff --check
```

如需评估容量体验，可在测试环境运行：

```bash
BASE_URL=http://127.0.0.1:8080/api USERNAME=admin PASSWORD=admin123 ./scripts/capacity_probe.sh
```

