-- IP whitelist table for controlling access to sensitive admin endpoints
CREATE TABLE IF NOT EXISTS ip_whitelist_entries (
    id          BIGSERIAL    PRIMARY KEY,
    cidr        VARCHAR(50)  NOT NULL UNIQUE,
    description VARCHAR(255),
    created_by  BIGINT       NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ip_whitelist_created_by ON ip_whitelist_entries(created_by);
CREATE INDEX IF NOT EXISTS idx_ip_whitelist_deleted_at ON ip_whitelist_entries(deleted_at);
