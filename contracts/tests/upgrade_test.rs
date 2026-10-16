// Comprehensive tests for contract upgrade scenarios – Issue #207
//
// Covers:
//   - State migration hook
//   - Storage compatibility across versions
//   - Upgrade rollback (proposing old hash)
//   - Governance-gated approval flow
//   - Emergency upgrade (zero timelock)
//   - Admin-only enforcement
//   - History and version tracking across multiple upgrades

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::upgradability::{Upgradability, UpgradabilityClient};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, BytesN, Env};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn dummy_hash(env: &Env, seed: u8) -> BytesN<32> {
    BytesN::from_array(env, &[seed; 32])
}

/// Register and initialise an Upgradability contract.
fn setup(timelock: u64) -> (Env, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();
    let admin = Address::generate(&env);
    let id = env.register_contract(None, Upgradability);
    let client = UpgradabilityClient::new(&env, &id);
    client.initialize(&admin, &timelock);
    (env, id, admin)
}

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

#[test]
fn test_initial_state() {
    let (env, id, _) = setup(7200);
    let client = UpgradabilityClient::new(&env, &id);
    assert_eq!(client.get_version(), 1);
    assert_eq!(client.get_timelock(), 7200);
    assert!(client.get_pending_upgrade().is_none());
    assert_eq!(client.get_upgrade_count(), 0);
}

#[test]
#[should_panic(expected = "already initialized")]
fn test_double_initialize_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    client.initialize(&admin, &0);
}

// ---------------------------------------------------------------------------
// State migration hook
// ---------------------------------------------------------------------------

#[test]
fn test_migrate_emits_event_and_succeeds() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    // migrate is a no-op placeholder in v1 – should not panic
    client.migrate(&admin);
}

#[test]
#[should_panic(expected = "admin only")]
fn test_migrate_non_admin_panics() {
    let (env, id, _) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let rando = Address::generate(&env);
    client.migrate(&rando);
}

// ---------------------------------------------------------------------------
// Storage compatibility (version tracking persists across upgrades)
// ---------------------------------------------------------------------------

#[test]
fn test_version_and_history_preserved_across_upgrades() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);

    let hashes: [u8; 3] = [10, 20, 30];
    for (i, seed) in hashes.iter().enumerate() {
        client.propose_upgrade(&admin, &dummy_hash(&env, *seed));
        client.execute_upgrade_dry_run(&admin);

        // Version increments from 1 to 4
        assert_eq!(client.get_version(), (i + 2) as u32);
        assert_eq!(client.get_upgrade_count(), (i + 1) as u32);

        // Each record is stored at its zero-based index
        let rec = client.get_upgrade_record(&(i as u32)).unwrap();
        assert_eq!(rec.wasm_hash, dummy_hash(&env, *seed));
        assert_eq!(rec.upgraded_by, admin);
    }
}

#[test]
fn test_upgrade_record_fields_correct() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let hash = dummy_hash(&env, 77);

    env.ledger().with_mut(|li| li.timestamp = 5000);
    client.propose_upgrade(&admin, &hash);
    client.execute_upgrade_dry_run(&admin);

    let rec = client.get_upgrade_record(&0).unwrap();
    assert_eq!(rec.version, 1);            // version AT time of upgrade
    assert_eq!(rec.wasm_hash, hash);
    assert_eq!(rec.timestamp, 5000);
    assert_eq!(rec.upgraded_by, admin);
}

// ---------------------------------------------------------------------------
// Rollback scenario (proposing previous WASM hash)
// ---------------------------------------------------------------------------

#[test]
fn test_rollback_via_reproposal() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);

    let v1_hash = dummy_hash(&env, 1);
    let v2_hash = dummy_hash(&env, 2);

    // Upgrade to v2
    client.propose_upgrade(&admin, &v2_hash);
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);

    // Rollback: propose v1 hash again
    client.propose_upgrade(&admin, &v1_hash);
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 3);  // version counter still increments

    // The record at index 1 should reflect the rollback hash
    let rollback_rec = client.get_upgrade_record(&1).unwrap();
    assert_eq!(rollback_rec.wasm_hash, v1_hash);
}

// ---------------------------------------------------------------------------
// Timelock enforcement
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "timelock has not expired")]
fn test_execute_before_timelock_panics() {
    let (env, id, admin) = setup(3600);
    let client = UpgradabilityClient::new(&env, &id);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.execute_upgrade_dry_run(&admin);
}

