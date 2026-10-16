extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::asset_token::{AssetToken, AssetTokenClient};
use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};
use kor_assetforge_contracts::governance::{Governance, GovernanceClient, ProposalStatus};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env, String};

fn setup(env: &Env) -> (GovernanceClient, Address, Address, Address) {
    let ec_id = env.register_contract(None, EmergencyControl);
    let ec_client = EmergencyControlClient::new(env, &ec_id);
    let admin = Address::generate(env);
    ec_client.initialize(&admin);

    let at_id = env.register_contract(None, AssetToken);
    let at_client = AssetTokenClient::new(env, &at_id);
    at_client.initialize(
        &admin,
        &String::from_str(env, "GovToken"),
        &String::from_str(env, "GOV"),
        &7,
        &0,
    );

    let gov_id = env.register_contract(None, Governance);
    let gov_client = GovernanceClient::new(env, &gov_id);
    gov_client.initialize(&admin, &at_id, &100, &50);

    (gov_client, at_id, ec_id, admin)
}

fn mint(env: &Env, at_id: &Address, ec_id: &Address, to: &Address, amount: i128) {
    AssetTokenClient::new(env, at_id).mint(to, &amount, &1, ec_id);
}

#[test]
fn test_create_snapshot_returns_id() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);
    let user = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &user, 200);

    let snap_id = gov.create_snapshot(&user, &3600);
    assert_eq!(snap_id, 1);
}

#[test]
fn test_snapshot_balance_matches_mint() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);
    let user = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &user, 300);

    let snap_id = gov.create_snapshot(&user, &3600);
    let bal = gov.get_snapshot_balance(&snap_id, &user);
    assert_eq!(bal, 300);
}

#[test]
#[should_panic(expected = "snapshot expired")]
fn test_snapshot_expires() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);
    let user = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &user, 100);

    let snap_id = gov.create_snapshot(&user, &3600);
    env.ledger().with_mut(|l| { l.timestamp += 3601; });
    gov.get_snapshot_balance(&snap_id, &user);
}

#[test]
fn test_vote_with_snapshot_counts_correctly() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);

    let proposer = Address::generate(&env);
    let voter = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);
    mint(&env, &at_id, &ec_id, &voter, 120);

    let snap_id = gov.create_snapshot(&voter, &7200);

    let pid = gov.create_proposal(
        &proposer,
        &1,
        &String::from_str(&env, "Snapshot vote test"),
        &3600,
    );

    gov.vote_with_snapshot(&voter, &pid, &snap_id, &true);

    let proposal = gov.get_proposal(&pid).unwrap();
    assert_eq!(proposal.votes_for, 120);
    assert!(gov.has_voted(&pid, &voter));
}

#[test]
#[should_panic(expected = "already voted")]
fn test_double_vote_with_snapshot_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);

    let proposer = Address::generate(&env);
    let voter = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);
    mint(&env, &at_id, &ec_id, &voter, 100);

    let snap_id = gov.create_snapshot(&voter, &7200);
    let pid = gov.create_proposal(
        &proposer,
        &1,
        &String::from_str(&env, "Double vote test"),
        &3600,
    );
    gov.vote_with_snapshot(&voter, &pid, &snap_id, &true);
    gov.vote_with_snapshot(&voter, &pid, &snap_id, &false);
}

#[test]
fn test_snapshot_ids_increment() {
    let env = Env::default();
    env.mock_all_auths();
    let (gov, at_id, ec_id, _admin) = setup(&env);
    let user = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &user, 100);

    let id1 = gov.create_snapshot(&user, &3600);
    let id2 = gov.create_snapshot(&user, &3600);
    assert_eq!(id2, id1 + 1);
}
