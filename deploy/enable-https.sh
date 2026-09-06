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
  echo "Задайте CERTBOT_EMAIL в .env — реальный ящик (например goimport@mail.ru)."
  exit 1
fi
if [[ "${EMAIL}" == *"ваш@"* || "${EMAIL}" == *"example.com" || "${EMAIL}" == *"you@"* ]]; then
  echo "CERTBOT_EMAIL сейчас заглушка (${EMAIL}). Укажите настоящий email и повторите."
  exit 1
fi
if [[ "${DOMAIN}" == *"--plai" || "${DOMAIN}" == *"xn--plai" ]]; then
  echo "Опечатка в TLS_DOMAIN=${DOMAIN}"
  echo "Нужно: xn--c1akmgcfiqd.xn--p1ai  (это .рф, буква p и цифра 1, не plai)"
  exit 1
fi
if [[ "${DOMAIN}" != "xn--c1akmgcfiqd.xn--p1ai" && "${DOMAIN}" != *".xn--p1ai" ]]; then
  echo "Проверьте TLS_DOMAIN=${DOMAIN} (ожидается punycode гоуимпорт.рф → xn--c1akmgcfiqd.xn--p1ai)"
fi

mkdir -p storage/uploads
chown -R 10001:10001 storage/uploads 2>/dev/null || true

echo "==> Поднимаю gateway на :80/:443"
docker compose --profile full up -d gateway

echo "==> Запрос сертификата для ${DOMAIN} (email ${EMAIL})"
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
echo "Готово. Замок в браузере должен появиться на https://гоуимпорт.рф"
echo "Проверка: curl -fsS https://${DOMAIN}/healthz"
echo
echo "Cron (вставьте так, не в обычную оболочку):"
echo "  crontab -e"
echo "  # добавьте строку:"
echo "  0 3 1 * * /root/crm/CRM-auto/deploy/ssl-renew.sh >> /var/log/ssl-renew.log 2>&1"
