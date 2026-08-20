-- Los importes son enteros en la unidad mínima (centavos): BIGINT, nunca coma flotante.
CREATE TABLE auctions (
    id                BIGSERIAL PRIMARY KEY,
    created_by        BIGINT NOT NULL REFERENCES users (id),
    title             TEXT NOT NULL,
    base_price        BIGINT NOT NULL,
    current_price     BIGINT NOT NULL,
    image_url         TEXT,
    minimum_increment BIGINT NOT NULL,
    start_at          TIMESTAMPTZ NOT NULL,
    end_at            TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL,
    winner_id         BIGINT REFERENCES users (id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT auctions_title_not_blank CHECK (btrim(title) <> ''),
    CONSTRAINT auctions_status_valid CHECK (status IN ('ACTIVE', 'FINISHED')),
    CONSTRAINT auctions_base_price_non_negative CHECK (base_price >= 0),
    -- current_price nunca puede bajar del precio base: es el precio de salida.
    CONSTRAINT auctions_current_price_valid CHECK (current_price >= base_price),
    CONSTRAINT auctions_minimum_increment_positive CHECK (minimum_increment > 0),
    CONSTRAINT auctions_period_valid CHECK (end_at > start_at)
);

CREATE INDEX auctions_created_by_idx ON auctions (created_by);

-- El catálogo se consulta filtrando por estado y ordenando por fecha de creación.
CREATE INDEX auctions_status_created_at_idx ON auctions (status, created_at DESC);

-- El cierre automático del Milestone 5 buscará las subastas ACTIVE ya vencidas.
CREATE INDEX auctions_active_end_at_idx ON auctions (end_at) WHERE status = 'ACTIVE';
