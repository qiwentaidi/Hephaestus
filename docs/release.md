# 发布流水线

Web 端与桌面端共用一套版本号、一套构建脚本、一次发布。改完版本号后只需要推一个 tag。

## 版本号规则

- 版本号写在 `backend/pkg/version/version.go`，`Version` 与 `DesktopVersion` 必须相等（`go test ./pkg/version` 会失败），桌面端的 `Info.plist` / Windows 版本资源也从这个常量生成。
- tag 必须与常量完全一致：工作流的 `prepare` 作业会在发布前用 `grep -Fx` 校验，不一致直接失败。
- 版本号只能单调递增。已安装的 3.x 客户端用字符串拆段比较版本，所以从 `v3.3.5` 统一到 `v4.0.0` 是可行的，而回退到 `v0.x` 会让老用户永远收不到更新。

## 一次发版

```bash
# 1. 改版本号（两个常量）
$EDITOR backend/pkg/version/version.go

# 2. 写更新日志：写成 docs/releases/v4.0.1.md，或用附注 tag 承载
git tag -a v4.0.1 -F notes.md

# 3. 推送，CI 全自动
git push origin v4.0.1
```

CI 只负责产出 Release，不再写官网记录。手动补写时 `publish_website.py` 的更新日志按这个顺序取：附注 tag 的消息 → `docs/releases/<version>.md` → 刚发布的 GitHub Release 正文 → 留空（后端保留原文），详见 [docs/releases/README.md](releases/README.md)。

## 产物清单

一个 tag 产出 10 个文件，全部挂在同一个 GitHub Release 下：

| 端 | 文件 |
| --- | --- |
| Web | `Hephaestus-windows-amd64-web.zip`、`Hephaestus-linux-{amd64,arm64}-web.zip`、`Hephaestus-darwin-{amd64,arm64}-web.zip` |
| 桌面端 | `Hephaestus-windows-amd64-desktop.zip`、`Hephaestus-linux-{amd64,arm64}-desktop.zip`、`Hephaestus-darwin-{amd64,arm64}-desktop.dmg` |
| 校验 | `SHA256SUMS` |

命名是**平台优先**：`Hephaestus-<os>-<arch>-<web|desktop>`。GitHub Release 页面按文件名排序，这样同一平台的两种产物会相邻，桌面端不会夹在 Web 的 darwin 与 linux 之间。

## 工作流结构

`.github/workflows/release.yml` 只在 `v*` tag 和手动触发时运行：

| 作业 | runner | 说明 |
| --- | --- | --- |
| `prepare` | ubuntu-24.04 | 校验 tag 格式与版本常量，输出 `version` |
| `macos` | macos-14 | Web darwin 两个 zip + 桌面端两个 dmg（签名、公证）+ 桌面端 Windows zip（mingw 交叉编译） |
| `linux` ×2 | ubuntu-24.04 / ubuntu-24.04-arm | 每台产出 Web 与桌面端各一个 zip；桌面端在 `Dockerfile.linux-cross` 内构建，保持 bookworm 的 glibc 下限 |
| `windows` | ubuntu-24.04 | Web Windows zip（`CGO_ENABLED=0` 交叉编译） |
| `publish` | ubuntu-24.04 | 复用 `publish-release.yml`：取回产物 → 拍平路径 → 校验 10 个文件名 → 写 `SHA256SUMS` → 发布 GitHub Release |

构建步骤全部复用仓库脚本，不在 YAML 里重复：

```bash
./scripts/build.sh all --platform darwin/arm64,darwin/amd64   # Web
./scripts/build-desktop-release.sh --target darwin,windows    # 桌面端
```

`docker-release.yml` 仍然按 `v*` tag 构建 `ghcr.io/qiwentaidi/hephaestus`。

## 需要的仓库配置

| 类型 | 名称 | 说明 |
| --- | --- | --- |
| Variable | `RELEASE_REPO` | 发布目标仓，例如 `qiwentaidi/Hephaestus`；不设则发回当前仓库 |
| Secret | `RELEASE_TOKEN` | fine-grained PAT，对目标仓 `Contents: write`（源码仓私有，`GITHUB_TOKEN` 不能跨仓） |


