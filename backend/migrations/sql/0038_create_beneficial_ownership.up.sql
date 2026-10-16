-- Beneficial ownership (UBO) declaration and verification (#379)
CREATE TABLE IF NOT EXISTS beneficial_owners (
    id                    BIGSERIAL     PRIMARY KEY,
    entity_user_id        BIGINT        NOT NULL REFERENCES users(id),
    full_name             VARCHAR(255)  NOT NULL,
    date_of_birth         DATE,
    nationality           VARCHAR(100),
    ownership_percent     NUMERIC(5,2)  NOT NULL DEFAULT 0,
    is_controlling_party  BOOLEAN       NOT NULL DEFAULT false,
    ownership_layer       TEXT,
    document_number_hash  VARCHAR(64),
    status                VARCHAR(20)   NOT NULL DEFAULT 'declared',
    review_notes          TEXT,
    verified_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_beneficial_owners_entity_user_id ON beneficial_owners (entity_user_id);
CREATE INDEX IF NOT EXISTS idx_beneficial_owners_status ON beneficial_owners (status);
CREATE INDEX IF NOT EXISTS idx_beneficial_owners_deleted_at ON beneficial_owners (deleted_at);

CREATE TABLE IF NOT EXISTS corporate_structure_documents (
    id              BIGSERIAL    PRIMARY KEY,
    entity_user_id  BIGINT       NOT NULL REFERENCES users(id),
    document_type   VARCHAR(50)  NOT NULL,
    file_name       VARCHAR(255) NOT NULL,
    file_hash       VARCHAR(64)  NOT NULL,
    storage_path    TEXT         NOT NULL,
    mime_type       VARCHAR(100),
    size_bytes      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_corp_structure_docs_entity_user_id ON corporate_structure_documents (entity_user_id);
