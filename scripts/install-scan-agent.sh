#!/usr/bin/env bash
# Repository convenience entry; published installers are embedded in the server.
set -e
exec bash "$(cd "$(dirname "$0")/.." && pwd)/backend/pkg/agentrelease/installers/install.sh" "$@"
