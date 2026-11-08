# Docker 部署

Hephaestus 的 Docker 镜像默认内置 PostgreSQL 17。`docker run` 或 `docker compose up`
不需要额外的数据库容器，首次启动会自动初始化数据库并生成随机密码。

## 快速启动

```bash
# 方式一：Compose（推荐）
cp .env.example .env   # 可选
docker compose up -d --build

# 方式二：直接运行镜像
docker build -t hephaestus:local .
docker run -d --name hephaestus \
  -p 18181:18181 \
  -v hephaestus-data:/var/lib/hephaestus \
  --restart unless-stopped \
  --shm-size=256m \
  hephaestus:local
```

启动后访问 `http://localhost:18181`，首次进入会引导创建管理员账号。

查看启动过程和内置 PostgreSQL 日志：

```bash
docker logs -f hephaestus
docker exec hephaestus tail -f /var/lib/hephaestus/logs/postgresql.log
```

## 数据与持久化

所有运行数据都在 `/var/lib/hephaestus`，对应 Compose 卷 `hephaestus-data`：

| 路径 | 内容 |
| --- | --- |
| `config.yaml` | 首次启动生成的配置 |
| `.pg-password` | 首次启动生成的内置 PostgreSQL 密码（0600） |
| `.jwt-secret` | 首次启动生成的服务端 JWT 密钥（0600） |
| `pgdata/` | PostgreSQL 数据目录 |
| `data/` | 上传文件、截图、SQLite（如启用）等 |
| `logs/` | 应用日志和 `postgresql.log` |
| `config/` | PoC、字典等可更新资源 |

`docker compose down` 不会删除卷；只有显式执行 `docker compose down -v` 才会删除数据。

备份内置数据库：

```bash
docker exec -e PGPASSWORD="$(docker exec hephaestus cat /var/lib/hephaestus/.pg-password)" \
  hephaestus pg_dump -h 127.0.0.1 -U postgres hephaestus > hephaestus.sql
```

恢复：

```bash
cat hephaestus.sql | docker exec -i -e PGPASSWORD="$(docker exec hephaestus cat /var/lib/hephaestus/.pg-password)" \
  hephaestus psql -h 127.0.0.1 -U postgres -d hephaestus
```

## 配置项

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `18181` | Web 端口，仅首次生成 `config.yaml` 时写入 |
| `HEPHAESTUS_DB_TYPE` | `postgres` | `postgres` 或 `sqlite` |
| `HEPHAESTUS_DB_HOST` | `local` | `local`/`127.0.0.1` 使用内置 PostgreSQL；其他值使用外部数据库 |
| `HEPHAESTUS_DB_PORT` | `5432` | 内置或外部 PostgreSQL 端口 |
| `HEPHAESTUS_DB_USER` | `postgres` | 数据库用户 |
| `HEPHAESTUS_DB_PASSWORD` | 内置数据库自动生成，外部数据库留空 | 内置数据库已有 `.pg-password` 时以文件为准。外部数据库需要密码时，请提供实际密码 |
| `HEPHAESTUS_JWT_SECRET` | 自动生成 | 服务端 JWT 签名密钥；留空时生成并保存到 `.jwt-secret`，设置后以环境变量为准 |
| `HEPHAESTUS_DB_NAME` | `hephaestus` | 数据库名，不存在时应用会自动创建 |
| `HEPHAESTUS_DB_SSLMODE` | `disable` | 外部 PostgreSQL 的 SSL 模式 |
| `HEPHAESTUS_EMBEDDED_POSTGRES` | `1` | 设为 `0` 时即使 host 为 local 也不启动内置 PostgreSQL |
| `HEPHAESTUS_DEPLOYMENT` | `docker` | 容器标识；设置为 `docker` 时禁用页面内在线自更新 |
| `HEPHAESTUS_PORT` | `18181` | 仅 Compose 使用的主机映射端口 |

已有 `config.yaml` 时不会覆盖，配置以该文件为准。要切换数据库，请先编辑或删除数据卷中的
`config.yaml`。

### 使用外部 PostgreSQL

```yaml
services:
  hephaestus:
    environment:
      HEPHAESTUS_DB_HOST: postgres
      HEPHAESTUS_DB_PORT: "5432"
      HEPHAESTUS_DB_USER: hephaestus
      HEPHAESTUS_DB_PASSWORD: "${HEPHAESTUS_DB_PASSWORD:?请设置外部 PostgreSQL 密码}"
      HEPHAESTUS_DB_NAME: hephaestus
      HEPHAESTUS_DB_SSLMODE: disable
```

