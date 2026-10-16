use soroban_sdk::{contract, contracterror, contractimpl, contracttype, Address, Env, Symbol, Vec};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum RoyaltyError {
    AlreadyInitialized = 1,
    Unauthorized = 2,
}

#[derive(Clone)]
#[contracttype]
pub struct RoyaltyBeneficiary {
    pub address: Address,
    pub share_bps: u32,
}

#[derive(Clone)]
#[contracttype]
pub struct RoyaltyConfig {
    pub percentage_bps: u32,
    pub cap: i128,
    pub beneficiaries: Vec<RoyaltyBeneficiary>,
}

#[derive(Clone)]
#[contracttype]
pub struct RoyaltyRecord {
    pub asset_id: u64,
    pub sale_amount: i128,
    pub royalty_amount: i128,
    pub timestamp: u64,
}

#[derive(Clone)]
#[contracttype]
pub enum RoyaltyDataKey {
    Admin,
    Config(u64),
    History(u64),
}

#[contract]
pub struct Royalty;

#[contractimpl]
impl Royalty {
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&RoyaltyDataKey::Admin) {
            panic!("already initialized");
        }
        admin.require_auth();
        env.storage().instance().set(&RoyaltyDataKey::Admin, &admin);
        env.events().publish((Symbol::new(&env, "royalty_initialized"),), admin);
    }

    pub fn set_royalty(
        env: Env,
        admin: Address,
        asset_id: u64,
        percentage_bps: u32,
        cap: i128,
        beneficiaries: Vec<RoyaltyBeneficiary>,
    ) {
        Self::require_admin(&env, &admin);
        assert!(percentage_bps <= 1000, "royalty must not exceed 10% (1000 bps)");
        assert!(cap >= 0, "cap must be non-negative");

        let mut total_shares: u32 = 0;
        for i in 0..beneficiaries.len() {
            let b = beneficiaries.get(i).unwrap();
            assert!(b.share_bps > 0, "share must be positive");
            total_shares += b.share_bps;
        }
        if beneficiaries.len() > 0 {
            assert_eq!(total_shares, 10000, "beneficiary shares must sum to 10000");
        }

        let config = RoyaltyConfig {
            percentage_bps,
            cap,
            beneficiaries,
        };

        env.storage().persistent().set(&RoyaltyDataKey::Config(asset_id), &config);
        env.events().publish(
            (Symbol::new(&env, "royalty_configured"), asset_id),
            (percentage_bps, cap),
        );
    }

    pub fn get_royalty(env: Env, asset_id: u64) -> Option<RoyaltyConfig> {
        env.storage().persistent().get(&RoyaltyDataKey::Config(asset_id))
    }

    pub fn calculate_royalty(env: Env, asset_id: u64, sale_amount: i128) -> i128 {
        assert!(sale_amount > 0, "sale amount must be positive");
        if let Some(config) = env.storage().persistent().get::<_, RoyaltyConfig>(&RoyaltyDataKey::Config(asset_id)) {
            let royalty = sale_amount
                .checked_mul(config.percentage_bps as i128)
                .expect("royalty calculation overflow")
                .checked_div(10_000)
                .expect("royalty calculation div error");
            if config.cap > 0 && royalty > config.cap {
                return config.cap;
            }
            royalty
        } else {
            0
        }
    }

    pub fn record_royalty(
        env: Env,
        asset_id: u64,
        buyer: Address,
        sale_amount: i128,
    ) -> i128 {
        let royalty = Self::calculate_royalty(env.clone(), asset_id, sale_amount);
        if royalty > 0 {
            let record = RoyaltyRecord {
                asset_id,
                sale_amount,
                royalty_amount: royalty,
                timestamp: env.ledger().timestamp(),
            };
            let mut history: Vec<RoyaltyRecord> = env
                .storage()
                .persistent()
                .get(&RoyaltyDataKey::History(asset_id))
                .unwrap_or(Vec::new(&env));
            history.push_back(record);
            env.storage().persistent().set(&RoyaltyDataKey::History(asset_id), &history);

            env.events().publish(
                (Symbol::new(&env, "royalty_paid"), asset_id),
                (buyer, royalty),
            );
        }
        royalty
    }

    pub fn get_royalty_history(env: Env, asset_id: u64) -> Vec<RoyaltyRecord> {
        env.storage().persistent().get(&RoyaltyDataKey::History(asset_id)).unwrap_or(Vec::new(&env))
    }

    pub fn get_royalty_admin(env: Env) -> Address {
        env.storage().instance().get(&RoyaltyDataKey::Admin).expect("not initialized")
    }

    fn require_admin(env: &Env, admin: &Address) {
        admin.require_auth();
        let stored: Address = env.storage().instance().get(&RoyaltyDataKey::Admin).expect("not initialized");
        if admin != &stored {
            panic!("not authorized");
        }
    }
}
