# Data Anonymization Policy

Approved erasure requests are processed by the anonymization job. Direct
identifiers are replaced with deterministic, non-reversible values derived from
the internal user ID; credentials and tokens are cleared. IP addresses and user
agents are removed, and KYC identity fields are replaced or cleared.

Numeric IDs and non-identifying financial and audit facts remain so foreign-key
relationships, accounting, fraud investigations, and statutory records remain
consistent. The operation is idempotent and must be approved by the privacy
owner and legal counsel before scheduling.