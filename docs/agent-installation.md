# Agent 下载、安装与更新

Agent 面向使用者提供预编译发行包，安装时无需 Go 或本地构建。公开发行仓库为 <https://github.com/qiwentaidi/Hephaestus-Agent>。SSH 主机管理、SSH 部署与主服务运行时编译已移除；旧数据库中的主机表不执行破坏性删除。

## 版本规则

Web 与 Agent 使用 `backend/pkg/version/version.go` 中的公共 `Version`。当前共享 PostgreSQL 架构使用精确版本匹配，安装器每次安装/更新都会查询主服务，不使用 GitHub `latest`。未来兼容规则由 `backend/pkg/agentrelease.Current` 统一调整，用户入口无需改变。

公共引导接口只返回发布元数据和安装脚本：

- `GET /api/v1/agent/release`：所需版本、五平台发行包与 SHA-256 清单地址。
- `GET /api/v1/agent/release?format=text`：安装器使用的所需版本。
- `GET /api/v1/agent/install.sh`：Linux/macOS Bash 安装器。
- `GET /api/v1/agent/install.bat`：包含完整 PowerShell 安装器的 Windows BAT，无需额外下载脚本。
- `GET /api/v1/agent/install.ps1`：PowerShell 安装器及后续更新器。

这些接口随新版主服务提供。仓库中还未发布对应版本时，安装器明确失败并保留当前程序，不回退到旧版或其他仓库。

## Linux / macOS

将示例地址换成节点能够访问的主服务地址。在终端运行：

```bash
curl -fsSL 'https://your-server/api/v1/agent/install.sh' -o install.sh
bash install.sh --server 'https://your-server'
```

默认目录为执行命令时所在目录的 `hephaestus-agent`。脚本检测 amd64/arm64，下载 ZIP 和 `SHA256SUMS.txt`，只提取预期的二进制，校验 SHA-256 并实际执行 `version` 检查，然后询问 PostgreSQL 连接与节点角色。数据库密码不回显，配置文件权限为 `0600`。已有 `config.yaml` 原样保留，不覆盖节点 ID、角色、容量或用户配置。非交互首次安装使用 `--config /path/to/node-config.yaml`。

安装需要 Bash、curl 或 wget、unzip、iconv，以及 sha256sum 或 shasum。配置使用 UTF-8；输入包含无效编码时会要求重新输入对应字段，已有配置包含无效编码时会在启动前提示修正。Linux 的 AI 执行工具仍需要 Bubblewrap；节点使用的 Python、stdio MCP 等依赖也需在该节点准备。安装器不关闭沙箱、不提升扫描进程权限来绕过缺失依赖。

默认前台运行，实际启动错误会返回终端。正常退出用 Ctrl+C。再次启动和更新：

```bash
bash ./hephaestus-agent/run.sh
# 更新前先正常停止前台节点。
bash ./hephaestus-agent/update.sh
```

更新器重新查询主服务兼容版本，保留配置和 `config/` 资源，留下 `hephaestus-scan-agent.previous`，随后前台启动。没有运行依赖安装或源码编译步骤。自定义目录使用 `--install-dir`，并使用该目录内的 run/update 脚本。只下载/配置而不启动使用 `--no-start`。

### Linux systemd（可选）

```bash
sudo bash install.sh --server 'https://your-server' --service
sudo bash ./hephaestus-agent/update.sh
```

默认目录仍为执行命令时所在目录下的 `hephaestus-agent`，与前台安装一致；`--service` 只改变运行方式，服务名为 `hephaestus-scan-agent`。需要其他目录时显式传入 `--install-dir`。安装服务需要 root；如果需要按最小权限运行，可由管理员按扫描能力配置服务账户、目录权限和所需 capabilities。

更新先完成下载与校验，再停止服务。服务必须确认 `inactive` 后才替换程序；停止超时不会强杀仍在运行的 Worker，也不会报告升级完成。新服务启动检查失败时恢复前一程序及版本记录。服务已启动仍需在 Agent 页面确认节点心跳、版本与能力；启动检查不等同于任务链路验收。

## Windows amd64

可以在 工作节点管理页下载 BAT 后双击，也可以在 PowerShell 中运行：

