#!/bin/bash

# Hephaestus Scan Agent 多平台构建脚本
# 支持: windows/amd64、macOS 原生目标，以及通过 Docker Buildx 构建的 Linux 目标

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目路径
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJECT_ROOT/backend"
DIST_DIR="$PROJECT_ROOT/dist/agent"
BIN_DIR="$DIST_DIR/bin"
PKG_DIR="$DIST_DIR/packages"
RUNTIME_DIR="$DIST_DIR/runtime"

# 应用名称
APP_NAME="hephaestus-agent"
# Keep the installed executable name compatible with existing launchers/services.
BINARY_NAME="hephaestus-scan-agent"

# 发布版本必须与二进制实际上报的公共版本常量一致。
SOURCE_VERSION=$(sed -n 's/^const Version = "\([^"]*\)"/\1/p' "$BACKEND_DIR/pkg/version/version.go")
VERSION=${VERSION:-$SOURCE_VERSION}
if [ -z "$SOURCE_VERSION" ] || [ "$VERSION" != "$SOURCE_VERSION" ]; then
    printf 'VERSION must match backend/pkg/version/version.go (%s)\n' "$SOURCE_VERSION" >&2
    exit 1
fi

# 构建时间
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')

# 目标平台
PLATFORMS=(
    "windows/amd64"
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "linux/arm64"
)

# 打印带颜色的消息
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

prepare_dirs() {
    mkdir -p "$DIST_DIR" "$BIN_DIR" "$PKG_DIR" "$RUNTIME_DIR"
    cleanup_legacy_packages
}

cleanup_legacy_packages() {
    rm -f "$PKG_DIR/${BINARY_NAME}_"*.zip \
        "$PKG_DIR/${BINARY_NAME}_"*.tar.gz "$PKG_DIR/${BINARY_NAME}_"*.tgz \
        "$PKG_DIR/${APP_NAME}_"*.zip \
        "$PKG_DIR/${APP_NAME}_"*.tar.gz "$PKG_DIR/${APP_NAME}_"*.tgz

    local package
    local base
    for package in "$PKG_DIR/${APP_NAME}-"*.zip; do
        [ -e "$package" ] || continue
        base="$(basename "$package")"
        case "$base" in
            "${APP_NAME}-windows-amd64.zip"|\
            "${APP_NAME}-darwin-arm64.zip"|\
            "${APP_NAME}-darwin-amd64.zip"|\
            "${APP_NAME}-linux-amd64.zip"|\
            "${APP_NAME}-linux-arm64.zip")
                ;;
            *)
                rm -f "$package"
                ;;
        esac
    done
}

clean() {
    log_info "清理 Agent 构建目录..."
    rm -rf "$DIST_DIR"
    log_success "清理完成: $DIST_DIR"
}

# 使用目标架构的 Linux 容器构建 CGO 依赖（包括 libpcap）。
build_linux_binary() {
    local arch="$1"
    local bin_path="$2"
    local export_dir

    if ! command -v docker >/dev/null 2>&1; then
        log_error "Linux 构建需要 Docker Buildx，请先启动 Docker 服务"
        return 1
    fi
    export_dir=$(mktemp -d)
    if ! docker buildx build \
        --platform "linux/$arch" \
        --target artifact \
        --build-arg "VERSION=$VERSION" \
        --build-arg "BUILD_TIME=$BUILD_TIME" \
        --build-arg "GOPROXY=${GOPROXY:-https://goproxy.cn|https://proxy.golang.org|direct}" \
        --output "type=local,dest=$export_dir" \
        -f "$BACKEND_DIR/cmd/scan-agent/Dockerfile" \
        "$PROJECT_ROOT"; then
        rm -rf "$export_dir"
        return 1
    fi

    cp "$export_dir/$BINARY_NAME" "$bin_path"
    rm -rf "$export_dir"
    chmod +x "$bin_path"
}

