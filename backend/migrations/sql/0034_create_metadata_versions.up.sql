-- Metadata version history for assets
CREATE TABLE IF NOT EXISTS metadata_versions (
    id            BIGSERIAL    PRIMARY KEY,
    asset_id      BIGINT       NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    version       INT          NOT NULL,
    metadata_uri  VARCHAR(512),
    metadata_hash VARCHAR(64),
    metadata      TEXT,
    changed_by    BIGINT       NOT NULL REFERENCES users(id),
    change_note   VARCHAR(500),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (asset_id, version)
);

CREATE INDEX IF NOT EXISTS idx_metadata_versions_asset_id ON metadata_versions(asset_id);
CREATE INDEX IF NOT EXISTS idx_metadata_versions_changed_by ON metadata_versions(changed_by);
