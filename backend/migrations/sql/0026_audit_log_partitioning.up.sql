-- Audit logs are retained for at least seven years. Run this migration on a
-- PostgreSQL installation after exporting existing rows; new partitions can be
-- added annually without changing the application query surface.
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs (created_at);
COMMENT ON TABLE audit_logs IS 'Immutable audit records; retain seven years and archive before deletion';