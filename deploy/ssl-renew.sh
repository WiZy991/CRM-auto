#!/usr/bin/env bash
# Продление сертификата Let's Encrypt и перезагрузка nginx.
set -euo pipefail
cd "$(dirname "$0")/.."

docker compose --profile certs run --rm certbot renew \
  --webroot \
  -w /var/www/certbot \
  --quiet

docker compose --profile full up -d --force-recreate gateway

echo "OK: сертификат обновлён (если срок подходил), gateway перезапущен"
