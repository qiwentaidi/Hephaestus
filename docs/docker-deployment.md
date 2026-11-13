# Docker 部署

Hephaestus 的 Docker 镜像默认内置 PostgreSQL 17。`docker run` 或 `docker compose up`
不需要额外的数据库容器，首次启动会自动初始化数据库并生成随机密码。

## 快速启动

官方镜像发布在 `ghcr.io/qiwentaidi/hephaestus`，同时提供 `linux/amd64` 与 `linux/arm64`。
公开包可以直接匿名拉取，不需要先登录 GHCR。

### 方式一：拉取官方镜像

```bash
docker pull ghcr.io/qiwentaidi/hephaestus:latest

docker run -d --name hephaestus \
  -p 18181:18181 \
  -v hephaestus-data:/var/lib/hephaestus \
  --restart unless-stopped \
  --shm-size=256m \
  ghcr.io/qiwentaidi/hephaestus:latest
```

用 Compose 管理时，在本仓库根目录叠加 `docker-compose.ghcr.yml`（只需要 `docker-compose.yml` 和这个覆盖文件），把本地构建换成已发布镜像：

```bash
docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up -d
```

镜像 tag 与 Release 版本号一致（如 `v0.5.7`），另有浮动的 `latest`。生产环境建议固定版本：

```bash
HEPHAESTUS_IMAGE=ghcr.io/qiwentaidi/hephaestus:v0.5.7 \
  docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up -d
```

升级时重新拉取镜像并重建容器，数据卷不变：

```bash
docker compose -f docker-compose.yml -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up -d
```

单容器部署同理：重新 `docker pull` 后，用相同参数重建 `hephaestus` 容器。

### 方式二：从源码构建

```bash
cp .env.example .env   # 可选
docker compose up -d --build
```

不使用 Compose 时：

```bash
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
| `HEPHAESTUS_DEPLOYMENT` | `docker` | 容器标识；Docker 在线升级由宿主机更新器执行 |
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
- `sqlmap 1.10.10`
- `nmap`、`curl`、`wget`、`git`、`jq`、`unzip`、`bash`

AI 会话创建的 `exec`/Python 命令以容器为隔离边界，和 ARTEX 一致：不再额外把命令限制在
会话目录内，也不需要 `--privileged`、`bwrap` 或 user namespace 支持。命令的副作用策略由
「工具拦截规则」和审计承担。需要把命令放进隔离网络执行时，改用下面的出口网关。

`Write`/`Edit` 文件工具仍只能写当前会话工作目录，`cmd-output` 为保留目录；这样遗留产物都
落在界面可删除的会话目录里。命令产生的临时文件也会通过 `TMPDIR` 指向会话目录的 `tmp`，
但命令显式写 `/tmp` 不会被拦截。担心临时目录膨胀时，可以给容器 `/tmp` 加内存盘上限：

```yaml
services:
  hephaestus:
    tmpfs:
      - /tmp:size=512m
```

`docker run` 使用 `--tmpfs /tmp:size=512m`。超出上限的写入会直接失败，不会撑满宿主机磁盘；
该目录由内存承载，重启容器即清空。

> 漏洞扫描的 SYN/原始套接字能力依赖 Docker 默认的 `NET_RAW` 权限；如果需要更完整的
> 网络能力，可自行增加 `cap_add: ["NET_RAW", "NET_ADMIN"]`。

### 按需安装 Chromium

需要网页截图或无头浏览器交互时，可在 Docker 宿主机上为运行中的 Hephaestus 容器安装 Chromium。

```bash
docker exec --user root hephaestus apk add --no-cache chromium
```

此命令适用于基于 Alpine 3.24 的镜像。若容器使用自定义名称，请替换 `hephaestus`。
安装后可用 `docker exec hephaestus chromium --version` 确认。普通重启会保留安装；在线升级
或其他操作重建容器后，请重新执行安装命令。

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

管理员可以在更新弹窗或「系统设置 → 系统更新」升级已接入宿主机更新器的 Docker 部署。
点击「更新」会先下载镜像。空闲时自动重启，有运行中任务时沿用 Web 版的重启确认。
升级沿用原 Compose 项目和数据卷，服务恢复并确认目标版本后页面会自动刷新。

在 Docker 宿主机上一次性接入更新器。需要 Linux 或 macOS、Go 1.26.3，以及支持
`!reset` 和 `up --wait` 的 Docker Compose V2。发布镜像位于
`ghcr.io/qiwentaidi/hephaestus`，使用版本号 tag 和 `latest`。宿主机需要有该镜像的拉取权限。

在原部署目录执行以下命令。已有部署请将 `项目名` 替换为原项目名，可用
`docker compose ls` 查看。新部署可以自行指定项目名。首次激活会切换到已发布的镜像。

```bash
(cd backend && go build -o ../.hephaestus-updater-bin ./cmd/docker-updater)
./.hephaestus-updater-bin -project-name 项目名 -activate
```

激活后让更新器随宿主机常驻。使用 systemd 或 launchd 托管时，启动命令去掉
`-activate`，工作目录设为原部署目录，并使用同一个能访问 Docker 的账号。
macOS 默认使用本机端口 18182，端口冲突时可以通过 `-desktop-port` 指定，
激活和后续启动需使用相同端口。运行状态和镜像引用保存在部署目录的
`.hephaestus-updater/` 中。

如需手动管理已接入更新器的容器，使用同一个项目和发布版 Compose 配置：

```bash
docker compose -p 项目名 --env-file .env --env-file .hephaestus-updater/image.env \
  -f docker-compose.yml -f docker-compose.release.yml ps
```

没有 `.env` 时省略对应参数。有额外 Compose 覆盖文件时，应继续使用原覆盖文件。
未接入更新器的官方镜像部署，按「快速启动」中的 `pull` + `up -d` 升级。
源码构建部署仍可用 `git pull --ff-only && docker compose build --pull && docker compose up -d` 升级。

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

**AI 本地命令被拦截**

AI 会话里的 `Bash`、Python、交互终端以容器为隔离边界，不再有额外的文件系统沙箱，因此
不会出现 bwrap / user namespace 相关报错。命令被拒绝时通常是「工具拦截规则」命中：

- 「系统设置 → 工具拦截规则」中查看命中的规则，按需调整或停用；
- 出口网关开启时（`HEPHAESTUS_AI_ENFORCE_EGRESS=1`），命令进入网关容器执行，网络请求
  还会经过出口审计，被拦截时提示会指向「出口审计」记录。

如果命令直接执行失败（非规则拦截），按普通容器排障处理：确认镜像里有对应工具、容器有
足够权限，并查看 `docker logs hephaestus`。
