#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export SMOKE_PROJECT="${SMOKE_PROJECT:-shop-smoke}"
export WEB_PORT=3100
export APP_ORIGIN="${HEALTH_BASE_URL:-http://127.0.0.1:3100}"
[[ "$SMOKE_PROJECT" == shop-smoke* ]] || { echo 'Use a shop-smoke-prefixed test project'; exit 1; }
compose() { docker compose --env-file config/demo.env -p "$SMOKE_PROJECT" "$@"; }
trap 'compose down' EXIT
compose up --build -d --wait
npm --prefix web test
