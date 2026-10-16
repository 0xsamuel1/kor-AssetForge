# Bridge Threat Model

Implements #343 — "Bridge Security Not Documented".

Companion to [BRIDGE_SECURITY_MODEL.md](./BRIDGE_SECURITY_MODEL.md). This
document enumerates what can go wrong, how likely it is, what the current
controls do about it, and what is left for operators to handle.

## Method

Scope: the cross-chain asset path — `BridgeSecurity` (requests, approvals,
limits, pause/emergency) and `BridgeValidator` (attestation, staking,
slashing). Anything outside the bridge is in scope only where it feeds the
bridge (oracle prices used for valuation, governance/admin keys).

Method: each asset in the bridge system is enumerated, then each way an attacker
could reach it. Assets are the bridged tokens escrowed on the source chain and
the minted claims on the destination chain, plus the bridge's own configuration.

Rating:

| Severity | Meaning |
| --- | --- |
| Critical | Total loss of bridged funds or unbounded minting |
| High | Loss bounded by limits, or a repeatable DoS |
| Medium | Requires unusual conditions or insider help |
| Low | Nuisance, recoverable |

## Assets

1. **Escrowed tokens** on the source chain.
2. **Minted claims** on the destination chain.
3. **Bridge configuration** (limits, relayer set, thresholds, pause flag).
4. **Validator stake** (economic security backing for attestation).
5. **Reputation data** (availability of decisions, not funds).

## Threats

### T1 — Collusion of relayers below the approval threshold

**Severity: Critical.** If `required_sigs` relayers collude (or their keys are
compromised), they can approve a fraudulent request and release funds that were
never escrowed, or approve the same request for a value exceeding what was
locked on the source chain.

**Controls:** `required_sigs` distinct signers enforced on-chain, duplicate
approval rejected, `max_bridge_amount` per request, per-sender rate limits and
cooldowns, and destination-side `min_validators` attestation.

**Residual risk:** the threshold itself. `required_sigs = 1` makes the bridge
trustless-but-brittle in the wrong direction — one key compromise is total loss.

**Mitigation:** set `required_sigs ≥ 3` over a set of ≥ 5 relayers operated by
independent entities in independent jurisdictions.

### T2 — Compromised relayer key (single)

**Severity: High.** A leaked key lets an attacker submit approvals and spam
requests from its own address.

**Controls:** approvals require relayer status; the per-sender rate limit and
cooldown bound a single-address drain; `remove_relayer` revokes instantly;
`set_paused` halts everything.

**Residual risk:** requests already approved and not yet executed.

**Mitigation:** short approval expiry enforced off-chain, plus continuous
reconciliation of `bridge_approved` events against escrow balances.

### T3 — Compromise of the admin key

**Severity: Critical.** The admin can add and remove relayers (so it can lower
`required_sigs`-effective security), set `max_bridge_amount`, pause, and
withdraw to `EmergencyRecipient`.

**Controls:** `emergency_withdraw` only works while paused and only to a
pre-configured recipient; every admin action emits an event.

**Residual risk:** a compromised admin can set the emergency recipient and then
pause + withdraw.

**Mitigation:** admin must be a timelocked multisig/governance contract; set the
emergency recipient to a second, independent multisig; alert on
`bridge_pause_changed` and `emergency_withdrawal` in real time.

### T4 — Forgery of a source-chain deposit event

**Severity: Critical.** The classic bridge exploit: an attacker convinces the
destination chain that tokens were locked when they were not, then mints claims
against them.

**Controls:** nothing on-chain — this is decided by the relayers and validators
observing the source chain. `min_validators` attestation is the last on-chain
line of defence; `submit_fraud_proof` + `slash_validator` are the recovery path.

**Residual risk:** high. This is the primary risk of every bridge.

