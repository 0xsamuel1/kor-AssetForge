// Integration tests for the upgradeable proxy – Issue #342
//
// Covers the full operational path an upgrade takes:
//   propose -> (timelock) -> governance approval -> execute -> migrate
// plus rollback and the "state stays put" property that makes the proxy
// pattern safe: the proxy address never changes, only the implementation it
// points at.

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::upgradeable::proxy::{Proxy, ProxyClient};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

const TIMELOCK: u64 = 3_600;

fn setup(timelock: u64) -> (Env, Address, Address, Address, ProxyClient<'static>) {
    let env = Env::default();
    env.mock_all_auths();
    env.ledger().with_mut(|li| li.timestamp = 1_000);

    let admin = Address::generate(&env);
    let implementation = Address::generate(&env);
    let proxy_id = env.register_contract(None, Proxy);
    let client = ProxyClient::new(&env, &proxy_id);

    client.initialize(&admin, &implementation);
    client.set_timelock(&admin, &timelock);

    (env, admin, implementation, proxy_id, client)
}

#[test]
fn upgrade_changes_implementation_but_not_proxy_address() {
    let (env, admin, implementation_v1, proxy_id, client) = setup(TIMELOCK);
    let implementation_v2 = Address::generate(&env);

    client.propose_implementation(&admin, &implementation_v2);
    env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
    client.execute_upgrade(&admin);

    assert_eq!(client.get_implementation(), implementation_v2);
    // The proxy contract address is unchanged: clients keep pointing at it and
    // every piece of stored state stays exactly where it was.
    assert_eq!(client.get_self(), proxy_id);
    assert_ne!(client.get_implementation(), implementation_v1);
}

#[test]
fn upgrade_flow_preserves_version_history() {
    let (env, admin, _impl_v1, _proxy_id, client) = setup(TIMELOCK);

    let mut versions: std::vec::Vec<Address> = std::vec::Vec::new();
    for step in 0..3u8 {
        let implementation = Address::generate(&env);
        versions.push(implementation.clone());
        client.propose_implementation(&admin, &implementation);
        env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
        client.execute_upgrade(&admin);
        assert_eq!(client.get_version(), 2 + step as u32);
    }

    assert_eq!(client.get_upgrade_count(), 3);
    for (i, implementation) in versions.iter().enumerate() {
        let record = client.get_upgrade_record(&(i as u32)).unwrap();
        assert_eq!(record.version, i as u32 + 2);
        assert_eq!(&record.implementation, implementation);
    }
}

#[test]
fn upgrade_can_be_rolled_back_to_the_previous_implementation() {
    let (env, admin, implementation_v1, _proxy_id, client) = setup(TIMELOCK);
    let implementation_v2 = Address::generate(&env);

    client.propose_implementation(&admin, &implementation_v2);
    env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
    client.execute_upgrade(&admin);

    client.rollback(&admin);

    assert_eq!(client.get_implementation(), implementation_v1);
    // Rollback is a new version, not a silent rewind of history.
    assert_eq!(client.get_version(), 3);
}

#[test]
fn pending_upgrade_can_be_cancelled() {
    let (env, admin, implementation_v1, _proxy_id, client) = setup(TIMELOCK);
    let implementation_v2 = Address::generate(&env);

    client.propose_implementation(&admin, &implementation_v2);
    client.cancel_upgrade(&admin);

    env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
    assert!(client.try_execute_upgrade(&admin).is_err());
    assert_eq!(client.get_implementation(), implementation_v1);
}

#[test]
fn upgrade_is_invisible_to_state_surviving_the_upgrade() {
    // The proxy keeps its own storage across upgrades, so anything written
    // before the upgrade is still readable afterwards.
    let (env, admin, implementation_v1, _proxy_id, client) = setup(TIMELOCK);

    let governance = Address::generate(&env);
    client.set_governance_approver(&admin, &Some(governance.clone()));
    let before = client.get_status();
    assert_eq!(before.implementation, implementation_v1);

    let implementation_v2 = Address::generate(&env);
    client.propose_implementation(&admin, &implementation_v2);
    env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
    client.approve_implementation(&governance);
    client.execute_upgrade(&admin);

    let after = client.get_status();
    assert_eq!(after.implementation, implementation_v2);
    assert_eq!(after.timelock_duration, before.timelock_duration);
    assert!(after.previous_implementation.is_some());
}

#[test]
fn migration_hook_is_available_after_an_upgrade() {
    let (env, admin, _impl_v1, _proxy_id, client) = setup(TIMELOCK);

    client.propose_implementation(&admin, &Address::generate(&env));
    env.ledger().with_mut(|li| li.timestamp += TIMELOCK);
    client.execute_upgrade(&admin);

    // The migration window is open once the new implementation is live.
    client.migrate(&admin);
}

#[test]
fn upgrade_script_preconditions_hold_by_default() {
    // Mirrors the checks scripts/upgrade_contract.sh performs before proposing.
    let (env, admin, _impl_v1, _proxy_id, client) = setup(TIMELOCK);

    assert!(client.get_pending_implementation().is_none());
    assert!(!client.is_upgrade_ready());
    assert_eq!(client.get_upgrade_count(), 0);

    client.propose_implementation(&admin, &Address::generate(&env));
    let status = client.get_status();
    assert_eq!(status.execute_after, env.ledger().timestamp() + TIMELOCK);
}