如果数据卷里已经有 `config.yaml`，直接改环境变量不会生效，需要同步修改该文件。外部
PostgreSQL 需要提前创建用户，并允许该用户创建 `HEPHAESTUS_DB_NAME` 指定的数据库
（应用会自动执行 `CREATE DATABASE`）。

### 使用 SQLite

```yaml
services:
  hephaestus:
    environment:
      HEPHAESTUS_DB_TYPE: sqlite
```

SQLite 文件位于 `/var/lib/hephaestus/data/hephaestus.db`。适合单机轻量使用；AI 工作台的
多节点/并发任务仍建议使用 PostgreSQL。

## AI 工作台运行时

镜像已经安装 AI 工作台需要的本地命令：

- Python 3、`pip`、`requests`
- `sqlmap 1.10.10`、`arjun 2.2.7`
- `dalfox 2.13.0`
- `nmap`、`curl`、`wget`、`git`、`jq`、`unzip`、`bash`
- `bubblewrap`（`bwrap`），用于 AI 本地命令的文件写入隔离

AI 会话创建的 `exec`/Python 命令会被限制在会话目录内，写入会话目录之外会被拒绝。
镜像调整了 bubblewrap 的 `/proc` 处理，因此默认 Docker 权限即可运行，不需要
`--privileged`。如果宿主机禁用了 user namespace，`bwrap` 会失败并按 fail-closed 返回阻断错误。

> 漏洞扫描的 SYN/原始套接字能力依赖 Docker 默认的 `NET_RAW` 权限；如果需要更完整的
> 网络能力，可自行增加 `cap_add: ["NET_RAW", "NET_ADMIN"]`。

## PoC 与字典资源

镜像不内置厂商的 PoC/字典更新包。首次部署后登录 Web，在“系统设置 → 系统更新 →
配置更新”中检查并下载配置包；下载后的文件会落在持久化卷的 `config/` 下，重建容器不会丢失。

## 健康检查

镜像内置 `/healthz`，会检查进程和数据库连通性：

```bash
curl -fsS http://localhost:18181/healthz
# {"status":"ok"}
```

Dockerfile 和 Compose 都配置了健康检查，`docker ps` 中会显示 `healthy`。

## 升级

容器部署不支持页面内的“在线更新/自更新”。相关接口会返回：

```text
容器部署不支持在线自更新，请拉取新镜像并重建容器
```

升级流程：

```bash
git pull
docker compose build --pull
docker compose up -d
```

数据卷保持不变。不要用页面内的更新按钮替换容器内二进制，否则更新会在容器重建后丢失，
并且可能让容器 PID 1 退出。

## 安全说明

- 容器内以 root 运行，以便原始套接字扫描和启动内置 PostgreSQL；PostgreSQL 本身以
  `postgres` 用户运行，且只监听 `127.0.0.1`，不会暴露到宿主机。
- 内置 PostgreSQL 使用随机密码；密码文件权限为 `0600`，并随数据卷持久化。
- 建议只把 18181 暴露给可信网络，并在外层加反向代理/TLS。
- 不建议添加 `--privileged`；当前镜像的 AI 沙箱和 PostgreSQL 都不需要它。

## 常见问题

**启动后一直 unhealthy**

```bash
docker logs hephaestus
docker exec hephaestus tail -n 100 /var/lib/hephaestus/logs/postgresql.log
```

常见原因是数据卷权限或宿主机资源不足。首次初始化 + 数据库迁移可能需要几十秒。

**忘记内置 PostgreSQL 密码**

密码在 `/var/lib/hephaestus/.pg-password`。如果文件丢失且数据库仍存在，需要以
`postgres` 用户进入容器重置密码：

```bash
docker exec -it -u postgres hephaestus psql -h /run/postgresql -d postgres
ALTER USER postgres PASSWORD 'new-password';
```

然后同步修改 `/var/lib/hephaestus/config.yaml` 中的 `database.password`。

**AI 命令提示 workspace blocked**

确认容器具有 user namespace 支持，并检查：

```bash
docker exec hephaestus bwrap --version
```

若宿主机内核禁用了 unprivileged user namespace，需要在宿主机启用，或改用独立扫描
Agent 节点执行 AI 工具。
