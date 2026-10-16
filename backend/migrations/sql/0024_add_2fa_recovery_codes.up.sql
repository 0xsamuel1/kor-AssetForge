CREATE TABLE IF NOT EXISTS user_recovery_codes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(255) NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_recovery_codes_user_id ON user_recovery_codes(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_recovery_codes_user_hash ON user_recovery_codes(user_id, code_hash);

ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_codes_generated_at TIMESTAMPTZ;
