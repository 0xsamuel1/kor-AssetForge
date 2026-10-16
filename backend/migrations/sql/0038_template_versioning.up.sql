-- Email template versioning support (#373)
-- The email_templates and email_template_versions tables were created in
-- migration 0036. This migration adds indexes and constraints to improve
-- versioning query performance and enforce data integrity.

-- Ensure version numbers are unique per template to prevent duplicate snapshots.
ALTER TABLE email_template_versions
    ADD CONSTRAINT uq_email_template_versions_template_version
    UNIQUE (template_id, version);

-- Index to speed up version history lookups ordered by newest-first.
CREATE INDEX IF NOT EXISTS idx_etv_template_version
    ON email_template_versions(template_id, version DESC);

-- Index to support audit queries by the user who made the change.
CREATE INDEX IF NOT EXISTS idx_etv_changed_by
    ON email_template_versions(changed_by)
    WHERE changed_by IS NOT NULL;

-- Ensure the live template's version counter is always positive.
ALTER TABLE email_templates
    ADD CONSTRAINT chk_email_templates_version_positive
    CHECK (version > 0);
