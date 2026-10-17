# Audit Log Retention

Audit records are retained for a minimum of seven years to satisfy regulatory
and financial-control requirements. Daily maintenance compresses old state
payloads. The archival job writes append-only gzip JSONL files to the configured
cold-storage mount, verifies the write, and only then removes archived rows.

Archives are restored by copying the relevant file to a controlled recovery
directory, decompressing and validating each JSON record, and importing it into
a staging table before an approved restore. Access is restricted, encrypted,
logged, and tested during disaster-recovery exercises.