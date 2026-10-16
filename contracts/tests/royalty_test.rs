#![cfg(test)]

use soroban_sdk::testutils::Address as _;
use soroban_sdk::{Address, Env, Vec};

use kor_assetforge_contracts::royalty::{
    Royalty, RoyaltyBeneficiary, RoyaltyClient,
};

fn setup() -> (Env, RoyaltyClient<'static>, Address) {
    let env = Env::default();
    env.mock_all_auths();

    let contract_id = env.register_contract(None, Royalty);
    let client = RoyaltyClient::new(&env, &contract_id);
    let admin = Address::generate(&env);

    client.initialize(&admin);
    (env, client, admin)
}

#[test]
fn test_initialize() {
    let env = Env::default();
    env.mock_all_auths();

    let contract_id = env.register_contract(None, Royalty);
    let client = RoyaltyClient::new(&env, &contract_id);
    let admin = Address::generate(&env);

    client.initialize(&admin);
    let stored_admin = client.get_royalty_admin();
    assert_eq!(stored_admin, admin);
}

#[test]
#[should_panic(expected = "already initialized")]
fn test_double_initialize_panics() {
    let env = Env::default();
    env.mock_all_auths();

    let contract_id = env.register_contract(None, Royalty);
    let client = RoyaltyClient::new(&env, &contract_id);
    let admin = Address::generate(&env);

    client.initialize(&admin);
    client.initialize(&admin);
}

#[test]
fn test_set_and_get_royalty() {
    let (env, client, admin) = setup();
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &100_000, &beneficiaries);

    let config = client.get_royalty(&1).unwrap();
    assert_eq!(config.percentage_bps, 500);
    assert_eq!(config.cap, 100_000);
    assert_eq!(config.beneficiaries.len(), 1);
    assert_eq!(config.beneficiaries.get(0).unwrap().address, beneficiary);
}

#[test]
fn test_get_royalty_none() {
    let (_env, client, _admin) = setup();
    let config = client.get_royalty(&999);
    assert!(config.is_none());
}

#[test]
fn test_calculate_royalty_basic() {
    let (env, client, admin) = setup();
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &0, &beneficiaries);

    let royalty = client.calculate_royalty(&1, &1_000_000);
    assert_eq!(royalty, 50_000);
}

#[test]
fn test_calculate_royalty_cap() {
    let (env, client, admin) = setup();
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &10_000, &beneficiaries);

    let royalty = client.calculate_royalty(&1, &1_000_000);
    assert_eq!(royalty, 10_000);
}

#[test]
fn test_calculate_royalty_no_config() {
    let (env, client, _admin) = setup();
    let royalty = client.calculate_royalty(&1, &1_000_000);
    assert_eq!(royalty, 0);
}

#[test]
fn test_record_royalty() {
    let (env, client, admin) = setup();
    let buyer = Address::generate(&env);
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &0, &beneficiaries);

    let amount = client.record_royalty(&1, &buyer, &1_000_000);
    assert_eq!(amount, 50_000);

    let history = client.get_royalty_history(&1);
    assert_eq!(history.len(), 1);
    let record = history.get(0).unwrap();
    assert_eq!(record.asset_id, 1);
    assert_eq!(record.sale_amount, 1_000_000);
    assert_eq!(record.royalty_amount, 50_000);
}

#[test]
fn test_record_royalty_no_config() {
    let (_env, client, _admin) = setup();
    let buyer = Address::generate(&_env);

    let amount = client.record_royalty(&1, &buyer, &1_000_000);
    assert_eq!(amount, 0);
}

#[test]
fn test_multi_beneficiary() {
    let (env, client, admin) = setup();
    let beneficiary1 = Address::generate(&env);
    let beneficiary2 = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary1.clone(), share_bps: 6000 },
        RoyaltyBeneficiary { address: beneficiary2.clone(), share_bps: 4000 },
    ]);

    client.set_royalty(&admin, &1, &500, &0, &beneficiaries);

    let config = client.get_royalty(&1).unwrap();
    assert_eq!(config.beneficiaries.len(), 2);

    let royalty = client.calculate_royalty(&1, &1_000_000);
    assert_eq!(royalty, 50_000);
}

#[test]
fn test_max_royalty_bps() {
    let (env, client, admin) = setup();
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &1000, &0, &beneficiaries);

    let config = client.get_royalty(&1).unwrap();
    assert_eq!(config.percentage_bps, 1000);

    let royalty = client.calculate_royalty(&1, &1_000_000);
    assert_eq!(royalty, 100_000);
}

#[test]
fn test_royalty_history_multiple() {
    let (env, client, admin) = setup();
    let buyer = Address::generate(&env);
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &0, &beneficiaries);

    client.record_royalty(&1, &buyer, &1_000_000);
    client.record_royalty(&1, &buyer, &2_000_000);

    let history = client.get_royalty_history(&1);
    assert_eq!(history.len(), 2);
    assert_eq!(history.get(0).unwrap().royalty_amount, 50_000);
    assert_eq!(history.get(1).unwrap().royalty_amount, 100_000);
}

#[test]
fn test_royalty_per_asset_isolation() {
    let (env, client, admin) = setup();
    let buyer = Address::generate(&env);
    let beneficiary = Address::generate(&env);

    let beneficiaries = Vec::from_array(&env, [
        RoyaltyBeneficiary { address: beneficiary.clone(), share_bps: 10000 },
    ]);

    client.set_royalty(&admin, &1, &500, &0, &beneficiaries);
    client.set_royalty(&admin, &2, &1000, &0, &beneficiaries);

    assert_eq!(client.calculate_royalty(&1, &1_000_000), 50_000);
    assert_eq!(client.calculate_royalty(&2, &1_000_000), 100_000);

    client.record_royalty(&1, &buyer, &1_000_000);

    let hist1 = client.get_royalty_history(&1);
    assert_eq!(hist1.len(), 1);

    let hist2 = client.get_royalty_history(&2);
    assert_eq!(hist2.len(), 0);
}
