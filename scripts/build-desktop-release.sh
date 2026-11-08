#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DESKTOP_DIR="${PROJECT_ROOT}/backend/cmd/desktop"
BACKEND_DIR="$(cd "${DESKTOP_DIR}/../.." && pwd)"
BUILD_DIR="${DESKTOP_DIR}/build"
BIN_DIR="${DESKTOP_DIR}/bin"
# Final release artifacts are published from dist/desktop; bin/ stays the
# toolchain's intermediate output directory.
DIST_DIR="${HEPHAESTUS_DESKTOP_DIST_DIR:-${PROJECT_ROOT}/dist/desktop}"
STAGING_DIR="${BIN_DIR}/release-staging"
APP_NAME="Hephaestus"
DMG_README_SOURCE="${BUILD_DIR}/darwin/dmg-readme.txt"
DMG_README_NAME="If app is damaged.txt"
WINDOWS_SYSO_DIR="${DESKTOP_DIR}"
LINUX_CROSS_IMAGE_REPO="${LINUX_CROSS_IMAGE_REPO:-wails-linux-cross}"
LINUX_CROSS_IMAGE_VERSION="${LINUX_CROSS_IMAGE_VERSION:-go1.26-pcap-v2}"
LINUX_CROSS_DOCKERFILE="${DESKTOP_DIR}/build/docker/Dockerfile.linux-cross"
LINUX_CROSS_REBUILD="${LINUX_CROSS_REBUILD:-0}"
DOCKER_BUILD_CACHE_PRUNE="${DOCKER_BUILD_CACHE_PRUNE:-1}"
DEBIAN_MIRROR="${DEBIAN_MIRROR:-https://mirrors.ustc.edu.cn/debian}"
DEBIAN_SECURITY_MIRROR="${DEBIAN_SECURITY_MIRROR:-https://mirrors.ustc.edu.cn/debian-security}"
LINUX_GO_BUILD_P="${LINUX_GO_BUILD_P:-1}"
LINUX_GOMAXPROCS="${LINUX_GOMAXPROCS:-${LINUX_GO_BUILD_P}}"

MINGW_AMD64_CC="${MINGW_AMD64_CC:-/opt/homebrew/bin/x86_64-w64-mingw32-gcc}"
MINGW_AMD64_CXX="${MINGW_AMD64_CXX:-/opt/homebrew/bin/x86_64-w64-mingw32-g++}"
MINGW_ARM64_CC="${MINGW_ARM64_CC:-/opt/homebrew/bin/aarch64-w64-mingw32-gcc}"
MINGW_ARM64_CXX="${MINGW_ARM64_CXX:-/opt/homebrew/bin/aarch64-w64-mingw32-g++}"

WINDOWS_AMD64_ZIP="${DIST_DIR}/${APP_NAME}-desktop-windows-amd64.zip"
LINUX_AMD64_ZIP="${DIST_DIR}/${APP_NAME}-desktop-linux-amd64.zip"
LINUX_ARM64_ZIP="${DIST_DIR}/${APP_NAME}-desktop-linux-arm64.zip"
DARWIN_AMD64_DMG="${DIST_DIR}/${APP_NAME}-desktop-darwin-amd64.dmg"
DARWIN_ARM64_DMG="${DIST_DIR}/${APP_NAME}-desktop-darwin-arm64.dmg"

MACOS_SIGN_IDENTITY="${MACOS_SIGN_IDENTITY:-${SIGN_IDENTITY:-}}"
MACOS_NOTARY_KEYCHAIN_PROFILE="${MACOS_NOTARY_KEYCHAIN_PROFILE:-${KEYCHAIN_PROFILE:-}}"
MACOS_NOTARIZE="${MACOS_NOTARIZE:-1}"
# An optional universal libpcap, or per-architecture libraries used by the
# macOS desktop release. The library is bundled into the .app; never ship a
# binary that relies on the build machine's Homebrew prefix.
MACOS_LIBPCAP_PATH="${MACOS_LIBPCAP_PATH:-}"
MACOS_LIBPCAP_AMD64_PATH="${MACOS_LIBPCAP_AMD64_PATH:-}"
MACOS_LIBPCAP_ARM64_PATH="${MACOS_LIBPCAP_ARM64_PATH:-}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "missing required file: $1" >&2
    exit 1
  fi
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

