use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, String, Symbol, Vec};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum DividendKey {
    Admin,
    DistributionNonce,
    Distribution(u64),
    Claimed(u64, Address),
    AutoSchedule(u64),
    ScheduleNonce,
    Paused,
    AssetToken,
}

// ---------------------------------------------------------------------------
// Data Structures
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct Distribution {
    pub id: u64,
    pub asset_token: Address,
    pub asset_id: u64,
    pub total_amount: i128,
    pub payout_asset: Address,
    pub timestamp: u64,
    pub total_supply: i128,
    pub tax_withholding_rate: u32,
    pub is_paused: bool,
    pub distributed: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct AutoSchedule {
    pub id: u64,
    pub asset_token: Address,
    pub asset_id: u64,
    pub interval_seconds: u64,
    pub next_run_at: u64,
    pub tax_withholding_rate: u32,
    pub active: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct ClaimRecord {
    pub distribution_id: u64,
    pub claimant: Address,
    pub amount: i128,
    pub withheld: i128,
    pub claimed_at: u64,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct DividendDistributor;

#[contractimpl]
impl DividendDistributor {
    pub fn initialize(env: Env, admin: Address, asset_token: Address) {
        admin.require_auth();
        if env.storage().instance().has(&DividendKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&DividendKey::Admin, &admin);
        env.storage().instance().set(&DividendKey::AssetToken, &asset_token);
    }

    pub fn get_admin(env: Env) -> Address {
        env.storage().instance().get(&DividendKey::Admin).expect("not initialized")
    }

    pub fn get_asset_token(env: Env) -> Address {
        env.storage().instance().get(&DividendKey::AssetToken).expect("not initialized")
    }

    fn require_admin(env: &Env, caller: &Address) {
        caller.require_auth();
        let admin: Address = env.storage().instance().get(&DividendKey::Admin).expect("not initialized");
        if *caller != admin {
            panic!("not authorized");
        }
    }

    fn require_not_paused(env: &Env) {
        let paused: bool = env.storage().instance().get(&DividendKey::Paused).unwrap_or(false);
        if paused {
            panic!("distributor paused");
        }
    }

    pub fn create_distribution(
        env: Env,
        admin: Address,
        asset_token: Address,
        asset_id: u64,
        total_amount: i128,
        payout_asset: Address,
        tax_withholding_rate: u32,
        total_supply: i128,
    ) -> u64 {
        Self::require_admin(&env, &admin);
        Self::require_not_paused(&env);
        assert!(total_amount > 0, "amount must be positive");
        assert!(tax_withholding_rate <= 10000, "tax rate too high");
        assert!(total_supply > 0, "supply must be positive");

        let dist_id = env.storage().instance().get(&DividendKey::DistributionNonce).unwrap_or(0u64) + 1;
        env.storage().instance().set(&DividendKey::DistributionNonce, &dist_id);

        let dist = Distribution {
            id: dist_id,
            asset_token,
            asset_id,
            total_amount,
            payout_asset,
            timestamp: env.ledger().timestamp(),
            total_supply,
            tax_withholding_rate,
            is_paused: false,
            distributed: false,
        };

        env.storage().instance().set(&DividendKey::Distribution(dist_id), &dist);
        env.events().publish((Symbol::new(&env, "distribution_created"),), (dist_id, total_amount));
        dist_id
    }

    pub fn get_distribution(env: Env, distribution_id: u64) -> Option<Distribution> {
        env.storage().instance().get(&DividendKey::Distribution(distribution_id))
    }

    pub fn claim(env: Env, distribution_id: u64, claimant: Address, holder_balance: i128) {
        claimant.require_auth();

        let mut dist: Distribution = env.storage().instance()
            .get(&DividendKey::Distribution(distribution_id))
            .expect("distribution not found");
        assert!(!dist.is_paused, "distribution paused");

        let claim_key = DividendKey::Claimed(distribution_id, claimant.clone());
        if env.storage().instance().has(&claim_key) {
            panic!("already claimed");
        }

        assert!(holder_balance > 0, "no tokens");
        assert!(dist.total_supply > 0, "no supply");

        let gross = (holder_balance * dist.total_amount) / dist.total_supply;
        let withheld = (gross * dist.tax_withholding_rate as i128) / 10000;
        let net = gross - withheld;

        let record = ClaimRecord {
            distribution_id,
            claimant: claimant.clone(),
            amount: net,
            withheld,
            claimed_at: env.ledger().timestamp(),
        };

        env.storage().instance().set(&claim_key, &record);
        env.events().publish(
            (Symbol::new(&env, "dividend_claimed"),),
            (distribution_id, claimant, net, withheld),
        );
    }

    pub fn get_claim(env: Env, distribution_id: u64, claimant: Address) -> Option<ClaimRecord> {
        env.storage().instance().get(&DividendKey::Claimed(distribution_id, claimant))
    }

    pub fn pause_distribution(env: Env, admin: Address, distribution_id: u64) {
        Self::require_admin(&env, &admin);
        let mut dist: Distribution = env.storage().instance()
            .get(&DividendKey::Distribution(distribution_id))
            .expect("distribution not found");
        dist.is_paused = true;
        env.storage().instance().set(&DividendKey::Distribution(distribution_id), &dist);
        env.events().publish((Symbol::new(&env, "distribution_paused"),), distribution_id);
    }

    pub fn resume_distribution(env: Env, admin: Address, distribution_id: u64) {
        Self::require_admin(&env, &admin);
        let mut dist: Distribution = env.storage().instance()
            .get(&DividendKey::Distribution(distribution_id))
            .expect("distribution not found");
        dist.is_paused = false;
        env.storage().instance().set(&DividendKey::Distribution(distribution_id), &dist);
        env.events().publish((Symbol::new(&env, "distribution_resumed"),), distribution_id);
    }

    // -----------------------------------------------------------------------
    // Auto-Schedule management
    // -----------------------------------------------------------------------

    pub fn create_auto_schedule(
        env: Env,
        admin: Address,
        asset_token: Address,
        asset_id: u64,
        interval_seconds: u64,
        tax_withholding_rate: u32,
    ) -> u64 {
        Self::require_admin(&env, &admin);
        assert!(interval_seconds > 0, "interval must be positive");
        assert!(tax_withholding_rate <= 10000, "tax rate too high");

        let schedule_id = env.storage().instance().get(&DividendKey::ScheduleNonce).unwrap_or(0u64) + 1;
        env.storage().instance().set(&DividendKey::ScheduleNonce, &schedule_id);

        let schedule = AutoSchedule {
            id: schedule_id,
            asset_token,
            asset_id,
            interval_seconds,
            next_run_at: env.ledger().timestamp() + interval_seconds,
            tax_withholding_rate,
            active: true,
        };

        env.storage().instance().set(&DividendKey::AutoSchedule(schedule_id), &schedule);
        env.events().publish((Symbol::new(&env, "auto_schedule_created"),), schedule_id);
        schedule_id
    }

    pub fn get_auto_schedule(env: Env, schedule_id: u64) -> Option<AutoSchedule> {
        env.storage().instance().get(&DividendKey::AutoSchedule(schedule_id))
    }

    pub fn run_auto_schedule(env: Env, schedule_id: u64, total_amount: i128, payout_asset: Address, total_supply: i128) -> u64 {
        let mut schedule: AutoSchedule = env.storage().instance()
            .get(&DividendKey::AutoSchedule(schedule_id))
            .expect("schedule not found");
        assert!(schedule.active, "schedule not active");

        let now = env.ledger().timestamp();
        if now < schedule.next_run_at {
            panic!("schedule not due yet");
        }

        let dist_id = Self::create_distribution(
            env.clone(),
            env.storage().instance().get(&DividendKey::Admin).expect("not initialized"),
            schedule.asset_token.clone(),
            schedule.asset_id,
            total_amount,
            payout_asset,
            schedule.tax_withholding_rate,
            total_supply,
        );

        schedule.next_run_at = now + schedule.interval_seconds;
        env.storage().instance().set(&DividendKey::AutoSchedule(schedule_id), &schedule);

        env.events().publish((Symbol::new(&env, "auto_schedule_executed"),), (schedule_id, dist_id));
        dist_id
    }

    pub fn deactivate_schedule(env: Env, admin: Address, schedule_id: u64) {
        Self::require_admin(&env, &admin);
        let mut schedule: AutoSchedule = env.storage().instance()
            .get(&DividendKey::AutoSchedule(schedule_id))
            .expect("schedule not found");
        schedule.active = false;
        env.storage().instance().set(&DividendKey::AutoSchedule(schedule_id), &schedule);
        env.events().publish((Symbol::new(&env, "schedule_deactivated"),), schedule_id);
    }

    pub fn activate_schedule(env: Env, admin: Address, schedule_id: u64) {
        Self::require_admin(&env, &admin);
        let mut schedule: AutoSchedule = env.storage().instance()
            .get(&DividendKey::AutoSchedule(schedule_id))
            .expect("schedule not found");
        schedule.active = true;
        env.storage().instance().set(&DividendKey::AutoSchedule(schedule_id), &schedule);
        env.events().publish((Symbol::new(&env, "schedule_activated"),), schedule_id);
    }

    // -----------------------------------------------------------------------
    // Emergency controls
    // -----------------------------------------------------------------------

    pub fn pause_all(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);
        env.storage().instance().set(&DividendKey::Paused, &true);
        env.events().publish((Symbol::new(&env, "distributor_paused"),), ());
    }

    pub fn unpause_all(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);
        env.storage().instance().set(&DividendKey::Paused, &false);
        env.events().publish((Symbol::new(&env, "distributor_unpaused"),), ());
    }

    pub fn is_paused(env: Env) -> bool {
        env.storage().instance().get(&DividendKey::Paused).unwrap_or(false)
    }

    pub fn get_unclaimed_total(env: Env, distribution_id: u64) -> i128 {
        let dist: Distribution = env.storage().instance()
            .get(&DividendKey::Distribution(distribution_id))
            .expect("distribution not found");
        dist.total_amount
    }
}

// ===========================================================================
// Unit Tests
// ===========================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    fn setup(env: &Env) -> (DividendDistributorClient<'_>, Address, Address) {
        let contract_id = env.register_contract(None, DividendDistributor);
        let client = DividendDistributorClient::new(env, &contract_id);
        let admin = Address::generate(env);
        let token = Address::generate(env);
        client.initialize(&admin, &token);
        (client, admin, token)
    }

    #[test]
    fn test_initialize() {
        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);
        let token = Address::generate(&env);
        let contract_id = env.register_contract(None, DividendDistributor);
        let client = DividendDistributorClient::new(&env, &contract_id);
        client.initialize(&admin, &token);

        assert_eq!(client.get_admin(), admin);
        assert_eq!(client.get_asset_token(), token);
    }

    #[test]
    fn test_create_distribution() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, token) = setup(&env);
        let payout = Address::generate(&env);
        let dist_id = client.create_distribution(&admin, &token, &1, &100_000, &payout, &500, &1_000_000);
        assert_eq!(dist_id, 1);

        let dist = client.get_distribution(&dist_id).unwrap();
        assert_eq!(dist.total_amount, 100_000);
        assert_eq!(dist.tax_withholding_rate, 500);
        assert_eq!(dist.total_supply, 1_000_000);
    }

    #[test]
    fn test_claim() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, token) = setup(&env);
        let payout = Address::generate(&env);
        let dist_id = client.create_distribution(&admin, &token, &1, &100_000, &payout, &500, &1_000_000);

        let user = Address::generate(&env);
        // User holds 100k out of 1M supply = 10%
        client.claim(&dist_id, &user, &100_000);

        let claim = client.get_claim(&dist_id, &user).unwrap();
        // Gross = 100_000 * 100_000 / 1_000_000 = 10_000
        // Withheld = 10_000 * 500 / 10000 = 500
        // Net = 9_500
        assert_eq!(claim.amount, 9_500);
        assert_eq!(claim.withheld, 500);
    }

    #[test]
    fn test_auto_schedule() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, token) = setup(&env);
        let payout = Address::generate(&env);
        let schedule_id = client.create_auto_schedule(&admin, &token, &1, &3600, &500);
        assert_eq!(schedule_id, 1);

        let schedule = client.get_auto_schedule(&schedule_id).unwrap();
        assert!(schedule.active);
        assert_eq!(schedule.interval_seconds, 3600);

        // Advance past next_run_at
        env.ledger().with_mut(|li| {
            li.timestamp += 7200;
        });

        let dist_id = client.run_auto_schedule(&schedule_id, &50_000, &payout, &500_000);
        assert_eq!(dist_id, 1);

        let dist = client.get_distribution(&dist_id).unwrap();
        assert_eq!(dist.total_amount, 50_000);
    }

    #[test]
    fn test_pause_resume_distribution() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, token) = setup(&env);
        let payout = Address::generate(&env);
        let dist_id = client.create_distribution(&admin, &token, &1, &100_000, &payout, &500, &1_000_000);

        client.pause_distribution(&admin, &dist_id);
        let dist = client.get_distribution(&dist_id).unwrap();
        assert!(dist.is_paused);

        client.resume_distribution(&admin, &dist_id);
        let dist = client.get_distribution(&dist_id).unwrap();
        assert!(!dist.is_paused);
    }

    #[test]
    fn test_global_pause() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, _token) = setup(&env);

        assert!(!client.is_paused());
        client.pause_all(&admin);
        assert!(client.is_paused());
        client.unpause_all(&admin);
        assert!(!client.is_paused());
    }
}
