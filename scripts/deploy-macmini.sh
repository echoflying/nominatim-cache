#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_DIR="$ROOT_DIR/server"
PM2_BIN="${HOME}/.npm-global/bin/pm2"

export PATH="/opt/local/bin:/opt/homebrew/bin:/usr/local/bin:${HOME}/.npm-global/bin:/usr/bin:/bin:/usr/sbin:/sbin"

if [[ ! -f "$ROOT_DIR/.env" ]]; then
  echo "[deploy] Missing $ROOT_DIR/.env"
  exit 1
fi

if [[ ! -x "$PM2_BIN" ]]; then
  echo "[deploy] Missing PM2 binary at $PM2_BIN"
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "[deploy] Missing node in PATH: $PATH"
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "[deploy] Missing npm in PATH: $PATH"
  exit 1
fi

cd "$ROOT_DIR"
git pull

cd "$SERVER_DIR"
npm install
npm run build

NOMINATIM_CACHE_SERVER_CWD="$SERVER_DIR" "$PM2_BIN" start ecosystem.config.cjs --update-env
"$PM2_BIN" save

echo "[deploy] Health check"
curl -s http://127.0.0.1:3000/health
echo
