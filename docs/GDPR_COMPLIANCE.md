# GDPR Compliance

## Data export

Authenticated users can request a JSON export from `POST /api/v1/legal/gdpr/export`.
The export includes the profile, KYC record, balances, listings, transactions,
sessions, recovery-code metadata, consent, export history, audit and compliance
logs, activity, comparisons, referrals, notifications, preferences, and
recommendation history. CSV requests include the same complete payload as
`data.json` inside the ZIP archive.

Exports are encrypted at rest, links expire after seven days, and requests are
processed asynchronously. The service-level target is completion within 30 days.
Access, retention, and disclosure decisions require legal review before a
production deployment.