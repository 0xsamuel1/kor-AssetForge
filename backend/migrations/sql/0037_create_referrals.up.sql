-- User referral program: codes, referral links, tiered rewards (#170)
CREATE TABLE IF NOT EXISTS referral_codes (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       NOT NULL UNIQUE REFERENCES users(id),
    code       VARCHAR(32)  NOT NULL UNIQUE,
    uses       INT          NOT NULL DEFAULT 0,
    max_uses   INT          NOT NULL DEFAULT 0,
    active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_referral_codes_deleted ON referral_codes(deleted_at);

CREATE TABLE IF NOT EXISTS referrals (
    id                BIGSERIAL    PRIMARY KEY,
    referrer_id       BIGINT       NOT NULL REFERENCES users(id),
    referee_id        BIGINT       NOT NULL UNIQUE REFERENCES users(id),
    code              VARCHAR(32)  NOT NULL,
    status            VARCHAR(20)  NOT NULL DEFAULT 'pending',
    tier              INT          NOT NULL DEFAULT 1,
    reward_amount_usd BIGINT       NOT NULL DEFAULT 0,
    signup_ip         VARCHAR(64),
    qualified_at      TIMESTAMPTZ,
    rewarded_at       TIMESTAMPTZ,
    reject_reason     TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_referrals_referrer ON referrals(referrer_id);
CREATE INDEX IF NOT EXISTS idx_referrals_code     ON referrals(code);
CREATE INDEX IF NOT EXISTS idx_referrals_status   ON referrals(status);
CREATE INDEX IF NOT EXISTS idx_referrals_deleted  ON referrals(deleted_at);

CREATE TABLE IF NOT EXISTS referral_rewards (
    id          BIGSERIAL    PRIMARY KEY,
    referral_id BIGINT       NOT NULL REFERENCES referrals(id) ON DELETE CASCADE,
    user_id     BIGINT       NOT NULL REFERENCES users(id),
    amount_usd  BIGINT       NOT NULL,
    type        VARCHAR(30)  NOT NULL,
    status      VARCHAR(20)  NOT NULL DEFAULT 'credited',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referral_rewards_referral ON referral_rewards(referral_id);
CREATE INDEX IF NOT EXISTS idx_referral_rewards_user     ON referral_rewards(user_id);
