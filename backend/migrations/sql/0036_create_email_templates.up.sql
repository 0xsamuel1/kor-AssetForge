-- Customizable, multi-language, versioned email templates with A/B testing (#163)
CREATE TABLE IF NOT EXISTS email_templates (
    id           BIGSERIAL    PRIMARY KEY,
    template_key VARCHAR(100) NOT NULL,
    language     VARCHAR(10)  NOT NULL DEFAULT 'en',
    name         VARCHAR(150) NOT NULL,
    description  TEXT,
    subject      TEXT         NOT NULL,
    body_html    TEXT         NOT NULL,
    body_text    TEXT,
    variables    TEXT,
    version      INT          NOT NULL DEFAULT 1,
    is_active    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by   BIGINT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_templates_key_lang ON email_templates(template_key, language);
CREATE INDEX IF NOT EXISTS idx_email_templates_active   ON email_templates(is_active);
CREATE INDEX IF NOT EXISTS idx_email_templates_deleted  ON email_templates(deleted_at);

CREATE TABLE IF NOT EXISTS email_template_versions (
    id          BIGSERIAL   PRIMARY KEY,
    template_id BIGINT      NOT NULL REFERENCES email_templates(id) ON DELETE CASCADE,
    version     INT         NOT NULL,
    subject     TEXT        NOT NULL,
    body_html   TEXT        NOT NULL,
    body_text   TEXT,
    variables   TEXT,
    changed_by  BIGINT,
    change_note TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_template_versions_template ON email_template_versions(template_id);

CREATE TABLE IF NOT EXISTS email_template_variants (
    id          BIGSERIAL   PRIMARY KEY,
    template_id BIGINT      NOT NULL REFERENCES email_templates(id) ON DELETE CASCADE,
    name        VARCHAR(150) NOT NULL,
    subject     TEXT        NOT NULL,
    body_html   TEXT        NOT NULL,
    body_text   TEXT,
    weight      INT         NOT NULL DEFAULT 1,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    sent_count  BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_template_variants_template ON email_template_variants(template_id);
CREATE INDEX IF NOT EXISTS idx_email_template_variants_active   ON email_template_variants(is_active);
