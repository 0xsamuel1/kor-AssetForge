extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::reputation::{
    ReputationContract, ReputationContractClient, DECAY_PERIOD, INITIAL_SCORE, MAX_SCORE,
    MIN_SCORE,
};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

fn setup(env: &Env) -> (ReputationContractClient, Address) {
    let cid = env.register_contract(None, ReputationContract);
    let client = ReputationContractClient::new(env, &cid);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_initial_score_is_500() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _admin) = setup(&env);
    let user = Address::generate(&env);
    assert_eq!(client.get_score(&user), INITIAL_SCORE);
}

#[test]
fn test_trade_completion_boosts_score() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade_completion(&admin, &user);
    assert_eq!(client.get_score(&user), INITIAL_SCORE + 10);
}

#[test]
fn test_dispute_reduces_score() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_dispute(&admin, &user);
    assert_eq!(client.get_score(&user), INITIAL_SCORE - 50);
}

#[test]
fn test_governance_participation_boosts_score() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_governance_participation(&admin, &user);
    assert_eq!(client.get_score(&user), INITIAL_SCORE + 5);
}

#[test]
fn test_score_capped_at_max() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    for _ in 0..60 {
        client.record_trade_completion(&admin, &user);
    }
    assert_eq!(client.get_score(&user), MAX_SCORE);
}

#[test]
fn test_score_floored_at_min() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    for _ in 0..20 {
        client.record_dispute(&admin, &user);
    }
    assert_eq!(client.get_score(&user), MIN_SCORE);
}

#[test]
fn test_decay_over_time() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade_completion(&admin, &user); // score = 510
    env.ledger().with_mut(|l| { l.timestamp += DECAY_PERIOD * 2; });
    assert_eq!(client.get_score(&user), 490); // 510 - 2*10
}

#[test]
fn test_record_stores_trade_count() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_trade_completion(&admin, &user);
    client.record_trade_completion(&admin, &user);
    let rec = client.get_record(&user);
    assert_eq!(rec.trade_completions, 2);
}

#[test]
fn test_record_stores_dispute_count() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, admin) = setup(&env);
    let user = Address::generate(&env);
    client.record_dispute(&admin, &user);
    let rec = client.get_record(&user);
    assert_eq!(rec.disputes, 1);
}

#[test]
#[should_panic(expected = "admin only")]
fn test_non_admin_panics() {
    let env = Env::default();
    env.mock_all_auths();
    let (client, _admin) = setup(&env);
    let user = Address::generate(&env);
    let non_admin = Address::generate(&env);
    client.record_trade_completion(&non_admin, &user);
}
