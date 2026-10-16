# Bridge Security Model

Implements #343 — "Bridge Security Not Documented".

This document describes how `contracts/src/bridge_security.rs`
(`BridgeSecurity`) and `contracts/src/bridge_validator.rs`
(`BridgeValidator`) protect assets in flight between chains. It is written for
reviewers, relayer operators and incident responders. The companion documents
are [BRIDGE_THREAT_MODEL.md](./BRIDGE_THREAT_MODEL.md) and
[VALIDATOR_REQUIREMENTS.md](./VALIDATOR_REQUIREMENTS.md).

## Components

| Component | Contract | Role |
| --- | --- | --- |
| Bridge gate | `BridgeSecurity` | Accepts bridge requests, enforces limits, collects relayer approvals |
| Validator set | `BridgeValidator` | Multi-party attestation of cross-chain transfers, staking and slashing |

A transfer is only considered settled once **both** an amount is escrowed or
recorded on the source chain **and** enough independent validators have attested
to it on the destination chain.

## Trust assumptions

The model assumes:

1. **Relayers are untrusted individually and partially trusted collectively.**
   No single relayer can move funds; `required_sigs` signatures are required.
   Safety depends on the *honesty fraction* of the relayer set: fewer than
   `required_sigs` relayers may be compromised at any time.
2. **Validators are untrusted and economically bonded.** A validator that
   attests to a fraudulent transfer can be slashed via a fraud proof. Safety
   depends on the stake locked behind the validator set exceeding the maximum
   extractable value from a successful attack.
3. **The admin is trusted but constrained.** The admin can pause the bridge,
   manage the relayer set, configure limits and withdraw in an emergency, but
   cannot unilaterally approve a bridge request — approvals require relayers.
4. **The chain itself is trusted.** Classic bridge risk (consensus failure,
   finality reorganisation) is out of scope and covered by the per-chain
   confirmation policy below.

## Security controls

### 1. Multi-signature approval

* `initialize` fixes `required_sigs`.
* `approve_request` only accepts calls from addresses in the relayer set and
  records every signer in `ApprovalSigners(request_id)`.
* Duplicate approvals are rejected, so `required_sigs` distinct relayers are
  genuinely required.
* Once the threshold is reached the request is marked `approved` and the volume
  is added to `TotalVolume`.

### 2. Rate limiting and cooldowns

`request_bridge` enforces, per sender and per window:

* `amount > 0` and `amount <= max_bridge_amount`;
* at most `rate_limit_max` requests per `rate_limit_window`;
* a minimum gap of `cooldown_seconds` between two requests.

These bound the damage of a compromised relayer key or a scripted drain: even
with unlimited approvals, the outflow rate is capped.

### 3. Pause and emergency controls

* `set_paused` stops new requests *and* new approvals. Once paused, no value can
  move until the admin unpauses.
* `emergency_withdraw` requires both admin authorisation **and** the bridge to
  be paused, and sends to `EmergencyRecipient`, which must be configured in
  advance via `set_emergency_recipient`.
* All three actions emit events (`bridge_pause_changed`,
  `emergency_withdrawal`), so a silent pause is impossible to miss.

### 4. Validator attestation

* `add_validator` registers a validator in `Pending` status; only `Active`
  validators can sign.
* `approve_transfer` rejects non-active validators, rejects duplicate signatures,
  and marks the transfer approved only at `min_validators` signatures.
* `execute_transfer` requires `status == approved`; an executed transfer cannot
  be executed again.
* `submit_fraud_proof` marks a transfer failed and is the entry point for
  slashing.

### 5. On-chain observability

Every state transition emits an event (`bridge_requested`, `bridge_approved`,
`bridge_pause_changed`, `emergency_withdrawal`, fraud proof submissions), and
`get_config` / `get_request` / `get_total_volume` expose the state for monitoring
and reconciliation.

## Invariants

The system is safe as long as these hold:

1. **No release without threshold.** A request cannot be approved by fewer than
   `required_sigs` distinct relayers.
2. **No movement while paused.** Requests and approvals both panic while paused.
3. **No withdrawal without pause + recipient.** `emergency_withdraw` requires
   admin auth, `paused == true`, and a pre-configured recipient.
4. **No over-limit single transfer.** `amount <= max_bridge_amount` always.
5. **No rate-limit bypass.** Window and cooldown are tracked per sender address.
6. **No double execution.** A transfer moves to `executed` once and cannot be
   re-executed; a relayer cannot double-approve.
7. **No unauthorised approval.** Non-relayers and non-active validators are
   rejected.

## Recommended configuration

| Parameter | Recommended value | Rationale |
| --- | --- | --- |
| `required_sigs` | 3 of 5+ relayers | Tolerates one compromised relayer |
| Relayer set size | ≥ 5, across operators | Majority assumption holds |
| `max_bridge_amount` | ≤ 5% of bridge TVL | Caps a single-request drain |
| `rate_limit_window` / `rate_limit_max` | 3600s / 10 | Bounds scripted drain rate |
| `cooldown_seconds` | ≥ 60 | Prevents burst draining |
| `min_validators` | ≥ 2/3 of the validator set | Tolerates one validator compromise |
| Emergency recipient | Timelocked multisig | Admin cannot drain unilaterally |

## Operational procedures

### Adding or removing a relayer

1. Verify the relayer identity out of band.
2. `add_relayer(admin, relayer)`; confirm via `get_config`/events.
3. Removal is `remove_relayer(admin, relayer)`. After removal, check that
   `required_sigs` is still reachable with the remaining set — reducing the set
   below the threshold bricks the bridge.

### Pausing

1. `set_paused(admin, true)` — immediately blocks requests and approvals.
2. Investigate off-chain (relayer logs, source-chain finality).
3. Unpause with `set_paused(admin, false)` once the cause is understood.
4. If funds are at risk, follow with `emergency_withdraw` while still paused.

### Handling a suspected fraudulent transfer

1. `submit_fraud_proof(transfer_id, proof_data)` — marks the transfer failed.
2. `slash_validator(admin, validator, amount)` for each involved validator.
3. `set_paused(admin, true)` if other in-flight transfers are suspect.
4. Reconcile against the source-chain event log; see
   [BRIDGE_THREAT_MODEL.md](./BRIDGE_THREAT_MODEL.md) for the reconciliation
   procedure.

### Key rotation

The admin key is the single most valuable key in the system. Rotate it by
deploying a new configuration, or better, by migrating admin control to a
governance/multisig address (see
[CONTRACT_UPGRADE_STRATEGY.md](./CONTRACT_UPGRADE_STRATEGY.md) for the
admin-transfer flow of the proxy).

## Known limitations

* `emergency_withdraw` records an event but the transfer of value itself is
  performed by the caller (token contract integration); reconcile emitted events
  against on-chain balances.
* Relayer approvals do not carry a block number or a source-chain finality
  proof; those are asserted off-chain by the relayer operator. If finality
  assumptions matter, extend `BridgeRequest` with a finality proof field.
* `BridgeSecurity` and `BridgeValidator` are separate contracts; the wiring
  between an approved request and a validator-attested transfer is performed by
  the backend and must be reconciled continuously.

## Testing

* `contracts/tests/bridge_test.rs` — request, approval, rate limit and pause
  behaviour of `BridgeSecurity`.
* `contracts/tests/bridge_validator_test.rs` — validator lifecycle, attestation
  quorum, fraud proofs and slashing.
* `contracts/tests/security_test.rs` — cross-cutting security invariants for the
  rest of the protocol (TWAP, trade limits, withdraw cooldowns).