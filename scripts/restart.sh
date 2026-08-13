#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

stop_port() {
  local port="$1"
  local pids
  pids="$(lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "${pids}" ]]; then
    echo "Stopping process on port ${port}: ${pids}"
    kill ${pids}
    sleep 1
  fi
}

case "${1:-all}" in
  backend)
    stop_port 8080
    cd "${project_root}/backend"
    exec go run ./cmd/server
    ;;
  frontend)
    stop_port 5173
    stop_port 5174
    cd "${project_root}/frontend"
    exec npm run dev -- --host 127.0.0.1 --port 5173
    ;;
  all)
    stop_port 8080
    stop_port 5173
    stop_port 5174
    cd "${project_root}/backend"
    go run ./cmd/server &
    backend_pid=$!
    trap 'kill "${backend_pid}" 2>/dev/null || true' EXIT INT TERM
    cd "${project_root}/frontend"
    exec npm run dev -- --host 127.0.0.1 --port 5173
    ;;
  *)
    echo "Usage: $0 [all|backend|frontend]" >&2
    exit 2
    ;;
esac
