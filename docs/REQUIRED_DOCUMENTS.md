# Required Documents

What must be submitted for identity verification (KYC), AML screening, and
accredited-investor verification, and where each document lands in the data
model. See [ACCREDITED_INVESTOR_VERIFICATION.md](ACCREDITED_INVESTOR_VERIFICATION.md)
for the verification logic these documents feed into.

## Identity verification (KYC)

Submitted via `POST /api/v1/kyc/documents` against an existing
`KYCRecord` (`backend/models/kyc.go`). `document_type` is one of:

| `document_type` | Required? | Notes |
| --- | --- | --- |
| `front` | Yes | Front of a government-issued photo ID. |
| `back` | Conditionally | Required for `driver_license` and `national_id`; most passports have no back page of ID data. |
| `selfie` | Yes | Live selfie for liveness/face-match against the ID photo. |
| `proof_of_address` | Yes | Utility bill, bank statement, or government letter dated within the last 90 days. |

Accepted `document_type` values for the identity document itself
(`submitKYCRequest.document_type` on `POST /kyc/submit`): `passport`,
`driver_license`, `national_id`.

The document number itself is never stored -- only its SHA-256 hash
(`KYCRecord.DocumentNumberHash`, `services.HashDocumentNumber`). File bytes
are never stored in this repository's database either; `UploadDocument`
only records metadata (hash, size, mime type, and a storage path) for an
out-of-band encrypted object store.

## Accredited investor verification

See the full criteria in ACCREDITED_INVESTOR_VERIFICATION.md. Today's
implementation only verifies the US net-worth test, so only the first row
below is enforced by code; the rest are listed because compliance staff
still need them to actually confirm a self-attested figure before relying
on it for a live offering.

| Required document | Purpose |
| --- | --- |
| Bank and/or brokerage statements (most recent) covering all accounts counted toward net worth | Substantiates the `net_worth_usd` figure submitted to `POST /kyc/accredited` |
| Most recent mortgage statement or deed, if a primary residence is owned | Needed to confirm it was correctly excluded from the net-worth figure |
| Letter from a CPA, attorney, investment adviser, or broker-dealer confirming accredited status | Accepted as an alternative to raw statements, per Rule 501(a) |
| Two most recent years of tax returns (W-2s/1099s) | Only needed if verifying via the income test (not yet automated -- see known gaps) |
| Professional license number (Series 7/65/82) | Only needed if verifying via the professional-certification test (not yet automated) |

## AML screening

AML screening (`backend/services/aml_screening.go`) requires no separate
document upload from the user -- it runs against the `full_name`,
`nationality`, and `date_of_birth` already captured on the `KYCRecord`. If a
screening comes back flagged and is escalated to manual review
(`POST /kyc/aml/review/:id`), the reviewer may request additional
documentation (e.g. an updated proof of address, or evidence the matched
watchlist entry refers to a different person) out of band; there is no
dedicated upload endpoint for that today -- reuse `POST /kyc/documents`.

## Beneficial ownership (UBO)

For a corporate/organizational account, each declared owner
(`POST /api/v1/compliance/ubo`, `backend/models/beneficial_owner.go`) above
the 25% ownership threshold, or with control regardless of equity, needs
supporting documentation of the ownership chain, uploaded via
`POST /api/v1/compliance/ubo/documents`:

| `document_type` | Purpose |
| --- | --- |
| `cap_table` | Current capitalization table showing each owner's stake. |
| `trust_deed` | If any layer of ownership runs through a trust. |
| `articles_of_incorporation` | Confirms the entity's legal structure and registered owners/officers. |
| `org_chart` | Visualizes a multi-layer ownership chain (holding companies, subsidiaries) when it is not obvious from the cap table alone. |

## General notes

- All document metadata records (`KYCDocument`, `CorporateStructureDocument`)
  are soft-deleted (`DeletedAt`), never hard-deleted, to preserve the audit
  trail required for compliance review.
- None of the file-hash fields are a substitute for actually verifying the
  document's contents; they only prove a specific uploaded file was not
  silently swapped after submission.