build_platform() {
    local platform="$1"
    local os
    local arch
    local cgo_enabled="0"
    os="$(echo "$platform" | cut -d'/' -f1)"
    arch="$(echo "$platform" | cut -d'/' -f2)"

    if [ "$os" = "darwin" ]; then
        cgo_enabled="1"
    fi

    local ext=""
    if [ "$os" = "windows" ]; then
        ext=".exe"
    fi

    local bin_name="${APP_NAME}-${os}-${arch}${ext}"
    local bin_path="$BIN_DIR/$bin_name"
    local package_name="${APP_NAME}-${os}-${arch}.zip"
    local package_path="$PKG_DIR/$package_name"
    local work_dir="$DIST_DIR/.tmp-${os}-${arch}"
    local runtime_dir="$RUNTIME_DIR/${os}-${arch}"

    log_info "构建 $os/$arch..."
    if [ "$os" = "linux" ]; then
        build_linux_binary "$arch" "$bin_path"
    else
        cd "$BACKEND_DIR"
        CGO_ENABLED="$cgo_enabled" GOOS="$os" GOARCH="$arch" go build \
            -trimpath \
            -ldflags "-s -w" \
            -o "$bin_path" \
            ./cmd/scan-agent
        cd "$PROJECT_ROOT"
    fi

    if [ "$os" = "darwin" ]; then
        # CGO may find Homebrew/MacPorts libpcap during cross-architecture
        # linking. macOS already supplies this ABI; ship a system-library
        # dependency instead of an absolute path on the publisher's machine.
        local pcap_dependency
        while IFS= read -r pcap_dependency; do
            [ -n "$pcap_dependency" ] || continue
            if [ "$pcap_dependency" != "/usr/lib/libpcap.A.dylib" ]; then
                install_name_tool -change "$pcap_dependency" /usr/lib/libpcap.A.dylib "$bin_path"
            fi
        done < <(otool -L "$bin_path" | awk '/libpcap.*\.dylib/ {print $1}')
        # Modifying load commands invalidates Go's ad-hoc signature.
        codesign --force --sign - "$bin_path"
        local dependency
        while IFS= read -r dependency; do
            case "$dependency" in
                /opt/*|/usr/local/*)
                    log_error "macOS Agent 仍依赖构建机的第三方动态库: $dependency"
                    return 1
                    ;;
            esac
        done < <(otool -L "$bin_path" | awk 'NR > 1 {print $1}')
    fi

    local size
    size=$(ls -lh "$bin_path" | awk '{print $5}')
    log_success "二进制构建成功: $bin_name ($size)"

    rm -rf "$work_dir"
    mkdir -p "$work_dir"
    cp "$bin_path" "$work_dir/$BINARY_NAME$ext"

    rm -rf "$runtime_dir"
    mkdir -p "$runtime_dir"
    cp -R "$work_dir/." "$runtime_dir/"

    rm -f "$package_path"
    (cd "$work_dir" && zip -X -qr "$package_path" .)
    rm -rf "$work_dir"

    local pkg_size
    pkg_size=$(ls -lh "$package_path" | awk '{print $5}')
    log_success "打包完成: $package_name ($pkg_size)"
}

write_checksums() {
    (cd "$BACKEND_DIR" && go run ./cmd/agent-release --output "$DIST_DIR/installer-assets")
    # Remove obsolete assets left by older builds.
    rm -f "$PKG_DIR/install.sh" "$PKG_DIR/install.bat" "$PKG_DIR/install.ps1" "$PKG_DIR/release.json" "$DIST_DIR/installer-assets/release.json"
    local checksum_file="$PKG_DIR/SHA256SUMS.txt"
    : > "$checksum_file"
    (cd "$PKG_DIR" && shasum -a 256 *.zip > SHA256SUMS.txt)
    log_success "已生成校验文件: $checksum_file"
}

build_all() {
    prepare_dirs
    log_info "开始构建 Scan Agent 多平台产物..."
    log_info "版本: $VERSION"
    log_info "构建时间(UTC): $BUILD_TIME"
    echo ""

    for platform in "${PLATFORMS[@]}"; do
        build_platform "$platform"
    done

    write_checksums

    echo ""
    log_success "========================================="
    log_success "Agent 多平台构建完成"
    log_success "========================================="
    log_info "二进制目录: $BIN_DIR"
    log_info "运行目录: $RUNTIME_DIR"
    log_info "发布包目录: $PKG_DIR"
    ls -lh "$PKG_DIR"
}

build_current() {
    prepare_dirs

    local os
    local arch
    os="$(go env GOOS)"
    arch="$(go env GOARCH)"

    build_platform "$os/$arch"
    write_checksums
    log_success "当前平台构建完成: $os/$arch"
}

build_one_platform() {
    local platform="$1"
    if [[ ! "$platform" =~ ^[^/]+/[^/]+$ ]]; then
        log_error "platform 参数格式错误，应为 os/arch（例如 linux/amd64）"
        exit 1
    fi
    prepare_dirs
    build_platform "$platform"
    write_checksums
}

show_help() {
    echo "Hephaestus Scan Agent 构建脚本"
    echo ""
    echo "用法: ./scripts/build-agent.sh [命令]"
    echo ""
    echo "命令:"
    echo "  all                    构建所有平台（默认）"
    echo "  current                构建当前平台"
    echo "  platform <os/arch>     构建指定平台（例如 linux/amd64）"
    echo "  clean                  清理 Agent 构建目录"
    echo "  help                   显示帮助"
    echo ""
    echo "环境变量:"
    echo "  VERSION                校验发布版本（必须与源码公共版本一致）"
    echo ""
    echo "依赖:"
    echo "  Linux 目标通过 Docker Buildx 构建，需要运行中的 Docker 服务"
    echo ""
    echo "输出目录:"
    echo "  dist/agent/bin         原始二进制"
    echo "  dist/agent/packages    发布包(.zip) + SHA256SUMS.txt"
    echo ""
    echo "示例:"
    echo "  ./scripts/build-agent.sh all"
    echo "  ./scripts/build-agent.sh current"
    echo "  ./scripts/build-agent.sh platform linux/amd64"
    echo "  将源码公共版本更新后再运行 ./scripts/build-agent.sh all"
}

main() {
    local cmd="${1:-all}"

    case "$cmd" in
        all)
            build_all
            ;;
        current)
            build_current
            ;;
        platform)
            if [ -z "$2" ]; then
                log_error "缺少 platform 参数（例如 linux/amd64）"
                exit 1
            fi
            build_one_platform "$2"
            ;;
        clean)
            clean
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $cmd"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
