<div align="center">
<a href="https://github.com/qiwentaidi/Hephaestus"><img src="assets/appicon.png" width="120"/></a>
</div>
<h1 align="center">Hephaestus</h1>

<div align="center">
  <a href="README.md">中文</a> •
  <a href="README_EN.md">English</a>
</div>
<br />

Hephaestus 是集资产收集、攻击面评估、漏洞验证与 AI 渗透对话于一体的安全测试平台。提供 Web 服务、桌面客户端及独立扫描 Agent，支持本机扫描、组合任务和分布式节点执行。

- Web 服务下载：[Releases](https://github.com/qiwentaidi/Hephaestus/releases)
- 桌面客户端：[Hephaestus-Desktop](https://github.com/qiwentaidi/Hephaestus-Desktop)
- 扫描节点与命令行工具：[Hephaestus-Agent](https://github.com/qiwentaidi/Hephaestus-Agent)
- 许可证注册：[官网](http://47.97.59.120)

## 核心亮点

- **组合扫描**：将网站、端口、目录、子域名及 JS/API 扫描按需组合，在同一任务中查看执行进度与结果。
- **AI 渗透对话**：围绕授权目标调用工具开展测试，支持记录发现、复现步骤与证据，方便后续复核。
- **分布式执行**：选择本机或远程 Agent 执行任务，通过平台统一管理节点、任务与扫描结果。

## 主要功能

| 模块 | 功能 |
| --- | --- |
| 攻击面评估 | 网站指纹与漏洞、端口与服务、弱口令、目录、子域名及 JS/API 扫描 |
| 任务管理 | 并发控制、暂停与恢复、取消、扫描历史、计划任务、参数模板与结果检索 |
| AI 工作台 | 项目与会话、工具调用、图片输入、测试过程记录与证据查看 |
| AI 批量任务 | 批量任务创建、队列管理与执行状态查看 |
| 漏洞管理 | 汇总扫描与 AI 发现，查看复现步骤和证据，跟踪人工处置状态 |
| 资产管理 | 资产中心、资产拓扑与地图，集中查看收集结果 |
| 空间引擎 | FOFA、Hunter、Quake 查询及聚合搜索，支持历史、配额查看与下载 |
| 企业信息 | 企业及关联资产收集，支持 Excel 导出与合并导出 |
| 请求转发 | HTTP 请求编辑与响应查看，支持标签页保存及 Nuclei 模板润色、调试 |
| PoC 管理 | Nuclei 模板检索、编辑、扩展目录、调试与 OOB 连通性测试 |
| 分析工具 | 编解码、JWT、API 文档测试、文本比较与杀软识别；桌面端支持小程序反编译 |
| 数据与凭据 | 数据库连接、数据查看与密钥管理 |

## 选择运行方式

| 方式 | 数据库与运行特点 | 适用场景 |
| --- | --- | --- |
| Web 服务 | 默认 PostgreSQL，可显式配置 SQLite；前端静态资源嵌入后端，默认端口 `18181` | 浏览器使用、团队或服务器部署 |
| 桌面客户端 | 默认 SQLite，也支持 PostgreSQL；内置本地服务与独立设置窗口 | 单机使用、本地工具整合 |
| 扫描 Agent | 常驻节点通过共享 PostgreSQL 领取任务、发送心跳和回传结果 | 多节点扫描、远程网络执行 |

## 快速开始：Web 服务

### 1. 下载并选择正确的程序

通过 [Releases](https://github.com/qiwentaidi/Hephaestus/releases) 下载对应系统的版本，解压后在终端启动，例如 Linux AMD64：

```bash
chmod +x ./hephaestus-linux-amd64
./hephaestus-linux-amd64
```

Web 版首次运行会生成 `~/.config/hephaestus/config.yaml`。若默认 PostgreSQL 尚未准备好，程序会提示数据库连接失败；完成下一步配置后重新启动。

### 2. 配置数据库

#### 方案 A：PostgreSQL（Web 默认，分布式部署必需）

已有 PostgreSQL 时可直接使用，也可使用 Docker 启动本地测试数据库：

```bash
docker run -d \
  --name hephaestus-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD='replace-with-your-password' \
  -e POSTGRES_DB=hephaestus \
  -p 127.0.0.1:5432:5432 \
  -v hephaestus-pgdata:/var/lib/postgresql/data \
  postgres:15
```

在 `config.yaml` 中填写匹配的连接信息：

```yaml
web:
  port: 18181
database:
  type: postgres
  host: 127.0.0.1
  port: "5432"
  user: postgres
  password: "replace-with-your-password"
  dbname: hephaestus
  sslmode: disable
```

#### 方案 B：SQLite（支持 SQLite 的单机产物）

无需启动 PostgreSQL，在 `config.yaml` 中配置：

```yaml
web:
  port: 18181
database:
  type: sqlite
```

省略 `database.path` 时，数据库默认保存到 `~/.config/hephaestus/data/hephaestus.db`。可以用 `path` 指定绝对路径。SQLite 单机模式不适用于当前共享数据库的分布式 Agent 部署。

### 3. 打开页面并完成激活与初始化

保存配置后重新启动程序，访问 [http://localhost:18181](http://localhost:18181)。部署在服务器上时，将 `localhost` 替换为服务器地址；修改端口时，以 `web.port` 的配置为准。

1. 按页面提示填写许可证 Key 完成在线激活，或导入离线许可证。许可证注册入口见上方官网链接。
2. 激活后，首次使用会进入管理员账户创建页面；已初始化的实例直接登录。

### 4. 准备首次扫描或 AI 会话

登录后，根据需要完成以下配置：

- **扫描资源**：进入「系统设置 → 系统更新」，通过「配置文件更新」下载或更新 PoC、目录字典和弱口令字典等资源。
- **空间引擎**：使用 FOFA、Hunter、Quake 等服务前，在「系统设置 → API密钥」填写对应 Key。
- **AI 工作台**：在「系统设置 → AI 配置」填写渠道、API 地址、Key 和模型，并测试连接。
- **远程节点**：使用 Agent 前，准备主服务与节点共用的 PostgreSQL，并按 [Agent 仓库](https://github.com/qiwentaidi/Hephaestus-Agent) 的说明部署节点。

完成所需配置后，即可创建扫描任务，或在 AI 工作台中新建会话并说明授权目标与测试范围。

## 快速开始：桌面客户端

从 [桌面仓库](https://github.com/qiwentaidi/Hephaestus-Desktop) 获取适合系统和 CPU 架构的安装包。

安装并打开后，按页面提示完成许可证激活、管理员账户创建或登录。扫描资源、API 密钥和 AI 配置的准备步骤与 Web 服务相同。

- 默认配置文件：`~/.config/hephaestus/config-desktop.yaml`。
- 默认数据库：`~/.config/hephaestus/data/hephaestus.db`，也可切换为 PostgreSQL。
- 内置服务默认监听 `127.0.0.1:38456`；端口被占用时会使用临时端口。可通过 `HEPHAESTUS_DESKTOP_BACKEND_PORT` 指定首选端口。
- 顶部齿轮打开独立设置窗口，和应用内设置页共用设置组件。
- 提供本地启动器、小程序反编译等桌面专属入口。

Web 与桌面使用不同配置文件，但默认配置基础目录相同；需要隔离数据时可分别指定 `HEPHAESTUS_CONFIG_DIR`。

## 常见问题

### 浏览器截图不可用

截图依赖可用的 Chromium 浏览器及其运行环境。Linux Server 和精简容器需检查浏览器、字体及图形依赖。

### AI 本地命令无法执行

AI 会话中的本地命令和交互终端采用文件写入隔离，产物写入会话工作目录。macOS 使用 `sandbox-exec`，Linux 使用 Bubblewrap（`bwrap`）；缺少隔离工具或初始化失败时会拒绝执行。

### TCP 发现和 SYN 权限

当前 TCP 发现默认使用 Connect 模式。选择 SYN 时检查系统权限、抓包依赖及 SYN 能力状态，需要时通过平台授权入口启用。
