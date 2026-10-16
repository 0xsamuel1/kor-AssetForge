# Validator Requirements

Implements #343 — "Bridge Security Not Documented".

Operational requirements for anyone running a **relayer** or a **validator** for
the kor-AssetForge bridge. These are on top of, not instead of, the on-chain
rules described in [BRIDGE_SECURITY_MODEL.md](./BRIDGE_SECURITY_MODEL.md) and
[BRIDGE_THREAT_MODEL.md](./BRIDGE_THREAT_MODEL.md).

## Roles

| Role | Contract entry points | Trust weight |
| --- | --- | --- |
| **Relayer** | `request_bridge`, `approve_request` | One signature towards `required_sigs` |
| **Validator** | `approve_transfer` | One attestation towards `min_validators` |
| **Admin** | `set_paused`, `add_relayer`, `remove_relayer`, `slash_validator`, `emergency_withdraw` | Full configuration control |
| **Executor** | `execute_transfer` | None — may be anyone, once approved |

Relayers gate *requests*; validators gate *cross-chain transfers*. Both are
required: a relayer approval without validator attestation is not a settlement,
and validator attestation without a matching request is not accepted by the
backend.

## Validator requirements

### Eligibility

1. **Identity.** A published, attributable legal identity. The validator's
   `ValidatorInfo` is permanent on-chain (`slash_count`, `reputation_score`,
   `signed_count` survive slashing), so an anonymous validator is worthless as a
   deterrent.
2. **Stake.** Minimum stake is set by policy, not by the contract. The working
   rule: **total staked value ≥ 2× the maximum daily bridge volume**, and no
   single validator ≥ 40% of the total (so the set cannot be captured by one
   compromise).
3. **Independence.** Validators must not be operated by the same entity as more
   than one other validator, and must not share signing infrastructure,
   cloud accounts or key custody with each other.
4. **Software.** Only the released validator binary, reproducibly built. Local
   modifications invalidate the security argument.
5. **Jury independence.** A validator must not be operated by the protocol team
   members who write the relayer, and ideally not by the same entity as the
   bridge admin.

### Runtime obligations

1. **Verify independently.** Confirm the source-chain deposit by querying the
   source chain yourself. Never attest to the payload supplied by a relayer or
   by the requester — that converts a multi-party check into a single point of
   trust.
2. **Respect finality.** Do not attest until the source-chain transaction has
   reached the required confirmation depth:

   | Chain class | Minimum confirmations |
   | --- | --- |
   | Instant-finality L2 / testnet | 1 finality checkpoint |
   | Typical PoS L1 | 30 blocks / ~5 minutes |
   | PoW L1 | 6 blocks **plus** 1 hour for amounts above 10% of `max_bridge_amount` |

3. **Check the invariants before signing.** Amount ≤ `max_bridge_amount`;
   destination is a supported chain (`add_supported_chain`); sender has not
   exceeded its window limit; no fraud proof is already recorded for the
   transfer.
4. **Never sign twice.** `approve_transfer` rejects a duplicate signature, and
   a duplicate is also evidence of a malfunctioning operator.
5. **Never sign after a fraud proof.** A transfer with a recorded fraud proof is
   failed; attesting to it is misconduct.
6. **Sign deterministically.** Given identical inputs, produce an identical
   decision. Do not "manually override" the software.
7. **Stay online.** `last_active` is part of the validator's record; extended
   inactivity is treated as unavailability in the next set review.
8. **Report anomalies within one hour.** A mismatch between what the validator
   verified and what was requested is a security incident, not a support ticket.

### After misconduct

1. `submit_fraud_proof(transfer_id, proof_data)` marks the transfer failed.
2. `slash_validator(admin, validator, amount)` burns stake, decrements
   reputation by 10, increments `slash_count`, and deactivates the validator once
   stake reaches zero.
3. The validator may rejoin only through the normal admission process, with
   stake restored in full. Slashed stake is **not** replenished by the protocol.

## Relayer requirements

1. **Key custody.** Hardware-backed signing. Relayer keys approve value, so
   treat them as hot keys with a documented rotation schedule.
2. **Independent verification.** Same rule as validators: verify the source
   chain directly, not from the submitter's payload.
3. **Rate limits are policy, not suggestions.** The per-sender window limit and
   cooldown exist to bound a compromised key. Never raise them to clear a queue.
4. **Escalate, do not retry blindly.** A request that keeps failing validation is
   a signal; re-approving it is how a bad request becomes an executed one.
5. **Never approve your own or a related party's request** for a non-zero
   amount, and declare such relationships to the admin.

## Admin requirements

1. **The admin key must be a timelocked multisig or governance contract.** A
   single EOA admin key makes threat T3 (admin compromise) critical.
2. **The emergency recipient must be a second, independent multisig**, set with
   `set_emergency_recipient` before it is ever needed.
3. **Never remove a relayer without checking** that the remaining set can still
   reach `required_sigs` — removal below the threshold freezes the bridge
   (threat T7).
4. **Slash only on evidence.** `slash_validator` is not a dispute mechanism; use
   it with a recorded fraud proof.
5. **Announce configuration changes** (limits, thresholds, relayer set) before
   making them.

## Monitoring duties

| Check | Frequency | Signal |
| --- | --- | --- |
| Attestation liveness | Per chain head | Missing attestations for open transfers |
| Attestation correctness | Continuous | Approved transfer with no matching source-chain deposit |
| Stake coverage | Daily | Total stake ÷ 24h volume below 2× |
| Validator activity | Weekly | `last_active` staleness, `signed_count` = 0 |
| Reputation drift | Weekly | `reputation_score` decay, `slash_count` > 0 |
| Bridge configuration | On change | `required_sigs`, `max_bridge_amount`, pause flag |

## Onboarding checklist

- [ ] Attributable identity published
- [ ] Stake deposited and confirmed on-chain
- [ ] Validator binary reproduced and checksum-verified
- [ ] Independent infrastructure from every other validator
- [ ] Finality policy configured per supported chain
- [ ] Runbook rehearsed: reject, escalate, respond to a fraud proof
- [ ] Alerting wired for liveness, stake coverage and correctness
- [ ] Added to the public validator set

## Offboarding checklist

- [ ] Stop attesting; announce the withdrawal to operators and the admin
- [ ] Confirm no open transfer depends solely on this validator
- [ ] Ensure the remaining set still meets `min_validators`
- [ ] Update the published set and the monitoring dashboards
- [ ] Unbond stake only after the observation window closes