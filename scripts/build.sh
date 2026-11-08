#!/bin/bash

# Hephaestus 多平台构建脚本
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
FRONTEND_DIR="$PROJECT_ROOT/frontend"
BACKEND_DIR="$PROJECT_ROOT/backend"
DIST_DIR="$PROJECT_ROOT/dist"

# 应用名称
APP_NAME="hephaestus"

# 版本号（从 git tag 获取，或使用默认值）
VERSION=${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --always 2>/dev/null || echo "dev")}

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
    printf '%b[INFO]%b %s\n' "$BLUE" "$NC" "$1"
}

log_success() {
    printf '%b[SUCCESS]%b %s\n' "$GREEN" "$NC" "$1"
}

log_warn() {
    printf '%b[WARN]%b %s\n' "$YELLOW" "$NC" "$1"
}

log_error() {
    printf '%b[ERROR]%b %s\n' "$RED" "$NC" "$1"
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        log_error "缺少必要命令: $1"
        exit 1
    fi
}

# 清理构建目录
clean() {
    log_info "清理构建目录..."
    rm -rf "$DIST_DIR"
    rm -rf "$BACKEND_DIR/webassets/dist"
    rm -rf "$FRONTEND_DIR/out"
    rm -rf "$BACKEND_DIR/bin"
    log_success "清理完成"
}

# 构建前端
build_frontend() {
    log_info "构建前端..."
    cd "$FRONTEND_DIR"

    # 安装依赖（如果需要）
    if [ ! -d "node_modules" ]; then
        log_info "安装前端依赖..."
        npm install
    fi

    # 使用静态导出模式构建
    log_info "执行 Next.js 静态导出构建..."
    STATIC_EXPORT=true npm run build

    # 复制构建产物到后端
    log_info "复制前端构建产物到后端..."
    rm -rf "$BACKEND_DIR/webassets/dist"
    mkdir -p "$BACKEND_DIR/webassets"
    cp -r "$FRONTEND_DIR/out" "$BACKEND_DIR/webassets/dist"

    log_success "前端构建完成"
    cd "$PROJECT_ROOT"
}

package_binary() {
    local binary_path=$1
    local package_name=$2
    local package_path="$DIST_DIR/${package_name}.zip"

    log_info "打包 $package_name.zip ..."

    (
        cd "$(dirname "$binary_path")"
        zip -q -j "$package_path" "$(basename "$binary_path")"
    )

    if [ $? -eq 0 ]; then
        local size=$(ls -lh "$package_path" | awk '{print $5}')
        log_success "打包成功: ${package_name}.zip ($size)"
    else
        log_error "打包失败: ${package_name}.zip"
        return 1
    fi
}

# 使用目标架构的 Linux 容器构建 CGO 依赖（包括 libpcap）。
build_linux_platform() {
    local arch=$1
    local output_name="${APP_NAME}-linux-${arch}"
    local output_path="$DIST_DIR/$output_name"
    local export_dir

    require_cmd docker
    if [ ! -f "$BACKEND_DIR/webassets/dist/index.html" ] || [ ! -d "$BACKEND_DIR/webassets/dist/_next" ]; then
        log_error "前端静态产物不完整，请先运行 ./scripts/build.sh frontend"
        return 1
    fi
    export_dir=$(mktemp -d)

    log_info "构建 linux/${arch}（Docker Buildx，复用前端静态产物）..."
    if ! docker buildx build \
        --platform "linux/$arch" \
        --target backend-artifact \
        --build-context "frontend-dist=$BACKEND_DIR/webassets/dist" \
        --build-arg "FRONTEND_SOURCE=frontend-prebuilt" \
        --build-arg "GO_BUILD_PARALLELISM=${GO_BUILD_PARALLELISM:-2}" \
        --build-arg "VERSION=$VERSION" \
        --build-arg "BUILD_TIME=$BUILD_TIME" \
        --build-arg "GOPROXY=${GOPROXY:-https://goproxy.cn|https://proxy.golang.org|direct}" \
        --output "type=local,dest=$export_dir" \
        -f "$PROJECT_ROOT/Dockerfile" \
        "$PROJECT_ROOT"; then
        rm -rf "$export_dir"
        return 1
    fi

    cp "$export_dir/hephaestus-web" "$output_path"
    rm -rf "$export_dir"

    local size=$(ls -lh "$output_path" | awk '{print $5}')
    log_success "构建成功: $output_name ($size)"
    package_binary "$output_path" "$output_name"
}

