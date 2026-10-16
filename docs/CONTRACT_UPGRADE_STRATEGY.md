# Contract Upgrade Strategy

Implements #342 — "No Smart Contract Upgrade Strategy".

## Why

Without an upgrade path, a bug found in a deployed contract can only be fixed by
migrating every balance, position and approval to a brand new contract. On a
tokenised asset platform that means: every holder has to be notified, every
listing has to be re-created, and the migration itself is a multi-day operation
during which the protocol is frozen or partially broken.

The proxy pattern removes the migration. Users keep talking to the same address
forever; only the implementation it points at changes.

## Architecture

```
user / backend / clients
        │  (always the same address)
        ▼
  ┌───────────────┐
  │  Proxy        │  admin, implementation, version, history,
  │  (immutable)  │  timelock, governance approver
  └───────┬───────┘
          │ points at
          ▼
  ┌───────────────┐
  │ Implementation│  all business logic and all user state
  │  (swap-able)  │
  └───────────────┘
```

The proxy holds no user state, so a swap is instantaneous and cannot corrupt
balances. Implementation storage is untouched by the upgrade.

Two complementary mechanisms exist in the codebase:

| Mechanism | Location | Use it for |
| --- | --- | --- |
| **Proxy** (new) | `contracts/src/upgradeable/proxy.rs` | Contracts that need bug fixes without moving user funds |
| `Upgradability` | `contracts/src/upgradability.rs` | Contracts that swap their own WASM in place (`update_current_contract_wasm`) |

Prefer the proxy for anything holding user balances: an in-place WASM swap is
recoverable, whereas a buggy implementation can still brick the contract, while
the proxy leaves a rollback target one call away.

## Upgrade lifecycle

```
  propose_implementation
          │
          │  timelock starts (default 24h)
          ▼
  [governance approval, if an approver is registered]
          │
          ▼
   execute_upgrade        ──▶  version += 1, history appended,
          │                    previous implementation recorded
          ▼
   migrate                ──▶  implementation migrates its own state layout
```

1. **`propose_implementation(admin, new_impl)`** — records the candidate and
   `execute_after = now + timelock`. Only one pending upgrade at a time. The
   implementation may not be the proxy itself and may not already be active.
2. **`approve_implementation(governance)`** — required only when
   `set_governance_approver` has been called. Execution is blocked until it
   runs.
3. **`execute_upgrade(admin)`** — fails before `execute_after`, requires
   governance approval when configured, then swaps the implementation, bumps
   the version and appends an immutable history record.
4. **`migrate(admin)`** — the window in which the new implementation migrates
   its own state layout. The proxy stores nothing user-facing, so this is purely
   a signal plus a place for the implementation to act.

Every step emits an event (`implementation_proposed`, `implementation_approved`,
`implementation_upgraded`, `implementation_rolled_back`, `upgrade_cancelled`,
`migration_run`), so an indexer can reconstruct the full history.

## Invariants

The proxy refuses to violate any of these:

* only the admin may propose, execute, cancel, roll back or migrate;
* an upgrade cannot execute before `execute_after`;
* at most one pending upgrade;
* the implementation is never the proxy itself and never the active one;
* governance approval is mandatory whenever an approver is registered;
* `rollback` records a **new** version rather than rewinding history, so the
  version number only ever increases.

## State migration strategy

The proxy itself needs no migration. Migration concerns the implementation:

1. **Additive first.** Ship a release that only adds data and functions. Old
   keys keep their meaning.
2. **Backfill in `migrate`.** The new implementation iterates over known records
   and writes the new layout. Keep it idempotent — it may be retried.
3. **Read both layouts for one release.** A record written by v1 must still be
   readable after v2. Prefer `Option<T>` and a `version` marker on the struct
   over guessing from missing fields.
4. **Remove old keys in a later release**, never in the same release that stops
   reading them.
5. **Never renumber existing storage keys.** Renaming a key in a `#[contracttype]`
   enum silently orphans user data — the new name simply finds nothing.

Version compatibility rules of thumb:

| Change | Same release OK? |
| --- | --- |
| Add a storage key | Yes |
| Add a contract function | Yes |
| Add a field to a struct | No — old records become unreadable |
| Remove a storage key | No — do it one release after the last read |
| Change a key name | Never |
| Tighten a validation rule | Yes, but expect pending proposals to fail |

## Rollback procedures

**Before executing** (normal case):

```bash
./scripts/upgrade_contract.sh --proxy <PROXY_ID> --cancel
```

or on-chain: `cancel_upgrade(admin)`.

**After executing** (bad release):

```bash
./scripts/upgrade_contract.sh --proxy <PROXY_ID> --rollback --execute
```

`rollback` re-points the proxy at the implementation used before the last
upgrade. If the bad implementation already migrated state, roll back *and*
deploy a fixed release of the old implementation that tolerates the new layout —
that is why migration should stay additive.

**Emergency override.** If the admin key is lost, the deployment must be
migrated by hand: deploy a new proxy, re-point the backend at it, and read the
state back through the old implementation. This is the one failure the timelock
cannot protect against; keep the admin key in a multisig or, preferably, behind
a governance contract.

## Operational runbook

1. **Prepare.** Merge the new implementation, run `cargo test` for the
   contract, deploy it (`stellar contract deploy`), and note the contract ID.
2. **Announce.** Open a PR/issue describing the change and the rollback target.
   Post the proposed `implementation` address publicly so token holders can
   review before the timelock expires.
3. **Propose.**

   ```bash
   ./scripts/upgrade_contract.sh --proxy <PROXY_ID> --implementation <NEW_ID>
   ```

   This is a dry run: it proposes and stops.
4. **Wait out the timelock** (default 24h). Emergency fixes may shorten it with
   `set_timelock`, but that removes the review window — say so explicitly in
   the incident record.
5. **Get governance approval** if an approver is configured:

   ```bash
   stellar contract invoke --id <PROXY_ID> -- <governance_addr> -- approve_implementation -- <governance_addr>
   ```

6. **Execute.**

   ```bash
   ./scripts/upgrade_contract.sh --proxy <PROXY_ID> --implementation <NEW_ID> --execute [--migrate]
   ```

7. **Verify.** `get_status` shows the new implementation and version;
   `get_upgrade_record(0)` shows who upgraded and when. Smoke-test one read and
   one write against the proxy.

## Monitoring

| Signal | Where |
| --- | --- |
| `implementation_proposed` / `implementation_upgraded` / `implementation_rolled_back` events | contract events, consumed by the event indexer |
| `get_status` / `get_version` / `get_upgrade_record` | on-chain queries, used by `scripts/upgrade_contract.sh --status` |
| Pending upgrade not executed after the timelock | alert on the proposal event with no matching upgrade event |

An upgrade that is proposed and then executed outside the announced window is
the single most important thing to detect — alert on it.

## Testing

* `contracts/src/upgradeable/proxy.rs` unit tests — lifecycle, timelock,
  governance, rollback, cancellation, admin transfer, invariants.
* `contracts/tests/proxy_upgrade_test.rs` — end-to-end flows, including the
  property that the proxy address and configuration survive an upgrade.