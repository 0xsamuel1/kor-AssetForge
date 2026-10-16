// Tests for emergency fund recovery mechanism – Issue #205
//
// Covers:
//   - Recovery destination whitelist (add/remove/enforce)
//   - 72-hour timelock enforcement
//   - Multi-sig approval flow (threshold gate)
//   - Emergency withdrawal lifecycle (propose → approve → execute)
//   - Cancellation of pending withdrawals
//   - asset_token emergency_recover hook
//   - Event emission at each stage

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::asset_token::{AssetToken, AssetTokenClient};
use kor_assetforge_contracts::emergency_control::{
    EmergencyControl, EmergencyControlClient, EMERGENCY_TIMELOCK_SECS,
};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env, String, Vec};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn setup_ec(env: &Env) -> (EmergencyControlClient, Address) {
    let id = env.register_contract(None, EmergencyControl);
    let client = EmergencyControlClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

fn make_signers(env: &Env, n: u32) -> Vec<Address> {
    let mut v = Vec::new(env);
    for _ in 0..n {
        v.push_back(Address::generate(env));
    }
    v
}

// ===========================================================================
// Recovery destination whitelist
// ===========================================================================

#[test]
fn test_add_and_query_whitelist() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);

    let wl = client.get_recovery_whitelist();
    assert_eq!(wl.len(), 1);
    assert_eq!(wl.get(0).unwrap(), dest);
}

#[test]
#[should_panic(expected = "destination already whitelisted")]
fn test_add_duplicate_whitelist_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);
    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    client.add_recovery_destination(&admin, &dest);
}

#[test]
fn test_remove_from_whitelist() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);
    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    client.remove_recovery_destination(&admin, &dest);
    assert_eq!(client.get_recovery_whitelist().len(), 0);
}

#[test]
#[should_panic(expected = "destination not in whitelist")]
fn test_remove_non_whitelisted_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);
    let dest = Address::generate(&env);
    client.remove_recovery_destination(&admin, &dest);
}

#[test]
#[should_panic(expected = "destination is not whitelisted")]
fn test_propose_to_non_whitelisted_destination_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &2);

    let dest = Address::generate(&env); // NOT whitelisted
    let reason = String::from_str(&env, "stuck funds");
    client.propose_emergency_withdrawal(&admin, &dest, &1000, &1, &reason);
}

// ===========================================================================
// Multi-sig configuration
// ===========================================================================

#[test]
#[should_panic(expected = "invalid threshold")]
fn test_threshold_exceeds_signers_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);
    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &5);
}

#[test]
#[should_panic(expected = "invalid threshold")]
fn test_zero_threshold_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);
    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &0);
}

// ===========================================================================
// 72-hour timelock enforcement
// ===========================================================================

#[test]
#[should_panic(expected = "72-hour timelock has not expired")]
fn test_execute_before_timelock_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &2);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "recovery");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &500, &1, &reason);

    // Approve with both signers
    client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);
    client.approve_emergency_withdrawal(&signers.get(1).unwrap(), &wid);

    // Try to execute immediately (timelock not expired)
    client.execute_emergency_withdrawal(&admin, &wid);
}

#[test]
fn test_execute_after_72h_succeeds() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &2);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "recovery");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &500, &1, &reason);

    client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);
    client.approve_emergency_withdrawal(&signers.get(1).unwrap(), &wid);

    // Advance 72 hours + 1 second
    env.ledger().with_mut(|li| li.timestamp += EMERGENCY_TIMELOCK_SECS + 1);
    client.execute_emergency_withdrawal(&admin, &wid);

    let w = client.get_emergency_withdrawal(&wid).unwrap();
    assert!(w.executed);
}

#[test]
fn test_withdrawal_record_stores_correct_fields() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 1);
    client.configure_recovery_multisig(&admin, &signers, &1);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "stuck");

    env.ledger().with_mut(|li| li.timestamp = 1000);
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &999, &5, &reason);

    let w = client.get_emergency_withdrawal(&wid).unwrap();
    assert_eq!(w.id, wid);
    assert_eq!(w.amount, 999);
    assert_eq!(w.asset_id, 5);
    assert_eq!(w.destination, dest);
    assert_eq!(w.proposed_at, 1000);
    assert_eq!(w.execute_after, 1000 + EMERGENCY_TIMELOCK_SECS);
    assert!(!w.executed);
    assert!(!w.cancelled);
    assert_eq!(w.approvals, 0);
}

// ===========================================================================
// Multi-sig approval flow
// ===========================================================================

#[test]
#[should_panic(expected = "insufficient multi-sig approvals")]
fn test_execute_without_enough_approvals_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 3);
    client.configure_recovery_multisig(&admin, &signers, &3);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "recovery");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &500, &1, &reason);

    // Only 1 of 3 approvals
    client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);

    env.ledger().with_mut(|li| li.timestamp += EMERGENCY_TIMELOCK_SECS + 1);
    client.execute_emergency_withdrawal(&admin, &wid);
}

#[test]
#[should_panic(expected = "caller is not a registered signer")]
fn test_non_signer_approval_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 2);
    client.configure_recovery_multisig(&admin, &signers, &2);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "recovery");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &500, &1, &reason);

    let rando = Address::generate(&env);
    client.approve_emergency_withdrawal(&rando, &wid);
}

