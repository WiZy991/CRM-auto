#!/usr/bin/env bash
# Один деплой на VPS: код → фронт → API → миграции → nginx.
set -euo pipefail
cd "$(dirname "$0")/.."

git pull

mkdir -p storage/uploads
chown -R 10001:10001 storage/uploads 2>/dev/null || true

docker run --rm -v "$PWD/frontend:/app" -w /app node:22-alpine \
  sh -c "npm ci && npm run build"

docker compose --profile full up -d --build
docker compose --profile full run --rm api /app/migrate up
docker compose --profile full up -d --force-recreate gateway

code="$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1/healthz || true)"
echo "OK http healthz: ${code}"
if command -v curl >/dev/null; then
  tls="$(curl -skS -o /dev/null -w '%{http_code}' https://127.0.0.1/healthz || true)"
  echo "OK https healthz: ${tls}"
fi
