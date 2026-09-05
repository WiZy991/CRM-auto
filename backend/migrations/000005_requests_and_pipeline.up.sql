-- ===========================================================================
--  Миграция 5. Заявки клиентов и воронка продаж (п. 3.5 ТЗ).
--
--  Этапы: Лид -> Выявление потребности -> Договор -> Оплата -> Привоз ->
--         Растаможка -> Выдача автомобиля.
--
--  Ключевое решение: текущая стадия лежит в deals.stage, а вся история
--  переходов — в deal_stage_history. Аналитика «сколько дней сделка провела
--  на растаможке» считается из истории, а не из логов приложения.
-- ===========================================================================

CREATE TABLE requests (
    id            uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    public_number  bigint        GENERATED ALWAYS AS IDENTITY,

    client_id     uuid           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Пусто, если клиент оставил заявку «в общий пул» и её ещё никто не взял.
    dealer_id     uuid           REFERENCES users(id) ON DELETE SET NULL,
    car_id        uuid           REFERENCES cars(id) ON DELETE SET NULL,

    status        request_status NOT NULL DEFAULT 'new',

    desired_brand text,
    desired_model text,
    year_from     smallint,
    year_to       smallint,
    origin        origin_country,
    budget_from_rub_minor bigint,
    budget_to_rub_minor   bigint,
    body          body_type,
    gearbox       transmission,

    comment       text           NOT NULL DEFAULT '',
    contact_preference text      NOT NULL DEFAULT 'phone',

    dealer_reply  text,
    replied_at    timestamptz,
    rejected_reason text,

    created_at    timestamptz    NOT NULL DEFAULT now(),
    updated_at    timestamptz    NOT NULL DEFAULT now(),

    CONSTRAINT requests_budget_order CHECK (
        budget_from_rub_minor IS NULL OR budget_to_rub_minor IS NULL
        OR budget_from_rub_minor <= budget_to_rub_minor
    ),
    CONSTRAINT requests_year_order CHECK (
        year_from IS NULL OR year_to IS NULL OR year_from <= year_to
    ),
    CONSTRAINT requests_comment_len CHECK (length(comment) <= 4000)
);

CREATE INDEX requests_client_idx ON requests (client_id, created_at DESC);
CREATE INDEX requests_dealer_status_idx ON requests (dealer_id, status, created_at DESC);
-- Общий пул необработанных заявок — самый частый запрос дилерского кабинета.
CREATE INDEX requests_open_pool_idx
    ON requests (created_at DESC)
    WHERE dealer_id IS NULL AND status = 'new';
CREATE INDEX requests_car_idx ON requests (car_id) WHERE car_id IS NOT NULL;

CREATE TRIGGER requests_set_updated_at
    BEFORE UPDATE ON requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  Сделки.
-- ---------------------------------------------------------------------------
CREATE TABLE deals (
    id            uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    public_number bigint        GENERATED ALWAYS AS IDENTITY,

    request_id    uuid          REFERENCES requests(id) ON DELETE SET NULL,
    client_id     uuid          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    dealer_id     uuid          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    car_id        uuid          REFERENCES cars(id) ON DELETE SET NULL,
    seller_id     uuid          REFERENCES sellers(id) ON DELETE SET NULL,

    stage         deal_stage    NOT NULL DEFAULT 'lead',
    outcome       deal_outcome  NOT NULL DEFAULT 'open',

    title         text          NOT NULL,
    amount_minor  bigint,
    currency      currency_code NOT NULL DEFAULT 'rub',
    amount_rub_minor bigint,
    paid_rub_minor   bigint     NOT NULL DEFAULT 0,

    -- Ключевые даты логистики: без них воронка «Привоз/Растаможка» не имеет
    -- смысла, а клиент не видит прогноза выдачи.
    contract_signed_at timestamptz,
    paid_at            timestamptz,
    shipped_at         timestamptz,
    arrived_at         timestamptz,
    customs_cleared_at timestamptz,
    handed_over_at     timestamptz,
    expected_handover_at date,

    stage_changed_at timestamptz NOT NULL DEFAULT now(),
    lost_reason   text,
    manager_note  text          NOT NULL DEFAULT '',

    closed_at     timestamptz,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    updated_at    timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT deals_title_len CHECK (length(title) BETWEEN 3 AND 200),
    CONSTRAINT deals_amount_positive CHECK (amount_minor IS NULL OR amount_minor > 0),
    CONSTRAINT deals_paid_non_negative CHECK (paid_rub_minor >= 0),
    -- Закрытая сделка обязана иметь исход, открытая — не иметь даты закрытия.
    CONSTRAINT deals_outcome_consistency CHECK (
        (outcome = 'open' AND closed_at IS NULL)
        OR (outcome <> 'open' AND closed_at IS NOT NULL)
    ),
    CONSTRAINT deals_lost_requires_reason CHECK (
        outcome <> 'lost' OR (lost_reason IS NOT NULL AND length(lost_reason) > 0)
    ),
    CONSTRAINT deals_parties_differ CHECK (client_id <> dealer_id)
);