has_file() {
  [[ -f "$1" ]]
}

macos_developer_id_signing_enabled() {
  [[ -n "${MACOS_SIGN_IDENTITY}" ]]
}

macos_notarization_enabled() {
  [[ "${MACOS_NOTARIZE}" != "0" && -n "${MACOS_NOTARY_KEYCHAIN_PROFILE}" ]]
}

validate_macos_signing_config() {
  if macos_notarization_enabled && ! macos_developer_id_signing_enabled; then
    echo "MACOS_NOTARY_KEYCHAIN_PROFILE requires MACOS_SIGN_IDENTITY or SIGN_IDENTITY." >&2
    exit 1
  fi
  if macos_developer_id_signing_enabled && ! macos_notarization_enabled; then
    echo "==> macOS Developer ID signing is enabled without notarization; Gatekeeper may still reject the app."
  fi
  if ! macos_developer_id_signing_enabled; then
    echo "==> macOS Developer ID signing is not configured; DMGs will use ad-hoc signatures."
  fi
}

has_docker_image() {
  docker image inspect "$1" >/dev/null 2>&1
}

docker_platform_for_arch() {
  local arch="$1"
  case "${arch}" in
    amd64)
      echo "linux/amd64"
      ;;
    arm64)
      echo "linux/arm64"
      ;;
    *)
      return 1
      ;;
  esac
}

linux_cross_image_for_arch() {
  local arch="$1"
  echo "${LINUX_CROSS_IMAGE_REPO}:${arch}-${LINUX_CROSS_IMAGE_VERSION}"
}

docker_go_cache_mount() {
  echo "-v ${GOPATH:-$HOME/go}/pkg/mod:/go/pkg/mod"
}

docker_replace_mounts() {
  grep -E '^replace .* => ' "${BACKEND_DIR}/go.mod" 2>/dev/null | while read -r line; do
    local path
    path=$(echo "$line" | sed -E 's/^replace .* => //' | tr -d '\r')
    if [ "${path#/}" = "$path" ]; then
      path="$(cd "${BACKEND_DIR}/$(dirname "$path")" 2>/dev/null && pwd)/$(basename "$path")"
    fi
    if [ -d "$path" ]; then
      echo "-v $path:$path:ro"
    fi
  done | tr '\n' ' '
}

ensure_wails_cross_image() {
  if ! has_cmd docker; then
    echo "Docker is required for this build target, but 'docker' is not installed." >&2
    exit 1
  fi

  if has_docker_image wails-cross; then
    return
  fi

  echo "==> Docker image 'wails-cross' not found, bootstrapping it with wails3"
  (
    cd "${DESKTOP_DIR}"
    wails3 task setup:docker
  )
}

ensure_linux_cross_image() {
  local arch="$1"
  local image
  local platform

  if ! has_cmd docker; then
    echo "Docker is required for Linux builds on macOS, but 'docker' is not installed." >&2
    exit 1
  fi

  image="$(linux_cross_image_for_arch "${arch}")"
  platform="$(docker_platform_for_arch "${arch}")"

  if [[ "${LINUX_CROSS_REBUILD}" != "1" ]] && has_docker_image "${image}"; then
    return
  fi

  if [[ "${LINUX_CROSS_REBUILD}" == "1" ]]; then
    echo "==> Rebuilding Docker image '${image}' for ${platform}"
  else
    echo "==> Docker image '${image}' not found, building Linux-only cross image for ${platform}"
  fi
  docker build \
    --platform "${platform}" \
    --build-arg "DEBIAN_MIRROR=${DEBIAN_MIRROR}" \
    --build-arg "DEBIAN_SECURITY_MIRROR=${DEBIAN_SECURITY_MIRROR}" \
    -t "${image}" \
    -f "${LINUX_CROSS_DOCKERFILE}" \
    "${DESKTOP_DIR}/build/docker"
}

