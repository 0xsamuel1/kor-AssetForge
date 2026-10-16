use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, Vec};

// ============================================================================
// Data Types
// ============================================================================

#[derive(Clone)]
#[contracttype]
pub struct Arbitrator {
    pub address: Address,
    pub stake_amount: i128,
    pub reputation: u32,
    pub active: bool,
    pub slash_count: u32,
}

#[derive(Clone)]
#[contracttype]
pub enum ArbitratorDataKey {
    Admin,
    MinStake,
    Arbitrator(Address),
    ArbitratorList,
    ActiveCount,
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct ArbContract;

#[contractimpl]
impl ArbContract {
    /// Initialize with an admin and minimum stake requirement.
    pub fn initialize(env: Env, admin: Address, min_stake_amount: i128) {
        if env.storage().instance().has(&ArbitratorDataKey::Admin) {
            panic!("already initialized");
        }
        if min_stake_amount <= 0 {
            panic!("min_stake_amount must be positive");
        }
        env.storage()
            .instance()
            .set(&ArbitratorDataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&ArbitratorDataKey::MinStake, &min_stake_amount);
        env.storage()
            .instance()
            .set(&ArbitratorDataKey::ActiveCount, &0u32);
    }

    /// Register as an arbitrator by staking the required amount.
    pub fn register(env: Env, arbitrator: Address, stake_amount: i128) {
        arbitrator.require_auth();

        let min_stake: i128 = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::MinStake)
            .expect("not initialized");

        if stake_amount < min_stake {
            panic!("stake_amount below minimum required");
        }

        if env
            .storage()
            .persistent()
            .has(&ArbitratorDataKey::Arbitrator(arbitrator.clone()))
        {
            let existing: Arbitrator = env
                .storage()
                .persistent()
                .get(&ArbitratorDataKey::Arbitrator(arbitrator.clone()))
                .unwrap();
            if existing.active {
                panic!("already registered as an arbitrator");
            }
        }

        let arb = Arbitrator {
            address: arbitrator.clone(),
            stake_amount,
            reputation: 100,
            active: true,
            slash_count: 0,
        };

        env.storage()
            .persistent()
            .set(&ArbitratorDataKey::Arbitrator(arbitrator.clone()), &arb);

        let mut list: Vec<Address> = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::ArbitratorList)
            .unwrap_or(Vec::new(&env));

        let mut already_in_list = false;
        for a in list.iter() {
            if a == arbitrator {
                already_in_list = true;
                break;
            }
        }
        if !already_in_list {
            list.push_back(arbitrator.clone());
            env.storage()
                .instance()
                .set(&ArbitratorDataKey::ArbitratorList, &list);
        }

        let count: u32 = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::ActiveCount)
            .unwrap_or(0);
        env.storage()
            .instance()
            .set(&ArbitratorDataKey::ActiveCount, &(count + 1));

        env.events().publish(
            (Symbol::new(&env, "arbitrator_registered"), arbitrator),
            stake_amount,
        );
    }

    /// Deregister and reclaim stake.
    pub fn deregister(env: Env, arbitrator: Address) {
        arbitrator.require_auth();

        let mut arb: Arbitrator = env
            .storage()
            .persistent()
            .get(&ArbitratorDataKey::Arbitrator(arbitrator.clone()))
            .expect("arbitrator not found");

        if !arb.active {
            panic!("arbitrator is not active");
        }

        arb.active = false;
        env.storage()
            .persistent()
            .set(&ArbitratorDataKey::Arbitrator(arbitrator.clone()), &arb);

        let count: u32 = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::ActiveCount)
            .unwrap_or(1);
        env.storage()
            .instance()
            .set(&ArbitratorDataKey::ActiveCount, &count.saturating_sub(1));

        env.events().publish(
            (Symbol::new(&env, "arbitrator_deregistered"), arbitrator),
            arb.stake_amount,
        );
    }

    /// Select up to `count` active arbitrators for a dispute using deterministic index stepping.
    /// Uses (dispute_id XOR ledger_sequence) as seed for index selection.
    /// NOTE: This selection can be influenced by validator timing. A commit-reveal
    ///       scheme would provide stronger randomness guarantees in adversarial settings.
    pub fn select_for_dispute(env: Env, dispute_id: u64, count: u32) -> Vec<Address> {
        let list: Vec<Address> = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::ArbitratorList)
            .unwrap_or(Vec::new(&env));

        let active_count: u32 = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::ActiveCount)
            .unwrap_or(0);

        if active_count == 0 {
            panic!("no active arbitrators available");
        }

        let want = if count > active_count { active_count } else { count };

        // Pseudo-random starting index derived from dispute_id XOR ledger sequence
        let seed = dispute_id ^ (env.ledger().sequence() as u64);
        let list_len = list.len() as u64;
        let mut start_idx = seed % list_len;

        let mut selected: Vec<Address> = Vec::new(&env);
        let mut checked: u64 = 0;

        while (selected.len() as u32) < want && checked < list_len {
            let idx = (start_idx % list_len) as u32;
            let addr = list.get(idx).unwrap();

            if let Some(arb) = env
                .storage()
                .persistent()
                .get::<ArbitratorDataKey, Arbitrator>(
                    &ArbitratorDataKey::Arbitrator(addr.clone()),
                )
            {
                if arb.active {
                    selected.push_back(addr);
                }
            }

            start_idx += 1;
            checked += 1;
        }

        selected
    }

    /// Admin: slash a malicious arbitrator (deducts 50% of stake, reduces reputation).
    pub fn slash(env: Env, admin: Address, arbitrator: Address) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&ArbitratorDataKey::Admin)
            .expect("not initialized");
        if admin != stored_admin {
            panic!("caller is not admin");
        }

        let mut arb: Arbitrator = env
            .storage()
            .persistent()
            .get(&ArbitratorDataKey::Arbitrator(arbitrator.clone()))
            .expect("arbitrator not found");

        arb.stake_amount /= 2;
        arb.slash_count += 1;
        arb.reputation = arb.reputation.saturating_sub(20);

        // Auto-deregister if reputation reaches zero
        if arb.reputation == 0 {
            arb.active = false;
            let count: u32 = env
                .storage()
                .instance()
                .get(&ArbitratorDataKey::ActiveCount)
                .unwrap_or(1);
            env.storage()
                .instance()
                .set(&ArbitratorDataKey::ActiveCount, &count.saturating_sub(1));
        }

        env.storage()
            .persistent()
            .set(&ArbitratorDataKey::Arbitrator(arbitrator.clone()), &arb);

        env.events().publish(
            (Symbol::new(&env, "arbitrator_slashed"), arbitrator),
            (arb.stake_amount, arb.reputation),
        );
    }

    /// Get arbitrator details.
    pub fn get_arbitrator(env: Env, address: Address) -> Option<Arbitrator> {
        env.storage()
            .persistent()
            .get(&ArbitratorDataKey::Arbitrator(address))
    }

    /// Get count of currently active arbitrators.
    pub fn get_active_count(env: Env) -> u32 {
        env.storage()
            .instance()
            .get(&ArbitratorDataKey::ActiveCount)
            .unwrap_or(0)
    }

    /// Get the full arbitrator list (includes inactive).
    pub fn get_arbitrator_list(env: Env) -> Vec<Address> {
        env.storage()
            .instance()
            .get(&ArbitratorDataKey::ArbitratorList)
            .unwrap_or(Vec::new(&env))
    }
}
