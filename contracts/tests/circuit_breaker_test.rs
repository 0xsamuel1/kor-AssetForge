extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient, PauseScope};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

fn setup(env: &Env) -> (EmergencyControlClient, Address) {
    let cid = env.register_contract(None, EmergencyControl);
    let client = EmergencyControlClient::new(env, &cid);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_price_crash_trips_breaker() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 1;

    let tripped = client.record_price(&admin, &asset_id, &1000);
    assert!(!tripped);

    env.ledger().with_mut(|l| { l.timestamp += 1800; });
    let tripped = client.record_price(&admin, &asset_id, &499);
    assert!(tripped);
    assert!(client.is_paused(&asset_id, &PauseScope::Trading));
}

#[test]
fn test_no_trip_on_moderate_price_drop() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 2;

    client.record_price(&admin, &asset_id, &1000);
    env.ledger().with_mut(|l| { l.timestamp += 1800; });
    let tripped = client.record_price(&admin, &asset_id, &700);
    assert!(!tripped);
    assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
}

#[test]
fn test_no_trip_outside_one_hour_window() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 3;

    client.record_price(&admin, &asset_id, &1000);
    env.ledger().with_mut(|l| { l.timestamp += 3601; });
    let tripped = client.record_price(&admin, &asset_id, &400);
    assert!(!tripped);
}

#[test]
fn test_volume_spike_trips_breaker() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 4;

    let tripped = client.record_volume(&admin, &asset_id, &1_000_001, &1_000_000);
    assert!(tripped);
    assert!(client.is_paused(&asset_id, &PauseScope::Trading));
}

#[test]
fn test_volume_below_threshold_no_trip() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 5;

    let tripped = client.record_volume(&admin, &asset_id, &999_999, &1_000_000);
    assert!(!tripped);
}

#[test]
fn test_circuit_breaker_manual_unpause() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 6;

    client.record_price(&admin, &asset_id, &1000);
    env.ledger().with_mut(|l| { l.timestamp += 1800; });
    client.record_price(&admin, &asset_id, &499);
    assert!(client.is_paused(&asset_id, &PauseScope::Trading));

    client.circuit_breaker_unpause(&admin, &asset_id);
    assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
}

#[test]
fn test_auto_unpause_after_cooldown() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let asset_id: u64 = 7;

    client.record_price(&admin, &asset_id, &1000);
    env.ledger().with_mut(|l| { l.timestamp += 1800; });
    client.record_price(&admin, &asset_id, &499);
    assert!(client.is_paused(&asset_id, &PauseScope::Trading));

    env.ledger().with_mut(|l| { l.sequence += 721; });
    assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
}