cleanup_docker_build_cache() {
  if [[ "${DOCKER_BUILD_CACHE_PRUNE}" == "0" ]]; then
    echo "==> Skipping Docker build cache cleanup (DOCKER_BUILD_CACHE_PRUNE=0)"
    return
  fi

  if ! has_cmd docker; then
    echo "==> Docker is not installed; skipping Docker build cache cleanup"
    return
  fi

  if ! docker info >/dev/null 2>&1; then
    echo "==> Docker daemon is unavailable; skipping Docker build cache cleanup"
    return
  fi

  echo "==> Cleaning unused Docker build cache"
  docker builder prune --all --force
}

cleanup() {
  rm -rf "${STAGING_DIR}"
}

build_frontend() {
  echo "==> Building desktop frontend assets"
  cd "${DESKTOP_DIR}"
  wails3 task common:build:frontend
}

generate_icons() {
  echo "==> Generating desktop icons"
  cd "${DESKTOP_DIR}"
  wails3 task common:generate:icons
}

prepare_dirs() {
  mkdir -p "${BIN_DIR}" "${DIST_DIR}" "${STAGING_DIR}"
  rm -f \
    "${WINDOWS_AMD64_ZIP}" \
    "${LINUX_AMD64_ZIP}" \
    "${LINUX_ARM64_ZIP}" \
    "${DARWIN_AMD64_DMG}" \
    "${DARWIN_ARM64_DMG}"
}

create_zip_from_file() {
  local source_path="$1"
  local archive_path="$2"
  local entry_name="$3"
  local temp_dir

  temp_dir="$(mktemp -d "${STAGING_DIR}/zip.XXXXXX")"
  cp "${source_path}" "${temp_dir}/${entry_name}"
  (
    cd "${temp_dir}"
    ditto -c -k --sequesterRsrc "${entry_name}" "${archive_path}"
  )
  rm -rf "${temp_dir}"
}

windows_mingw_for_arch() {
  local arch="$1"
  case "${arch}" in
    amd64)
      echo "${MINGW_AMD64_CC}|${MINGW_AMD64_CXX}"
      ;;
    arm64)
      echo "${MINGW_ARM64_CC}|${MINGW_ARM64_CXX}"
      ;;
    *)
      return 1
      ;;
  esac
}

generate_windows_syso() {
  local arch="$1"
  local syso_path="${WINDOWS_SYSO_DIR}/wails_windows_${arch}.syso"

  (
    cd "${DESKTOP_DIR}"
    wails3 task windows:generate:syso ARCH="${arch}"
  ) >&2 || return
  echo "${syso_path}"
}

build_windows_binary_native() {
  local arch="$1"
  local output_binary="$2"
  local mingw_pair
  local cc
  local cxx
  local syso_path

  mingw_pair="$(windows_mingw_for_arch "${arch}")"
  cc="${mingw_pair%%|*}"
  cxx="${mingw_pair##*|}"

  require_file "${cc}"
  require_file "${cxx}"

  syso_path="$(generate_windows_syso "${arch}")"
  (
    cd "${DESKTOP_DIR}"
    GOOS=windows \
    GOARCH="${arch}" \
    CGO_ENABLED=1 \
    CC="${cc}" \
    CXX="${cxx}" \
    CGO_CFLAGS="-w" \
    go build \
      -tags production \
      -trimpath \
      -buildvcs=false \
      -ldflags="-w -s -H windowsgui" \
      -o "${output_binary}" \
      .
  )
  rm -f "${syso_path}"
}