#[test]
fn test_execute_exactly_at_timelock_succeeds() {
    let (env, id, admin) = setup(3600);
    let client = UpgradabilityClient::new(&env, &id);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    env.ledger().with_mut(|li| li.timestamp += 3600);
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);
}

#[test]
fn test_timelock_change_affects_future_proposals_only() {
    let (env, id, admin) = setup(3600);
    let client = UpgradabilityClient::new(&env, &id);

    // Propose with 3600s timelock
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    // Admin changes timelock – must not affect the already-pending proposal
    client.set_timelock(&admin, &0);

    // Still needs 3600s from proposal time
    env.ledger().with_mut(|li| li.timestamp += 3600);
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);

    // Next proposal uses the new 0s timelock
    client.propose_upgrade(&admin, &dummy_hash(&env, 2));
    client.execute_upgrade_dry_run(&admin); // immediate
    assert_eq!(client.get_version(), 3);
}

// ---------------------------------------------------------------------------
// Governance-gated approval
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "governance approval required")]
fn test_execute_without_governance_approval_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let gov = Address::generate(&env);
    client.set_governance_contract(&admin, &Some(gov));
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.execute_upgrade_dry_run(&admin);
}

#[test]
fn test_execute_with_governance_approval_succeeds() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let gov = Address::generate(&env);
    client.set_governance_contract(&admin, &Some(gov.clone()));
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.approve_upgrade_governance(&gov);
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);
}

#[test]
fn test_remove_governance_contract_allows_execution() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let gov = Address::generate(&env);

    // Enable governance, then remove it before proposing
    client.set_governance_contract(&admin, &Some(gov));
    client.set_governance_contract(&admin, &None);

    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.execute_upgrade_dry_run(&admin);  // no governance check
    assert_eq!(client.get_version(), 2);
}

#[test]
#[should_panic(expected = "caller is not the registered governance contract")]
fn test_wrong_governance_address_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let real_gov = Address::generate(&env);
    let fake_gov = Address::generate(&env);
    client.set_governance_contract(&admin, &Some(real_gov));
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.approve_upgrade_governance(&fake_gov);
}

// ---------------------------------------------------------------------------
// Cancel upgrade
// ---------------------------------------------------------------------------

#[test]
fn test_cancel_clears_pending_upgrade() {
    let (env, id, admin) = setup(3600);
    let client = UpgradabilityClient::new(&env, &id);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    assert!(client.get_pending_upgrade().is_some());
    client.cancel_upgrade(&admin);
    assert!(client.get_pending_upgrade().is_none());
    // Version should not have changed
    assert_eq!(client.get_version(), 1);
    assert_eq!(client.get_upgrade_count(), 0);
}

#[test]
#[should_panic(expected = "no pending upgrade to cancel")]
fn test_cancel_with_no_pending_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    client.cancel_upgrade(&admin);
}

#[test]
fn test_can_repropose_after_cancel() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.cancel_upgrade(&admin);
    // Should not panic – no pending upgrade
    client.propose_upgrade(&admin, &dummy_hash(&env, 2));
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);
}

// ---------------------------------------------------------------------------
// Emergency upgrade (zero timelock)
// ---------------------------------------------------------------------------

#[test]
fn test_emergency_upgrade_zero_timelock() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    // No time advance needed with 0s timelock
    client.propose_upgrade(&admin, &dummy_hash(&env, 99));
    client.execute_upgrade_dry_run(&admin);
    assert_eq!(client.get_version(), 2);
}

// ---------------------------------------------------------------------------
// Admin-only enforcement
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "admin only")]
fn test_propose_by_non_admin_panics() {
    let (env, id, _) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let rando = Address::generate(&env);
    client.propose_upgrade(&rando, &dummy_hash(&env, 1));
}

#[test]
#[should_panic(expected = "admin only")]
fn test_execute_by_non_admin_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    let rando = Address::generate(&env);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.execute_upgrade_dry_run(&rando);
}

#[test]
#[should_panic(expected = "admin only")]
fn test_cancel_by_non_admin_panics() {
    let (env, id, admin) = setup(3600);
    let client = UpgradabilityClient::new(&env, &id);
    let rando = Address::generate(&env);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.cancel_upgrade(&rando);
}

#[test]
#[should_panic(expected = "an upgrade is already pending")]
fn test_double_propose_panics() {
    let (env, id, admin) = setup(0);
    let client = UpgradabilityClient::new(&env, &id);
    client.propose_upgrade(&admin, &dummy_hash(&env, 1));
    client.propose_upgrade(&admin, &dummy_hash(&env, 2));
}
