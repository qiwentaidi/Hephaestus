#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/../.." && pwd)"
build_dir="$(mktemp -d)"
trap 'rm -rf "$build_dir"' EXIT
arch="$(docker info --format '{{.Architecture}}')"
case "$arch" in aarch64|arm64) arch=arm64;; x86_64|amd64) arch=amd64;; *) echo "Unsupported Docker architecture: $arch" >&2; exit 1;; esac
cd "$repo_dir/backend"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -mod=readonly -o "$build_dir/hephaestus-egress" ./cmd/egress-gateway
cp "$repo_dir/deploy/egress/Dockerfile" "$build_dir/Dockerfile"
docker build -t "${HEPHAESTUS_EGRESS_IMAGE:-hephaestus-egress:local}" "$build_dir"