build_windows_binary() {
  local arch="$1"
  local output_binary="$2"

  if has_file "$(windows_mingw_for_arch "${arch}" | cut -d'|' -f1)" && has_file "$(windows_mingw_for_arch "${arch}" | cut -d'|' -f2)"; then
    build_windows_binary_native "${arch}" "${output_binary}"
    return
  fi

  if has_cmd docker; then
    ensure_wails_cross_image
    cd "${DESKTOP_DIR}"
    wails3 task windows:build ARCH="${arch}" CGO_ENABLED=1
    mv "${BIN_DIR}/${APP_NAME}.exe" "${output_binary}"
    return
  fi

  echo "unable to build Windows ${arch}: neither Docker image 'wails-cross' nor local mingw toolchain is available" >&2
  echo "expected one of:" >&2
  echo "  Docker: wails3 task setup:docker" >&2
  echo "  Local toolchain: ${MINGW_AMD64_CC}, ${MINGW_ARM64_CC}" >&2
  exit 1
}

build_windows_zip() {
  local arch="$1"
  local archive_path="$2"
  local output_binary="${STAGING_DIR}/${APP_NAME}-windows-${arch}.exe"

  echo "==> Building Windows ${arch}"
  build_windows_binary "${arch}" "${output_binary}"
  create_zip_from_file "${output_binary}" "${archive_path}" "${APP_NAME}.exe"
}

build_linux_zip() {
  local arch="$1"
  local archive_path="$2"
  local output_binary="${STAGING_DIR}/${APP_NAME}-linux-${arch}"
  local image
  local platform
  local go_cache_mount
  local replace_mounts

  echo "==> Building Linux ${arch}"
  ensure_linux_cross_image "${arch}"
  image="$(linux_cross_image_for_arch "${arch}")"
  platform="$(docker_platform_for_arch "${arch}")"
  go_cache_mount="$(docker_go_cache_mount)"
  replace_mounts="$(docker_replace_mounts)"

  cd "${BACKEND_DIR}"
  docker run --rm \
    --platform "${platform}" \
    -v "${BACKEND_DIR}:/app" \
    ${go_cache_mount} \
    ${replace_mounts} \
    -e APP_NAME="${APP_NAME}" \
    -e GO_BUILD_P="${LINUX_GO_BUILD_P}" \
    -e GOMAXPROCS="${LINUX_GOMAXPROCS}" \
    "${image}" \
    "${arch}"
  mv "${BIN_DIR}/${APP_NAME}-linux-${arch}" "${output_binary}"
  chmod +x "${output_binary}"
  create_zip_from_file "${output_binary}" "${archive_path}" "${APP_NAME}"
}

create_app_bundle() {
  local binary_path="$1"
  local app_bundle_path="$2"
  local libpcap_path="$3"

  rm -rf "${app_bundle_path}"
  mkdir -p "${app_bundle_path}/Contents/MacOS" "${app_bundle_path}/Contents/Resources" "${app_bundle_path}/Contents/Frameworks"
  cp "${BUILD_DIR}/darwin/icons.icns" "${app_bundle_path}/Contents/Resources/"
  if [[ -f "${BUILD_DIR}/darwin/Assets.car" ]]; then
    cp "${BUILD_DIR}/darwin/Assets.car" "${app_bundle_path}/Contents/Resources/"
  fi
  cp "${binary_path}" "${app_bundle_path}/Contents/MacOS/${APP_NAME}"
  bundle_macos_libpcap "${app_bundle_path}" "${libpcap_path}"
  node "${BUILD_DIR}/desktop-version.mjs" plist "${BUILD_DIR}/darwin/Info.plist" "${app_bundle_path}/Contents/Info.plist"
  sign_app_bundle "${app_bundle_path}"
  notarize_app_bundle "${app_bundle_path}"
}

