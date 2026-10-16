-- Asset rental and lease agreements
CREATE TABLE IF NOT EXISTS rentals (
    id                BIGSERIAL    PRIMARY KEY,
    asset_id          BIGINT       NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    lessor_id         BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lessee_id         BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period            VARCHAR(20)  NOT NULL DEFAULT 'monthly',
    rate_amount       BIGINT       NOT NULL,
    rate_currency     VARCHAR(10)  NOT NULL DEFAULT 'USD',
    security_deposit  BIGINT       NOT NULL DEFAULT 0,
    start_date        TIMESTAMPTZ  NOT NULL,
    end_date          TIMESTAMPTZ  NOT NULL,
    status            VARCHAR(20)  NOT NULL DEFAULT 'active',
    auto_renew        BOOLEAN      NOT NULL DEFAULT FALSE,
    late_fee_percent  FLOAT        NOT NULL DEFAULT 5.0,
    terms             TEXT,
    signed_by_lessor  BOOLEAN      NOT NULL DEFAULT FALSE,
    signed_by_lessee  BOOLEAN      NOT NULL DEFAULT FALSE,
    contract_hash     TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rentals_asset_id   ON rentals(asset_id);
CREATE INDEX IF NOT EXISTS idx_rentals_lessor_id  ON rentals(lessor_id);
CREATE INDEX IF NOT EXISTS idx_rentals_lessee_id  ON rentals(lessee_id);
CREATE INDEX IF NOT EXISTS idx_rentals_status     ON rentals(status);

CREATE TABLE IF NOT EXISTS rental_payments (
    id                BIGSERIAL    PRIMARY KEY,
    rental_id         BIGINT       NOT NULL REFERENCES rentals(id) ON DELETE CASCADE,
    amount            BIGINT       NOT NULL,
    currency          VARCHAR(10)  NOT NULL DEFAULT 'USD',
    due_date          TIMESTAMPTZ  NOT NULL,
    paid_at           TIMESTAMPTZ,
    status            VARCHAR(20)  NOT NULL DEFAULT 'pending',
    transaction_hash  TEXT,
    late_fee          BIGINT       NOT NULL DEFAULT 0,
    notes             TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rental_payments_rental_id ON rental_payments(rental_id);
CREATE INDEX IF NOT EXISTS idx_rental_payments_status    ON rental_payments(status);

CREATE TABLE IF NOT EXISTS rental_history (
    id         BIGSERIAL    PRIMARY KEY,
    rental_id  BIGINT       NOT NULL REFERENCES rentals(id) ON DELETE CASCADE,
    event      VARCHAR(64)  NOT NULL,
    detail     TEXT,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rental_history_rental_id ON rental_history(rental_id);
