#![cfg(test)]

use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env, String, Vec};

use kor_assetforge_contracts::arbitrator::{ArbContract, ArbContractClient};
use kor_assetforge_contracts::dispute_resolution::{
    DisputeOutcome, DisputeResolution, DisputeResolutionClient, DisputeStatus,
};

fn setup_arbitrator(env: &Env, min_stake: i128) -> (ArbContractClient, Address) {
    let id = env.register_contract(None, ArbContract);
    let client = ArbContractClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin, &min_stake);
    (client, admin)
}

fn setup_dispute(env: &Env) -> (DisputeResolutionClient, Address) {
    let id = env.register_contract(None, DisputeResolution);
    let client = DisputeResolutionClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_register_arbitrator() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup_arbitrator(&env, 1_000);
    let arb = Address::generate(&env);

    client.register(&arb, &5_000);

    let info = client.get_arbitrator(&arb).unwrap();
    assert_eq!(info.stake_amount, 5_000);
    assert!(info.active);
    assert_eq!(info.reputation, 100);
    assert_eq!(info.slash_count, 0);
    assert_eq!(client.get_active_count(), 1);
}

#[test]
fn test_deregister_arbitrator() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup_arbitrator(&env, 1_000);
    let arb = Address::generate(&env);

    client.register(&arb, &2_000);
    assert_eq!(client.get_active_count(), 1);

    client.deregister(&arb);

    let info = client.get_arbitrator(&arb).unwrap();
    assert!(!info.active);
    assert_eq!(client.get_active_count(), 0);
}

#[test]
#[should_panic(expected = "stake_amount below minimum required")]
fn test_register_below_min_stake() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup_arbitrator(&env, 5_000);
    let arb = Address::generate(&env);

    client.register(&arb, &100); // below minimum
}

#[test]
fn test_select_arbitrators_for_dispute() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup_arbitrator(&env, 100);

    let arb1 = Address::generate(&env);
    let arb2 = Address::generate(&env);
    let arb3 = Address::generate(&env);

    client.register(&arb1, &1_000);
    client.register(&arb2, &1_000);
    client.register(&arb3, &1_000);

    assert_eq!(client.get_active_count(), 3);

    let selected = client.select_for_dispute(&1u64, &3u32);
    assert_eq!(selected.len(), 3);
}

#[test]
fn test_select_fewer_than_active() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup_arbitrator(&env, 100);

    for _ in 0..5 {
        let arb = Address::generate(&env);
        client.register(&arb, &500);
    }

    let selected = client.select_for_dispute(&42u64, &3u32);
    assert_eq!(selected.len(), 3);
}

#[test]
fn test_slash_arbitrator() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_arbitrator(&env, 100);
    let arb = Address::generate(&env);

    client.register(&arb, &1_000);

    let info_before = client.get_arbitrator(&arb).unwrap();
    assert_eq!(info_before.stake_amount, 1_000);
    assert_eq!(info_before.reputation, 100);

    client.slash(&admin, &arb);

    let info_after = client.get_arbitrator(&arb).unwrap();
    assert_eq!(info_after.stake_amount, 500); // 50% slashed
    assert_eq!(info_after.reputation, 80);  // -20 reputation
    assert_eq!(info_after.slash_count, 1);
}

#[test]
fn test_multiple_slashes_deactivate_arbitrator() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_arbitrator(&env, 100);
    let arb = Address::generate(&env);

    client.register(&arb, &100_000);

    // Slash 5 times (5 * 20 = 100 reputation → 0)
    for _ in 0..5 {
        client.slash(&admin, &arb);
    }

    let info = client.get_arbitrator(&arb).unwrap();
    assert_eq!(info.reputation, 0);
    assert!(!info.active); // auto-deregistered at 0 reputation
    assert_eq!(client.get_active_count(), 0);
}