macOS 签名与公证沿用既有的一组 secret：`APPLE_CERTIFICATE_BASE64`、`APPLE_CERTIFICATE_PASSWORD`、`APPLE_SIGN_IDENTITY`、`APPLE_ID`、`APPLE_APP_SPECIFIC_PASSWORD`、`APPLE_TEAM_ID`、`APPLE_KEYCHAIN_PASSWORD`。缺省时 dmg 会做 ad-hoc 签名并在日志里给出 warning，用户首次打开会被 Gatekeeper 拦下。

## 发布与分发

- CI 产出的 10 个文件全部挂在**同一个 GitHub Release** 下，这是唯一的产物来源。
- 官网与客户端更新记录只指向 GitHub：`publish_website.py` 不再下发 `*_mirror_url`（除非显式传 `--mirror-base-*`），Gitee 链接已从官网移除并停止维护。
- CI 负责 GitHub 一侧；发布完成后 Actions 页面可以直接下载全部产物，或用 `gh release download <tag> --repo <RELEASE_REPO>`。
- 以后若要再减少上传，可在 `WEBSITE_PRIMARY_BASE` 指向新的国内源并在 `trustedUpdateDownloadHosts` 里加入该域名；白名单编译在客户端里，新域名必须比下载地址提前一个版本生效。
- **`Hephaestus-Desktop` 仓库**：`v4.0.0` 之后不再接收产物，历史 `v3.x` Release 保留但不再更新；桌面端下载统一走 `Hephaestus` 仓库。建议在该仓库 README 顶部留一句重定向说明。

## 重试与排障

- `prepare` 会跑一遍脚本离线测试（`python3 -B -m unittest discover -s scripts -p 'test_publish_*.py'`）。
- 发布环节出错时不要重新打 tag。`publish-release.yml` 支持手动指定构建 run 补跑发布，产物直接从该 run 的 artifacts 取回，不重新编译：

  ```bash
  gh workflow run publish-release.yml -R <owner>/<repo> \
    -f version=v4.0.0 -f run_id=<构建 run 的 id>
  ```
- `upload-artifact` 会按公共父目录保留相对路径（桌面端产物落在 `desktop/` 下），发布工作流会先拍平再校验，改产物路径时注意这一点。
- 只想验证构建：手动触发 `release` 工作流（`workflow_dispatch`），`publish` 作业只在 tag 构建时执行。

## 常用操作

```bash
# 只构建、不发布：Actions 里手动触发 release 工作流即可（publish 作业只在 tag 构建时运行）
# 本地产出与 CI 同名的产物
./scripts/build.sh all
./scripts/build-desktop-release.sh

# 单独补写官网记录（内部工具，不在仓库里）
python3 scripts/publish_website.py --version v4.0.1 --sha256-file SHA256SUMS \
  --changelog-file notes.md --primary-base https://github.com/qiwentaidi/Hephaestus/releases/download

# 回滚官网指向：把旧版本重新设为最新
curl -X POST "$WEBSITE_API_BASE/api/releases/<旧版本 id>/set-latest" \
  -H "Authorization: Bearer <token>"
```

## 已知限制

- Gitee 已停止作为下载源（单附件 100MB、每仓库附件总量 1GB 的免费配额本来也放不下 330MB 的版本包），官网与客户端更新记录只保留 GitHub 链接。
- 官网后端目前不保存 sha256（`POST /api/releases` 会接收但未落库），校验和只在 Release 的 `SHA256SUMS` 里。
- Web 的 Linux 产物在 `golang:1.26-alpine` 里以 `-linkmode external -extldflags '-static'` 链接，是静态二进制，不依赖发行版 glibc；桌面端 Linux 产物在 bookworm 内动态链接，实测最高只要求 `GLIBC_2.34`（Ubuntu 22.04 及以上可运行）。所以两者都必须继续在 Docker 镜像内构建，改用 runner 原生构建会把下限推到 2.39。
- 桌面端 Windows 产物用 mingw 在 macOS runner 上交叉编译，因为 `gopacket/pcap` 需要 CGO + libpcap；Windows runner 上的原生 CGO 构建需要 Npcap SDK，暂未启用。
- macOS 的 Web 二进制必须链接系统的 `/usr/lib/libpcap.A.dylib`（由 dyld 共享缓存提供）。`scripts/build.sh` 会用 `CGO_LDFLAGS=-L$(xcrun --sdk macosx --show-sdk-path)/usr/lib` 固定链接目标，并在打包前用 `otool -L` 校验；一旦链接到 `/opt/homebrew` 或 `/opt/local`，构建直接失败，避免发出在用户机器上起不来的包。