resolve_macos_libpcap() {
  local arch="$1"
  local lipo_arch=""
  local path_variable=""
  local configured_path=""
  local homebrew_prefix=""
  local candidate_path=""
  local can_build_from_source=0

  case "${arch}" in
    amd64)
      lipo_arch="x86_64"
      path_variable="MACOS_LIBPCAP_AMD64_PATH"
      configured_path="${MACOS_LIBPCAP_AMD64_PATH:-${MACOS_LIBPCAP_PATH}}"
      ;;
    arm64)
      lipo_arch="arm64"
      path_variable="MACOS_LIBPCAP_ARM64_PATH"
      configured_path="${MACOS_LIBPCAP_ARM64_PATH:-${MACOS_LIBPCAP_PATH}}"
      ;;
    *)
      echo "Unsupported macOS libpcap architecture: ${arch}" >&2
      exit 1
      ;;
  esac

  if [[ -n "${configured_path}" ]]; then
    candidate_path="${configured_path}"
  elif has_cmd brew; then
    homebrew_prefix="$(brew --prefix libpcap 2>/dev/null || true)"
    if [[ -n "${homebrew_prefix}" ]]; then
      candidate_path="${homebrew_prefix}/lib/libpcap.A.dylib"
    fi
    if [[ "${arch}" == "amd64" && "$(uname -m)" == "arm64" ]]; then
      can_build_from_source=1
    fi
  fi

  if [[ -n "${candidate_path}" && -f "${candidate_path}" ]] && lipo "${candidate_path}" -verify_arch "${lipo_arch}" >/dev/null 2>&1; then
    echo "${candidate_path}"
    return
  fi

  # Homebrew on Apple Silicon only provides arm64 bottles. Build a private
  # x86_64 libpcap from Homebrew's source tarball so the amd64 release remains
  # self-contained instead of inheriting the build host's architecture.
  if [[ -z "${configured_path}" && "${can_build_from_source}" == "1" ]]; then
    build_macos_libpcap_from_source "${arch}" "${lipo_arch}"
    return
  fi

  if [[ -z "${candidate_path}" || ! -f "${candidate_path}" ]]; then
    cat >&2 <<EOF
Unable to find libpcap for macOS ${arch}.
Set ${path_variable} to an ${arch} (or universal) libpcap.A.dylib,
or set MACOS_LIBPCAP_PATH to a universal library before building the release.
EOF
    exit 1
  fi

  echo "libpcap does not contain the required ${arch} slice: ${candidate_path}" >&2
  exit 1
}

build_macos_libpcap_from_source() {
  local arch="$1"
  local lipo_arch="$2"
  local build_root="${STAGING_DIR}/libpcap-${arch}"
  local source_tarball=""
  local source_dir="${build_root}/src"
  local install_dir="${build_root}/install"
  local candidate_path=""
  local sdk_root=""
  local clang_path=""
  local jobs="1"

  require_cmd brew
  require_cmd tar
  require_cmd make
  require_cmd xcrun

  source_tarball="$(brew --cache --build-from-source libpcap 2>/dev/null || true)"
  if [[ ! -f "${source_tarball}" ]]; then
    brew fetch --build-from-source libpcap >&2
    source_tarball="$(brew --cache --build-from-source libpcap)"
  fi
  if [[ ! -f "${source_tarball}" ]]; then
    echo "Unable to obtain the libpcap source tarball from Homebrew." >&2
    exit 1
  fi

  echo "==> Building private macOS ${arch} libpcap from source" >&2
  mkdir -p "${source_dir}"
  tar -xzf "${source_tarball}" -C "${source_dir}" --strip-components=1
  sdk_root="$(xcrun --sdk macosx --show-sdk-path)"
  clang_path="$(xcrun --sdk macosx --find clang)"
  jobs="$(sysctl -n hw.ncpu 2>/dev/null || echo 1)"

  (
    cd "${source_dir}"
    CC="${clang_path} -arch ${lipo_arch} -isysroot ${sdk_root}" \
    CFLAGS="-arch ${lipo_arch} -isysroot ${sdk_root} -mmacosx-version-min=10.15" \
    LDFLAGS="-arch ${lipo_arch} -isysroot ${sdk_root} -mmacosx-version-min=10.15" \
    ./configure --host="${lipo_arch}-apple-darwin" --prefix="${install_dir}" --enable-ipv6 --disable-universal
    make -j"${jobs}"
    make install
  ) >&2

  candidate_path="${install_dir}/lib/libpcap.A.dylib"
  if ! lipo "${candidate_path}" -verify_arch "${lipo_arch}" >/dev/null 2>&1; then
    echo "Generated libpcap does not contain the required ${arch} slice: ${candidate_path}" >&2
    exit 1
  fi

  echo "${candidate_path}"
}

