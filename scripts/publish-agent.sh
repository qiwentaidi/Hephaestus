#!/usr/bin/env bash
# Build and publish the public Agent release from the private source checkout.
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 "$PROJECT_ROOT/scripts/publish_agent.py" "$@"
