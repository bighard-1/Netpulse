# NetPulse

NetPulse 是面向华为/H3C/通用 SNMP 设备的网络监控系统，包含：
- Go 后端（API + 采集引擎 + 自动建表）
- TimescaleDB 时序数据库
- Vue3 Web 管理端
- iOS/Android 客户端

## 核心功能

### 监控与采集
- SNMP 全版本支持：`v1 / v2c / v3`
- CPU/内存/端口流量采集（轮询间隔可配置）
- 端口信息增量同步（保留备注）
- 设备状态诊断：在线/离线/未知 + 状态原因
- 连通性双判定：SNMP 失败时自动做 `ICMP + TCP161` 探测

### 资产与运维
- 资产新增、删除、备注管理
- 端口备注管理
- 设备日志（最近 100 条）
- 批量导入设备（CSV）
- 设备模板管理
- 自动发现（CIDR 网段扫描）
- 拓扑链路管理（Topology Links）
- 配置快照追踪（Config Snapshot）

### 告警与报表
- 告警规则中心（Alert Rules）
- 告警事件记录（Alert Events）
- 阈值告警（CPU/内存）+ Webhook 推送
- 报表导出（CSV）

### 系统能力
- 备份下载与恢复
- 备份校验与回滚演练报告（支持定期自动演练）
- 审计日志增强（方法、路径、IP、状态码、耗时、客户端）
- RBAC 权限控制（角色-权限）

### 协议接入
- Syslog 接收（UDP 514）
- SNMP Trap 接收（默认 UDP 9162）

## 目录结构

```text
cmd/netpulse            # 主程序入口
internal/api            # API 与认证
internal/db             # Repository 与自动建表 SQL
internal/snmp           # SNMP 采集/轮询/Trap/Syslog
web                     # Vue3 Web 前端
mobile/ios              # iOS 客户端
mobile/android          # Android 客户端
deploy                  # Docker/1Panel 配置
package                 # 构建产物目录
scripts                 # 冒烟/辅助脚本
```

## 快速部署（推荐 Docker / 1Panel）

### 1. 准备 TimescaleDB
- 镜像：`timescale/timescaledb:latest-pg15`
- 数据目录持久化：`/var/lib/postgresql/data`
- 运行用户不要设置为 `root`
- 账号建议：
  - `DB_USER=postgres`
  - `DB_PASSWORD=强密码`
  - `DB_NAME=netpulse`

### 2. 启动 NetPulse 容器
- 镜像：`ghcr.io/bighard-1/netpulse:latest`
- 端口：
  - `8080/tcp` Web/API
  - `514/udp` Syslog
  - `9162/udp` Trap（如果你启用 Trap）

### 3. 必填环境变量
- `DB_HOST`：TimescaleDB 地址
- `DB_PORT`：默认 `5432`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`
- `DB_SSLMODE`：默认 `disable`
- `JWT_SECRET`：JWT 密钥（务必修改）
- `ADMIN_USERNAME`：Web 管理员用户名
- `ADMIN_PASSWORD`：Web 管理员密码

### 4. 强烈建议环境变量
- `NETPULSE_CRED_KEY`：**32 字节**密钥，用于加密 SNMP 凭据（community/v3 密码）
- `SNMP_POLL_INTERVAL_SEC`：轮询间隔默认值（仅首次写入，后续可在 Web 设置里修改）
- `SNMP_DEVICE_TIMEOUT_SEC`：采集超时默认值（仅首次写入，后续可在 Web 设置里修改）
- `STATUS_ONLINE_WINDOW_SEC`：在线判定窗口默认值（仅首次写入，后续可在 Web 设置里修改）
- `ALERT_WEBHOOK_URL`：告警 webhook 默认值（后续可在 Web 设置里修改）
- `ALERT_CPU_THRESHOLD`：CPU 告警阈值默认值（后续可在 Web 设置里修改）
- `ALERT_MEM_THRESHOLD`：内存告警阈值默认值（后续可在 Web 设置里修改）
- `BACKUP_DRILL_EVERY_HOURS`：备份演练周期小时（默认 `168`，即每周）
- `SYSLOG_ADDR`：默认 `:514`
- `SNMP_TRAP_ADDR`：默认 `:9162`

## 1Panel 图形化部署步骤（详细）

1. 在 1Panel 部署 TimescaleDB（应用商店或容器方式均可）。  
2. 配置 TimescaleDB 环境变量：`POSTGRES_DB/POSTGRES_USER/POSTGRES_PASSWORD`。  
3. 绑定 TimescaleDB 数据卷到 `/var/lib/postgresql/data`。  
4. 部署 NetPulse 容器，镜像填：`ghcr.io/bighard-1/netpulse:latest`。  
5. 配置 NetPulse 环境变量（见上文必填 + 建议）。  
6. 配置端口映射：`18080:8080`（示例）、`514:514/udp`、`9162:9162/udp`。  
7. 启动后访问：`http://你的服务器IP:映射端口`。  
8. 首次登录用 `ADMIN_USERNAME/ADMIN_PASSWORD`。  
9. 添加设备时选择 SNMP 版本并填写对应参数。  

## 数据库兼容与防坑说明（重点）

- 项目采用 `repo.EnsureSchema()` 自动建表/自动迁移。  
- 已做 `IF NOT EXISTS` 兼容，避免重复创建报错。  
- 针对历史版本已兼容常见字段缺失问题（如审计表列变更）。  
- 建议升级方式：
  1. 先备份数据库  
  2. 再替换新镜像  
  3. 查看容器日志确认 `ensure schema` 成功  

若出现历史表结构异常，优先检查：
- `users` 表是否有 `password_hash/role`
- `audit_logs` 是否有 `ts/user_id/action/target`
- `devices` 是否有 `snmp_version/snmp_port/v3_*`

## Web 端功能

- 资产列表、设备详情、端口详情
- 系统设置中心（运行参数在线修改：轮询/超时/在线窗口/告警阈值/Webhook）
- 告警规则管理（API 已就绪）
- 拓扑链路管理（API 已就绪）
- 备份下载、恢复、备份演练
- 审计日志、用户管理

## 移动端功能

### iOS
- 用户登录 + 生物识别
- 资产总览、设备详情、端口流量图
- 端口备注编辑、日志查看
- 长按复制 IP

### Android
- 用户登录 + 生物识别
- 资产总览、设备详情、端口流量图（支持缩放/拖动）
- 端口备注编辑、日志查看
- 长按复制 IP

## 本地构建

### 后端与镜像
```bash
./build_release.sh
```

### Android
```bash
cd mobile/android
./gradlew --no-daemon :app:assembleRelease
./gradlew --no-daemon :app:assembleDebug
```

### iOS（无签名 IPA）
```bash
xcodebuild -project mobile/ios/NetPulseMobile.xcodeproj \
  -scheme NetPulseMobile -configuration Release -sdk iphoneos \
  -derivedDataPath mobile/ios/build \
  CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO build
```

## 冒烟测试

```bash
chmod +x scripts/smoke_e2e.sh
BASE_URL=http://127.0.0.1:8080/api ADMIN_USER=admin ADMIN_PASS=admin123 ./scripts/smoke_e2e.sh
```

脚本会校验：
- 登录
- 添加设备
- 查询设备详情
- 更新端口备注（有端口时）
- 查询设备日志

## 当前版本产物约定

`package/` 目录会放置：
- `NetPulse_v0.5.0_linux_amd64.tar`
- `NetPulse_v0.5.0_android_universal.apk`
- `NetPulse_v0.5.0_android_amd64.apk`
- `NetPulse_v0.5.0_ios_unsigned.ipa`