bundle_macos_libpcap() {
  local app_bundle_path="$1"
  local libpcap_path="$2"
  local executable_path="${app_bundle_path}/Contents/MacOS/${APP_NAME}"
  local bundled_libpcap_path="${app_bundle_path}/Contents/Frameworks/libpcap.A.dylib"
  local linked_libpcap_path=""
  local framework_rpath="@executable_path/../Frameworks"

  # Dereference Homebrew's versioned symlink so the release contains the
  # actual dylib rather than a link back into the build machine's prefix.
  cp -L "${libpcap_path}" "${bundled_libpcap_path}"
  chmod u+w "${bundled_libpcap_path}"
  install_name_tool -id "@rpath/libpcap.A.dylib" "${bundled_libpcap_path}"

  linked_libpcap_path="$(otool -L "${executable_path}" | awk '/libpcap(\.A)?\.dylib/ { print $1; exit }')"
  if [[ -z "${linked_libpcap_path}" ]]; then
    echo "Built macOS executable does not link libpcap: ${executable_path}" >&2
    exit 1
  fi

  if [[ "${linked_libpcap_path}" != "@rpath/libpcap.A.dylib" ]]; then
    install_name_tool -change "${linked_libpcap_path}" "@rpath/libpcap.A.dylib" "${executable_path}"
  fi
  if ! otool -l "${executable_path}" | grep -Fq "path ${framework_rpath}"; then
    install_name_tool -add_rpath "${framework_rpath}" "${executable_path}"
  fi
}

sign_macos_code() {
  local path="$1"

  if macos_developer_id_signing_enabled; then
    codesign --force --options runtime --timestamp --sign "${MACOS_SIGN_IDENTITY}" "${path}"
    return
  fi

  codesign --force --sign - "${path}"
}

sign_app_bundle() {
  local app_bundle_path="$1"
  local framework_path="${app_bundle_path}/Contents/Frameworks/libpcap.A.dylib"

  sign_macos_code "${framework_path}"
  sign_macos_code "${app_bundle_path}"

  codesign --verify --deep --strict --verbose=2 "${app_bundle_path}"
}

notarize_app_bundle() {
  local app_bundle_path="$1"
  local notary_zip_path

  if ! macos_notarization_enabled; then
    return
  fi

  notary_zip_path="${STAGING_DIR}/${APP_NAME}-notary.zip"
  rm -f "${notary_zip_path}"
  ditto -c -k --keepParent "${app_bundle_path}" "${notary_zip_path}"
  xcrun notarytool submit "${notary_zip_path}" \
    --keychain-profile "${MACOS_NOTARY_KEYCHAIN_PROFILE}" \
    --wait
  xcrun stapler staple "${app_bundle_path}"
  rm -f "${notary_zip_path}"
}

