-- amount en centavos, igual que los importes de auctions.
CREATE TABLE bids (
    id         BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions (id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users (id),
    amount     BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bids_amount_positive CHECK (amount > 0),
    -- Refuerza el invariante del motor de pujas: los importes aceptados de una
    -- subasta son estrictamente crecientes, así que dos pujas no pueden compartir
    -- precio. No es el mecanismo de concurrencia (eso son el mutex por subasta y
    -- SELECT ... FOR UPDATE); es la red que haría fallar ruidosamente un bug.
    CONSTRAINT bids_unique_amount_per_auction UNIQUE (auction_id, amount)
);

-- Sirve al historial (WHERE auction_id = $1 ORDER BY id DESC) y a la búsqueda de
-- la última puja válida que necesitará el cierre automático.
CREATE INDEX bids_auction_id_id_idx ON bids (auction_id, id DESC);
