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

# 应用名称。产物统一以平台优先命名：Hephaestus-<os>-<arch>-web / -desktop，
# 这样 GitHub Release 页面上同一平台的两种产物会相邻排列。
APP_NAME="Hephaestus"

# 版本号（从 git tag 获取，或使用默认值）
VERSION=${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --match 'v[0-9]*' --always 2>/dev/null || echo "dev")}

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

# CI 与本地都可以只构建所需的平台：--platform os/arch[,os/arch...]
REQUESTED_PLATFORMS=""
DRY_RUN=0
CMD="all"

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

# 记录一个平台到过滤列表
append_platform() {
    local platform=$1
    if [ -z "$REQUESTED_PLATFORMS" ]; then
        REQUESTED_PLATFORMS="$platform"
    else
        REQUESTED_PLATFORMS="$REQUESTED_PLATFORMS,$platform"
    fi
}

validate_platform() {
    local platform=$1
    local allowed

    for allowed in "${PLATFORMS[@]}"; do
        if [ "$allowed" = "$platform" ]; then
            return 0
        fi
    done

    log_error "不支持的平台: ${platform}（可选: ${PLATFORMS[*]}）"
    exit 2
}

# 校验并记录逗号分隔的平台列表
add_platforms() {
    local list=$1
    local platform
    local old_ifs=$IFS

    IFS=','
    for platform in $list; do
        IFS=$old_ifs
        [ -n "$platform" ] || continue
        validate_platform "$platform"
        append_platform "$platform"
    done
    IFS=$old_ifs
}

# 解析 --platform / --dry-run 与子命令
parse_args() {
    local have_command=0

    while [ $# -gt 0 ]; do
        case "$1" in
            --platform)
                if [ $# -lt 2 ]; then
                    log_error "缺少 --platform 的取值"
                    exit 2
                fi
                add_platforms "$2"
                shift 2
                ;;
            --platform=*)
                add_platforms "${1#*=}"
                shift
                ;;
            --dry-run)
                DRY_RUN=1
                shift
                ;;
            *)
                if [ $have_command -eq 1 ]; then
                    log_error "未知参数: $1"
                    exit 2
                fi
                CMD=$1
                have_command=1
                shift
                ;;
        esac
    done
}

# 输出需要构建的平台（每行一个）
platform_list() {
    if [ -z "$REQUESTED_PLATFORMS" ]; then
        printf '%s\n' "${PLATFORMS[@]}"
        return 0
    fi

    local platform
    local old_ifs=$IFS
    IFS=','
    for platform in $REQUESTED_PLATFORMS; do
        printf '%s\n' "$platform"
    done
    IFS=$old_ifs
}

print_plan() {
    log_info "将构建以下平台（VERSION=${VERSION}）:"
    local platform
    while IFS= read -r platform; do
        echo "  - $platform"
    done < <(platform_list)
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
    local output_name="${APP_NAME}-linux-${arch}-web"
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

# macOS 上 gopacket/pcap 通过 #cgo LDFLAGS: -lpcap 链接，链接器默认可能挑到
# Homebrew(/opt/homebrew) 或 MacPorts(/opt/local) 的 libpcap，并把那个绝对路径
# 写进 LC_LOAD_DYLIB。用户机器上没有这些路径时进程会在 dyld 阶段直接起不来，
# 所以强制用 SDK 里的 stub（/usr/lib/libpcap.A.dylib，由 dyld 共享缓存提供）。
verify_macos_libpcap_linkage() {
    local binary=$1
    local linked

    linked="$(otool -L "$binary" | awk '/libpcap\.A\.dylib/ { print $1; exit }')"
    if [ -z "$linked" ]; then
        log_error "产物没有链接 libpcap：$binary"
        return 1
    fi
    if [ "$linked" != "/usr/lib/libpcap.A.dylib" ]; then
        log_error "产物链接了构建机的 libpcap：$linked"
        log_error "用户机器上缺少该路径时会在 dyld 阶段启动失败，请勿发布此产物"
        return 1
    fi
    return 0
}

# 构建单个平台
build_platform() {
    local platform=$1
    local os=$(echo $platform | cut -d'/' -f1)
    local arch=$(echo $platform | cut -d'/' -f2)
    local host_os=$(go env GOOS)
    local cgo_enabled=0
    local cgo_ldflags=""

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
            require_cmd xcrun
            cgo_ldflags="-L$(xcrun --sdk macosx --show-sdk-path)/usr/lib"
            ;;
    esac

    local output_name="${APP_NAME}-${os}-${arch}-web"
    if [ "$os" = "windows" ]; then
        output_name="${output_name}.exe"
    fi

    local output_path="$DIST_DIR/$output_name"

    log_info "构建 $os/$arch..."

    cd "$BACKEND_DIR"

    local build_env=("CGO_ENABLED=$cgo_enabled" "GOOS=$os" "GOARCH=$arch")
    if [ -n "$cgo_ldflags" ]; then
        build_env+=("CGO_LDFLAGS=$cgo_ldflags")
    fi

    env "${build_env[@]}" go build \
        -ldflags "-s -w -X main.Version=$VERSION -X 'main.BuildTime=$BUILD_TIME'" \
        -o "$output_path" \
        main.go

    if [ $? -eq 0 ]; then
        if [ "$os" = "darwin" ] && ! verify_macos_libpcap_linkage "$output_path"; then
            return 1
        fi
        local size=$(ls -lh "$output_path" | awk '{print $5}')
        log_success "构建成功: $output_name ($size)"
        package_binary "$output_path" "${APP_NAME}-${os}-${arch}-web"
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

    # 构建选定的平台
    local platform
    while IFS= read -r platform; do
        build_platform "$platform"
    done < <(platform_list)

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

    local platform
    while IFS= read -r platform; do
        build_platform "$platform"
    done < <(platform_list)

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
    echo "用法: ./scripts/build.sh [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  all        构建所有平台（默认）"
    echo "  frontend   仅构建前端"
    echo "  backend    仅构建后端（需要先构建前端）"
    echo "  current    仅构建当前平台"
    echo "  clean      清理构建目录"
    echo "  help       显示此帮助信息"
    echo ""
    echo "选项:"
    echo "  --platform os/arch   只构建指定平台，可重复或用逗号分隔"
    echo "  --dry-run            只打印将要构建的平台"
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
    echo "  ./scripts/build.sh all                                    # 构建所有平台"
    echo "  ./scripts/build.sh current                                # 快速构建当前平台用于测试"
    echo "  ./scripts/build.sh all --platform darwin/arm64,linux/amd64 # CI 只构建所需平台"
    echo "  ./scripts/build.sh all --dry-run                          # 只打印构建计划"
    echo "  VERSION=v4.0.0 ./scripts/build.sh                         # 指定版本号构建"
}

# 主函数
main() {
    parse_args "$@"

    if [ "$DRY_RUN" = "1" ]; then
        print_plan
        return 0
    fi

    case $CMD in
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
            log_error "未知命令: $CMD"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