create_dmg_from_bundle() {
  local app_bundle_path="$1"
  local dmg_path="$2"
  local dmg_stage_dir

  require_file "${DMG_README_SOURCE}"
  dmg_stage_dir="$(mktemp -d "${STAGING_DIR}/dmg.XXXXXX")"
  cp -R "${app_bundle_path}" "${dmg_stage_dir}/"

  if ! create-dmg \
    --volname "${APP_NAME}" \
    --window-pos 200 120 \
    --window-size 900 560 \
    --icon-size 140 \
    --icon "${APP_NAME}.app" 220 260 \
    --hide-extension "${APP_NAME}.app" \
    --app-drop-link 680 260 \
    --add-file "${DMG_README_NAME}" "${DMG_README_SOURCE}" 450 460 \
    "${dmg_path}" \
    "${dmg_stage_dir}"; then
    echo "==> create-dmg could not apply Finder layout; building a functional fallback DMG"
    ln -s /Applications "${dmg_stage_dir}/Applications"
    cp "${DMG_README_SOURCE}" "${dmg_stage_dir}/${DMG_README_NAME}"
    hdiutil create \
      -volname "${APP_NAME}" \
      -srcfolder "${dmg_stage_dir}" \
      -format UDZO \
      -ov \
      "${dmg_path}"
  fi

  sign_dmg "${dmg_path}"
  notarize_dmg "${dmg_path}"

  rm -rf "${dmg_stage_dir}"
}

sign_dmg() {
  local dmg_path="$1"

  if ! macos_developer_id_signing_enabled; then
    return
  fi

  codesign --force --timestamp --sign "${MACOS_SIGN_IDENTITY}" "${dmg_path}"
}

notarize_dmg() {
  local dmg_path="$1"

  if ! macos_notarization_enabled; then
    return
  fi

  xcrun notarytool submit "${dmg_path}" \
    --keychain-profile "${MACOS_NOTARY_KEYCHAIN_PROFILE}" \
    --wait
  xcrun stapler staple "${dmg_path}"
}

build_darwin_dmg() {
  local arch="$1"
  local dmg_path="$2"
  local output_binary="${STAGING_DIR}/${APP_NAME}-darwin-${arch}"
  local app_bundle_path="${STAGING_DIR}/${APP_NAME}.app"
  local libpcap_path

  libpcap_path="$(resolve_macos_libpcap "${arch}")"

  echo "==> Building macOS ${arch}"
  cd "${DESKTOP_DIR}"
  wails3 task darwin:build ARCH="${arch}" OUTPUT="${output_binary}" MACOS_LIBPCAP_DIR="$(dirname "${libpcap_path}")"
  create_app_bundle "${output_binary}" "${app_bundle_path}" "${libpcap_path}"
  create_dmg_from_bundle "${app_bundle_path}" "${dmg_path}"
  rm -rf "${app_bundle_path}"
}

print_outputs() {
  cat <<EOF

Build complete:
  macOS amd64 DMG:
    ${DARWIN_AMD64_DMG}

  macOS arm64 DMG:
    ${DARWIN_ARM64_DMG}

  Windows amd64 ZIP:
    ${WINDOWS_AMD64_ZIP}

  Linux amd64 ZIP:
    ${LINUX_AMD64_ZIP}

  Linux arm64 ZIP:
    ${LINUX_ARM64_ZIP}

Common macOS "app is damaged" repair script:
  ${DESKTOP_DIR}/scripts/macos-damage-fix.sh "/Applications/${APP_NAME}.app"

EOF
}

main() {
  trap cleanup EXIT

  require_cmd go
  require_cmd wails3
  require_cmd ditto
  require_cmd create-dmg
  require_cmd codesign
  require_cmd hdiutil
  require_cmd install_name_tool
  require_cmd lipo
  require_cmd otool
  if [[ "${OSTYPE}" == darwin* ]] && macos_notarization_enabled; then
    require_cmd xcrun
  fi
  require_file "${DMG_README_SOURCE}"
  validate_macos_signing_config

  prepare_dirs
  cleanup_docker_build_cache
  build_frontend
  generate_icons

  build_windows_zip amd64 "${WINDOWS_AMD64_ZIP}"
  build_linux_zip amd64 "${LINUX_AMD64_ZIP}"
  build_linux_zip arm64 "${LINUX_ARM64_ZIP}"
  build_darwin_dmg amd64 "${DARWIN_AMD64_DMG}"
  build_darwin_dmg arm64 "${DARWIN_ARM64_DMG}"

  print_outputs
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
