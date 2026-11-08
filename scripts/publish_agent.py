#!/usr/bin/env python3
"""Build and publish Agent binaries; never copy private source into the public repo."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = "qiwentaidi/Hephaestus-Agent"
PLATFORMS = ("darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64")
INSTALLERS = ("install.sh", "install.ps1", "install.bat")
PACKAGES = tuple(f"hephaestus-agent-{p}.zip" for p in PLATFORMS)
LEGACY = tuple(f"hephaestus-scan-agent_{p.replace('-', '_')}.zip" for p in PLATFORMS)


def log(message):
    print(f"[publish-agent] {message}", flush=True)


def run(args, *, cwd=ROOT, env=None, capture=True):
    return subprocess.run(args, cwd=cwd, env=env, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def validate_artifacts(dist):
    packages = dist / "packages"
    if {p.name for p in packages.iterdir()} != set(PACKAGES) | {"SHA256SUMS.txt"}:
        raise RuntimeError("发布目录必须只包含五个平台 ZIP 和 SHA256SUMS.txt，请重新运行全平台构建。")
    checks = {}
    for line in (packages / "SHA256SUMS.txt").read_text().splitlines():
        digest, name = line.split()
        name = name.removeprefix("*")
        if name in checks or not re.fullmatch(r"[a-fA-F0-9]{64}", digest):
            raise RuntimeError("校验清单包含重复文件或无效哈希。")
        checks[name] = digest.lower()
    if set(checks) != set(PACKAGES):
        raise RuntimeError("校验清单与五个平台不匹配。")
    for name in PACKAGES:
        path = packages / name
        if sha256(path) != checks[name]:
            raise RuntimeError(f"校验失败：{name}")
        with zipfile.ZipFile(path) as archive:
            executable = "hephaestus-scan-agent" + (".exe" if "windows" in name else "")
            if archive.namelist() != [executable] or archive.testzip() is not None:
                raise RuntimeError(f"ZIP 内容不符合发布约定：{name}")
    for name in INSTALLERS:
        if not (dist / "installer-assets" / name).is_file():
            raise RuntimeError(f"缺少导出的安装器：{name}")
    if b"__HEPHAESTUS_POWERSHELL_BASE64__" in (dist / "installer-assets/install.bat").read_bytes():
        raise RuntimeError("install.bat 仍为模板，必须先运行安装器导出。")
    return {name: sha256(packages / name) for name in (*PACKAGES, "SHA256SUMS.txt")}


def github_environment():
    env = os.environ.copy()
    if subprocess.run(["gh", "auth", "status"], env=env, stdout=subprocess.DEVNULL,
                      stderr=subprocess.DEVNULL).returncode == 0:
        return env
    # Reuse the configured helper without printing or persisting its credential.
    result = subprocess.run(["git", "credential", "fill"], cwd=ROOT,
                            input="protocol=https\nhost=github.com\n\n", text=True,
                            capture_output=True, env={**env, "GIT_TERMINAL_PROMPT": "0"})
    credential = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
    if not credential.get("password"):
        raise RuntimeError("缺少 GitHub 登录凭据，请先运行 gh auth login，或设置 GH_TOKEN。")
    env["GH_TOKEN"] = credential["password"]
    return env


class GitHub:
    def __init__(self, version, env):
        self.version, self.env = version, env

    def call(self, *args):
        return run(["gh", *args], env=self.env)

    def release(self):
        result = subprocess.run(["gh", "api", f"repos/{REPOSITORY}/releases/tags/{self.version}"],
                                env=self.env, text=True, capture_output=True)
        if result.returncode:
            if "(HTTP 404)" in result.stderr:
                # The tag endpoint only resolves published releases. Include drafts
                # so a new or interrupted publication can be resumed safely.
                query = '.[] | select(.tag_name == "' + self.version + '") | @json'
                drafts = self.call("api", f"repos/{REPOSITORY}/releases", "--paginate", "--jq", query)
                matches = [json.loads(line) for line in drafts.splitlines() if line.strip()]
                if len(matches) > 1:
                    raise RuntimeError("同一版本存在多个 Release，请先在 GitHub 整理后重试。")
                return matches[0] if matches else None
            raise RuntimeError(result.stderr.strip())
        return json.loads(result.stdout)

    def command(self, action, *args):
        return self.call("release", action, self.version, "--repo", REPOSITORY, *args)


def verify_remote(release, names, hashes, packages):
    assets = {a["name"]: a for a in release["assets"]}
    for name in names:
        asset = assets.get(name, {})
        if (asset.get("state") != "uploaded" or asset.get("digest") != "sha256:" + hashes[name]
                or asset.get("size") != (packages / name).stat().st_size):
            raise RuntimeError(f"远端文件校验失败：{name}；停止后续发布操作。")


def publish(github, dist, hashes, sync_installers, notes_file):
    packages = dist / "packages"
    before = github.release()
    if before:
        (dist / f"release-before-{github.version}.json").write_text(
            json.dumps(before, ensure_ascii=False, indent=2))
    else:
        github.command("create", "--draft", "--target", "main", "--title", f"Hephaestus Agent {github.version}",
                       "--notes", f"Hephaestus Agent {github.version}，配套同版本主服务。提供五个平台 ZIP 与 SHA256SUMS.txt。")
    assets = {a["name"]: a for a in (before or {"assets": []})["assets"]}
    pending = [n for n in PACKAGES if assets.get(n, {}).get("digest") != "sha256:" + hashes[n]
               or assets.get(n, {}).get("state") != "uploaded"]
    if pending:
        log(f"上传 {len(pending)} 个 ZIP（可能需要数分钟）；相同 SHA256 的附件会跳过。")
        github.command("upload", *[str(packages / n) for n in pending], "--clobber")
    verify_remote(github.release(), PACKAGES, hashes, packages)
    log("五个平台远端哈希一致，上传校验清单。")
    github.command("upload", str(packages / "SHA256SUMS.txt"), "--clobber")
    verify_remote(github.release(), (*PACKAGES, "SHA256SUMS.txt"), hashes, packages)
    commit = sync_installers()
    if notes_file:
        github.command("edit", "--notes-file", str(notes_file))
    if before is None or before["draft"]:
        github.command("edit", "--draft=false", "--target", commit)
    # Remove only known old Agent packages, after new assets and installers are ready.
    for asset in (before or {"assets": []})["assets"]:
        if asset["name"] in LEGACY:
            github.command("delete-asset", asset["name"], "--yes")
    final = github.release()
    verify_remote(final, (*PACKAGES, "SHA256SUMS.txt"), hashes, packages)
    log(f"发布完成：{final['html_url']}")


def main():
    parser = argparse.ArgumentParser(description="全平台构建并发布 Hephaestus-Agent（默认重新编译并推送）。")
    parser.add_argument("--version", help="可选版本校验，必须等于 backend/pkg/version/version.go")
    parser.add_argument("--agent-repo-dir", type=Path, default=ROOT.parent / "Hephaestus-Agent",
                        help="公开仓库本地路径，默认同级 Hephaestus-Agent")
    parser.add_argument("--notes-file", type=Path, help="可选 UTF-8 Release 说明文件；省略则保留已有说明")
    parser.add_argument("--check", action="store_true", help="只校验现有本地产物，不编译、不登录、不推送")
    args = parser.parse_args()
    match = re.search(r'^const Version = "(v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)"$',
                      (ROOT / "backend/pkg/version/version.go").read_text(), re.M)
    if not match:
        raise RuntimeError("无法读取源码公共版本号。")
    version = match[1]
    if (args.version and args.version != version) or os.environ.get("VERSION", version) != version:
        raise RuntimeError(f"发布版本必须与源码公共版本 {version} 一致。")
    dist = ROOT / "dist/agent"
    if args.check:
        validate_artifacts(dist)
        log("本地 ZIP、校验清单和安装器检查通过；未编译或修改远端。此检查不证明产物版本，正式发布仍会重新编译。")
        return
    for tool in ("git", "gh", "go", "docker", "zip", "shasum", "codesign", "otool", "install_name_tool"):
        if not shutil.which(tool):
            raise RuntimeError(f"缺少依赖 {tool}；全平台发布请在配置好 macOS 构建工具链的机器上运行。")
    run(["docker", "info"], capture=True)
    notes = args.notes_file.resolve() if args.notes_file else None
    if notes:
        notes.read_text(encoding="utf-8")
    env = github_environment()
    github = GitHub(version, env)
    repo = args.agent_repo_dir.resolve()

    def git(*command):
        return run(["git", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential",
                    *command], cwd=repo, env=env).strip()

    allowed = {f"https://github.com/{REPOSITORY}", f"https://github.com/{REPOSITORY}.git",
               f"git@github.com:{REPOSITORY}.git", f"ssh://git@github.com/{REPOSITORY}.git"}
    if (git("remote", "get-url", "--push", "origin") not in allowed
            or git("remote", "get-url", "origin") not in allowed):
        raise RuntimeError(f"公开仓库 origin 必须指向 {REPOSITORY}。")
    if git("branch", "--show-current") != "main" or git("status", "--porcelain"):
        raise RuntimeError("公开仓库必须位于 main 且工作区干净；请先处理本地改动。")
    git("fetch", "origin", "main")
    if git("rev-parse", "HEAD") != git("rev-parse", "FETCH_HEAD"):
        raise RuntimeError("公开仓库 main 与远端不同步；请先同步后再发布，脚本不会推送已有的无关提交。")
    initial_head = git("rev-parse", "HEAD")
    # Verify access and reject immutable releases before expensive compilation.
    metadata = json.loads(github.call("api", f"repos/{REPOSITORY}"))
    if not metadata.get("permissions", {}).get("push"):
        raise RuntimeError("当前 GitHub 凭据没有公开仓库的写入权限。")
    release = github.release()
    if release and release.get("immutable"):
        raise RuntimeError("该 Release 已启用不可变发布，不能覆盖；请使用新的源码版本号。")
    log(f"准备发布 {REPOSITORY} {version}，开始全平台构建。")
    run(["bash", str(ROOT / "scripts/build-agent.sh"), "all"], env=env, capture=False)
    hashes = validate_artifacts(dist)

    def sync_installers():
        if git("status", "--porcelain") or git("rev-parse", "HEAD") != initial_head:
            raise RuntimeError("构建期间公开仓库出现改动，停止推送安装器。")
        paths = [f"scripts/{name}" for name in INSTALLERS]
        (repo / "scripts").mkdir(exist_ok=True)
        for name in INSTALLERS:
            shutil.copyfile(dist / "installer-assets" / name, repo / "scripts" / name)
        git("add", "--", *paths)
        if git("diff", "--cached", "--name-only"):
            git("commit", "-m", f"chore(agent): sync installers for {version}")
        git("push", "origin", "HEAD:main")
        return git("rev-parse", "HEAD")

    publish(github, dist, hashes, sync_installers, notes)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, zipfile.BadZipFile, subprocess.CalledProcessError) as error:
        print(f"[publish-agent] ERROR: {error}", file=sys.stderr)
        sys.exit(1)
