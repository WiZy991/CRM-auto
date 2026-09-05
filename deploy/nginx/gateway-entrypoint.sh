#!/bin/sh
# Готовит TLS-файлы для nginx и запускает шлюз.
# Если есть сертификат Let's Encrypt — копирует его и включает редирект HTTP→HTTPS.
# Иначе — self-signed на :443, а на :80 сайт продолжает открываться как раньше.
set -eu

DOMAIN="${TLS_DOMAIN:-xn--c1akmgcfiqd.xn--p1ai}"
LE_DIR="/etc/letsencrypt/live/${DOMAIN}"
OUT_DIR="/etc/nginx/certs"
GEN_DIR="/etc/nginx/generated"

mkdir -p "${OUT_DIR}" "${GEN_DIR}"

if [ -f "${LE_DIR}/fullchain.pem" ] && [ -f "${LE_DIR}/privkey.pem" ]; then
  cp -L "${LE_DIR}/fullchain.pem" "${OUT_DIR}/fullchain.pem"
  cp -L "${LE_DIR}/privkey.pem" "${OUT_DIR}/privkey.pem"
  cat > "${GEN_DIR}/http-tail.inc" <<'EOF'
# Боевой сертификат есть — весь HTTP уводим на HTTPS (кроме ACME выше).
location = /healthz {
    access_log off;
    proxy_pass http://api_backend;
    include /etc/nginx/conf.d/proxy-common.inc;
}
location / {
    return 301 https://$host$request_uri;
}
EOF
  cat > "${GEN_DIR}/hsts.inc" <<'EOF'
add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;
EOF
  echo "gateway: Let's Encrypt (${DOMAIN}), HTTP→HTTPS redirect on"
else
  if ! command -v openssl >/dev/null 2>&1; then
    apk add --no-cache openssl >/dev/null
  fi
  openssl req -x509 -nodes -newkey rsa:2048 -days 7 \
    -keyout "${OUT_DIR}/privkey.pem" \
    -out "${OUT_DIR}/fullchain.pem" \
    -subj "/CN=${DOMAIN}" >/dev/null 2>&1
  cat > "${GEN_DIR}/http-tail.inc" <<'EOF'
# До выпуска LE отдаём приложение по HTTP, чтобы сайт не «ломался».
include /etc/nginx/conf.d/app-locations.inc;
EOF
  : > "${GEN_DIR}/hsts.inc"
  echo "gateway: temporary self-signed (${DOMAIN}) — HTTP still serves the app"
  echo "gateway: run deploy/enable-https.sh for a real certificate"
fi

exec nginx -g "daemon off;"
