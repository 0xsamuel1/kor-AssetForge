# KYC Document Storage Security

This document describes the security controls, access policies, retention rules,
audit logging and compliance posture for KYC (Know Your Customer) document
storage in KOR-AssetForge.

---

## Encryption at Rest

All KYC documents — identity scans, selfies, proof-of-address files — are
encrypted before being written to persistent storage.

| Layer | Mechanism | Notes |
|-------|-----------|-------|
| Application-layer | AES-256-GCM | Key derived per document via `StorageConfig.EncryptionKey` (32-byte). See `backend/config/storage.go`. |
| Field-level (DB) | SHA-256 hash only | The raw `DocumentNumber` is never stored; only its hash (`document_number_hash`) is persisted. |
| Infrastructure | Disk/volume encryption | Must be enabled on the host (e.g. AWS EBS encryption, GCP CMEK). Verified at deployment time. |

### Key Management

- The `DOCUMENT_ENCRYPTION_KEY` environment variable (32 hex bytes) seeds
  AES-256-GCM encryption for all new uploads.
- Keys must be rotated at least annually. After rotation, re-encrypt existing
  documents using the `document_storage` service's `ReEncrypt` helper.
- Never log or expose key values. Reference keys by environment variable name
  only.

---

## Access Control

### Storage Paths

Encrypted documents are written to a path of the form:

```
kyc/{userID}/{kycRecordID}/{documentType}/{uuid}.enc
```

The `.enc` extension signals that the file must not be served raw. Direct
public access to the storage bucket/directory must be **denied** at the
infrastructure level (no public-read ACL).

### Application-Level Checks

| Operation | Required Role / Condition |
|-----------|--------------------------|
| Upload document | Authenticated user, must be the document owner (`KYCRecord.UserID == requestUserID`) |
| Read / download document | Admin role OR document owner |
| Delete document | Admin role only |
| List all documents (compliance view) | Admin / Compliance Officer role |

Route-level enforcement is applied in `backend/handlers/kyc.go` via the
`authMiddleware.RequireRole` and user-ownership checks.

### Database Column Security

- `KYCDocument.StoragePath` is tagged `json:"-"` — it is never serialised in
  API responses.
- `KYCRecord.DocumentNumberHash` is tagged `json:"-"` — the raw document number
  never leaves the server.

---

## Retention Policies

| Document Category | Retention Period | Basis |
|-------------------|-----------------|-------|
| Identity documents (passport, driver licence) | 5 years after account closure | AML/CFT regulations, FATF Recommendation 11 |
| Proof of address | 5 years after account closure | Same as above |
| AML screening records | 7 years | EU 4th / 5th AML Directive, FinCEN |
| Compliance audit logs | 7 years (immutable) | SOX, GDPR Art. 30 |
| Rejected / expired KYC records | 2 years | Regulatory minimum + fraud prevention |

Documents past their retention deadline must be securely deleted (overwrite +
unlink). The `document_storage` service exposes a `PurgeExpired` method that
the platform's scheduled job invokes weekly.

---

## Audit Logging for Document Access

Every interaction with KYC documents is appended to `compliance_audit_logs`
(model: `models.ComplianceAuditLog`). The table is append-only — rows are never
updated or soft-deleted.

Logged events include:

| Action | Entity | Details recorded |
|--------|--------|-----------------|
| `document_uploaded` | `KYCDocument` | file hash, mime type, size, uploader IP |
| `document_accessed` | `KYCDocument` | accessor user ID, IP address, timestamp |
| `document_deleted` | `KYCDocument` | deleting admin ID, reason |
| `kyc_submitted` | `KYCRecord` | provider record ID, status |
| `kyc_status_changed` | `KYCRecord` | before/after status, changed by |
| `aml_screened` | `AMLScreening` | screening ID, risk level, match count |
| `document_re_encrypted` | `KYCDocument` | old key version, new key version |

Audit logs are replicated to an append-only cold-storage bucket nightly and are
excluded from GDPR erasure requests per Art. 17(3)(b) (legal obligation).

---

## Compliance Certifications and Standards

The platform is designed to align with the following frameworks. Formal
third-party audits should be conducted before going live with real user data.

| Standard | Applicability | Status |
|----------|--------------|--------|
| GDPR (EU 2016/679) | EU user data; right to access, portability, erasure | Architecture compliant; DPA required before launch |
| AML/CFT (FATF Recommendations) | Anti-money-laundering; CDD/EDD requirements | Document collection + AML screening implemented |
| EU 5th AML Directive (AMLD5) | Enhanced due diligence for high-risk customers | Risk scoring + accredited investor checks in place |
| SOC 2 Type II | Availability, confidentiality, security | Controls documented; formal audit pending |
| ISO 27001 | Information security management | Gap analysis recommended |

---

## Data Flow Diagram

```
Client
  │
  │  POST /api/v1/kyc/documents  (HTTPS/TLS 1.3)
  ▼
KYC Handler (kyc.go)
  │  1. Auth check + ownership validation
  │  2. Validate request (mime type, size, hash)
  ▼
Document Storage Service (document_storage.go)
  │  3. Generate random AES-256-GCM nonce
  │  4. Encrypt document bytes
  │  5. Write <uuid>.enc to StoragePath
  ▼
KYCDocument record (DB)
  │  6. Persist metadata (file hash, storage path, status)
  ▼
ComplianceAuditLog (DB)
  │  7. Append document_uploaded event
  ▼
Response → { id, document_type, file_name, status }
           (StoragePath and raw bytes never returned)
```

---

## Incident Response

If a storage breach is suspected:

1. Rotate `DOCUMENT_ENCRYPTION_KEY` immediately and re-encrypt all documents.
2. Revoke all storage access credentials.
3. Notify affected users within 72 hours (GDPR Art. 33).
4. Contact `security@kor-assetforge.example` and open a P0 incident.
5. Preserve audit logs — do not delete any `compliance_audit_logs` rows.