```powershell
Invoke-WebRequest -Uri 'https://your-server/api/v1/agent/install.bat' -OutFile install.bat
.\install.bat -Server 'https://your-server'
```

BAT 自带 PowerShell 脚本，首次运行交互输入 PostgreSQL 连接与角色。默认目录为执行命令时所在目录下的 `hephaestus-agent`，配置文件仅当前用户与 SYSTEM 可访问。默认前台运行；正常停止使用 Ctrl+C。

```powershell
& '.\hephaestus-agent\run.bat'
# 更新前先正常停止前台节点。
& '.\hephaestus-agent\update.bat'
```

PowerShell 参数包括 `-InstallDir`、`-ConfigFile`、`-Version`、`-ReleaseBase`、`-NoStart`。运行中的程序会被拒绝更新，不使用强制终止代替 Worker 退出确认。替换使用 Windows 原子文件替换并保留 `.previous`。此入口不注册 Windows 服务；macOS 入口同样以前台运行为默认。

## 发布者操作

发布入口为源码仓库的 `scripts/publish-agent.sh`，实现位于 `scripts/publish_agent.py`。只编译仍使用 `scripts/build-agent.sh`。

全平台发布在 macOS 上执行，需要 Python 3.9+、Git、GitHub CLI (`gh`)、Go、zip、shasum、Xcode 命令行工具及运行中的 Docker / Buildx。GitHub 认证优先使用 `gh auth login` 或 `GH_TOKEN` / `GITHUB_TOKEN`；未配置 gh 登录时，会尝试系统 Git credential helper，凭据只在进程内使用，不写文件或输出到日志。

公开仓库默认位于源码仓库同级的 `Hephaestus-Agent`，需要 checkout `main`、工作区干净且与 `origin/main` 同步。脚本只向 `qiwentaidi/Hephaestus-Agent` 发布，不推送私有源码仓库。

```bash
# 一次性准备（已经登录、已有公开仓库时不用重复）
gh auth login
git clone https://github.com/qiwentaidi/Hephaestus-Agent.git ../Hephaestus-Agent

# 重新构建全部平台，并创建或更新源码版本对应的 GitHub Release
./scripts/publish-agent.sh

# 可选：显式校验版本，防止发布错版本
./scripts/publish-agent.sh --version v0.4.7

# 只检查现有 ZIP、校验清单和导出的安装器；不会编译或修改远端
./scripts/publish-agent.sh --check

# 公开仓库在其他目录，或需要提供发布说明时
./scripts/publish-agent.sh --agent-repo-dir /path/to/Hephaestus-Agent --notes-file /path/to/release-notes.md

# 仅编译，不发布
./scripts/build-agent.sh all
```

发布新版本前，修改 `backend/pkg/version/version.go` 的公共 `Version`，并按同一版本构建、部署主服务。`--version` 仅用于核对，不会修改源码版本。

脚本会重新构建五个平台，校验 ZIP 只包含约定的可执行文件、检查 SHA256，再上传并逐一比对 GitHub 返回的文件大小和哈希。随后上传并验证校验清单，将导出的三个安装器同步到公开仓库 `scripts/`，有变更时单独提交并推送。已有 Release 保留原说明，只有传入 `--notes-file` 才替换；新 Release 先创建草稿，完成校验和安装器推送后再公开。已有标签不会强制移动。

同版本重发会覆盖同名附件，相同哈希的 ZIP 跳过上传。新包与安装器就绪后，清理五个旧命名的 `hephaestus-scan-agent_<os>_<arch>.zip`，其他附件保留。发布前的 Release 元数据保存在 `dist/agent/release-before-<version>.json`。已有安装如果仍使用旧文件名，应先获取公开仓库 main 分支的新版安装器。

上传出错时脚本立即停止，不会宣称成功；远端可能已有部分新附件。排除问题后可以重跑，仍会重新构建。若安装器提交成功但 Git push 失败，先在公开仓库检查并推送该提交、同步 main，再重跑。`--check` 仅检查本地完整性，不证明产物版本，也不检查登录权限；正式发布始终重新编译。

版本参数必须与源码公共版本一致，防止发行标签与实际节点版本不同。Linux 构建需要 Docker Buildx；macOS CGO 构建须使用适合目标架构的工具链。构建产物位于 `dist/agent/packages/`：

