#![cfg(test)]

use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

use kor_assetforge_contracts::staking_rewards::{
    StakingRewards, StakingRewardsClient, StrategyType,
};
use kor_assetforge_contracts::yield_strategy::{YieldStrategy, YieldStrategyClient, YieldStrategyConfig};

fn setup_staking(env: &Env) -> (StakingRewardsClient, Address) {
    let id = env.register_contract(None, StakingRewards);
    let client = StakingRewardsClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

fn setup_yield_strategy(env: &Env) -> (YieldStrategyClient, Address) {
    let id = env.register_contract(None, YieldStrategy);
    let client = YieldStrategyClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_stake_with_strategy() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let staker = Address::generate(&env);
    let asset_id: u64 = 1;

    client.configure_rewards(&admin, &asset_id, &1000, &0);
    client.stake_with_strategy(&staker, &asset_id, &5_000_000, &StrategyType::Aggressive);

    let pos = client.get_stake_position(&staker, &asset_id).unwrap();
    assert_eq!(pos.amount, 5_000_000);
    assert!(pos.active);
    assert_eq!(pos.strategy, StrategyType::Aggressive);
}

#[test]
fn test_aggressive_earns_more_than_conservative() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let staker_con = Address::generate(&env);
    let staker_agg = Address::generate(&env);
    let asset_id: u64 = 2;

    // 10% APR base, no min duration
    client.configure_rewards(&admin, &asset_id, &1000, &0);
    client.stake_with_strategy(&staker_con, &asset_id, &10_000_000, &StrategyType::Conservative);
    client.stake_with_strategy(&staker_agg, &asset_id, &10_000_000, &StrategyType::Aggressive);

    // Advance ~1 year
    env.ledger().with_mut(|li| {
        li.timestamp = 31_536_000;
    });

    let rewards_con = client.claim_rewards(&staker_con, &asset_id);
    let rewards_agg = client.claim_rewards(&staker_agg, &asset_id);

    // Aggressive (1.2×) should earn more than Conservative (0.8×)
    assert!(rewards_agg > rewards_con, "aggressive should earn more than conservative");
    // Aggressive earns 1.5× conservative (1.2/0.8 = 1.5)
    assert_eq!(rewards_agg, rewards_con * 15 / 10);
}

#[test]
fn test_balanced_is_default_strategy() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let staker = Address::generate(&env);
    let asset_id: u64 = 3;

    client.configure_rewards(&admin, &asset_id, &1000, &0);
    // Use stake() (not stake_with_strategy) — defaults to Balanced
    client.stake(&staker, &asset_id, &1_000_000);

    let pos = client.get_stake_position(&staker, &asset_id).unwrap();
    assert_eq!(pos.strategy, StrategyType::Balanced);
}

#[test]
fn test_compound_rewards() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let staker = Address::generate(&env);
    let asset_id: u64 = 4;

    client.configure_rewards(&admin, &asset_id, &1000, &0);
    client.stake_with_strategy(&staker, &asset_id, &10_000_000, &StrategyType::Balanced);

    env.ledger().with_mut(|li| {
        li.timestamp = 31_536_000;
    });

    let pos_before = client.get_stake_position(&staker, &asset_id).unwrap();
    client.compound_rewards(&staker, &asset_id);
    let pos_after = client.get_stake_position(&staker, &asset_id).unwrap();

    // Principal increased, accrued rewards reset to 0
    assert!(pos_after.amount > pos_before.amount, "amount should grow after compounding");
    assert_eq!(pos_after.accrued_rewards, 0);
}

#[test]
fn test_set_auto_compound() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let asset_id: u64 = 5;

    client.configure_rewards(&admin, &asset_id, &500, &0);
    client.set_auto_compound(&admin, &asset_id, &true);

    let config = client.get_stake_position(
        &Address::generate(&env),
        &asset_id,
    );
    // Position doesn't exist yet, but config was set (we can't query config directly)
    // Just ensure no panic occurred
    assert!(config.is_none());
}

#[test]
fn test_strategy_performance_tracking() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_staking(&env);
    let staker1 = Address::generate(&env);
    let staker2 = Address::generate(&env);
    let asset_id: u64 = 6;

    client.configure_rewards(&admin, &asset_id, &1000, &0);
    client.stake_with_strategy(&staker1, &asset_id, &3_000_000, &StrategyType::Aggressive);
    client.stake_with_strategy(&staker2, &asset_id, &2_000_000, &StrategyType::Aggressive);

    let perf = client.get_strategy_performance(&asset_id, &StrategyType::Aggressive).unwrap();
    assert_eq!(perf.total_staked, 5_000_000);
    assert_eq!(perf.staker_count, 2);
}

#[test]
fn test_yield_strategy_contract_config() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_yield_strategy(&env);
    let asset_id: u64 = 1;

    let config = YieldStrategyConfig {
        strategy: StrategyType::Aggressive,
        base_apr_bps: 1500,
        lock_period: 86_400,
        auto_compound: true,
        risk_level: 4,
    };

    client.set_strategy_config(&admin, &asset_id, &StrategyType::Aggressive, &config);

    let retrieved = client.get_strategy_config(&asset_id, &StrategyType::Aggressive).unwrap();
    assert_eq!(retrieved.base_apr_bps, 1500);
    assert_eq!(retrieved.risk_level, 4);
    assert!(retrieved.auto_compound);
}

#[test]
fn test_yield_strategy_performance_metrics() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_yield_strategy(&env);
    let asset_id: u64 = 2;

    let config = YieldStrategyConfig {
        strategy: StrategyType::Conservative,
        base_apr_bps: 300,
        lock_period: 0,
        auto_compound: false,
        risk_level: 1,
    };
    client.set_strategy_config(&admin, &asset_id, &StrategyType::Conservative, &config);

    client.record_stake(&asset_id, &StrategyType::Conservative, &1_000_000);
    client.record_stake(&asset_id, &StrategyType::Conservative, &500_000);
    client.record_reward(&asset_id, &StrategyType::Conservative, &15_000);

    let perf = client.get_performance(&asset_id, &StrategyType::Conservative).unwrap();
    assert_eq!(perf.total_staked, 1_500_000);
    assert_eq!(perf.total_rewards_distributed, 15_000);
    assert_eq!(perf.staker_count, 2);
}
