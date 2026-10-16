use soroban_sdk::testutils::{Address as _, Ledger as _};
use soroban_sdk::{Address, Env, Vec};

use kor_assetforge_contracts::vesting::{Vesting, VestingClient, VestingStep};

fn setup() -> (Env, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, Vesting);
    let admin = Address::generate(&env);
    let client = VestingClient::new(&env, &contract_id);
    client.initialize(&admin);
    (env, admin, contract_id)
}

fn client<'a>(env: &'a Env, contract_id: &'a Address) -> VestingClient<'a> {
    VestingClient::new(env, contract_id)
}

#[test]
fn test_initialize() {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, Vesting);
    let admin = Address::generate(&env);
    let c = VestingClient::new(&env, &contract_id);
    c.initialize(&admin);
}

#[test]
fn test_create_and_get_schedule() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(
        &admin,
        &beneficiary,
        &200_000,
        &40_000,
        &200,
        &2000,
        &false,
        &0,
    );

    let schedule = c.get_schedule(&beneficiary).unwrap();
    assert_eq!(schedule.total_amount, 200_000);
    assert_eq!(schedule.cliff_amount, 40_000);
    assert_eq!(schedule.cliff_duration, 200);
    assert_eq!(schedule.total_duration, 2000);
    assert_eq!(schedule.start_ledger, 100);
    assert!(!schedule.revoked);
    assert!(!schedule.is_stepped);
}

#[test]
fn test_no_vesting_at_start() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 0);
}

#[test]
fn test_partial_linear_vesting() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);
    env.ledger().with_mut(|l| l.sequence_number = 600);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 50_000);
}

#[test]
fn test_full_vesting_after_duration() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);
    env.ledger().with_mut(|l| l.sequence_number = 1100);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 100_000);
}

#[test]
fn test_cliff_period_locks_tokens() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &30_000, &200, &1000, &false, &0);

    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 0);

    env.ledger().with_mut(|l| l.sequence_number = 150);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 30_000);

    env.ledger().with_mut(|l| l.sequence_number = 350);
    let vested = c.get_vested_amount(&beneficiary);
    assert!(vested > 30_000);
}

#[test]
fn test_claim_basic() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);
    env.ledger().with_mut(|l| l.sequence_number = 600);
    let claimed = c.claim(&beneficiary);
    assert_eq!(claimed, 50_000);

    let available = c.get_available_amount(&beneficiary);
    assert_eq!(available, 0);
}

#[test]
fn test_claim_multiple_times() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &2000, &false, &0);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    let first = c.claim(&beneficiary);
    assert_eq!(first, 25_000);

    env.ledger().with_mut(|l| l.sequence_number = 1600);
    let second = c.claim(&beneficiary);
    assert_eq!(second, 50_000);
}

#[test]
fn test_early_unlock_penalty() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &2000);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    let claimed = c.claim(&beneficiary);
    assert_eq!(claimed, 40_000);
}

#[test]
fn test_no_penalty_after_full_vesting() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &2000);

    env.ledger().with_mut(|l| l.sequence_number = 1100);
    let claimed = c.claim(&beneficiary);
    assert_eq!(claimed, 100_000);
}

#[test]
fn test_revoke_before_vesting() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);

    let unvested = c.revoke(&admin, &beneficiary);
    assert_eq!(unvested, 100_000);

    let schedule = c.get_schedule(&beneficiary).unwrap();
    assert!(schedule.revoked);
}

#[test]
fn test_revoke_partial() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    c.claim(&beneficiary);

    let unvested = c.revoke(&admin, &beneficiary);
    assert_eq!(unvested, 50_000);
}

#[test]
fn test_stepped_vesting_schedule() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    let mut steps: Vec<VestingStep> = Vec::new(&env);
    steps.push_back(VestingStep { time_pct: 25, release_pct: 25 });
    steps.push_back(VestingStep { time_pct: 50, release_pct: 50 });
    steps.push_back(VestingStep { time_pct: 75, release_pct: 75 });
    steps.push_back(VestingStep { time_pct: 100, release_pct: 100 });

    c.create_schedule_with_steps(
        &admin, &beneficiary, &100_000, &0, &0, &1000, &0, &steps,
    );

    env.ledger().with_mut(|l| l.sequence_number = 350);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 25_000);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 50_000);

    env.ledger().with_mut(|l| l.sequence_number = 1100);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 100_000);
}

#[test]
fn test_dashboard_view() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);

    let dashboard = c.get_dashboard(&beneficiary).unwrap();
    assert_eq!(dashboard.schedule.total_amount, 100_000);
    assert_eq!(dashboard.vested, 0);
    assert_eq!(dashboard.available, 0);
    assert_eq!(dashboard.progress_pct, 0);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    let dashboard = c.get_dashboard(&beneficiary).unwrap();
    assert_eq!(dashboard.vested, 50_000);
    assert_eq!(dashboard.available, 50_000);
    assert_eq!(dashboard.progress_pct, 50);
}

#[test]
fn test_beneficiaries_list() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);

    let b1 = Address::generate(&env);
    let b2 = Address::generate(&env);
    let b3 = Address::generate(&env);

    c.create_schedule(&admin, &b1, &50_000, &0, &0, &500, &false, &0);
    c.create_schedule(&admin, &b2, &75_000, &0, &0, &750, &false, &0);
    c.create_schedule(&admin, &b3, &100_000, &10_000, &100, &1000, &false, &500);

    let list = c.get_beneficiaries();
    assert_eq!(list.len(), 3);
}

#[test]
fn test_schedule_not_found() {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, Vesting);
    let c = VestingClient::new(&env, &contract_id);
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        c.get_vested_amount(&Address::generate(&env));
    }));
    assert!(result.is_err());
}

#[test]
fn test_nothing_to_claim() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);

    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        c.claim(&beneficiary);
    }));
    assert!(result.is_err());
}

#[test]
fn test_stepped_vesting_with_cliff() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    let mut steps: Vec<VestingStep> = Vec::new(&env);
    steps.push_back(VestingStep { time_pct: 50, release_pct: 50 });
    steps.push_back(VestingStep { time_pct: 100, release_pct: 100 });

    c.create_schedule_with_steps(
        &admin, &beneficiary, &100_000, &20_000, &200, &1000, &500, &steps,
    );

    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 0);

    env.ledger().with_mut(|l| l.sequence_number = 150);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 20_000);

    env.ledger().with_mut(|l| l.sequence_number = 600);
    let vested = c.get_vested_amount(&beneficiary);
    assert_eq!(vested, 60_000);
}

#[test]
fn test_revoke_twice_fails() {
    let (env, admin, cid) = setup();
    let c = client(&env, &cid);
    env.ledger().with_mut(|l| l.sequence_number = 100);
    let beneficiary = Address::generate(&env);

    c.create_schedule(&admin, &beneficiary, &100_000, &0, &0, &1000, &false, &0);

    c.revoke(&admin, &beneficiary);
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        c.revoke(&admin, &beneficiary);
    }));
    assert!(result.is_err());
}
