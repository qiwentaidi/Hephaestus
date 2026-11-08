# 本地开发与原生运行

## 前后端分离开发

准备 Go 1.26.3 或更新版本、Node.js 22 和 npm，以及 CGO 工具链。依赖 libpcap 的扫描功能需要对应平台的开发库，例如 macOS 的 `libpcap` 或 Ubuntu 的 `libpcap-dev`。Windows 上使用相关抓包功能时需安装 Npcap。

### 后端

从仓库根目录启动后端，默认端口为 `8080`。

```bash
cd backend
go run -tags dev .
```

原生配置通常位于 `~/.config/hephaestus/config.yaml`，也可用 `HEPHAESTUS_CONFIG_DIR` 指定配置目录。Web 后端默认连接 `127.0.0.1:5432` 上的 PostgreSQL，用户名为 `postgres`，数据库为 `hephaestus`。如需密码或其他连接参数，请修改 `config.yaml` 中的 `database` 配置后重启。

单机开发也可以使用 SQLite。以下示例在独立目录中保存配置与数据库。从仓库根目录执行。

```bash
cd backend
mkdir -p .dev-config
cat > .dev-config/config.yaml <<EOF_CONFIG
database:
  type: sqlite
  path: "$(pwd)/.dev-config/hephaestus.db"
EOF_CONFIG

HEPHAESTUS_CONFIG_DIR="$(pwd)/.dev-config" go run -tags dev .
```

如需在原生后端重启后保留登录状态，请为该环境生成随机的 `HEPHAESTUS_JWT_SECRET`，并在每次启动时使用同一个值。

### 前端

另开一个终端，从仓库根目录执行。

```bash
cd frontend
npm ci
npm run dev
```

打开 [http://localhost:3000](http://localhost:3000)。前端默认连接后端的 `8080` 端口。如果后端用 `PORT` 变更了开发端口，请在启动前端时设置相同的 `NEXT_PUBLIC_BACKEND_PORT`。

## 原生 Web 二进制

构建脚本将前端静态产物嵌入 Go 二进制，并把可执行文件和 ZIP 包输出到 `dist/`。Linux 构建需要 Docker Buildx。

```bash
./scripts/build.sh current
```

以 macOS ARM64 为例，从仓库根目录运行。

```bash
./dist/hephaestus-darwin-arm64
```

默认访问 [http://localhost:18181](http://localhost:18181)。原生二进制从 `config.yaml` 的 `web.port` 读取端口，并使用其中的数据库连接配置。多平台构建和产物路径见[构建与发布脚本](../scripts/README.md)。

## 桌面客户端

桌面客户端基于 Wails 3，默认使用 SQLite，本地后端端口为 `38456`。各平台还需准备原生工具链和 WebView 运行环境，例如 macOS 的 Xcode 命令行工具、Windows 的 WebView2、Linux 的 GTK 和 WebKit 开发库。

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.78
cd backend/cmd/desktop
wails3 task dev
```

在同一目录运行 `wails3 task package` 可打包当前平台。发布打包使用仓库根目录的 `./scripts/build-desktop-release.sh`，产物默认位于 `dist/desktop/`。

## 可选的 AI 工具出口网关

默认的 `toolguard` 在工具调用前执行规则检查。需要把支持的命令、Python 和 HTTP 工具放到受控网络中运行时，可在宿主机原生运行的 Web 服务上启用 Docker 出口网关。宿主机需要 Docker daemon 和 Go，执行以下命令前先完成原生二进制构建。

```bash
./deploy/egress/build.sh
docker image inspect hephaestus-egress:local

# 以 macOS ARM64 为例
HEPHAESTUS_AI_ENFORCE_EGRESS=1 ./dist/hephaestus-darwin-arm64
```

执行期间可用 `docker ps --filter label=hephaestus.egress=1` 查看临时网关和 Worker 容器。工具结束后会清理容器。出口镜像内的工具依赖在 [deploy/egress/Dockerfile](../deploy/egress/Dockerfile) 中管理。

当前 Web 运行镜像缺少 Docker CLI 和 Docker socket，网关代理绑定宿主机的回环地址。使用默认 Compose 部署时请保持该选项关闭。