#[test]
fn test_cast_vote_and_finalize_arbitration() {
    let env = Env::default();
    env.mock_all_auths();

    let (dispute_client, dispute_admin) = setup_dispute(&env);
    let (arb_client, _) = setup_arbitrator(&env, 100);

    let arb1 = Address::generate(&env);
    let arb2 = Address::generate(&env);
    let arb3 = Address::generate(&env);

    arb_client.register(&arb1, &1_000);
    arb_client.register(&arb2, &1_000);
    arb_client.register(&arb3, &1_000);

    let buyer = Address::generate(&env);
    let seller = Address::generate(&env);

    let dispute_id = dispute_client.file_dispute(
        &buyer,
        &seller,
        &100,
        &String::from_str(&env, "item not received"),
        &500,
    );

    // Configure 3-day voting period
    let arb_contract_addr = arb_client.current_contract_address();
    dispute_client.set_arbitration_config(&dispute_admin, &arb_contract_addr, &259_200u64);

    // Admin initiates arbitration with 3 selected arbitrators
    let mut selected: Vec<Address> = Vec::new(&env);
    selected.push_back(arb1.clone());
    selected.push_back(arb2.clone());
    selected.push_back(arb3.clone());
    dispute_client.initiate_arbitration(&dispute_admin, &dispute_id, &selected);

    // Two arbitrators vote BuyerFavor, one votes SellerFavor
    dispute_client.cast_vote(&arb1, &dispute_id, &DisputeOutcome::BuyerFavor);
    dispute_client.cast_vote(&arb2, &dispute_id, &DisputeOutcome::BuyerFavor);
    dispute_client.cast_vote(&arb3, &dispute_id, &DisputeOutcome::SellerFavor);

    let summary = dispute_client.get_vote_summary(&dispute_id).unwrap();
    assert_eq!(summary.buyer_favor, 2);
    assert_eq!(summary.seller_favor, 1);
    assert_eq!(summary.split, 0);

    // Advance past voting deadline
    env.ledger().with_mut(|li| {
        li.timestamp = 259_200 + 1;
    });

    let release_to = dispute_client.finalize_arbitration(&dispute_id);
    assert_eq!(release_to, buyer); // majority BuyerFavor

    let dispute = dispute_client.get_dispute(&dispute_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);
    assert!(dispute.escrow_released);
}

#[test]
#[should_panic(expected = "arbitrator has already voted")]
fn test_cannot_vote_twice() {
    let env = Env::default();
    env.mock_all_auths();

    let (dispute_client, dispute_admin) = setup_dispute(&env);
    let (arb_client, _) = setup_arbitrator(&env, 100);

    let arb = Address::generate(&env);
    arb_client.register(&arb, &1_000);

    let buyer = Address::generate(&env);
    let seller = Address::generate(&env);

    let dispute_id = dispute_client.file_dispute(
        &buyer,
        &seller,
        &1,
        &String::from_str(&env, "test"),
        &100,
    );

    let arb_contract = arb_client.current_contract_address();
    dispute_client.set_arbitration_config(&dispute_admin, &arb_contract, &259_200u64);

    let mut selected: Vec<Address> = Vec::new(&env);
    selected.push_back(arb.clone());
    dispute_client.initiate_arbitration(&dispute_admin, &dispute_id, &selected);

    dispute_client.cast_vote(&arb, &dispute_id, &DisputeOutcome::BuyerFavor);
    dispute_client.cast_vote(&arb, &dispute_id, &DisputeOutcome::BuyerFavor); // panics
}

#[test]
#[should_panic(expected = "voting period has ended")]
fn test_cannot_vote_after_deadline() {
    let env = Env::default();
    env.mock_all_auths();

    let (dispute_client, dispute_admin) = setup_dispute(&env);
    let (arb_client, _) = setup_arbitrator(&env, 100);

    let arb = Address::generate(&env);
    arb_client.register(&arb, &1_000);

    let buyer = Address::generate(&env);
    let seller = Address::generate(&env);

    let dispute_id = dispute_client.file_dispute(
        &buyer,
        &seller,
        &1,
        &String::from_str(&env, "test"),
        &100,
    );

    let arb_contract = arb_client.current_contract_address();
    dispute_client.set_arbitration_config(&dispute_admin, &arb_contract, &259_200u64);

    let mut selected: Vec<Address> = Vec::new(&env);
    selected.push_back(arb.clone());
    dispute_client.initiate_arbitration(&dispute_admin, &dispute_id, &selected);

    // Advance past voting deadline
    env.ledger().with_mut(|li| {
        li.timestamp = 259_200 + 1;
    });

    dispute_client.cast_vote(&arb, &dispute_id, &DisputeOutcome::SellerFavor); // panics
}

#[test]
fn test_admin_fast_path_resolve_still_works() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup_dispute(&env);
    let buyer = Address::generate(&env);
    let seller = Address::generate(&env);

    let dispute_id = client.file_dispute(
        &buyer,
        &seller,
        &1,
        &String::from_str(&env, "admin resolves directly"),
        &200,
    );

    client.start_review(&admin, &dispute_id);
    let release_to = client.resolve_dispute(&admin, &dispute_id, &DisputeOutcome::SellerFavor);
    assert_eq!(release_to, seller);

    let dispute = client.get_dispute(&dispute_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);
}
