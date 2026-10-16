use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, Vec};

// ============================================================================
// Constants
// ============================================================================

/// Fixed-point precision used for the per-LP-token fee accumulators.
/// Fees are scaled by this factor when accrued so that small per-token
/// amounts are not lost to integer truncation.
const FEE_PRECISION: i128 = 1_000_000_000_000; // 1e12

/// Fixed-point scale used when recording spot prices for the TWAP oracle.
const PRICE_SCALE: i128 = 10_000_000; // 1e7

// ============================================================================
// Fee Tiers (Issue #209)
// ============================================================================

/// Pre-defined fee tiers for liquidity pools.
///
/// Each tier targets a different class of asset pair:
/// * `Low`    – 0.05% (5 bps)   – stable / correlated pairs.
/// * `Medium` – 0.30% (30 bps)  – standard pairs (default).
/// * `High`   – 1.00% (100 bps) – volatile / exotic pairs.
#[derive(Clone, Copy, PartialEq, Debug)]
#[contracttype]
pub enum FeeTier {
    Low,
    Medium,
    High,
}

impl FeeTier {
    /// Fee charged by this tier in basis points.
    pub fn to_bps(&self) -> u32 {
        match self {
            FeeTier::Low => 5,
            FeeTier::Medium => 30,
            FeeTier::High => 100,
        }
    }

    /// Classify an arbitrary basis-point value into the nearest tier.
    /// Used to keep backward compatibility with `create_pool`, which accepts
    /// a raw `fee_bps` value.
    fn classify(fee_bps: u32) -> FeeTier {
        // Midpoints: (5,30) -> 17, (30,100) -> 65
        if fee_bps <= 17 {
            FeeTier::Low
        } else if fee_bps <= 65 {
            FeeTier::Medium
        } else {
            FeeTier::High
        }
    }
}

// ============================================================================
// Data Types
// ============================================================================

