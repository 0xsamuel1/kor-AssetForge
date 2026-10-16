-- Configurable limits for asset fractionalization
CREATE TABLE IF NOT EXISTS fractionalization_configs (
    id                        BIGSERIAL    PRIMARY KEY,
    asset_type                VARCHAR(64)  NOT NULL UNIQUE,
    min_fraction_size         FLOAT        NOT NULL DEFAULT 0.01,
    max_fraction_size         FLOAT        NOT NULL DEFAULT 100.0,
    min_investment_amount     FLOAT        NOT NULL DEFAULT 10.0,
    max_fractional_owners     INT          NOT NULL DEFAULT 1000,
    require_accreditation     BOOLEAN      NOT NULL DEFAULT FALSE,
    min_holding_period_days   INT          NOT NULL DEFAULT 0,
    max_holding_per_owner_percent FLOAT    NOT NULL DEFAULT 25.0,
    enabled                   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by                BIGINT       REFERENCES users(id),
    updated_by                BIGINT       REFERENCES users(id),
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at                TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS asset_fraction_limits (
    id                     BIGSERIAL    PRIMARY KEY,
    asset_id               BIGINT       NOT NULL UNIQUE REFERENCES assets(id) ON DELETE CASCADE,
    min_fraction_size      FLOAT,
    max_fraction_size      FLOAT,
    min_investment         FLOAT,
    max_owners             INT,
    max_per_owner_percent  FLOAT,
    override_global        BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_frac_config_asset_type ON fractionalization_configs(asset_type);
CREATE INDEX IF NOT EXISTS idx_frac_config_enabled    ON fractionalization_configs(enabled);
