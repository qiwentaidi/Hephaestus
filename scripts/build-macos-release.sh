#!/usr/bin/env bash

# Builds only the two macOS desktop release DMGs. The shared release script
# retains responsibility for libpcap cross-compilation, signing, notarization,
# and application bundle creation.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=build-desktop-release.sh
source "${SCRIPT_DIR}/build-desktop-release.sh"

print_macos_outputs() {
  cat <<EOF

macOS build complete:
  Intel (amd64) DMG:
    ${DARWIN_AMD64_DMG}

  Apple Silicon (arm64) DMG:
    ${DARWIN_ARM64_DMG}

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
  if macos_notarization_enabled; then
    require_cmd xcrun
  fi
  require_file "${DMG_README_SOURCE}"
  validate_macos_signing_config

  prepare_dirs
  build_frontend
  generate_icons

  build_darwin_dmg amd64 "${DARWIN_AMD64_DMG}"
  build_darwin_dmg arm64 "${DARWIN_ARM64_DMG}"

  print_macos_outputs
}

main "$@"
