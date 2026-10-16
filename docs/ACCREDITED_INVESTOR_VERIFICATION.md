# Accredited Investor Verification Process

This document describes how AssetForge verifies accredited-investor status,
and is the authoritative reference for the behavior implemented in
`backend/services/kyc_provider.go` (`KYCProvider.VerifyAccreditedInvestor`)
and exposed via `POST /api/v1/kyc/accredited`
(`backend/handlers/kyc.go`, `KYCHandler.VerifyAccreditedInvestor`).

See [REQUIRED_DOCUMENTS.md](REQUIRED_DOCUMENTS.md) for what a user or entity
must submit to be verified.

## Why this exists

Certain assets and offerings on the platform are restricted to accredited
investors (enforced on-chain via `contracts/src/whitelist.rs`'s
`AccreditedTier`, and at the API layer by
`User.AccreditedInvestor`/`KYCRecord.AccreditedInvestor`). Before a user can
be marked accredited, their claim must be verified against a recognized
legal standard.

## Verification criteria

The current implementation verifies against the **US SEC net-worth test**
only (Regulation D, Rule 501(a)(5)):

> Net worth exceeding $1,000,000 USD, individually or jointly with a spouse,
> **excluding the value of the investor's primary residence**.

This is the simplest of several qualifying tests the SEC recognizes. The
others -- which this implementation does **not** yet verify -- are listed
below for completeness and because jurisdictions differ (see
"Jurisdictional differences").

| Test | Threshold | Implemented? |
| --- | --- | --- |
| Net worth | > $1,000,000 (excl. primary residence) | Yes |
| Income | > $200,000 individual / $300,000 joint, in each of the prior two years, with a reasonable expectation of the same in the current year | No |
| Professional certification | Series 7, 65, or 82 license holder in good standing | No |
| Knowledgeable employee | Employee of the fund issuing the security, in a role giving access to its investments | No |

**Submitting a net worth figure is a self-attestation, not an audited
proof.** `VerifyAccreditedInvestor` trusts the `net_worth_usd` value it is
given; it does not independently confirm it. Required supporting documents
(bank/brokerage statements, a CPA/attorney letter, or a tax return) must be
collected and reviewed by compliance staff before the attestation is relied
upon for an actual restricted offering -- see REQUIRED_DOCUMENTS.md. The
automated check only enforces the numeric threshold once a net-worth figure
has been accepted.

## Jurisdictional differences

The $1,000,000 / $200k / $300k thresholds above are **US-specific** (17 CFR
§230.501). AssetForge does not currently branch verification logic by the
investor's jurisdiction; every user is checked against the US test
regardless of nationality. This is a known gap:

- **EU** (MiFID II, Annex II): a different three-part test (balance sheet,
  net turnover, own funds), and a distinct "professional client"
  classification that does not map onto the US accredited-investor
  categories at all.
- **UK** (FCA COBS 4.12): "certified high net worth individual" or
  "certified sophisticated investor" self-certification, with its own
  numeric thresholds (£100k net income or £250k net assets, as of the FCA's
  current rules -- verify against the current Handbook before relying on
  this).
- **Other jurisdictions**: not yet researched. **Do not list an offering as
  open to investors outside the US without first confirming the correct
  local qualification test and obtaining legal sign-off** -- this document
  and the current code only cover the US case.

## Review timeline

There is no SLA enforced in code today. `VerifyAccreditedInvestor` responds
synchronously (it is a pure threshold comparison, not an async provider
call), so the *check itself* completes immediately. In practice the
timeline that matters is how quickly a compliance reviewer confirms the
supporting documents before the attestation is trusted for a live
offering -- target **2 business days** from document submission, consistent
with the KYC review timeline the platform otherwise aims for. This is an
operational target, not a contractual guarantee, and is not currently
tracked or alerted on.

## Status lifecycle

1. User submits KYC (`POST /kyc/submit`) and, separately, their self-attested
   net worth (`POST /kyc/accredited`).
2. `VerifyAccreditedInvestor` compares `net_worth_usd` against the threshold
   and flips `User.AccreditedInvestor` accordingly. This happens immediately
   and does not wait for document review.
3. Supporting documents are reviewed by compliance staff out of band (there
   is no dedicated document-upload flow for accreditation specifically today
   -- reuse the KYC document upload at `POST /kyc/documents` with
   `document_type` noting its accreditation purpose until a dedicated
   endpoint exists).
4. A compliance action that revokes `AccreditedInvestor` after a failed
   document review is a manual database update today; there is no
   dedicated "revoke accreditation" endpoint. Track that as a follow-up if
   this becomes a frequent operation.

## Known gaps

- Income-test and professional-certification paths are not implemented.
- No jurisdiction-aware branching.
- No expiry: unlike `KYCRecord.ExpiresAt`, accredited status does not expire
  and is not re-verified periodically. The SEC does not mandate a specific
  re-verification cadence for all transactions, but funds commonly
  re-verify every 90 days for an ongoing offering; this platform does not
  do so automatically.
- Self-attested net worth is not cross-checked against the required
  supporting documents by code -- only by a human reviewer, and only if one
  is actually assigned to look.

None of the above should be read as legal advice. Confirm current
requirements with securities counsel before relying on this document for an
actual offering.
