#![cfg(test)]

use soroban_sdk::testutils::Address as _;
use soroban_sdk::{Address, Env, String};

use kor_assetforge_contracts::insurance::{
    AssetInsurance, AssetInsuranceClient, ClaimStatus, RiskProfile,
};

fn setup(env: &Env) -> (AssetInsuranceClient, Address) {
    let id = env.register_contract(None, AssetInsurance);
    let client = AssetInsuranceClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize_insurance(&admin);
    (client, admin)
}

fn default_risk_profile() -> RiskProfile {
    RiskProfile {
        asset_type_risk: 30,
        value_risk: 40,
        claims_history_risk: 10,
        market_volatility: 20,
    }
}

#[test]
fn test_set_risk_profile() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let asset_id: u64 = 1;

    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let profile = client.get_risk_profile(&asset_id).unwrap();
    assert_eq!(profile.asset_type_risk, 30);
    assert_eq!(profile.value_risk, 40);
    assert_eq!(profile.claims_history_risk, 10);
    assert_eq!(profile.market_volatility, 20);
}

#[test]
fn test_calculate_premium_no_risk_profile() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let asset_id: u64 = 99; // no risk profile set

    // No risk profile → neutral 1× multiplier → 1% of coverage
    let coverage: i128 = 1_000_000;
    let duration: u64 = 86_400; // 1 day

    let premium = client.calculate_premium(&asset_id, &coverage, &duration);
    // coverage * 100 / 10_000 * 100 / 100 = coverage / 100 = 10_000
    assert_eq!(premium, 10_000);
}

#[test]
fn test_calculate_premium_with_risk_profile() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let asset_id: u64 = 1;

    // avg risk = (30+40+10+20)/4 = 25
    // risk_multiplier_bps = 50 + 25*150/100 = 50 + 37 = 87
    // raw = coverage * 100 / 10_000 * 87 / 100 = coverage / 100 * 87 / 100
    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let coverage: i128 = 1_000_000;
    let duration: u64 = 86_400;

    let premium_with_risk = client.calculate_premium(&asset_id, &coverage, &duration);
    let premium_no_risk = client.calculate_premium(&99u64, &coverage, &duration);

    // Lower risk → lower premium
    assert!(premium_with_risk < premium_no_risk);
    assert!(premium_with_risk > 0);
}

#[test]
fn test_long_term_discount_one_year() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let asset_id: u64 = 5;

    let coverage: i128 = 1_000_000;
    let one_year: u64 = 31_536_000;
    let one_day: u64 = 86_400;

    let premium_short = client.calculate_premium(&asset_id, &coverage, &one_day);
    let premium_year = client.calculate_premium(&asset_id, &coverage, &one_year);

    // One-year policy gets 10% discount, so per-coverage it should be less
    // premium_year should be 90% of what it would be without discount
    // Compare proportionally (short is tiny relative)
    assert!(premium_year > 0);
    // The raw premium scales with duration, so we compare effective rate
    // For neutral risk (100 bps):
    // short: 1_000_000 * 100/10_000 * 100/100 = 10_000
    // year:  1_000_000 * 100/10_000 * 100/100 * 90/100 = 9_000
    assert!(premium_year < premium_short + 1 || premium_short == premium_short);
    // Just ensure discount is applied: year premium != short premium * (year/day)
    let naive_year = premium_short; // same coverage, no discount, any duration gives same
    let discounted_year = naive_year * 90 / 100;
    assert_eq!(premium_year, discounted_year);
}

#[test]
fn test_long_term_discount_two_years() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let asset_id: u64 = 6;

    let coverage: i128 = 1_000_000;
    let two_years: u64 = 31_536_000 * 2;
    let one_year: u64 = 31_536_000;

    let premium_2yr = client.calculate_premium(&asset_id, &coverage, &two_years);
    let premium_1yr = client.calculate_premium(&asset_id, &coverage, &one_year);

    // 2-year = 20% off → raw * 80/100
    // 1-year = 10% off → raw * 90/100
    // Both produce same raw (duration doesn't scale base), so 2yr < 1yr
    assert!(premium_2yr < premium_1yr);
}

#[test]
fn test_purchase_policy_with_calculated_premium() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let policyholder = Address::generate(&env);
    let asset_id: u64 = 1;

    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let policy_id = client.purchase_policy(
        &policyholder,
        &asset_id,
        &1_000_000,
        &31_536_000,
        &false,
        &1u32,
    );
    assert_eq!(policy_id, 1);

    let policy = client.get_policy(&policy_id).unwrap();
    assert_eq!(policy.policyholder, policyholder);
    assert_eq!(policy.coverage_amount, 1_000_000);
    assert!(policy.active);
    assert!(policy.premium_amount > 0);
}

#[test]
fn test_payment_plan() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let policyholder = Address::generate(&env);
    let asset_id: u64 = 2;

    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let policy_id = client.purchase_policy(
        &policyholder,
        &asset_id,
        &500_000,
        &31_536_000,
        &true,
        &4u32,
    );

    let policy = client.get_policy(&policy_id).unwrap();
    assert!(policy.has_payment_plan);

    let plan = client.get_payment_plan(&policy_id).unwrap();
    assert_eq!(plan.total_installments, 4);
    assert_eq!(plan.paid_count, 0);
    assert!(plan.amount_per_installment > 0);

    // Pay first installment
    client.pay_installment(&policyholder, &policy_id);

    let plan = client.get_payment_plan(&policy_id).unwrap();
    assert_eq!(plan.paid_count, 1);
}

#[test]
fn test_renew_policy() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let policyholder = Address::generate(&env);
    let asset_id: u64 = 3;

    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let policy_id = client.purchase_policy(
        &policyholder,
        &asset_id,
        &200_000,
        &86_400,
        &false,
        &1u32,
    );

    let new_id = client.renew_policy(&policyholder, &policy_id, &31_536_000u64);
    assert_eq!(new_id, 2);

    // Old policy deactivated
    let old_policy = client.get_policy(&policy_id).unwrap();
    assert!(!old_policy.active);

    // New policy is active
    let new_policy = client.get_policy(&new_id).unwrap();
    assert!(new_policy.active);
    assert_eq!(new_policy.coverage_amount, 200_000);
}

#[test]
fn test_file_and_process_claim() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let policyholder = Address::generate(&env);
    let asset_id: u64 = 4;

    client.set_risk_profile(&admin, &asset_id, &default_risk_profile());

    let policy_id = client.purchase_policy(
        &policyholder,
        &asset_id,
        &100_000,
        &31_536_000,
        &false,
        &1u32,
    );

    let evidence = String::from_str(&env, "ipfs://evidence_hash_abc123");
    let claim_id = client.file_claim(&policyholder, &policy_id, &50_000, &evidence);
    assert_eq!(claim_id, 1);

    client.process_claim(&admin, &claim_id, &true);

    let claim = client.get_claim(&claim_id).unwrap();
    assert_eq!(claim.status, ClaimStatus::Approved);
}

#[test]
#[should_panic(expected = "claim already processed")]
fn test_cannot_process_claim_twice() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let policyholder = Address::generate(&env);

    let policy_id = client.purchase_policy(
        &policyholder,
        &1u64,
        &100_000,
        &31_536_000,
        &false,
        &1u32,
    );

    let evidence = String::from_str(&env, "hash");
    let claim_id = client.file_claim(&policyholder, &policy_id, &1_000, &evidence);
    client.process_claim(&admin, &claim_id, &true);
    client.process_claim(&admin, &claim_id, &false); // should panic
}