**Mitigation:** require validators to verify against an independently fetched
source-chain proof (not the relayer's payload), enforce source-chain finality
depth before attestation, and require validator unanimity on first-time
destinations.

### T5 — Stale or manipulated oracle feeding bridge valuation

**Severity: Medium.** If a bridged asset is valued for fees or collateral using
a stale feed, an attacker can time deposits to extract value.

**Controls:** the oracle now rejects data older than five minutes
([ORACLE_STALENESS.md](./ORACLE_STALENESS.md)) and falls back to a second
source; valuation paths must use `get_fresh_price`.

**Residual risk:** a live but *wrong* price within the staleness window.

**Mitigation:** deviation alerting (`price_deviation` event) and a bounded
valuation haircut for freshly bridged assets.

### T6 — Rate-limit / cooldown bypass by splitting requests

**Severity: Medium.** An attacker splits a large drain into many small
requests to stay under `max_bridge_amount` and the window limit.

**Controls:** limits are enforced *per sender address per window*, not per
transaction, so splitting alone does not raise the total within a window.

**Residual risk:** Sybil — many sender addresses, each with a fresh window.

**Mitigation:** apply the same limits to the *source-chain escrow address*, and
monitor aggregate outflow per destination, not just per sender.

### T7 — Denial of service via relayer removal

**Severity: High.** If the relayer set is reduced below `required_sigs`,
approvals can never be reached and the bridge is frozen.

**Controls:** none enforced on-chain.

**Mitigation:** operational rule — verify `required_sigs ≤ |relayers|` before
every removal; alert on a relayer-set size that approaches the threshold.

### T8 — Validator compromise or insufficient stake

**Severity: Critical.** Validators are the attestation layer on the destination
chain. Compromise at or above `min_validators` lets an attacker approve a
fabricated transfer.

**Controls:** `Active` status gate, duplicate-signature rejection,
`min_validators` quorum, slashing via `submit_fraud_proof` + `slash_validator`,
reputation tracking.

**Residual risk:** if total stake is below the value extractable from an
attack, slashing is not a deterrent.

**Mitigation:** require total staked value ≥ a multiple (≥ 2×) of the maximum
daily bridge volume; see [VALIDATOR_REQUIREMENTS.md](./VALIDATOR_REQUIREMENTS.md).

### T9 — Replay of a previously executed transfer

**Severity: High.** Re-submitting a legitimate transfer ID to mint again.

**Controls:** `execute_transfer` requires `status == approved` and sets
`status = executed`; `transfer_id` is a `BytesN<32>` identifier; duplicate
validator signatures are rejected.

**Residual risk:** the backend must guarantee transfer IDs are unique and
content-derived (e.g. hash of source-chain event), not client-generated.

**Mitigation:** derive the ID deterministically on-chain from the source event
hash; reject non-conforming IDs at the relayer.

### T10 — Chain reorganisation / finality failure

**Severity: Medium–High** depending on the source chain. A source-chain reorg
invalidates deposits already attested on the destination chain.

**Controls:** none on-chain; this is a confirmation-policy decision.

**Mitigation:** per-chain finality depth (see VALIDATOR_REQUIREMENTS), and
delay release of large transfers until the source chain is final.

### T11 — Emergency withdrawal abuse

**Severity: High.** Legitimate-looking emergency withdrawals that in fact move
user funds.

**Controls:** admin-only, requires `paused == true`, and a pre-configured
`EmergencyRecipient`.

**Residual risk:** admin can set the recipient, pause, and withdraw.

**Mitigation:** recipient must be a multisig with its own timelock; reconcile
every `emergency_withdrawal` event against the incident record.

### T12 — Insider risk in the relayer/validator operator

**Severity: Medium.** An operator signs something it should not.

**Controls:** reputation and slashing make it attributable and costly;
`remove_relayer` / `slash_validator` are available immediately.

**Mitigation:** publish attestation logs, require signed operational runbooks
(see VALIDATOR_REQUIREMENTS), and keep an inactive-but-attributable identity
for each operator.

## Attack trees

**Drain the bridge (Critical path)**

```
drain
├── forge approvals      → need T1 (collusion ≥ required_sigs)
├── steal admin key      → T3, then set recipient + pause + withdraw
├── forge source deposit → T4, then min_validators attestation
└── oracle manipulation  → T5, requires live-but-wrong price
```

**Freeze the bridge (High)**

```
freeze
└── reduce relayer set below required_sigs  → T7
```

**Steal via accounting error (Medium)**

```
accounting mismatch
├── double execute       → T9 (requires unique IDs)
├── rate-limit split     → T6 (Sybil)
└── stale valuation      → T5
```

## Risk register

| ID | Threat | Severity | Control strength | Action |
| --- | --- | --- | --- | --- |
| T1 | Relayer collusion | Critical | Strong if `required_sigs ≥ 3` | Enforce in config |
| T2 | Single relayer key | High | Strong | Key rotation, short approval expiry |
| T3 | Admin key | Critical | Weak | Move admin to timelocked multisig |
| T4 | Forged source deposit | Critical | Medium | Independent proofs, finality depth |
| T5 | Stale/manipulated oracle | Medium | Strong (5 min) | Use `get_fresh_price` everywhere |
| T6 | Rate-limit bypass | Medium | Medium | Aggregate monitoring |
| T7 | Relayer-set freeze | High | Weak | Pre-removal invariant check |
| T8 | Validator compromise | Critical | Medium | Stake ≥ 2× daily volume |
| T9 | Replay | High | Strong | Content-derived transfer IDs |
| T10 | Reorg | Medium–High | Weak | Per-chain finality policy |
| T11 | Emergency abuse | High | Medium | Multisig recipient + reconciliation |
| T12 | Insider | Medium | Medium | Slashing + attributable identities |

## Detection

| Signal | Source | Alert |
| --- | --- | --- |
| Bridge paused | `bridge_pause_changed` event | immediate, critical |
| Emergency withdrawal | `emergency_withdrawal` event | immediate, critical |
| Approvals without matching escrow | reconciliation job | within one window |
| Outflow above `max_bridge_amount` in a window | `TotalVolume` | warning |
| Relayer set size approaching `required_sigs` | `get_config` | warning |
| Validator stake below 2× daily volume | `get_validator` sweep | critical |
| Fraud proof submitted | contract event | immediate, critical |

Operational response steps for each alert are in
[BRIDGE_SECURITY_MODEL.md](./BRIDGE_SECURITY_MODEL.md#operational-procedures).

## Residual risk statement

The bridge's strongest guarantee is economic: an attacker must either control a
majority of relayers, a majority of validators, or the admin key. Every remaining
risk concentrates in three places — the admin key, the honesty of relayers and
validators, and the correctness of off-chain source-chain verification. None of
these can be removed by contract code alone; they are managed by key custody,
stake sizing, operator independence, and continuous reconciliation.