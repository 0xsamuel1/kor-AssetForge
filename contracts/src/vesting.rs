use soroban_sdk::{contract, contracterror, contractimpl, contracttype, Address, Env, Symbol, Vec};

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
#[contracterror]
pub enum VestingError {
    AlreadyInitialized = 1,
    NotAuthorized = 2,
    ScheduleNotFound = 3,
    CliffNotReached = 4,
    NothingToClaim = 5,
    AlreadyRevoked = 6,
    InvalidDuration = 7,
    InvalidAmount = 8,
    InvalidStep = 9,
    AlreadyHasSchedule = 10,
}

#[derive(Clone)]
#[contracttype]
pub struct VestingSchedule {
    pub beneficiary: Address,
    pub total_amount: i128,
    pub cliff_amount: i128,
    pub cliff_duration: u32,
    pub total_duration: u32,
    pub start_ledger: u32,
    pub withdrawn: i128,
    pub revoked: bool,
    pub is_stepped: bool,
    pub early_unlock_penalty_bps: u32,
}

#[derive(Clone)]
#[contracttype]
pub struct VestingStep {
    pub time_pct: u32,
    pub release_pct: u32,
}

#[derive(Clone)]
#[contracttype]
pub struct VestingDashboard {
    pub schedule: VestingSchedule,
    pub vested: i128,
    pub available: i128,
    pub progress_pct: u32,
}

#[derive(Clone)]
#[contracttype]
pub enum VestingDataKey {
    Admin,
    Schedule(Address),
    BeneficiaryList,
    Steps(Address),
}

#[contract]
pub struct Vesting;

