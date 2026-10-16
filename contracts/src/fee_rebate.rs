use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol};

/// 30-day volume window in seconds.
pub const VOLUME_WINDOW: u64 = 30 * 24 * 3600;

/// Tier thresholds (volume amounts) and rebate basis points (bps).
/// Tier 1: >= 10_000  → 10% rebate (1000 bps)
/// Tier 2: >= 50_000  → 25% rebate (2500 bps)
/// Tier 3: >= 100_000 → 50% rebate (5000 bps)
pub const TIER1_THRESHOLD: i128 = 10_000;
pub const TIER2_THRESHOLD: i128 = 50_000;
pub const TIER3_THRESHOLD: i128 = 100_000;

pub const TIER1_BPS: i128 = 1000; // 10%
pub const TIER2_BPS: i128 = 2500; // 25%
pub const TIER3_BPS: i128 = 5000; // 50%

#[derive(Clone)]
#[contracttype]
pub struct VolumeRecord {
    /// Cumulative volume in current 30-day window.
    pub volume: i128,
    /// Timestamp when the current window started.
    pub window_start: u64,
    /// Accumulated rebate owed to the user (unpaid).
    pub pending_rebate: i128,
}

#[derive(Clone)]
#[contracttype]
pub enum RebateKey {
    Admin,
    User(Address),
}

#[contract]
pub struct FeeRebateContract;

#[contractimpl]
impl FeeRebateContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        if env.storage().instance().has(&RebateKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&RebateKey::Admin, &admin);
    }

    /// Record a trade fee paid by `user`. Called by admin/marketplace.
    /// `fee_paid` is the fee amount charged for this trade.
    /// Returns the rebate credited for this trade.
    pub fn record_trade(env: Env, admin: Address, user: Address, fee_paid: i128) -> i128 {
        Self::require_admin(&env, &admin);
        if fee_paid <= 0 {
            return 0;
        }
        let now = env.ledger().timestamp();
        let mut rec = Self::load_record(&env, &user, now);

        // Calculate rebate based on tier
        let rebate_bps = Self::rebate_bps(rec.volume);
        let rebate = fee_paid * rebate_bps / 10000;
        rec.pending_rebate += rebate;
        rec.volume += fee_paid;

        env.storage().persistent().set(&RebateKey::User(user.clone()), &rec);

        if rebate > 0 {
            env.events().publish(
                (Symbol::new(&env, "rebate_credited"), user),
                (rebate, rec.pending_rebate),
            );
        }
        rebate
    }

    /// Claim pending rebate. Admin transfers to user off-chain / mints tokens.
    /// Returns amount claimed.
    pub fn claim_rebate(env: Env, user: Address) -> i128 {
        user.require_auth();
        let now = env.ledger().timestamp();
        let mut rec = Self::load_record(&env, &user, now);
        let amount = rec.pending_rebate;
        if amount == 0 {
            panic!("no pending rebate");
        }
        rec.pending_rebate = 0;
        env.storage().persistent().set(&RebateKey::User(user.clone()), &rec);
        env.events().publish(
            (Symbol::new(&env, "rebate_claimed"), user),
            amount,
        );
        amount
    }

    /// Get current 30-day volume and pending rebate for a user.
    pub fn get_record(env: Env, user: Address) -> VolumeRecord {
        let now = env.ledger().timestamp();
        Self::load_record(&env, &user, now)
    }

    /// Return rebate bps for a given 30-day volume.
    pub fn rebate_tier_bps(env: Env, volume: i128) -> i128 {
        let _ = env;
        Self::rebate_bps(volume)
    }

    // -------------------------------------------------------------------------

    fn require_admin(env: &Env, caller: &Address) {
        let admin: Address = env
            .storage()
            .instance()
            .get(&RebateKey::Admin)
            .expect("not initialized");
        if *caller != admin {
            panic!("admin only");
        }
        caller.require_auth();
    }

    fn rebate_bps(volume: i128) -> i128 {
        if volume >= TIER3_THRESHOLD {
            TIER3_BPS
        } else if volume >= TIER2_THRESHOLD {
            TIER2_BPS
        } else if volume >= TIER1_THRESHOLD {
            TIER1_BPS
        } else {
            0
        }
    }

    fn load_record(env: &Env, user: &Address, now: u64) -> VolumeRecord {
        let rec: Option<VolumeRecord> = env.storage().persistent().get(&RebateKey::User(user.clone()));
        match rec {
            None => VolumeRecord {
                volume: 0,
                window_start: now,
                pending_rebate: 0,
            },
            Some(mut r) => {
                // Reset window if expired
                if now >= r.window_start + VOLUME_WINDOW {
                    r.volume = 0;
                    r.window_start = now;
                }
                r
            }
        }
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    fn setup() -> (Env, Address, Address) {
        let env = Env::default();
        env.mock_all_auths();
        let cid = env.register_contract(None, FeeRebateContract);
        let client = FeeRebateContractClient::new(&env, &cid);
        let admin = Address::generate(&env);
        client.initialize(&admin);
        (env, cid, admin)
    }

    #[test]
    fn test_no_rebate_below_tier1() {
        let (env, cid, admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        // volume = 0 so no tier
        let rebate = client.record_trade(&admin, &user, &100);
        assert_eq!(rebate, 0);
    }

    #[test]
    fn test_tier1_rebate_10_percent() {
        let (env, cid, admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        // Push volume into tier1 range
        client.record_trade(&admin, &user, &TIER1_THRESHOLD);
        // Next trade gets 10% rebate
        let rebate = client.record_trade(&admin, &user, &1000);
        assert_eq!(rebate, 100); // 10%
    }

    #[test]
    fn test_tier3_rebate_50_percent() {
        let (env, cid, admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.record_trade(&admin, &user, &TIER3_THRESHOLD);
        let rebate = client.record_trade(&admin, &user, &1000);
        assert_eq!(rebate, 500); // 50%
    }

    #[test]
    fn test_claim_rebate() {
        let (env, cid, admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.record_trade(&admin, &user, &TIER2_THRESHOLD);
        client.record_trade(&admin, &user, &1000);
        let claimed = client.claim_rebate(&user);
        assert_eq!(claimed, 250); // 25%
        // pending should be 0 now
        let rec = client.get_record(&user);
        assert_eq!(rec.pending_rebate, 0);
    }

    #[test]
    #[should_panic(expected = "no pending rebate")]
    fn test_claim_with_no_rebate_panics() {
        let (env, cid, _admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.claim_rebate(&user);
    }

    #[test]
    fn test_window_resets_after_30_days() {
        let (env, cid, admin) = setup();
        let client = FeeRebateContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.record_trade(&admin, &user, &TIER3_THRESHOLD);
        // Advance past window
        env.ledger().with_mut(|l| { l.timestamp += VOLUME_WINDOW + 1; });
        // Volume resets; no tier, no rebate
        let rebate = client.record_trade(&admin, &user, &1000);
        assert_eq!(rebate, 0);
    }
}
