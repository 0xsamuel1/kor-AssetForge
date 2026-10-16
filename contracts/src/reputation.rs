use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol};

/// Score range 0–1000. New users start at 500.
pub const INITIAL_SCORE: i128 = 500;
pub const MAX_SCORE: i128 = 1000;
pub const MIN_SCORE: i128 = 0;

/// Seconds in 30 days – used for decay calculation.
pub const DECAY_PERIOD: u64 = 30 * 24 * 3600;
/// Decay per period (points)
pub const DECAY_AMOUNT: i128 = 10;

#[derive(Clone)]
#[contracttype]
pub struct ReputationRecord {
    pub score: i128,
    pub last_updated: u64,
    pub trade_completions: u32,
    pub disputes: u32,
}

#[derive(Clone)]
#[contracttype]
pub enum RepKey {
    Admin,
    User(Address),
}

#[contract]
pub struct ReputationContract;

#[contractimpl]
impl ReputationContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        if env.storage().instance().has(&RepKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&RepKey::Admin, &admin);
    }

    /// Get or initialise a user's reputation record.
    pub fn get_score(env: Env, user: Address) -> i128 {
        let rec = Self::load_with_decay(&env, &user);
        rec.score
    }

    pub fn get_record(env: Env, user: Address) -> ReputationRecord {
        Self::load_with_decay(&env, &user)
    }

    /// Called by admin (or marketplace) when a trade completes successfully.
    pub fn record_trade_completion(env: Env, admin: Address, user: Address) {
        Self::require_admin(&env, &admin);
        let mut rec = Self::load_with_decay(&env, &user);
        rec.trade_completions += 1;
        rec.score = (rec.score + 10).min(MAX_SCORE);
        rec.last_updated = env.ledger().timestamp();
        env.storage().persistent().set(&RepKey::User(user.clone()), &rec);
        env.events().publish(
            (Symbol::new(&env, "rep_trade"), user),
            rec.score,
        );
    }

    /// Called when a dispute is filed against a user.
    pub fn record_dispute(env: Env, admin: Address, user: Address) {
        Self::require_admin(&env, &admin);
        let mut rec = Self::load_with_decay(&env, &user);
        rec.disputes += 1;
        rec.score = (rec.score - 50).max(MIN_SCORE);
        rec.last_updated = env.ledger().timestamp();
        env.storage().persistent().set(&RepKey::User(user.clone()), &rec);
        env.events().publish(
            (Symbol::new(&env, "rep_dispute"), user),
            rec.score,
        );
    }

    /// Called when a user participates in governance.
    pub fn record_governance_participation(env: Env, admin: Address, user: Address) {
        Self::require_admin(&env, &admin);
        let mut rec = Self::load_with_decay(&env, &user);
        rec.score = (rec.score + 5).min(MAX_SCORE);
        rec.last_updated = env.ledger().timestamp();
        env.storage().persistent().set(&RepKey::User(user.clone()), &rec);
    }

    // -------------------------------------------------------------------------

    fn require_admin(env: &Env, caller: &Address) {
        let admin: Address = env
            .storage()
            .instance()
            .get(&RepKey::Admin)
            .expect("not initialized");
        if *caller != admin {
            panic!("admin only");
        }
        caller.require_auth();
    }

    /// Load a record applying time-based decay.
    fn load_with_decay(env: &Env, user: &Address) -> ReputationRecord {
        let now = env.ledger().timestamp();
        let mut rec: ReputationRecord = env
            .storage()
            .persistent()
            .get(&RepKey::User(user.clone()))
            .unwrap_or(ReputationRecord {
                score: INITIAL_SCORE,
                last_updated: now,
                trade_completions: 0,
                disputes: 0,
            });
        // Apply decay for each full DECAY_PERIOD elapsed since last_updated
        if now > rec.last_updated {
            let elapsed = now - rec.last_updated;
            let periods = (elapsed / DECAY_PERIOD) as i128;
            if periods > 0 {
                rec.score = (rec.score - periods * DECAY_AMOUNT).max(MIN_SCORE);
                rec.last_updated = now;
            }
        }
        rec
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    fn setup() -> (Env, Address, Address) {
        let env = Env::default();
        env.mock_all_auths();
        let cid = env.register_contract(None, ReputationContract);
        let client = ReputationContractClient::new(&env, &cid);
        let admin = Address::generate(&env);
        client.initialize(&admin);
        (env, cid, admin)
    }

    #[test]
    fn test_initial_score() {
        let (env, cid, _admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        assert_eq!(client.get_score(&user), INITIAL_SCORE);
    }

    #[test]
    fn test_trade_completion_increases_score() {
        let (env, cid, admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.record_trade_completion(&admin, &user);
        assert_eq!(client.get_score(&user), INITIAL_SCORE + 10);
    }

    #[test]
    fn test_dispute_decreases_score() {
        let (env, cid, admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        client.record_dispute(&admin, &user);
        assert_eq!(client.get_score(&user), INITIAL_SCORE - 50);
    }

    #[test]
    fn test_score_capped_at_max() {
        let (env, cid, admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        for _ in 0..60 {
            client.record_trade_completion(&admin, &user);
        }
        assert_eq!(client.get_score(&user), MAX_SCORE);
    }

    #[test]
    fn test_score_floored_at_min() {
        let (env, cid, admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        for _ in 0..20 {
            client.record_dispute(&admin, &user);
        }
        assert_eq!(client.get_score(&user), MIN_SCORE);
    }

    #[test]
    fn test_decay_reduces_score_over_time() {
        let (env, cid, admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let user = Address::generate(&env);
        // Set initial score above 500 so decay is visible
        client.record_trade_completion(&admin, &user);
        env.ledger().with_mut(|l| { l.timestamp += DECAY_PERIOD * 2; });
        let score = client.get_score(&user);
        // 510 - 2*10 = 490
        assert_eq!(score, 490);
    }

    #[test]
    #[should_panic(expected = "admin only")]
    fn test_non_admin_panics() {
        let (env, cid, _admin) = setup();
        let client = ReputationContractClient::new(&env, &cid);
        let non_admin = Address::generate(&env);
        let user = Address::generate(&env);
        client.record_trade_completion(&non_admin, &user);
    }
}
