# NetPulse

NetPulse 是面向华为/H3C 设备的网络监控系统，包含：
- Go 后端（SNMP 采集、REST API、自动建表、备份恢复）
- Vue 3 Web 管理端（中文界面、管理员/用户管理、审计日志）
- iOS / Android 移动端（普通用户登录、生物识别快捷登录）

## 1. 运行前准备

- 数据库：`timescale/timescaledb:latest-pg15`
- 应用镜像：本仓库构建得到 `netpulse:latest`
- 建议端口：
  - NetPulse Web/API：`8080`（示例映射到宿主 `18080`）
  - Syslog UDP：`514`

## 2. 环境变量（最新）

部署 NetPulse 容器时请至少填写：

- `DB_HOST`：TimescaleDB 主机/IP（如 `timescaledb` 或 `127.0.0.1`）
- `DB_PORT`：数据库端口（默认 `5432`）
- `DB_USER`：数据库用户名（建议 `postgres` 或独立账号）
- `DB_PASSWORD`：数据库密码
- `DB_NAME`：数据库名（如 `netpulse`）
- `HTTP_ADDR`：监听地址（默认 `:8080`）
- `JWT_SECRET`：JWT 密钥（请改为强随机字符串）
- `ADMIN_USERNAME`：管理员用户名（只允许 Web 登录）
- `ADMIN_PASSWORD`：管理员密码（首次启动自动写入/更新）

可选：
- `TZ`：时区（如 `Asia/Shanghai`）

## 3. 1Panel 图形化部署（不使用 compose）

### 3.1 部署 TimescaleDB

1. 进入 1Panel -> 容器 -> 新建容器。
2. 镜像填写：`timescale/timescaledb:latest-pg15`。
3. 启动用户不要填 `root`，保持镜像默认用户。
4. 环境变量：
   - `POSTGRES_USER=postgres`
   - `POSTGRES_PASSWORD=<你的强密码>`
   - `POSTGRES_DB=netpulse`
5. 端口映射：
   - 若仅供内网容器访问，可不映射宿主端口；
   - 若需要外部工具连接，映射 `5432:5432`（注意冲突）。
6. 持久化：
   - 挂载数据目录到 `/var/lib/postgresql/data`。
7. 启动并确认日志无报错。

> 如果出现 `"root" execution of the PostgreSQL server is not permitted`，说明容器以 root 启动，请改回镜像默认用户并重建容器。

### 3.2 部署 NetPulse

1. 进入 1Panel -> 容器 -> 新建容器。
2. 镜像填写你导入/拉取的 `netpulse:latest`。
3. 端口映射：
   - `18080:8080`（Web/API）
   - `514:514/udp`（可选，Syslog）
4. 环境变量按“第2节”填写。
5. 启动后访问：`http://<服务器IP>:18080`。

## 4. 首次启动逻辑

- 应用会自动执行 `EnsureSchema()`：
  - 自动创建业务表（devices/interfaces/metrics/device_logs/users/audit_logs）
  - 自动创建 Timescale 扩展与视图（幂等）
- 应用会自动 Upsert 管理员账号（来自 `ADMIN_USERNAME/ADMIN_PASSWORD`）。

## 5. 权限与登录规则

- 管理员：仅允许 Web 端登录。
- 普通用户：可 Web 或移动端登录。
- 移动端：首次必须账号密码登录；后续可 Face ID/Touch ID（iOS）或指纹（Android）快捷登录。
- 审计日志：记录登录与关键管理操作。

## 6. 构建产物

本仓库 `package/` 目录默认保存：
- `netpulse_latest.tar`（Docker 镜像）
- `NetPulse_NetPulseMobile_debug.apk`
- `NetPulse_NetPulseMobile_unsigned.ipa`

## 7. 使用建议

- 生产环境务必修改默认管理员密码和 `JWT_SECRET`。
- 建议通过反向代理绑定域名，并在公网场景启用 HTTPS。
- TimescaleDB 数据建议单独卷并定期做备份。
- 移动端连接地址建议固定为网关或域名，避免设备 IP 变更。