#[derive(Clone)]
#[contracttype]
pub struct Pool {
    pub id: u64,
    pub asset_a: u64,
    pub asset_b: u64,
    pub reserve_a: i128,
    pub reserve_b: i128,
    pub total_lp: i128,
    pub fee_bps: u32,
    pub fee_tier: FeeTier,
    /// Cumulative fees (asset_a) accrued per LP token, scaled by `FEE_PRECISION`.
    pub acc_fee_per_share_a: i128,
    /// Cumulative fees (asset_b) accrued per LP token, scaled by `FEE_PRECISION`.
    pub acc_fee_per_share_b: i128,
    /// Time-weighted cumulative price of asset_a denominated in asset_b
    /// (scaled by `PRICE_SCALE`). Powers the TWAP oracle.
    pub price_cumulative: i128,
    /// Ledger timestamp at which `price_cumulative` was last updated.
    pub last_twap_ts: u64,
    pub creator: Address,
    pub active: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct LPPosition {
    pub pool_id: u64,
    pub provider: Address,
    pub lp_tokens: i128,
    /// Settled, claimable fees accrued in asset_a.
    pub fees_earned_a: i128,
    /// Settled, claimable fees accrued in asset_b.
    pub fees_earned_b: i128,
    /// Fee accounting checkpoints (reward-debt pattern).
    pub fee_debt_a: i128,
    pub fee_debt_b: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct SwapResult {
    pub output_amount: i128,
    pub fee_amount: i128,
    pub price_impact_bps: u32,
}

/// Flash-loan protection parameters (Issue #210). All limits default to the
/// permissive value (0 / unlimited) so that existing behaviour is preserved
/// until an admin explicitly tightens them.
#[derive(Clone)]
#[contracttype]
pub struct TradeGuardConfig {
    /// Maximum number of swaps allowed against a single pool within one ledger
    /// (block). `0` disables the limit.
    pub max_trades_per_block: u32,
    /// Minimum number of seconds that must elapse between adding liquidity and
    /// withdrawing it. `0` disables the cooldown.
    pub deposit_withdraw_cooldown: u64,
    /// Spot-vs-TWAP deviation (in bps) above which a `price_deviation_alert`
    /// event is emitted. `0` disables deviation alerts.
    pub max_price_deviation_bps: u32,
}

/// A stored TWAP anchor used to compute the average price over a window.
#[derive(Clone)]
#[contracttype]
pub struct TwapObservation {
    pub timestamp: u64,
    pub price_cumulative: i128,
}

#[derive(Clone)]
#[contracttype]
pub enum LPDataKey {
    Admin,
    PoolNonce,
    Pool(u64),
    Position(u64, Address), // (pool_id, provider)
    PoolIndex,              // Vec<u64> of all pool IDs
    // --- Flash-loan protection (Issue #210) ---
    GuardConfig,                  // TradeGuardConfig (instance)
    BlockTrades(u64, u32),        // (pool_id, ledger_seq) -> u32 swap count
    LastDeposit(u64, Address),    // (pool_id, provider) -> timestamp
    Twap(u64),                    // pool_id -> TwapObservation anchor
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct LiquidityPool;

#[contractimpl]
impl LiquidityPool {
    /// Initialize with an admin address.
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&LPDataKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&LPDataKey::Admin, &admin);
    }

    /// Create a new AMM pool for two assets using a raw fee (basis points).
    ///
    /// Retained for backward compatibility. The supplied `fee_bps` is mapped to
    /// the nearest pre-defined [`FeeTier`] for reporting purposes.
    pub fn create_pool(
        env: Env,
        creator: Address,
        asset_a: u64,
        asset_b: u64,
        fee_bps: u32,
    ) -> u64 {
        if fee_bps > 10_000 {
            panic!("fee_bps must not exceed 10000");
        }
        let tier = FeeTier::classify(fee_bps);
        Self::create_pool_internal(env, creator, asset_a, asset_b, fee_bps, tier)
    }

    /// Create a new AMM pool choosing one of the pre-defined fee tiers
    /// (Issue #209). The pool's `fee_bps` is derived from the tier.
    pub fn create_pool_with_tier(
        env: Env,
        creator: Address,
        asset_a: u64,
        asset_b: u64,
        tier: FeeTier,
    ) -> u64 {
        let fee_bps = tier.to_bps();
        Self::create_pool_internal(env, creator, asset_a, asset_b, fee_bps, tier)
    }

    fn create_pool_internal(
        env: Env,
        creator: Address,
        asset_a: u64,
        asset_b: u64,
        fee_bps: u32,
        tier: FeeTier,
    ) -> u64 {
        creator.require_auth();

        if asset_a == asset_b {
            panic!("pool assets must be different");
        }

        // Canonical asset ordering to prevent duplicate pools
        let (a, b) = if asset_a < asset_b {
            (asset_a, asset_b)
        } else {
            (asset_b, asset_a)
        };

        // Check for duplicate pool
        let pool_ids: Vec<u64> = env
            .storage()
            .instance()
            .get(&LPDataKey::PoolIndex)
            .unwrap_or(Vec::new(&env));
        for pid in pool_ids.iter() {
            if let Some(p) = env.storage().persistent().get::<_, Pool>(&LPDataKey::Pool(pid)) {
                if p.asset_a == a && p.asset_b == b {
                    panic!("pool already exists for this pair");
                }
            }
        }

        let pool_id: u64 = env
            .storage()
            .instance()
            .get(&LPDataKey::PoolNonce)
            .unwrap_or(0)
            + 1;
        env.storage().instance().set(&LPDataKey::PoolNonce, &pool_id);

        let pool = Pool {
            id: pool_id,
            asset_a: a,
            asset_b: b,
            reserve_a: 0,
            reserve_b: 0,
            total_lp: 0,
            fee_bps,
            fee_tier: tier,
            acc_fee_per_share_a: 0,
            acc_fee_per_share_b: 0,
            price_cumulative: 0,
            last_twap_ts: env.ledger().timestamp(),
            creator: creator.clone(),
            active: true,
        };

        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);

        let mut index: Vec<u64> = pool_ids;
        index.push_back(pool_id);
        env.storage().instance().set(&LPDataKey::PoolIndex, &index);

        env.events().publish(
            (Symbol::new(&env, "pool_created"), pool_id),
            (creator, a, b, fee_bps, tier),
        );

        pool_id
    }

