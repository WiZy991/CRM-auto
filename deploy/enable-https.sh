#!/usr/bin/env bash
# Первый выпуск Let's Encrypt для боевого домена.
# Требования: DNS A-запись на этот VPS, порты 80/443 открыты наружу.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ ! -f .env ]]; then
  echo "Нет .env — скопируйте .env.example и заполните."
  exit 1
fi

# shellcheck disable=SC1091
set -a
source .env
set +a

DOMAIN="${TLS_DOMAIN:-xn--c1akmgcfiqd.xn--p1ai}"
EMAIL="${CERTBOT_EMAIL:-}"
if [[ -z "${EMAIL}" ]]; then
  echo "Задайте CERTBOT_EMAIL в .env (почта для Let's Encrypt)."
  exit 1
fi

mkdir -p storage/uploads
chown -R 10001:10001 storage/uploads 2>/dev/null || true

echo "==> Поднимаю gateway на :80/:443"
docker compose --profile full up -d gateway

echo "==> Запрос сертификата для ${DOMAIN}"
docker compose --profile certs run --rm certbot certonly \
  --webroot \
  -w /var/www/certbot \
  --email "${EMAIL}" \
  --agree-tos \
  --no-eff-email \
  --non-interactive \
  -d "${DOMAIN}"

echo "==> Подставляю сертификат в nginx"
docker compose --profile full up -d --force-recreate gateway

echo
echo "Готово. В .env выставьте:"
echo "  APP_PUBLIC_URL=https://гоуимпорт.рф"
echo "  CORS_ALLOWED_ORIGINS=https://гоуимпорт.рф,https://${DOMAIN}"
echo "  STORAGE_PUBLIC_BASE_URL=https://гоуимпорт.рф/uploads"
echo "  NGINX_HTTP_PORT=80"
echo "  NGINX_HTTPS_PORT=443"
echo "Затем: docker compose --profile full up -d --force-recreate api worker gateway"
echo "Проверка: curl -fsS https://${DOMAIN}/healthz"
echo "Продление: ./deploy/ssl-renew.sh  (cron раз в месяц)"