#[contractimpl]
impl Vesting {
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&VestingDataKey::Admin) {
            panic!("already initialized");
        }
        admin.require_auth();
        env.storage().instance().set(&VestingDataKey::Admin, &admin);
        env.storage().instance().set(&VestingDataKey::BeneficiaryList, &Vec::<Address>::new(&env));
    }

    pub fn create_schedule(
        env: Env,
        admin: Address,
        beneficiary: Address,
        total_amount: i128,
        cliff_amount: i128,
        cliff_duration: u32,
        total_duration: u32,
        is_stepped: bool,
        early_unlock_penalty_bps: u32,
    ) {
        Self::require_admin(&env, &admin);
        assert!(total_amount > 0, "total amount must be positive");
        assert!(cliff_amount <= total_amount, "cliff exceeds total");
        assert!(total_duration > 0, "total duration must be > 0");
        assert!(cliff_duration <= total_duration, "cliff exceeds total duration");
        assert!(early_unlock_penalty_bps <= 10000, "penalty must be <= 10000");

        if env.storage().persistent().has(&VestingDataKey::Schedule(beneficiary.clone())) {
            panic!("beneficiary already has a schedule");
        }

        let schedule = VestingSchedule {
            beneficiary: beneficiary.clone(),
            total_amount,
            cliff_amount,
            cliff_duration,
            total_duration,
            start_ledger: env.ledger().sequence(),
            withdrawn: 0,
            revoked: false,
            is_stepped,
            early_unlock_penalty_bps,
        };

        env.storage().persistent().set(&VestingDataKey::Schedule(beneficiary.clone()), &schedule);

        let mut list: Vec<Address> = env.storage().instance()
            .get(&VestingDataKey::BeneficiaryList).unwrap_or(Vec::<Address>::new(&env));
        list.push_back(beneficiary.clone());
        env.storage().instance().set(&VestingDataKey::BeneficiaryList, &list);

        env.events().publish(
            (Symbol::new(&env, "schedule_created"),),
            (beneficiary, total_amount, cliff_duration, total_duration),
        );
    }

    pub fn create_schedule_with_steps(
        env: Env,
        admin: Address,
        beneficiary: Address,
        total_amount: i128,
        cliff_amount: i128,
        cliff_duration: u32,
        total_duration: u32,
        early_unlock_penalty_bps: u32,
        steps: Vec<VestingStep>,
    ) {
        Self::require_admin(&env, &admin);
        assert!(total_amount > 0, "total amount must be positive");
        assert!(total_duration > 0, "total duration must be > 0");
        assert!(cliff_duration <= total_duration, "cliff exceeds total duration");
        assert!(steps.len() > 0, "must provide at least one step");
        assert!(steps.len() <= 20, "max 20 steps");

        for i in 0..steps.len() {
            let step = steps.get(i).unwrap();
            assert!(step.time_pct <= 100, "time_pct must be <= 100");
            assert!(step.release_pct <= 100, "release_pct must be <= 100");
            if i > 0 {
                let prev = steps.get(i - 1).unwrap();
                assert!(step.time_pct > prev.time_pct, "steps must be in ascending order");
                assert!(step.release_pct >= prev.release_pct, "release_pct must be non-decreasing");
            }
        }
        if cliff_duration == 0 {
            assert!(steps.get(0).unwrap().time_pct > 0, "first step must have time_pct > 0");
        }

        if env.storage().persistent().has(&VestingDataKey::Schedule(beneficiary.clone())) {
            panic!("beneficiary already has a schedule");
        }

        let schedule = VestingSchedule {
            beneficiary: beneficiary.clone(),
            total_amount,
            cliff_amount,
            cliff_duration,
            total_duration,
            start_ledger: env.ledger().sequence(),
            withdrawn: 0,
            revoked: false,
            is_stepped: true,
            early_unlock_penalty_bps,
        };

        env.storage().persistent().set(&VestingDataKey::Schedule(beneficiary.clone()), &schedule);
        let mut all_steps: Vec<VestingStep> = Vec::new(&env);
        for i in 0..steps.len() {
            all_steps.push_back(steps.get(i).unwrap());
        }
        env.storage().persistent().set(&VestingDataKey::Steps(beneficiary.clone()), &all_steps);

        let mut list: Vec<Address> = env.storage().instance()
            .get(&VestingDataKey::BeneficiaryList).unwrap_or(Vec::<Address>::new(&env));
        list.push_back(beneficiary.clone());
        env.storage().instance().set(&VestingDataKey::BeneficiaryList, &list);

        env.events().publish(
            (Symbol::new(&env, "schedule_created"),),
            (beneficiary, total_amount, cliff_duration, total_duration),
        );
    }

    pub fn claim(env: Env, beneficiary: Address) -> i128 {
        beneficiary.require_auth();

        let mut schedule: VestingSchedule = env.storage().persistent()
            .get(&VestingDataKey::Schedule(beneficiary.clone()))
            .expect("no vesting schedule found");

        if schedule.revoked {
            panic!("schedule has been revoked");
        }

        let vested = Self::compute_vested(&env, &schedule);
        let available = vested - schedule.withdrawn;
        if available <= 0 {
            panic!("nothing to claim");
        }

        let penalty = if env.ledger().sequence() < schedule.start_ledger + schedule.total_duration {
            available * schedule.early_unlock_penalty_bps as i128 / 10000
        } else {
            0
        };

        let claim_amount = available - penalty;
        schedule.withdrawn += available;

        env.storage().persistent().set(&VestingDataKey::Schedule(beneficiary.clone()), &schedule);

        env.events().publish(
            (Symbol::new(&env, "vesting_claimed"), beneficiary),
            (claim_amount, penalty, available),
        );

        claim_amount
    }

    pub fn revoke(env: Env, admin: Address, beneficiary: Address) -> i128 {
        Self::require_admin(&env, &admin);

        let mut schedule: VestingSchedule = env.storage().persistent()
            .get(&VestingDataKey::Schedule(beneficiary.clone()))
            .expect("no vesting schedule found");

        if schedule.revoked {
            panic!("already revoked");
        }

        schedule.revoked = true;
        let vested = Self::compute_vested(&env, &schedule);
        let unvested = schedule.total_amount - vested;
        schedule.withdrawn += vested;

        env.storage().persistent().set(&VestingDataKey::Schedule(beneficiary.clone()), &schedule);

        env.events().publish(
            (Symbol::new(&env, "vesting_revoked"), admin),
            (beneficiary, unvested),
        );

        unvested
    }

    pub fn get_vested_amount(env: Env, beneficiary: Address) -> i128 {
        let schedule: VestingSchedule = env.storage().persistent()
            .get(&VestingDataKey::Schedule(beneficiary))
            .expect("no vesting schedule found");
        Self::compute_vested(&env, &schedule)
    }

    pub fn get_available_amount(env: Env, beneficiary: Address) -> i128 {
        let schedule: VestingSchedule = env.storage().persistent()
            .get(&VestingDataKey::Schedule(beneficiary))
            .expect("no vesting schedule found");
        let vested = Self::compute_vested(&env, &schedule);
        vested - schedule.withdrawn
    }

    pub fn get_schedule(env: Env, beneficiary: Address) -> Option<VestingSchedule> {
        env.storage().persistent().get(&VestingDataKey::Schedule(beneficiary))
    }

    pub fn get_dashboard(env: Env, beneficiary: Address) -> Option<VestingDashboard> {
        let schedule = env.storage().persistent().get::<_, VestingSchedule>(
            &VestingDataKey::Schedule(beneficiary)
        )?;
        let vested = Self::compute_vested(&env, &schedule);
        let available = vested - schedule.withdrawn;
        let progress_pct = if schedule.total_amount > 0 {
            ((vested * 100) / schedule.total_amount) as u32
        } else {
            0
        };
        Some(VestingDashboard { schedule, vested, available, progress_pct })
    }

    pub fn get_beneficiaries(env: Env) -> Vec<Address> {
        env.storage().instance()
            .get(&VestingDataKey::BeneficiaryList).unwrap_or(Vec::<Address>::new(&env))
    }

    fn require_admin(env: &Env, admin: &Address) {
        admin.require_auth();
        let stored: Address = env.storage().instance()
            .get(&VestingDataKey::Admin).expect("not initialized");
        if admin != &stored {
            panic!("not authorized");
        }
    }

    fn compute_vested(env: &Env, schedule: &VestingSchedule) -> i128 {
        if schedule.revoked {
            return schedule.withdrawn;
        }

        let current_ledger = env.ledger().sequence();
        if current_ledger <= schedule.start_ledger {
            return 0;
        }
        let elapsed = (current_ledger - schedule.start_ledger) as u128;

        // Cliff period - only cliff_amount is vested
        if elapsed <= schedule.cliff_duration as u128 {
            return schedule.cliff_amount;
        }

        // After total duration - fully vested
        if elapsed >= schedule.total_duration as u128 {
            return schedule.total_amount;
        }

        // During vesting period
        let cliff_vested = schedule.cliff_amount;
        let remaining_amount = schedule.total_amount - cliff_vested;
        let remaining_duration = (schedule.total_duration - schedule.cliff_duration) as u128;
        let time_into_vesting = elapsed - schedule.cliff_duration as u128;

        if schedule.is_stepped {
            let steps: Vec<VestingStep> = env.storage().persistent()
                .get(&VestingDataKey::Steps(schedule.beneficiary.clone()))
                .unwrap_or(Vec::new(env));

            let progress_pct = if schedule.total_duration > 0 {
                (elapsed * 100 / schedule.total_duration as u128) as u32
            } else {
                100
            };

            let mut release_pct: u32 = 0;
            for i in 0..steps.len() {
                let step = steps.get(i).unwrap();
                if progress_pct >= step.time_pct {
                    if release_pct < step.release_pct {
                        release_pct = step.release_pct;
                    }
                }
            }

            cliff_vested + remaining_amount * release_pct as i128 / 100
        } else {
            // Linear vesting
            let vested_ratio = time_into_vesting * remaining_amount as u128 / remaining_duration;
            cliff_vested + vested_ratio as i128
        }
    }
}