-- Канбан дилера: выборка открытых сделок по стадиям. Частичный индекс
-- исключает архив закрытых сделок, который растёт быстрее всего.
CREATE INDEX deals_dealer_pipeline_idx
    ON deals (dealer_id, stage, stage_changed_at DESC)
    WHERE outcome = 'open';

CREATE INDEX deals_client_idx ON deals (client_id, created_at DESC);
CREATE INDEX deals_dealer_closed_idx ON deals (dealer_id, closed_at DESC) WHERE outcome <> 'open';
CREATE INDEX deals_car_idx ON deals (car_id) WHERE car_id IS NOT NULL;
CREATE INDEX deals_seller_idx ON deals (seller_id) WHERE seller_id IS NOT NULL;
-- Для отчёта «сделки, которые зависли на этапе дольше N дней».
CREATE INDEX deals_stale_idx ON deals (stage_changed_at) WHERE outcome = 'open';

CREATE TRIGGER deals_set_updated_at
    BEFORE UPDATE ON deals
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  История переходов по этапам.
--  duration_seconds заполняется при переходе: сколько сделка провела на
--  предыдущем этапе. Это позволяет считать средний срок этапа одним
--  агрегатом вместо оконных функций по всей таблице.
-- ---------------------------------------------------------------------------
CREATE TABLE deal_stage_history (
    id           bigserial   PRIMARY KEY,
    deal_id      uuid        NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    from_stage   deal_stage,
    to_stage     deal_stage  NOT NULL,
    outcome      deal_outcome NOT NULL DEFAULT 'open',
    changed_by   uuid        REFERENCES users(id) ON DELETE SET NULL,
    comment      text        NOT NULL DEFAULT '',
    duration_seconds integer,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX deal_stage_history_deal_idx ON deal_stage_history (deal_id, created_at);
CREATE INDEX deal_stage_history_stage_idx ON deal_stage_history (to_stage, created_at DESC);

-- ---------------------------------------------------------------------------
--  Задачи по сделке: «позвонить клиенту», «оплатить фрахт», «подать ГТД».
-- ---------------------------------------------------------------------------
CREATE TABLE deal_tasks (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    deal_id     uuid        NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    title       text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    stage       deal_stage,
    assignee_id uuid        REFERENCES users(id) ON DELETE SET NULL,
    created_by  uuid        REFERENCES users(id) ON DELETE SET NULL,
    due_at      timestamptz,
    done_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_tasks_title_len CHECK (length(title) BETWEEN 2 AND 200)
);

-- Индекс под напоминания: только незавершённые задачи со сроком.
CREATE INDEX deal_tasks_due_idx ON deal_tasks (due_at) WHERE done_at IS NULL AND due_at IS NOT NULL;
CREATE INDEX deal_tasks_deal_idx ON deal_tasks (deal_id, done_at NULLS FIRST, due_at);
CREATE INDEX deal_tasks_assignee_idx ON deal_tasks (assignee_id, done_at NULLS FIRST);

-- ---------------------------------------------------------------------------
--  Документы сделки: договор, инвойс, ГТД, акт приёма-передачи.
-- ---------------------------------------------------------------------------
CREATE TABLE deal_documents (
    id          uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    deal_id     uuid          NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    kind        document_kind NOT NULL,
    title       text          NOT NULL,
    file_url    text          NOT NULL,
    mime_type   text          NOT NULL,
    bytes       integer       NOT NULL,
    -- Документы содержат персональные данные, поэтому доступ к ним всегда
    -- проходит через проверку участия в сделке, а не по прямой ссылке.
    uploaded_by uuid          REFERENCES users(id) ON DELETE SET NULL,
    visible_to_client boolean NOT NULL DEFAULT true,
    created_at  timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT deal_documents_bytes_positive CHECK (bytes > 0)
);

CREATE INDEX deal_documents_deal_idx ON deal_documents (deal_id, created_at DESC);
