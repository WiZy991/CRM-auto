#!/bin/bash
# Поднимает Postgres и Redis в Ubuntu WSL под порты из .env проекта.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

apt-get update -qq
apt-get install -y -qq postgresql postgresql-contrib redis-server

PG_VER="$(ls /etc/postgresql | sort -V | tail -n1)"
PG_CONF="/etc/postgresql/${PG_VER}/main/postgresql.conf"
PG_HBA="/etc/postgresql/${PG_VER}/main/pg_hba.conf"

sed -i "s/^#\\?listen_addresses.*/listen_addresses = '*'/" "$PG_CONF"
sed -i "s/^#\\?port = .*/port = 5433/" "$PG_CONF"

if ! grep -q 'host all all 0.0.0.0/0' "$PG_HBA"; then
  printf '\nhost all all 0.0.0.0/0 scram-sha-256\nhost all all ::/0 scram-sha-256\n' >> "$PG_HBA"
fi

service postgresql restart || service postgresql start

sudo -u postgres psql -p 5433 -v ON_ERROR_STOP=1 <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'autoimport_migrator') THEN
    CREATE ROLE autoimport_migrator LOGIN PASSWORD 'dev_only_migrator_password';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'autoimport_app') THEN
    CREATE ROLE autoimport_app LOGIN PASSWORD 'dev_only_pg_password';
  END IF;
END
$$;
SQL

EXISTS=$(sudo -u postgres psql -p 5433 -tAc "SELECT 1 FROM pg_database WHERE datname='autoimport'" | tr -d '[:space:]')
if [ "$EXISTS" != "1" ]; then
  sudo -u postgres psql -p 5433 -v ON_ERROR_STOP=1 -c "CREATE DATABASE autoimport OWNER autoimport_migrator"
fi

sudo -u postgres psql -p 5433 -d autoimport -v ON_ERROR_STOP=1 <<'SQL'
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON DATABASE autoimport FROM PUBLIC;
ALTER SCHEMA public OWNER TO autoimport_migrator;
GRANT CONNECT ON DATABASE autoimport TO autoimport_migrator;
GRANT CONNECT ON DATABASE autoimport TO autoimport_app;
GRANT CREATE ON DATABASE autoimport TO autoimport_migrator;
GRANT USAGE ON SCHEMA public TO autoimport_app;
ALTER DEFAULT PRIVILEGES FOR ROLE autoimport_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO autoimport_app;
ALTER DEFAULT PRIVILEGES FOR ROLE autoimport_migrator IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO autoimport_app;
ALTER DEFAULT PRIVILEGES FOR ROLE autoimport_migrator IN SCHEMA public
  GRANT EXECUTE ON FUNCTIONS TO autoimport_app;
ALTER ROLE autoimport_app SET statement_timeout = '15s';
ALTER ROLE autoimport_app SET idle_in_transaction_session_timeout = '30s';
ALTER ROLE autoimport_app SET lock_timeout = '5s';
ALTER ROLE autoimport_migrator SET statement_timeout = '600s';
SQL

REDIS_CONF=/etc/redis/redis.conf
sed -i 's/^port .*/port 6380/' "$REDIS_CONF"
if grep -q '^# *requirepass ' "$REDIS_CONF"; then
  sed -i 's/^# *requirepass .*/requirepass dev_only_redis_password/' "$REDIS_CONF"
elif grep -q '^requirepass ' "$REDIS_CONF"; then
  sed -i 's/^requirepass .*/requirepass dev_only_redis_password/' "$REDIS_CONF"
else
  echo 'requirepass dev_only_redis_password' >> "$REDIS_CONF"
fi
sed -i 's/^bind .*/bind 0.0.0.0 ::1/' "$REDIS_CONF" || true
sed -i 's/^protected-mode yes/protected-mode no/' "$REDIS_CONF" || true
service redis-server restart || service redis-server start

echo '[ok] postgres :5433  redis :6380'
