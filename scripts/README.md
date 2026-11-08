# 构建与发布脚本

日常构建、打包和发布入口统一放在本目录。从项目根目录执行：

| 命令 | 用途 | 产物位置 |
| --- | --- | --- |
| `./scripts/build.sh all` | Web 前端及后端多平台构建 | `dist/` |
| `./scripts/build-agent.sh all` | Agent 五平台构建 | `dist/agent/` |
| `./scripts/publish-agent.sh` | Agent 五平台构建、校验和 GitHub 发布 | `dist/agent/` 和公开 Release |
| `./scripts/build-desktop-release.sh` | 桌面端多平台打包 | `dist/desktop/` |
| `./scripts/build-macos-release.sh` | 仅打包两种 macOS 架构的桌面端 | `dist/desktop/` |

脚本根据自身位置定位项目，可以通过绝对路径从其他工作目录调用。原根目录和桌面模块下的旧构建入口已迁移，不保留副本。

Web 与 Agent 构建支持 `help`、`current`、`all` 等命令；Agent 发布支持 `--help`、`--check`、`--version`、`--notes-file`。例如：

```bash
./scripts/build.sh help
./scripts/build-agent.sh platform linux/amd64
./scripts/publish-agent.sh --check
./scripts/publish-agent.sh --version v0.4.7
```

`publish_agent.py` 是 Agent 发布实现，`test_publish_agent.py` 是其离线回归测试：

```bash
python3 -B -m unittest discover -s scripts -p test_publish_agent.py
```

详细要求见 [Agent 发布说明](../docs/agent-installation.md#发布者操作) 和 [桌面打包说明](../backend/cmd/desktop/scripts/README.md)。Wails 的 Taskfile、生成的底层构建资源及安装包生命周期脚本保留在 `backend/cmd/desktop/build/`，由工具链管理。`install-scan-agent.sh` 是源码安装便利入口。
