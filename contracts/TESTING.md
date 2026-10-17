# Contract Testing Guide

This crate (`kor-assetforge-contracts`) contains the Soroban smart contracts for
AssetForge. Tests live in two places:

- **Inline unit tests** — `#[cfg(test)] mod ...` blocks inside `src/*.rs`.
  These exercise a single contract's storage and logic in isolation.
- **Integration tests** — files under `tests/*.rs`. These register one or more
  contracts and exercise cross-contract flows.

## Running the tests

```bash
cd contracts
cargo test
```

To run a single contract's tests:

```bash
cargo test access_control
```

## Writing a unit test

Inline tests use the `soroban_sdk` test utilities. A minimal example, taken from
`src/access_control.rs`:

```rust
#[cfg(test)]
mod rbac_unit_tests {
    use super::*;
    use soroban_sdk::testutils::Address as _;

    fn setup() -> (Env, Address, AccessControlClient<'static>) {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let contract_id = env.register_contract(None, AccessControl);
        let client = AccessControlClient::new(&env, &contract_id);
        client.initialize(&admin);
        (env, admin, client)
    }

    #[test]
    fn initialize_grants_admin_role() {
        let (_env, admin, client) = setup();
        assert!(client.has_role(&admin, &(Role::Admin as u32)));
    }
}
```

Use `env.mock_all_auths()` to satisfy `require_auth()` checks, and
`env.ledger().with_mut(|li| li.timestamp += n)` to advance time for
ledger timestamp dependent logic (e.g. expiring roles).

## Coverage status

Unit tests have been added for `access_control` (RBAC). Existing inline tests
cover `asset_token`, `auction`, `dispute_resolution`, `dividend_distributor`,
`emergency_control`, `fee_rebate`, `governance`, `liquidity_pool`, `marketplace`,
`multisig`, `oracle`, `p2p_market`, `reputation`, `staking_rewards` and
`upgradability`. Integration suites exist under `tests/`. Contracts still
needing dedicated unit tests include `bridge_security`, `bridge_validator`,
`asset_bundle`, `insurance`, `vesting`, `whitelist`, `yield_strategy` and
`events`.

## Known toolchain issue

`cargo test` may fail while resolving `ed25519-dalek`: `soroban-env-host`
21.2.1 currently resolves to `ed25519-dalek` 3.0.0, which introduced a breaking
`rand_core` change. Until an upstream fix lands, the CI contract job is run in
non-blocking mode. A workaround is to pin the dependency with a committed
`Cargo.lock`.
