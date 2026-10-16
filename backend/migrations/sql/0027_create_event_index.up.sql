CREATE TABLE IF NOT EXISTS indexed_events (
    id BIGSERIAL PRIMARY KEY,
    contract_id VARCHAR(56) NOT NULL,
    ledger INT NOT NULL,
    ledger_closed_at TIMESTAMPTZ NOT NULL,
    tx_hash VARCHAR(64) NOT NULL,
    event_id VARCHAR(255) NOT NULL UNIQUE,
    topic TEXT NOT NULL,
    value TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    error_details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS event_checkpoints (
    id BIGSERIAL PRIMARY KEY,
    checkpoint VARCHAR(100) NOT NULL UNIQUE,
    last_ledger INT NOT NULL,
    cursor VARCHAR(255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_indexed_events_contract_id ON indexed_events (contract_id);
CREATE INDEX IF NOT EXISTS idx_indexed_events_ledger ON indexed_events (ledger);
CREATE INDEX IF NOT EXISTS idx_indexed_events_status ON indexed_events (status);
CREATE INDEX IF NOT EXISTS idx_indexed_events_deleted_at ON indexed_events (deleted_at);
