extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::fee_rebate::{
    FeeRebateContract, FeeRebateContractClient, TIER1_THRESHOLD, TIER2_THRESHOLD,
    TIER3_THRESHOLD, VOLUME_WINDOW,
};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

fn setup(env: &Env) -> (FeeRebateContractClient, Address) {
    let cid = env.register_contract(None, FeeRebateContract);
    let client = FeeRebateContractClient::new(env, &cid);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_no_rebate_below_tier1() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    let rebate = client.record_trade(&admin, &user, &100);
    assert_eq!(rebate, 0);
}

#[test]
fn test_tier1_10_percent_rebate() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    // Reach tier1 volume
    client.record_trade(&admin, &user, &TIER1_THRESHOLD);
    let rebate = client.record_trade(&admin, &user, &1000);
    assert_eq!(rebate, 100);
}

#[test]
fn test_tier2_25_percent_rebate() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade(&admin, &user, &TIER2_THRESHOLD);
    let rebate = client.record_trade(&admin, &user, &1000);
    assert_eq!(rebate, 250);
}

#[test]
fn test_tier3_50_percent_rebate() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade(&admin, &user, &TIER3_THRESHOLD);
    let rebate = client.record_trade(&admin, &user, &1000);
    assert_eq!(rebate, 500);
}

#[test]
fn test_claim_rebate_clears_pending() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade(&admin, &user, &TIER2_THRESHOLD);
    client.record_trade(&admin, &user, &1000);
    let claimed = client.claim_rebate(&user);
    assert_eq!(claimed, 250);
    assert_eq!(client.get_record(&user).pending_rebate, 0);
}

#[test]
#[should_panic(expected = "no pending rebate")]
fn test_claim_with_no_rebate_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _admin) = setup(&env);
    let user = Address::generate(&env);
    client.claim_rebate(&user);
}

#[test]
fn test_window_reset_clears_volume() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade(&admin, &user, &TIER3_THRESHOLD);
    env.ledger().with_mut(|l| { l.timestamp += VOLUME_WINDOW + 1; });
    // Volume resets, no tier
    let rebate = client.record_trade(&admin, &user, &1000);
    assert_eq!(rebate, 0);
}

#[test]
fn test_rebate_tier_bps_query() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _admin) = setup(&env);
    assert_eq!(client.rebate_tier_bps(&0), 0);
    assert_eq!(client.rebate_tier_bps(&TIER1_THRESHOLD), 1000);
    assert_eq!(client.rebate_tier_bps(&TIER2_THRESHOLD), 2500);
    assert_eq!(client.rebate_tier_bps(&TIER3_THRESHOLD), 5000);
}

#[test]
#[should_panic(expected = "admin only")]
fn test_non_admin_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _admin) = setup(&env);
    let user = Address::generate(&env);
    let non_admin = Address::generate(&env);
    client.record_trade(&non_admin, &user, &1000);
}