```text
hephaestus-agent-linux-amd64.zip
hephaestus-agent-linux-arm64.zip
hephaestus-agent-darwin-amd64.zip
hephaestus-agent-darwin-arm64.zip
hephaestus-agent-windows-amd64.zip
SHA256SUMS.txt
```

将这些文件发布到 `qiwentaidi/Hephaestus-Agent` 的同版本 Release。不要上传源码、真实配置或用户资源。GitHub 自动生成的 Source code 包只包含该公开仓库自己的说明文件，不会引用私有主仓库。

安装脚本单独生成到 `dist/agent/installer-assets/`，发布脚本将其中的 `install.sh`、`install.bat`、`install.ps1` 提交到公开仓库的 `scripts/` 目录。安装说明使用 main 分支的稳定下载入口；这些脚本不作为 Release 附件上传。主服务仍通过 `/api/v1/agent/install.*` 提供内嵌安装器。

版本与兼容信息由主服务的 `/api/v1/agent/release` 提供，不生成或上传 `release.json` 附件。

只导出可下载安装器（不构建扫描程序）：

```bash
cd backend
go run ./cmd/agent-release --output ../dist/agent/installer-assets
```

原始 `backend/pkg/agentrelease/installers/install.bat` 是嵌入模板；必须使用主服务下载接口或上面的导出器生成完整 BAT。`scripts/install-scan-agent.sh` 仅为源码仓库内的便利入口，不是面向用户的下载地址。

## 平台管理更新

工作节点管理页提供「安装工作节点」和「更新管理」。支持单节点与最多 100 个节点的批量请求，批次按请求顺序逐个执行。目标版本由主服务指定；节点在开始更新及替换前再次核对主服务兼容版本，不追随 GitHub latest，也不依赖 Release 中的 JSON 附件。

更新请求保存在 `agent_update`，节点通过 PostgreSQL 接收。独立更新进程取得全平台批次锁后暂停该节点领取 AI、扫描任务，不改变用户的启用设置。当前任务继续执行并续租；等任务结束后下载 ZIP、检查 SHA-256、实际程序版本与更新能力。只有子进程的所有 Worker 完成退出、节点会话释放且进程真正退出后，更新进程才替换程序。Windows 的更新进程使用独立程序副本，避免运行中的 EXE 阻止替换。

平台展示排队、等待任务、下载、校验、重启、心跳确认和回滚状态。等待与下载阶段可以取消；重启开始后不可取消。成功需要新进程的 PID、目标版本和后续心跳同时匹配。失败尝试恢复上一程序并确认旧版本重新上线；未能确认退出时持续保留等待状态，不强杀任务或提前报告成功。配置、角色、启用状态、节点 ID 和资源目录保持原样。

发布时先部署新版主服务并完成迁移，再发布包含更新能力的新 Agent 版本。已发布的原始 v0.4.6 二进制不具备此能力，不能仅靠新增平台按钮直接更新；不要覆盖该既有版本的发行附件。现有节点首次需用新版安装器和新版本二进制手动升级，然后通过 `run.sh`、`run.bat` 启动。Linux 既有 systemd 安装首次迁移应加 `--service`，将服务入口更新为安装目录中的 `run.sh`。直接启动 Worker 二进制不会声明平台更新能力。

公开发行附件仍只有各平台 ZIP 与 `SHA256SUMS.txt`。安装脚本在公开仓库的 `scripts/` 中，更新所需的守护能力集成于 Agent 程序，不额外发布 SH、BAT 或 JSON 附件。

## 保留现有节点

已有节点可显式指定旧安装目录与原节点配置，例如 `--install-dir /opt/hephaestus/agent --config /opt/hephaestus/agent/config/config.yaml`。首次采用新安装器前，正常停止原进程或原服务；旧安装器创建的 systemd 服务不会自动被当作新安装器管理的服务。若需迁移为新管理服务，停止旧服务后使用 `--service`，检查角色与容量的旧环境覆盖，再确认节点复用了原 ID。

前台启动器留下 `.run-lock`，避免运行期间更新；异常强杀后须先确认没有残留节点进程，才能清理该锁。安装器也使用 `.install-lock` 防止并发安装。更新不重写已有配置；需要更换数据库/主服务连接时应编辑配置并按正常退出流程重启。
