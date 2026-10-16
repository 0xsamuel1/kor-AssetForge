// Integration tests for liquidity-pool fee tiers (Issue #209).
//
// Covers tier selection at pool creation, fee-tier migration, proportional
// fee distribution to LPs, and the tier-recommendation helper.

#![cfg(test)]

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::liquidity_pool::{FeeTier, LiquidityPool, LiquidityPoolClient};
use soroban_sdk::testutils::Address as _;
use soroban_sdk::{Address, Env};

fn setup(env: &Env) -> (LiquidityPoolClient, Address) {
    let id = env.register_contract(None, LiquidityPool);
    let client = LiquidityPoolClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

// ---------------------------------------------------------------------------
// Tier selection
// ---------------------------------------------------------------------------

#[test]
fn test_create_pool_with_each_tier() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);
    let creator = Address::generate(&env);

    let low = client.create_pool_with_tier(&creator, &1, &2, &FeeTier::Low);
    let med = client.create_pool_with_tier(&creator, &3, &4, &FeeTier::Medium);
    let high = client.create_pool_with_tier(&creator, &5, &6, &FeeTier::High);

    assert_eq!(client.get_pool(&low).unwrap().fee_bps, 5);
    assert_eq!(client.get_pool(&low).unwrap().fee_tier, FeeTier::Low);
    assert_eq!(client.get_pool(&med).unwrap().fee_bps, 30);
    assert_eq!(client.get_pool(&high).unwrap().fee_bps, 100);
    assert_eq!(client.get_pool(&high).unwrap().fee_tier, FeeTier::High);
}

#[test]
fn test_legacy_create_pool_maps_to_nearest_tier() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);
    let creator = Address::generate(&env);

    // 5 bps -> Low, 30 bps -> Medium, 100 bps -> High.
    let p_low = client.create_pool(&creator, &1, &2, &5);
    let p_med = client.create_pool(&creator, &3, &4, &30);
    let p_high = client.create_pool(&creator, &5, &6, &100);

    assert_eq!(client.get_pool(&p_low).unwrap().fee_tier, FeeTier::Low);
    assert_eq!(client.get_pool(&p_med).unwrap().fee_tier, FeeTier::Medium);
    assert_eq!(client.get_pool(&p_high).unwrap().fee_tier, FeeTier::High);
}

// ---------------------------------------------------------------------------
// Fee-tier migration
// ---------------------------------------------------------------------------

#[test]
fn test_migrate_fee_tier_by_creator() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);
    let creator = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&creator, &1, &2, &FeeTier::Medium);
    client.migrate_fee_tier(&creator, &pool_id, &FeeTier::High);

    let pool = client.get_pool(&pool_id).unwrap();
    assert_eq!(pool.fee_tier, FeeTier::High);
    assert_eq!(pool.fee_bps, 100);
}

#[test]
fn test_migrate_fee_tier_by_admin() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let creator = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&creator, &1, &2, &FeeTier::High);
    client.migrate_fee_tier(&admin, &pool_id, &FeeTier::Low);

    assert_eq!(client.get_pool(&pool_id).unwrap().fee_bps, 5);
}

#[test]
#[should_panic(expected = "only pool creator or admin")]
fn test_migrate_fee_tier_rejects_stranger() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);
    let creator = Address::generate(&env);
    let stranger = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&creator, &1, &2, &FeeTier::Medium);
    client.migrate_fee_tier(&stranger, &pool_id, &FeeTier::High);
}

// ---------------------------------------------------------------------------
// Proportional fee distribution
// ---------------------------------------------------------------------------

#[test]
fn test_fees_distributed_proportionally() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);

    let lp1 = Address::generate(&env);
    let lp2 = Address::generate(&env);
    let trader = Address::generate(&env);

    // 0.30% tier so the fee on a 1_000_000 swap is exactly 3000.
    let pool_id = client.create_pool_with_tier(&lp1, &1, &2, &FeeTier::Medium);

    // Two providers with equal stakes -> each owns half the pool.
    client.add_liquidity(&lp1, &pool_id, &10_000_000, &10_000_000);
    client.add_liquidity(&lp2, &pool_id, &10_000_000, &10_000_000);

    // Swap asset_a -> asset_b. Fee accrues in asset_a.
    let result = client.swap(&trader, &pool_id, &1, &1_000_000, &0);
    assert_eq!(result.fee_amount, 3000);

    let (p1_a, _p1_b) = client.pending_fees(&pool_id, &lp1);
    let (p2_a, _p2_b) = client.pending_fees(&pool_id, &lp2);

    // Equal stake -> equal split of the 3000 fee.
    assert_eq!(p1_a, 1500);
    assert_eq!(p2_a, 1500);

    // Claiming pays out and zeroes the pending balance.
    let (claimed_a, _claimed_b) = client.claim_fees(&lp1, &pool_id);
    assert_eq!(claimed_a, 1500);
    assert_eq!(client.pending_fees(&pool_id, &lp1), (0, 0));
}

#[test]
fn test_fees_track_changing_share() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup(&env);

    let lp1 = Address::generate(&env);
    let lp2 = Address::generate(&env);
    let trader = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp1, &1, &2, &FeeTier::Medium);

    // lp1 is the sole provider for the first swap -> earns the whole fee.
    client.add_liquidity(&lp1, &pool_id, &10_000_000, &10_000_000);
    client.swap(&trader, &pool_id, &1, &1_000_000, &0);

    let (p1_after_first, _) = client.pending_fees(&pool_id, &lp1);
    assert_eq!(p1_after_first, 3000);

    // lp2 joins; subsequent fees split, but lp2 never earns the earlier fee.
    client.add_liquidity(&lp2, &pool_id, &10_000_000, &10_000_000);
    let (p2_initial, _) = client.pending_fees(&pool_id, &lp2);
    assert_eq!(p2_initial, 0);
}

// ---------------------------------------------------------------------------
// Tier recommendation
// ---------------------------------------------------------------------------

#[test]
fn test_recommend_tier() {
    let env = Env::default();
    let (client, _) = setup(&env);

    // High volatility -> High tier.
    assert_eq!(client.recommend_tier(&600, &0), FeeTier::High);
    // Low volatility + deep liquidity -> Low tier.
    assert_eq!(client.recommend_tier(&10, &2_000_000), FeeTier::Low);
    // Low volatility but shallow liquidity -> Medium tier.
    assert_eq!(client.recommend_tier(&10, &100), FeeTier::Medium);
    // Moderate volatility -> Medium tier.
    assert_eq!(client.recommend_tier(&200, &5_000_000), FeeTier::Medium);
}