#[test]
fn test_approvals_accumulate_correctly() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 3);
    client.configure_recovery_multisig(&admin, &signers, &3);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "recovery");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &500, &1, &reason);

    client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);
    let w1 = client.get_emergency_withdrawal(&wid).unwrap();
    assert_eq!(w1.approvals, 1);

    client.approve_emergency_withdrawal(&signers.get(1).unwrap(), &wid);
    let w2 = client.get_emergency_withdrawal(&wid).unwrap();
    assert_eq!(w2.approvals, 2);

    client.approve_emergency_withdrawal(&signers.get(2).unwrap(), &wid);
    let w3 = client.get_emergency_withdrawal(&wid).unwrap();
    assert_eq!(w3.approvals, 3);
}

// ===========================================================================
// Cancellation
// ===========================================================================

#[test]
fn test_cancel_pending_withdrawal() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 1);
    client.configure_recovery_multisig(&admin, &signers, &1);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "cancel test");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &100, &1, &reason);

    client.cancel_emergency_withdrawal(&admin, &wid);

    let w = client.get_emergency_withdrawal(&wid).unwrap();
    assert!(w.cancelled);
    assert!(!w.executed);
}

#[test]
#[should_panic(expected = "already cancelled")]
fn test_cancel_twice_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 1);
    client.configure_recovery_multisig(&admin, &signers, &1);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "cancel");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &100, &1, &reason);

    client.cancel_emergency_withdrawal(&admin, &wid);
    client.cancel_emergency_withdrawal(&admin, &wid);
}

#[test]
#[should_panic(expected = "already executed")]
fn test_cancel_after_execute_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_ec(&env);

    let signers = make_signers(&env, 1);
    client.configure_recovery_multisig(&admin, &signers, &1);

    let dest = Address::generate(&env);
    client.add_recovery_destination(&admin, &dest);
    let reason = String::from_str(&env, "exec then cancel");
    let wid = client.propose_emergency_withdrawal(&admin, &dest, &100, &1, &reason);

    client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);
    env.ledger().with_mut(|li| li.timestamp += EMERGENCY_TIMELOCK_SECS + 1);
    client.execute_emergency_withdrawal(&admin, &wid);
    client.cancel_emergency_withdrawal(&admin, &wid);
}

// ===========================================================================
// asset_token emergency_recover hook (Issue #205)
// ===========================================================================

#[test]
fn test_emergency_recover_transfers_tokens() {
    let env = Env::default();
    env.mock_all_auths();

    let at_id = env.register_contract(None, AssetToken);
    let ec_id = env.register_contract(None, EmergencyControl);
    let at_client = AssetTokenClient::new(&env, &at_id);
    let ec_client = EmergencyControlClient::new(&env, &ec_id);

    let admin = Address::generate(&env);
    ec_client.initialize(&admin);

    at_client.initialize(
        &admin,
        &String::from_str(&env, "Token"),
        &String::from_str(&env, "TKN"),
        &7,
        &0,
    );

    let holder = Address::generate(&env);
    let recovery_dest = Address::generate(&env);

    // Mint tokens to holder
    at_client.mint(&holder, &1_000_000, &1, &ec_id);

    let before = at_client.balance(&holder);
    assert_eq!(before, 1_000_000);

    // Execute emergency recovery hook
    at_client.emergency_recover(&ec_id, &holder, &recovery_dest, &400_000, &1, &1);

    assert_eq!(at_client.balance(&holder), 600_000);
    assert_eq!(at_client.balance(&recovery_dest), 400_000);
}

#[test]
#[should_panic(expected = "insufficient balance for emergency recovery")]
fn test_emergency_recover_insufficient_balance_panics() {
    let env = Env::default();
    env.mock_all_auths();

    let at_id = env.register_contract(None, AssetToken);
    let ec_id = env.register_contract(None, EmergencyControl);
    let at_client = AssetTokenClient::new(&env, &at_id);
    let ec_client = EmergencyControlClient::new(&env, &ec_id);

    let admin = Address::generate(&env);
    ec_client.initialize(&admin);
    at_client.initialize(
        &admin,
        &String::from_str(&env, "Token"),
        &String::from_str(&env, "TKN"),
        &7,
        &0,
    );

    let holder = Address::generate(&env);
    let dest = Address::generate(&env);
    at_client.mint(&holder, &100, &1, &ec_id);

    at_client.emergency_recover(&ec_id, &holder, &dest, &999_999, &1, &1);
}

// ===========================================================================
// Full end-to-end scenario
// ===========================================================================

#[test]
fn test_full_recovery_scenario() {
    let env = Env::default();
    env.mock_all_auths();

    let (ec_client, admin) = setup_ec(&env);
    let signers = make_signers(&env, 3);
    // Require 2-of-3 multi-sig
    ec_client.configure_recovery_multisig(&admin, &signers, &2);

    let dest = Address::generate(&env);
    ec_client.add_recovery_destination(&admin, &dest);

    let reason = String::from_str(&env, "stuck LP funds");
    let wid = ec_client.propose_emergency_withdrawal(&admin, &dest, &50_000, &7, &reason);

    // Collect 2 approvals
    ec_client.approve_emergency_withdrawal(&signers.get(0).unwrap(), &wid);
    ec_client.approve_emergency_withdrawal(&signers.get(1).unwrap(), &wid);

    // 72-hour wait
    env.ledger().with_mut(|li| li.timestamp += EMERGENCY_TIMELOCK_SECS + 10);

    ec_client.execute_emergency_withdrawal(&admin, &wid);

    let w = ec_client.get_emergency_withdrawal(&wid).unwrap();
    assert!(w.executed);
    assert_eq!(w.approvals, 2);
    assert_eq!(w.destination, dest);
    assert_eq!(w.amount, 50_000);
}
