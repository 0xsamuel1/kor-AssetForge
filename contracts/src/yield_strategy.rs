use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol};

use crate::staking_rewards::StrategyType;

// ============================================================================
// Data Types
// ============================================================================

/// Configuration for a specific yield strategy applied to an asset.
#[derive(Clone)]
#[contracttype]
pub struct YieldStrategyConfig {
    pub strategy: StrategyType,
    /// Base APR in basis points before strategy multiplier (e.g. 500 = 5%)
    pub base_apr_bps: u32,
    /// Minimum lock period in seconds before rewards can be claimed
    pub lock_period: u64,
    /// Whether auto-compounding is enabled for this strategy
    pub auto_compound: bool,
    /// Risk level 1–5 (1 = lowest, 5 = highest)
    pub risk_level: u32,
}

/// Aggregated performance metrics for a strategy on a specific asset.
#[derive(Clone)]
#[contracttype]
pub struct StrategyPerformance {
    pub total_staked: i128,
    pub total_rewards_distributed: i128,
    pub staker_count: u32,
    pub last_updated: u64,
}

#[derive(Clone)]
#[contracttype]
pub enum YieldStrategyDataKey {
    Admin,
    StrategyConfig(u64, StrategyType), // (asset_id, strategy)
    Performance(u64, StrategyType),    // (asset_id, strategy)
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct YieldStrategy;

#[contractimpl]
impl YieldStrategy {
    /// Initialize the yield strategy registry.
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&YieldStrategyDataKey::Admin) {
            panic!("already initialized");
        }
        env.storage()
            .instance()
            .set(&YieldStrategyDataKey::Admin, &admin);
    }

    /// Admin: configure a yield strategy for an asset.
    pub fn set_strategy_config(
        env: Env,
        admin: Address,
        asset_id: u64,
        strategy: StrategyType,
        config: YieldStrategyConfig,
    ) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&YieldStrategyDataKey::Admin)
            .expect("not initialized");
        if admin != stored_admin {
            panic!("caller is not admin");
        }
        if config.base_apr_bps > 100_000 {
            panic!("base_apr_bps must not exceed 100000");
        }
        if config.risk_level < 1 || config.risk_level > 5 {
            panic!("risk_level must be between 1 and 5");
        }

        let key = YieldStrategyDataKey::StrategyConfig(asset_id, strategy.clone());
        env.storage().persistent().set(&key, &config);

        env.events().publish(
            (Symbol::new(&env, "strategy_config_set"), asset_id),
            (strategy, config.base_apr_bps, config.risk_level),
        );
    }

    /// Get strategy configuration for an asset+strategy pair.
    pub fn get_strategy_config(
        env: Env,
        asset_id: u64,
        strategy: StrategyType,
    ) -> Option<YieldStrategyConfig> {
        env.storage()
            .persistent()
            .get(&YieldStrategyDataKey::StrategyConfig(asset_id, strategy))
    }

    /// Record a stake event (called by integrating systems to update performance).
    pub fn record_stake(env: Env, asset_id: u64, strategy: StrategyType, amount: i128) {
        if amount <= 0 {
            panic!("amount must be positive");
        }
        let key = YieldStrategyDataKey::Performance(asset_id, strategy);
        let now = env.ledger().timestamp();
        let mut perf: StrategyPerformance = env
            .storage()
            .persistent()
            .get(&key)
            .unwrap_or(StrategyPerformance {
                total_staked: 0,
                total_rewards_distributed: 0,
                staker_count: 0,
                last_updated: now,
            });

        perf.total_staked = perf.total_staked.checked_add(amount).unwrap_or(perf.total_staked);
        perf.staker_count += 1;
        perf.last_updated = now;

        env.storage().persistent().set(&key, &perf);
    }

    /// Record a reward distribution event to update performance metrics.
    pub fn record_reward(env: Env, asset_id: u64, strategy: StrategyType, reward: i128) {
        if reward <= 0 {
            panic!("reward must be positive");
        }
        let key = YieldStrategyDataKey::Performance(asset_id, strategy);
        let now = env.ledger().timestamp();
        let mut perf: StrategyPerformance = env
            .storage()
            .persistent()
            .get(&key)
            .unwrap_or(StrategyPerformance {
                total_staked: 0,
                total_rewards_distributed: 0,
                staker_count: 0,
                last_updated: now,
            });

        perf.total_rewards_distributed = perf
            .total_rewards_distributed
            .checked_add(reward)
            .unwrap_or(perf.total_rewards_distributed);
        perf.last_updated = now;

        env.storage().persistent().set(&key, &perf);
    }

    /// Get performance metrics for an asset+strategy pair.
    pub fn get_performance(
        env: Env,
        asset_id: u64,
        strategy: StrategyType,
    ) -> Option<StrategyPerformance> {
        env.storage()
            .persistent()
            .get(&YieldStrategyDataKey::Performance(asset_id, strategy))
    }
}