# 构建单个平台
build_platform() {
    local platform=$1
    local os=$(echo $platform | cut -d'/' -f1)
    local arch=$(echo $platform | cut -d'/' -f2)
    local host_os=$(go env GOOS)
    local cgo_enabled=0

    if [ "$os" = "linux" ]; then
        build_linux_platform "$arch"
        return
    fi

    # gonmap 依赖 gopacket/pcap。除 windows/amd64 外，pcap 需要目标平台的
    # CGO 工具链和 libpcap；不要以 CGO_ENABLED=0 继续编译并产生误导性报错。
    case "$os/$arch" in
        windows/amd64)
            ;;
        darwin/arm64|darwin/amd64)
            if [ "$host_os" != "darwin" ]; then
                log_warn "跳过 $os/$arch：需要 macOS SDK 与 CGO 工具链"
                return 0
            fi
            cgo_enabled=1
            ;;
    esac

    local output_name="${APP_NAME}-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        output_name="${output_name}.exe"
    fi

    local output_path="$DIST_DIR/$output_name"

    log_info "构建 $os/$arch..."

    cd "$BACKEND_DIR"

    CGO_ENABLED=$cgo_enabled GOOS=$os GOARCH=$arch go build \
        -ldflags "-s -w -X main.Version=$VERSION -X 'main.BuildTime=$BUILD_TIME'" \
        -o "$output_path" \
        main.go

    if [ $? -eq 0 ]; then
        local size=$(ls -lh "$output_path" | awk '{print $5}')
        log_success "构建成功: $output_name ($size)"
        package_binary "$output_path" "${APP_NAME}-${os}-${arch}"
    else
        log_error "构建失败: $os/$arch"
        return 1
    fi

    cd "$PROJECT_ROOT"
}

# 构建所有平台
build_all() {
    log_info "开始多平台构建..."
    log_info "版本: $VERSION"
    log_info "构建时间: $BUILD_TIME"
    echo ""

    require_cmd zip

    # 创建输出目录
    mkdir -p "$DIST_DIR"

    # 构建前端
    build_frontend

    echo ""
    log_info "开始编译后端..."

    # 构建所有平台
    for platform in "${PLATFORMS[@]}"; do
        build_platform "$platform"
    done

    echo ""
    log_success "========================================="
    log_success "所有可用构建目标完成！"
    log_success "========================================="
    echo ""
    log_info "构建产物位于: $DIST_DIR"
    ls -lh "$DIST_DIR"
}

# 仅构建后端（跳过前端构建，假设前端已经构建）
build_backend_only() {
    log_info "仅构建后端..."

    require_cmd zip

    if [ ! -d "$BACKEND_DIR/webassets/dist" ]; then
        log_error "前端构建产物不存在，请先运行 ./scripts/build.sh frontend 或 ./scripts/build.sh all"
        exit 1
    fi

    mkdir -p "$DIST_DIR"

    for platform in "${PLATFORMS[@]}"; do
        build_platform "$platform"
    done

    echo ""
    log_success "后端构建完成！"
    ls -lh "$DIST_DIR"
}

# 仅构建前端
build_frontend_only() {
    build_frontend
}

# 构建当前平台（用于开发测试）
build_current() {
    log_info "构建当前平台..."

    require_cmd zip

    # 检测当前平台
    local os=$(go env GOOS)
    local arch=$(go env GOARCH)

    mkdir -p "$DIST_DIR"

    # 构建前端
    build_frontend

    # 构建当前平台
    build_platform "$os/$arch"

    log_success "当前平台构建完成: $os/$arch"
}

# 显示帮助
show_help() {
    echo "Hephaestus 构建脚本"
    echo ""
    echo "用法: ./scripts/build.sh [命令]"
    echo ""
    echo "命令:"
    echo "  all        构建所有平台（默认）"
    echo "  frontend   仅构建前端"
    echo "  backend    仅构建后端（需要先构建前端）"
    echo "  current    仅构建当前平台"
    echo "  clean      清理构建目录"
    echo "  help       显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  VERSION    指定版本号（默认从 git tag 获取）"
    echo "  GO_BUILD_PARALLELISM  Linux Go 编译并发数（默认 2）"
    echo "  GOPROXY    Go 模块代理链（默认 goproxy.cn → proxy.golang.org → direct，错误时自动回退）"
    echo ""
    echo "依赖:"
    echo "  Linux 目标通过 Docker Buildx 构建，需要运行中的 Docker 服务"
    echo ""
    echo "示例:"
    echo "  ./scripts/build.sh all              # 构建所有平台"
    echo "  ./scripts/build.sh current          # 快速构建当前平台用于测试"
    echo "  VERSION=1.0.0 ./scripts/build.sh    # 指定版本号构建"
}

# 主函数
main() {
    local cmd=${1:-all}

    case $cmd in
        all)
            build_all
            ;;
        frontend)
            build_frontend_only
            ;;
        backend)
            build_backend_only
            ;;
        current)
            build_current
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
