# Информационная безопасность

Платформа защищается эшелонированно. Приложение не заменяет защиту сети
хостера: объёмный L3/L4-флуд отражается на периметре (Cloudflare или anti-DDoS
провайдера). Здесь зафиксировано то, что делает сам код.

## Рубежи

1. **Сеть.** В бою перед API стоит nginx (`deploy/nginx`) с `limit_req`,
   `limit_conn`, таймаутами против slowloris и лимитом тела запроса.
2. **Шлюз Go.** Реальный IP берётся из `X-Forwarded-For` только для сетей из
   `HTTP_TRUSTED_PROXIES`. Дальше: WAF-lite, Redis token-bucket, автобан IP,
   CSP и security-заголовки, CSRF double-submit на изменяющих запросах.
3. **Приложение.** RBAC в маршрутизаторе и повторная проверка владения в SQL.
   Access-токен живёт в памяти вкладки, refresh — HttpOnly cookie с ротацией
   и детектом повторного использования. Пароли — argon2id. PII шифруется
   AES-256-GCM ключом из конфигурации.
4. **База.** Роль приложения без DDL. `statement_timeout`, отдельная роль
   мигратора. Бэкапы — `scripts/backup-wsl.cmd` (`pg_dump` + gzip, опционально
   `openssl enc`).

## Загрузки

Эндпоинт `/api/v1/uploads/images` принимает только аутентифицированного
подтверждённого пользователя, ограничен по частоте и размеру. Перед записью
на диск проверяются magic bytes (JPEG/PNG); расширение из имени файла не
доверяется. Файл полностью декодируется и перекодируется в JPEG: EXIF,
полиглот и «картинка с приписанным PHP» на диск не попадают. Путь в
хранилище генерируется сервером.

## Персональные данные

Паспорт и адрес шифруются AES-256-GCM до записи в `passport_encrypted` /
`address_encrypted`. Расшифровка только в `GET /auth/me` владельца сессии.
В логах ключи `passport`, `password`, `token` маскируются, `email` / `phone` /
`address` усекаются. Дамп Postgres без ключа `PII_ENCRYPTION_KEY` эти поля
не раскрывает.

## IDOR

Чужой UUID сделки, заявки, задачи или документа даёт **404**, не 403: иначе
перебор подтверждал бы существование объекта. Проверка участия стоит в SQL
(`deals.client_id` / `deals.dealer_id`, `INSERT … SELECT WHERE EXISTS`).
Клиент участник видит сделку, но не меняет этапы (это уже 403 на manage).

## Угрозы, которые закрывает код

| Угроза | Мера |
| --- | --- |
| Кража сессии с XSS | Access не в localStorage; cookie HttpOnly |
| Replay refresh | Ротация + запись reused → отзыв всех сессий |
| CSRF | SameSite=Strict + заголовок `X-CSRF-Token` |
| IDOR | Фильтр `client_id` / `dealer_id` в SQL, не только в handler |
| Брутфорс входа | Лимит по IP и логину, прогрессивная блокировка, капча |
| Сканеры и path traversal | WAF-lite до разбора тела |
| Подделка IP | Trusted proxies allowlist |
| Вредоносная загрузка | Magic bytes + перекодирование JPEG |
| Утечка PII из бэкапа | AES-GCM at rest, ключ вне базы |
| Утечка PII из логов | redact/mask в slog |

## Что приложение не закрывает

- Объёмный L3/L4-флуд и исчерпание канала — это периметр (Cloudflare /
  anti-DDoS хостера). Token-bucket в Redis режет HTTP-запросы, не пакеты.
- Компрометация ключей в `.env` и доступ к диску с загрузками.
- Физический доступ к серверу Postgres / Redis.
- Социальная инженерия и фишинг пароля пользователя.

## TLS (HTTPS)

На VPS сертификат Let's Encrypt выпускается скриптом `deploy/enable-https.sh`
(нужны DNS на сервер, `CERTBOT_EMAIL`, порты 80/443). Продление —
`deploy/ssl-renew.sh` (cron раз в месяц). После выпуска в `.env` должны быть
`https://` в `APP_PUBLIC_URL`, `CORS_ALLOWED_ORIGINS` и `STORAGE_PUBLIC_BASE_URL`.

## Что проверять перед продом

- `make check` (`gofmt`, `go vet`, `go test`, `govulncheck`, `npm audit`)
- Тесты IDOR: чужая сделка → 404, не 403
- HTTPS: замок в браузере, `curl -fsS https://домен/healthz`
- Cloudflare (или аналог) включён, origin принимает только его адреса
- `APP_ENV=production`, `RATELIMIT_ENABLED=true`, SMTP включён