    /// Add liquidity to a pool; issues LP tokens.
    pub fn add_liquidity(
        env: Env,
        provider: Address,
        pool_id: u64,
        amount_a: i128,
        amount_b: i128,
    ) -> i128 {
        provider.require_auth();

        if amount_a <= 0 || amount_b <= 0 {
            panic!("deposit amounts must be positive");
        }

        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        if !pool.active {
            panic!("pool is inactive");
        }

        // Update the TWAP accumulator using the reserves in effect so far.
        Self::accumulate_price(&env, &mut pool);

        let lp_tokens = if pool.total_lp == 0 {
            // Initial liquidity — integer approximation of sqrt(a*b)
            Self::isqrt(amount_a.saturating_mul(amount_b))
        } else {
            let lp_a = amount_a.saturating_mul(pool.total_lp) / pool.reserve_a;
            let lp_b = amount_b.saturating_mul(pool.total_lp) / pool.reserve_b;
            lp_a.min(lp_b)
        };

        if lp_tokens <= 0 {
            panic!("deposit too small to issue LP tokens");
        }

        pool.reserve_a = pool.reserve_a.saturating_add(amount_a);
        pool.reserve_b = pool.reserve_b.saturating_add(amount_b);
        pool.total_lp = pool.total_lp.saturating_add(lp_tokens);

        let mut position: LPPosition = env
            .storage()
            .persistent()
            .get(&LPDataKey::Position(pool_id, provider.clone()))
            .unwrap_or(LPPosition {
                pool_id,
                provider: provider.clone(),
                lp_tokens: 0,
                fees_earned_a: 0,
                fees_earned_b: 0,
                fee_debt_a: 0,
                fee_debt_b: 0,
            });

        // Settle any fees accrued on the existing balance before changing it.
        Self::settle_position(&pool, &mut position);
        position.lp_tokens = position.lp_tokens.saturating_add(lp_tokens);
        Self::reset_fee_debt(&pool, &mut position);

        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);
        env.storage()
            .persistent()
            .set(&LPDataKey::Position(pool_id, provider.clone()), &position);

        // Record the deposit time to enforce withdrawal cooldowns (Issue #210).
        env.storage().persistent().set(
            &LPDataKey::LastDeposit(pool_id, provider.clone()),
            &env.ledger().timestamp(),
        );

        env.events().publish(
            (Symbol::new(&env, "liquidity_added"), pool_id),
            (provider, amount_a, amount_b, lp_tokens),
        );

