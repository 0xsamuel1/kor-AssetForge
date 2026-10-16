// Security / flash-loan protection tests (Issue #210).
//
// Covers the TWAP oracle, per-block trade limits, deposit/withdraw cooldowns
// and price-deviation detection in the liquidity pool, plus the per-block
// purchase limit in the marketplace.

#![cfg(test)]

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};
use kor_assetforge_contracts::liquidity_pool::{FeeTier, LiquidityPool, LiquidityPoolClient};
use kor_assetforge_contracts::marketplace::{Marketplace, MarketplaceClient};
use soroban_sdk::testutils::{Address as _, Ledger as _};
use soroban_sdk::{Address, Env};

fn setup_pool(env: &Env) -> (LiquidityPoolClient, Address) {
    let id = env.register_contract(None, LiquidityPool);
    let client = LiquidityPoolClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

// ---------------------------------------------------------------------------
// TWAP oracle
// ---------------------------------------------------------------------------

#[test]
fn test_twap_tracks_average_price() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _) = setup_pool(&env);
    let lp = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.add_liquidity(&lp, &pool_id, &1_000_000, &1_000_000);

    // 1:1 reserves -> spot price == PRICE_SCALE (1e7).
    assert_eq!(client.get_spot_price(&pool_id), 10_000_000);

    // Anchor the TWAP window, then let time pass with no trades.
    client.snapshot_twap(&pool_id);
    env.ledger().with_mut(|li| li.timestamp += 100);

    // With a flat price over the window the TWAP equals the spot price.
    assert_eq!(client.get_twap(&pool_id), 10_000_000);
}

// ---------------------------------------------------------------------------
// Per-block trade limit (liquidity pool)
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "per-block trade limit exceeded")]
fn test_pool_per_block_trade_limit() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_pool(&env);
    let lp = Address::generate(&env);
    let trader = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.add_liquidity(&lp, &pool_id, &100_000_000, &100_000_000);

    // Allow at most 2 swaps per block.
    client.set_guard_config(&admin, &2, &0, &0);

    client.swap(&trader, &pool_id, &1, &10_000, &0);
    client.swap(&trader, &pool_id, &1, &10_000, &0);
    // Third swap in the same ledger must revert.
    client.swap(&trader, &pool_id, &1, &10_000, &0);
}

#[test]
fn test_pool_trade_limit_resets_next_block() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_pool(&env);
    let lp = Address::generate(&env);
    let trader = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.add_liquidity(&lp, &pool_id, &100_000_000, &100_000_000);
    client.set_guard_config(&admin, &1, &0, &0);

    client.swap(&trader, &pool_id, &1, &10_000, &0);
    // Advance to the next ledger; the per-block counter resets.
    env.ledger().with_mut(|li| li.sequence_number += 1);
    client.swap(&trader, &pool_id, &1, &10_000, &0);
}

// ---------------------------------------------------------------------------
// Deposit / withdraw cooldown
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "deposit/withdraw cooldown active")]
fn test_withdraw_cooldown_blocks_immediate_withdraw() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_pool(&env);
    let lp = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.set_guard_config(&admin, &0, &3600, &0);

    let lp_tokens = client.add_liquidity(&lp, &pool_id, &1_000_000, &1_000_000);
    // Withdrawing within the cooldown window must revert.
    client.remove_liquidity(&lp, &pool_id, &lp_tokens);
}

#[test]
fn test_withdraw_allowed_after_cooldown() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_pool(&env);
    let lp = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.set_guard_config(&admin, &0, &3600, &0);

    let lp_tokens = client.add_liquidity(&lp, &pool_id, &1_000_000, &1_000_000);
    env.ledger().with_mut(|li| li.timestamp += 3601);

    let (a, b) = client.remove_liquidity(&lp, &pool_id, &lp_tokens);
    assert!(a > 0 && b > 0);
}

// ---------------------------------------------------------------------------
// Price deviation
// ---------------------------------------------------------------------------

#[test]
fn test_large_swap_creates_price_deviation() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup_pool(&env);
    let lp = Address::generate(&env);
    let trader = Address::generate(&env);

    let pool_id = client.create_pool_with_tier(&lp, &1, &2, &FeeTier::Medium);
    client.add_liquidity(&lp, &pool_id, &1_000_000, &1_000_000);

    // Enable deviation alerts (200 bps threshold).
    client.set_guard_config(&admin, &0, &0, &200);

    // Anchor TWAP, let time pass so the window has weight at the old price.
    client.snapshot_twap(&pool_id);
    env.ledger().with_mut(|li| li.timestamp += 100);

    // A swap consuming a third of the pool moves the spot price sharply.
    client.swap(&trader, &pool_id, &1, &500_000, &0);

    // Spot now diverges far from the (flat) TWAP.
    let deviation = client.price_deviation_bps(&pool_id);
    assert!(deviation > 1_000, "expected large deviation, got {}", deviation);
}

// ---------------------------------------------------------------------------
// Marketplace per-block purchase limit
// ---------------------------------------------------------------------------

#[test]
#[should_panic(expected = "per-block trade limit exceeded")]
fn test_marketplace_per_block_purchase_limit() {
    let env = Env::default();
    env.mock_all_auths();

    let ec_id = env.register_contract(None, EmergencyControl);
    let ec_client = EmergencyControlClient::new(&env, &ec_id);
    let admin = Address::generate(&env);
    ec_client.initialize(&admin);

    let mp_id = env.register_contract(None, Marketplace);
    let mp_client = MarketplaceClient::new(&env, &mp_id);
    mp_client.initialize(&admin);

    // Allow a single purchase per block per buyer.
    mp_client.configure_flash_loan_guard(&admin, &1, &0);

    let buyer = Address::generate(&env);
    let asset_id = 1u64;

    mp_client.purchase(&buyer, &1, &100, &asset_id, &ec_id);
    // Second purchase in the same ledger must revert.
    mp_client.purchase(&buyer, &1, &100, &asset_id, &ec_id);
}

#[test]
fn test_marketplace_guard_defaults_permissive() {
    let env = Env::default();
    env.mock_all_auths();

    let ec_id = env.register_contract(None, EmergencyControl);
    let ec_client = EmergencyControlClient::new(&env, &ec_id);
    let admin = Address::generate(&env);
    ec_client.initialize(&admin);

    let mp_id = env.register_contract(None, Marketplace);
    let mp_client = MarketplaceClient::new(&env, &mp_id);
    mp_client.initialize(&admin);

    let buyer = Address::generate(&env);
    // With no guard configured, repeated purchases are unrestricted.
    assert!(mp_client.purchase(&buyer, &1, &100, &1, &ec_id));
    assert!(mp_client.purchase(&buyer, &1, &100, &1, &ec_id));
    assert!(mp_client.purchase(&buyer, &1, &100, &1, &ec_id));
}
