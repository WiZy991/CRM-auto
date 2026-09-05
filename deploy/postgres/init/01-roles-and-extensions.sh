#!/bin/sh
# ---------------------------------------------------------------------------
#  Инициализация базы: расширения и разделение прав.
#
#  Создаются две роли вместо одной, потому что приложение не должно иметь
#  права менять структуру базы. Даже при полной компрометации API-процесса
#  атакующий не сможет удалить таблицу или создать вредоносную функцию.
#
#    autoimport_migrator — владелец схемы, применяет миграции (cmd/migrate)
#    autoimport_app      — только SELECT/INSERT/UPDATE/DELETE (cmd/api)
#
#  Скрипт выполняется один раз при первичной инициализации тома данных.
# ---------------------------------------------------------------------------
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- citext: регистронезависимый email без функциональных индексов
    -- pgcrypto: gen_random_uuid() и хеширование
    -- pg_trgm: быстрый поиск по подстроке в марках и моделях
    CREATE EXTENSION IF NOT EXISTS citext;
    CREATE EXTENSION IF NOT EXISTS pgcrypto;
    CREATE EXTENSION IF NOT EXISTS pg_trgm;

    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${APP_DB_MIGRATOR_USER}') THEN
            CREATE ROLE ${APP_DB_MIGRATOR_USER} LOGIN PASSWORD '${APP_DB_MIGRATOR_PASSWORD}';
        END IF;

        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${APP_DB_USER}') THEN
            CREATE ROLE ${APP_DB_USER} LOGIN PASSWORD '${APP_DB_PASSWORD}';
        END IF;
    END
    \$\$;

    -- Никто, кроме владельца схемы, не может создавать в ней объекты.
    REVOKE CREATE ON SCHEMA public FROM PUBLIC;
    REVOKE ALL ON DATABASE ${POSTGRES_DB} FROM PUBLIC;

    ALTER SCHEMA public OWNER TO ${APP_DB_MIGRATOR_USER};

    GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO ${APP_DB_MIGRATOR_USER};
    GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO ${APP_DB_USER};
    -- Право CREATE на базе нужно migrator, чтобы миграции могли доустановить
    -- доверенные расширения на чистой базе без участия суперпользователя.
    GRANT CREATE ON DATABASE ${POSTGRES_DB} TO ${APP_DB_MIGRATOR_USER};
    GRANT USAGE ON SCHEMA public TO ${APP_DB_USER};

    -- Права на будущие таблицы выдаются автоматически: миграции создают
    -- объекты от имени migrator, приложение сразу получает к ним доступ.
    ALTER DEFAULT PRIVILEGES FOR ROLE ${APP_DB_MIGRATOR_USER} IN SCHEMA public
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${APP_DB_USER};
    ALTER DEFAULT PRIVILEGES FOR ROLE ${APP_DB_MIGRATOR_USER} IN SCHEMA public
        GRANT USAGE, SELECT ON SEQUENCES TO ${APP_DB_USER};
    ALTER DEFAULT PRIVILEGES FOR ROLE ${APP_DB_MIGRATOR_USER} IN SCHEMA public
        GRANT EXECUTE ON FUNCTIONS TO ${APP_DB_USER};

    -- Приложение не должно ждать блокировок и висеть в транзакциях.
    ALTER ROLE ${APP_DB_USER} SET statement_timeout = '15s';
    ALTER ROLE ${APP_DB_USER} SET idle_in_transaction_session_timeout = '30s';
    ALTER ROLE ${APP_DB_USER} SET lock_timeout = '5s';

    -- Миграциям нужен больший лимит: создание индексов на больших таблицах.
    ALTER ROLE ${APP_DB_MIGRATOR_USER} SET statement_timeout = '600s';
EOSQL

echo "[init] Роли ${APP_DB_MIGRATOR_USER} и ${APP_DB_USER} готовы, расширения установлены."
