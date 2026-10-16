-- Rollback migration 0038_template_versioning
ALTER TABLE email_templates
    DROP CONSTRAINT IF EXISTS chk_email_templates_version_positive;

DROP INDEX IF EXISTS idx_etv_changed_by;
DROP INDEX IF EXISTS idx_etv_template_version;

ALTER TABLE email_template_versions
    DROP CONSTRAINT IF EXISTS uq_email_template_versions_template_version;