        lp_tokens
    }

    /// Remove liquidity by burning LP tokens; returns (amount_a, amount_b).
    pub fn remove_liquidity(
        env: Env,
        provider: Address,
        pool_id: u64,
        lp_tokens: i128,
    ) -> (i128, i128) {
        provider.require_auth();

        if lp_tokens <= 0 {
            panic!("lp_tokens must be positive");
        }

        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        if pool.total_lp == 0 {
            panic!("pool has no liquidity");
        }

        // Enforce the deposit/withdraw cooldown (Issue #210).
        Self::enforce_withdraw_cooldown(&env, pool_id, &provider);

        Self::accumulate_price(&env, &mut pool);

        let mut position: LPPosition = env
            .storage()
            .persistent()
            .get(&LPDataKey::Position(pool_id, provider.clone()))
            .expect("no LP position found");

        if position.lp_tokens < lp_tokens {
            panic!("insufficient LP tokens");
        }

        let withdraw_a = lp_tokens.saturating_mul(pool.reserve_a) / pool.total_lp;
        let withdraw_b = lp_tokens.saturating_mul(pool.reserve_b) / pool.total_lp;

        pool.reserve_a -= withdraw_a;
        pool.reserve_b -= withdraw_b;
        pool.total_lp -= lp_tokens;
        if pool.reserve_a < 0 {
            pool.reserve_a = 0;
        }
        if pool.reserve_b < 0 {
            pool.reserve_b = 0;
        }

        // Settle fees on the current balance, then shrink the position.
        Self::settle_position(&pool, &mut position);
        position.lp_tokens -= lp_tokens;
        Self::reset_fee_debt(&pool, &mut position);

        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);
        env.storage()
            .persistent()
            .set(&LPDataKey::Position(pool_id, provider.clone()), &position);

        env.events().publish(
            (Symbol::new(&env, "liquidity_removed"), pool_id),
            (provider, withdraw_a, withdraw_b),
        );

        (withdraw_a, withdraw_b)
    }

    /// Execute a swap using the constant-product AMM formula.
    ///
    /// The fee is diverted to the per-LP fee accumulators (Issue #209) rather
    /// than compounding into the reserves, so liquidity providers can claim
    /// their proportional share via [`LiquidityPool::claim_fees`].
    pub fn swap(
        env: Env,
        trader: Address,
        pool_id: u64,
        input_asset: u64,
        input_amount: i128,
        min_output: i128,
    ) -> SwapResult {
        trader.require_auth();

        if input_amount <= 0 {
            panic!("input amount must be positive");
        }

        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        if !pool.active {
            panic!("pool is inactive");
        }

        // Flash-loan guard: cap the number of swaps per pool per ledger.
        Self::enforce_block_trade_limit(&env, pool_id);

        // Update the TWAP accumulator with the pre-swap reserves.
        Self::accumulate_price(&env, &mut pool);

        let (reserve_in, reserve_out, is_a_to_b) = if input_asset == pool.asset_a {
            (pool.reserve_a, pool.reserve_b, true)
        } else if input_asset == pool.asset_b {
            (pool.reserve_b, pool.reserve_a, false)
        } else {
            panic!("input asset not in pool");
        };

        if reserve_in <= 0 || reserve_out <= 0 {
            panic!("insufficient pool liquidity");
        }

        let fee_bps = pool.fee_bps as i128;
        let amount_in_after_fee = input_amount.saturating_mul(10_000 - fee_bps) / 10_000;
        let fee_amount = input_amount - amount_in_after_fee;

        // x * y = k → output = dy = y * dx' / (x + dx')
        let output_amount = amount_in_after_fee.saturating_mul(reserve_out)
            / reserve_in.saturating_add(amount_in_after_fee);

        if output_amount <= 0 {
            panic!("swap output too small");
        }

        if min_output > 0 && output_amount < min_output {
            panic!("slippage too high");
        }

        let price_impact_bps = (output_amount.saturating_mul(10_000) / reserve_out) as u32;

        // Accrue the fee proportionally to all LPs via the per-share accumulator.
        if pool.total_lp > 0 && fee_amount > 0 {
            let per_share = fee_amount.saturating_mul(FEE_PRECISION) / pool.total_lp;
            if is_a_to_b {
                pool.acc_fee_per_share_a = pool.acc_fee_per_share_a.saturating_add(per_share);
            } else {
                pool.acc_fee_per_share_b = pool.acc_fee_per_share_b.saturating_add(per_share);
            }
        }

        // Only the post-fee amount enters the reserves; the fee sits in the
        // accumulator awaiting LP claims.
        if is_a_to_b {
            pool.reserve_a = pool.reserve_a.saturating_add(amount_in_after_fee);
            pool.reserve_b -= output_amount;
            if pool.reserve_b < 0 {
                pool.reserve_b = 0;
            }
        } else {
            pool.reserve_b = pool.reserve_b.saturating_add(amount_in_after_fee);
            pool.reserve_a -= output_amount;
            if pool.reserve_a < 0 {
                pool.reserve_a = 0;
            }
        }

        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);

        // Emit a price-deviation alert if the post-swap spot price strays too
        // far from the TWAP (Issue #210).
        Self::maybe_emit_deviation_alert(&env, &pool);

        env.events().publish(
            (Symbol::new(&env, "swap_executed"), pool_id),
            (trader, input_asset, input_amount, output_amount, fee_amount),
        );

        SwapResult {
            output_amount,
            fee_amount,
            price_impact_bps,
        }
    }

    /// Claim the caller's accrued share of pool fees (Issue #209).
    /// Returns the claimed (asset_a, asset_b) fee amounts.
    pub fn claim_fees(env: Env, provider: Address, pool_id: u64) -> (i128, i128) {
        provider.require_auth();

        let pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        let mut position: LPPosition = env
            .storage()
            .persistent()
            .get(&LPDataKey::Position(pool_id, provider.clone()))
            .expect("no LP position found");

        Self::settle_position(&pool, &mut position);

        let claimed_a = position.fees_earned_a;
        let claimed_b = position.fees_earned_b;
        position.fees_earned_a = 0;
        position.fees_earned_b = 0;

        env.storage()
            .persistent()
            .set(&LPDataKey::Position(pool_id, provider.clone()), &position);

        env.events().publish(
            (Symbol::new(&env, "fees_claimed"), pool_id),
            (provider, claimed_a, claimed_b),
        );

        (claimed_a, claimed_b)
    }

    /// Return the currently claimable (unsettled + settled) fees for a provider.
    pub fn pending_fees(env: Env, pool_id: u64, provider: Address) -> (i128, i128) {
        let pool: Pool = match env.storage().persistent().get(&LPDataKey::Pool(pool_id)) {
            Some(p) => p,
            None => return (0, 0),
        };
        let position: LPPosition = match env
            .storage()
            .persistent()
            .get(&LPDataKey::Position(pool_id, provider))
        {
            Some(p) => p,
            None => return (0, 0),
        };
        let (pending_a, pending_b) = Self::pending(&pool, &position);
        (
            position.fees_earned_a.saturating_add(pending_a),
            position.fees_earned_b.saturating_add(pending_b),
        )
    }

    /// Migrate a pool to a new fee tier (Issue #209).
    ///
    /// Outstanding fees are settled into the accumulator before the change so
    /// that no LP is over- or under-credited across the migration boundary.
    /// Callable by the pool creator or the contract admin.
    pub fn migrate_fee_tier(env: Env, caller: Address, pool_id: u64, new_tier: FeeTier) {
        caller.require_auth();

        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        let is_creator = caller == pool.creator;
        let is_admin = env
            .storage()
            .instance()
            .get::<_, Address>(&LPDataKey::Admin)
            .map(|a| a == caller)
            .unwrap_or(false);
        if !is_creator && !is_admin {
            panic!("only pool creator or admin can migrate fee tier");
        }

        let old_tier = pool.fee_tier;
        pool.fee_tier = new_tier;
        pool.fee_bps = new_tier.to_bps();

        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);

        env.events().publish(
            (Symbol::new(&env, "fee_tier_migrated"), pool_id),
            (caller, old_tier, new_tier, pool.fee_bps),
        );
    }

    /// Recommend a fee tier for a pair based on its observed volatility and
    /// liquidity depth (Issue #209). Pure helper — does not touch storage.
    ///
    /// * `volatility_bps` – recent price volatility expressed in basis points.
    /// * `liquidity_depth` – total value locked (in asset_b units) available.
    pub fn recommend_tier(_env: Env, volatility_bps: u32, liquidity_depth: i128) -> FeeTier {
        // Highly volatile pairs warrant the highest fee to compensate LPs for
        // impermanent loss risk.
        if volatility_bps >= 500 {
            return FeeTier::High;
        }
        // Low-volatility, deeply-liquid pairs (e.g. stable pairs) can sustain
        // the cheapest tier and still reward LPs on volume.
        if volatility_bps <= 50 && liquidity_depth >= 1_000_000 {
            return FeeTier::Low;
        }
        FeeTier::Medium
    }

    // -----------------------------------------------------------------------
    // TWAP oracle (Issue #210)
    // -----------------------------------------------------------------------

    /// Store the current cumulative price as the new TWAP anchor. Permissionless;
    /// anyone may refresh the window the average is measured over.
    pub fn snapshot_twap(env: Env, pool_id: u64) {
        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");
        Self::accumulate_price(&env, &mut pool);
        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);

        let obs = TwapObservation {
            timestamp: env.ledger().timestamp(),
            price_cumulative: pool.price_cumulative,
        };
        env.storage().persistent().set(&LPDataKey::Twap(pool_id), &obs);

        env.events().publish(
            (Symbol::new(&env, "twap_snapshot"), pool_id),
            (obs.timestamp, obs.price_cumulative),
        );
    }

    /// Return the time-weighted average price of asset_a (denominated in
    /// asset_b, scaled by `PRICE_SCALE`) since the last stored anchor.
    ///
    /// Falls back to the current spot price when no anchor exists or no time
    /// has elapsed since the anchor was taken.
    pub fn get_twap(env: Env, pool_id: u64) -> i128 {
        let pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");

        let now = env.ledger().timestamp();
        let cum_now = Self::current_cumulative(&pool, now);

        if let Some(anchor) = env
            .storage()
            .persistent()
            .get::<_, TwapObservation>(&LPDataKey::Twap(pool_id))
        {
            let dt = now.saturating_sub(anchor.timestamp);
            if dt > 0 {
                return (cum_now - anchor.price_cumulative) / dt as i128;
            }
        }
        Self::spot_price(&pool)
    }

    /// Current spot price of asset_a in asset_b, scaled by `PRICE_SCALE`.
    pub fn get_spot_price(env: Env, pool_id: u64) -> i128 {
        let pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");
        Self::spot_price(&pool)
    }

    /// Current deviation (in bps) of the spot price from the TWAP.
    pub fn price_deviation_bps(env: Env, pool_id: u64) -> u32 {
        let pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");
        let spot = Self::spot_price(&pool);
        let twap = Self::get_twap(env, pool_id);
        Self::deviation_bps(spot, twap)
    }

    // -----------------------------------------------------------------------
    // Admin: flash-loan guard configuration (Issue #210)
    // -----------------------------------------------------------------------

    /// Configure the flash-loan protection parameters. Admin only.
    pub fn set_guard_config(
        env: Env,
        admin: Address,
        max_trades_per_block: u32,
        deposit_withdraw_cooldown: u64,
        max_price_deviation_bps: u32,
    ) {
        Self::require_admin(&env, &admin);
        let cfg = TradeGuardConfig {
            max_trades_per_block,
            deposit_withdraw_cooldown,
            max_price_deviation_bps,
        };
        env.storage().instance().set(&LPDataKey::GuardConfig, &cfg);
        env.events().publish(
            (Symbol::new(&env, "guard_config_set"),),
            (max_trades_per_block, deposit_withdraw_cooldown, max_price_deviation_bps),
        );
    }

    /// Return the active flash-loan guard configuration (defaults to permissive).
    pub fn get_guard_config(env: Env) -> TradeGuardConfig {
        Self::guard_config(&env)
    }

    // -----------------------------------------------------------------------
    // Query helpers
    // -----------------------------------------------------------------------

    /// Get pool details.
    pub fn get_pool(env: Env, pool_id: u64) -> Option<Pool> {
        env.storage().persistent().get(&LPDataKey::Pool(pool_id))
    }

    /// Get LP position for a provider.
    pub fn get_position(env: Env, pool_id: u64, provider: Address) -> Option<LPPosition> {
        env.storage()
            .persistent()
            .get(&LPDataKey::Position(pool_id, provider))
    }

    /// Get all pool IDs.
    pub fn get_pool_ids(env: Env) -> Vec<u64> {
        env.storage()
            .instance()
            .get(&LPDataKey::PoolIndex)
            .unwrap_or(Vec::new(&env))
    }

    /// Admin: deactivate a pool.
    pub fn deactivate_pool(env: Env, admin: Address, pool_id: u64) {
        Self::require_admin(&env, &admin);
        let mut pool: Pool = env
            .storage()
            .persistent()
            .get(&LPDataKey::Pool(pool_id))
            .expect("pool not found");
        pool.active = false;
        env.storage().persistent().set(&LPDataKey::Pool(pool_id), &pool);
        env.events()
            .publish((Symbol::new(&env, "pool_deactivated"), pool_id), admin);
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    /// Spot price of asset_a in asset_b, scaled by `PRICE_SCALE`.
    fn spot_price(pool: &Pool) -> i128 {
        if pool.reserve_a <= 0 {
            return 0;
        }
        pool.reserve_b.saturating_mul(PRICE_SCALE) / pool.reserve_a
    }

    /// Compute the cumulative price as of `now`, without persisting it.
    fn current_cumulative(pool: &Pool, now: u64) -> i128 {
        let elapsed = now.saturating_sub(pool.last_twap_ts);
        if elapsed == 0 || pool.reserve_a <= 0 {
            return pool.price_cumulative;
        }
        let spot = Self::spot_price(pool);
        pool.price_cumulative
            .saturating_add(spot.saturating_mul(elapsed as i128))
    }

    /// Advance the pool's stored cumulative price up to the current timestamp.
    fn accumulate_price(env: &Env, pool: &mut Pool) {
        let now = env.ledger().timestamp();
        pool.price_cumulative = Self::current_cumulative(pool, now);
        pool.last_twap_ts = now;
    }

    /// Absolute deviation between two prices, in basis points.
    fn deviation_bps(spot: i128, reference: i128) -> u32 {
        if reference <= 0 {
            return 0;
        }
        let diff = (spot - reference).abs();
        (diff.saturating_mul(10_000) / reference) as u32
    }

    fn maybe_emit_deviation_alert(env: &Env, pool: &Pool) {
        let cfg = Self::guard_config(env);
        if cfg.max_price_deviation_bps == 0 {
            return;
        }
        let spot = Self::spot_price(pool);
        let twap = Self::get_twap(env.clone(), pool.id);
        let dev = Self::deviation_bps(spot, twap);
        if dev > cfg.max_price_deviation_bps {
            env.events().publish(
                (Symbol::new(env, "price_deviation_alert"), pool.id),
                (spot, twap, dev),
            );
        }
    }

    fn guard_config(env: &Env) -> TradeGuardConfig {
        env.storage()
            .instance()
            .get(&LPDataKey::GuardConfig)
            .unwrap_or(TradeGuardConfig {
                max_trades_per_block: 0,
                deposit_withdraw_cooldown: 0,
                max_price_deviation_bps: 0,
            })
    }

    fn enforce_block_trade_limit(env: &Env, pool_id: u64) {
        let cfg = Self::guard_config(env);
        if cfg.max_trades_per_block == 0 {
            return;
        }
        let seq = env.ledger().sequence();
        let key = LPDataKey::BlockTrades(pool_id, seq);
        let count: u32 = env.storage().persistent().get(&key).unwrap_or(0);
        if count >= cfg.max_trades_per_block {
            panic!("per-block trade limit exceeded");
        }
        env.storage().persistent().set(&key, &(count + 1));
    }

    fn enforce_withdraw_cooldown(env: &Env, pool_id: u64, provider: &Address) {
        let cfg = Self::guard_config(env);
        if cfg.deposit_withdraw_cooldown == 0 {
            return;
        }
        if let Some(last) = env
            .storage()
            .persistent()
            .get::<_, u64>(&LPDataKey::LastDeposit(pool_id, provider.clone()))
        {
            let unlock = last.saturating_add(cfg.deposit_withdraw_cooldown);
            if env.ledger().timestamp() < unlock {
                panic!("deposit/withdraw cooldown active");
            }
        }
    }

    /// Pending (unsettled) fees for a position, given the current accumulator.
    fn pending(pool: &Pool, position: &LPPosition) -> (i128, i128) {
        let acc_a = position.lp_tokens.saturating_mul(pool.acc_fee_per_share_a) / FEE_PRECISION;
        let acc_b = position.lp_tokens.saturating_mul(pool.acc_fee_per_share_b) / FEE_PRECISION;
        (
            (acc_a - position.fee_debt_a).max(0),
            (acc_b - position.fee_debt_b).max(0),
        )
    }

    /// Move pending fees into the settled `fees_earned` buckets.
    fn settle_position(pool: &Pool, position: &mut LPPosition) {
        let (pending_a, pending_b) = Self::pending(pool, position);
        position.fees_earned_a = position.fees_earned_a.saturating_add(pending_a);
        position.fees_earned_b = position.fees_earned_b.saturating_add(pending_b);
        Self::reset_fee_debt(pool, position);
    }

    /// Re-checkpoint the position's fee debt to the current accumulator value.
    fn reset_fee_debt(pool: &Pool, position: &mut LPPosition) {
        position.fee_debt_a =
            position.lp_tokens.saturating_mul(pool.acc_fee_per_share_a) / FEE_PRECISION;
        position.fee_debt_b =
            position.lp_tokens.saturating_mul(pool.acc_fee_per_share_b) / FEE_PRECISION;
    }

    fn isqrt(n: i128) -> i128 {
        if n <= 0 {
            return 0;
        }
        let mut x = n;
        let mut y = (x + 1) / 2;
        while y < x {
            x = y;
            y = (x + n / x) / 2;
        }
        x
    }

    fn require_admin(env: &Env, caller: &Address) {
        caller.require_auth();
        let admin: Address = env
            .storage()
            .instance()
            .get(&LPDataKey::Admin)
            .expect("admin not set");
        if *caller != admin {
            panic!("caller is not admin");
        }
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::Address as _;
    use soroban_sdk::{Address, Env};

    fn setup(env: &Env) -> (LiquidityPoolClient, Address) {
        let id = env.register_contract(None, LiquidityPool);
        let client = LiquidityPoolClient::new(env, &id);
        let admin = Address::generate(env);
        client.initialize(&admin);
        (client, admin)
    }

    #[test]
    fn test_create_pool_and_add_liquidity() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let creator = Address::generate(&env);

        let pool_id = client.create_pool(&creator, &1, &2, &30);
        assert_eq!(pool_id, 1);

        let lp = client.add_liquidity(&creator, &pool_id, &1_000_000, &1_000_000);
        assert!(lp > 0);

        let pool = client.get_pool(&pool_id).unwrap();
        assert_eq!(pool.reserve_a, 1_000_000);
        assert_eq!(pool.reserve_b, 1_000_000);
        assert_eq!(pool.fee_tier, FeeTier::Medium);
    }

    #[test]
    fn test_swap() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let provider = Address::generate(&env);
        let trader = Address::generate(&env);

        let pool_id = client.create_pool(&provider, &1, &2, &30);
        client.add_liquidity(&provider, &pool_id, &10_000_000, &10_000_000);

        let result = client.swap(&trader, &pool_id, &1, &100_000, &0);
        assert!(result.output_amount > 0);
        assert!(result.fee_amount > 0);
    }

    #[test]
    fn test_remove_liquidity() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let provider = Address::generate(&env);

        let pool_id = client.create_pool(&provider, &10, &20, &30);
        let lp_tokens = client.add_liquidity(&provider, &pool_id, &5_000_000, &5_000_000);

        let (a, b) = client.remove_liquidity(&provider, &pool_id, &lp_tokens);
        assert!(a > 0);
        assert!(b > 0);
    }

    #[test]
    #[should_panic(expected = "pool already exists for this pair")]
    fn test_duplicate_pool_rejected() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let creator = Address::generate(&env);

        client.create_pool(&creator, &1, &2, &30);
        client.create_pool(&creator, &1, &2, &30); // should panic
    }

    #[test]
    fn test_create_pool_with_tier_sets_fee() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let creator = Address::generate(&env);

        let pool_id = client.create_pool_with_tier(&creator, &1, &2, &FeeTier::Low);
        let pool = client.get_pool(&pool_id).unwrap();
        assert_eq!(pool.fee_tier, FeeTier::Low);
        assert_eq!(pool.fee_bps, 5);
    }
}
