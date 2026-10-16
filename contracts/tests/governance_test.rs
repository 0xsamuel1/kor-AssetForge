// Integration tests for governance proposal templates (Issue #211).
//
// Covers template listing, versioning, parameter-bounds validation, and
// end-to-end proposal creation from a template.

#![cfg(test)]

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::asset_token::{AssetToken, AssetTokenClient};
use kor_assetforge_contracts::emergency_control::EmergencyControl;
use kor_assetforge_contracts::governance::{
    Governance, GovernanceClient, ProposalStatus, ProposalTemplate,
};
use soroban_sdk::testutils::{Address as _, Ledger as _};
use soroban_sdk::{Address, Env, String};

fn setup() -> (Env, Address, Address, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();

    let ec_id = env.register_contract(None, EmergencyControl);
    let ec_client =
        kor_assetforge_contracts::emergency_control::EmergencyControlClient::new(&env, &ec_id);
    let admin = Address::generate(&env);
    ec_client.initialize(&admin);

    let at_id = env.register_contract(None, AssetToken);
    let at_client = AssetTokenClient::new(&env, &at_id);
    at_client.initialize(
        &admin,
        &String::from_str(&env, "GovToken"),
        &String::from_str(&env, "GOV"),
        &7,
        &0,
    );

    let gov_id = env.register_contract(None, Governance);
    let gov_client = GovernanceClient::new(&env, &gov_id);
    gov_client.initialize(&admin, &at_id, &100, &50);

    (env, gov_id, at_id, ec_id, admin)
}

fn mint(env: &Env, at_id: &Address, ec_id: &Address, to: &Address, amount: i128) {
    let at_client = AssetTokenClient::new(env, at_id);
    at_client.mint(to, &amount, &1, ec_id);
}

// ---------------------------------------------------------------------------
// Listing & versioning
// ---------------------------------------------------------------------------

#[test]
fn test_list_templates_returns_full_catalogue() {
    let (env, gov_id, _at, _ec, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);
    assert_eq!(client.list_templates().len(), 10);
}

#[test]
fn test_template_version_defaults_to_one() {
    let (env, gov_id, _at, _ec, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);
    assert_eq!(client.get_template_version(&ProposalTemplate::FeeChange), 1);
}

#[test]
fn test_register_template_version_bumps() {
    let (env, gov_id, _at, _ec, admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);

    client.register_template_version(&admin, &ProposalTemplate::FeeChange, &2);
    assert_eq!(client.get_template_version(&ProposalTemplate::FeeChange), 2);
}

// ---------------------------------------------------------------------------
// Parameter-bounds validation
// ---------------------------------------------------------------------------

#[test]
fn test_validate_template_params() {
    let (env, gov_id, _at, _ec, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);

    // FeeChange caps fee bps at 1000; 500 is valid, 2000 is not.
    assert!(client.validate_template_params(
        &ProposalTemplate::FeeChange,
        &1,
        &500,
        &0,
        &None
    ));
    assert!(!client.validate_template_params(
        &ProposalTemplate::FeeChange,
        &1,
        &2000,
        &0,
        &None
    ));

    // TreasurySpend requires a target address.
    let recipient = Address::generate(&env);
    assert!(client.validate_template_params(
        &ProposalTemplate::TreasurySpend,
        &1,
        &0,
        &1_000,
        &Some(recipient)
    ));
    assert!(!client.validate_template_params(
        &ProposalTemplate::TreasurySpend,
        &1,
        &0,
        &1_000,
        &None
    ));

    // Wrong version fails validation.
    assert!(!client.validate_template_params(
        &ProposalTemplate::FeeChange,
        &99,
        &500,
        &0,
        &None
    ));
}

#[test]
#[should_panic(expected = "template parameters out of bounds")]
fn test_create_from_template_rejects_out_of_bounds() {
    let (env, gov_id, at_id, ec_id, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);
    let proposer = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);

    // 2000 bps exceeds the FeeChange ceiling of 1000.
    client.create_proposal_from_template(
        &proposer,
        &ProposalTemplate::FeeChange,
        &1,
        &2000,
        &0,
        &None,
        &String::from_str(&env, "raise fee"),
        &3600,
    );
}

#[test]
#[should_panic(expected = "template requires a target address")]
fn test_create_from_template_requires_target() {
    let (env, gov_id, at_id, ec_id, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);
    let proposer = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);

    client.create_proposal_from_template(
        &proposer,
        &ProposalTemplate::TreasurySpend,
        &1,
        &0,
        &1_000,
        &None,
        &String::from_str(&env, "spend treasury"),
        &3600,
    );
}

#[test]
#[should_panic(expected = "unsupported template version")]
fn test_create_from_template_rejects_stale_version() {
    let (env, gov_id, at_id, ec_id, admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);
    let proposer = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);

    // Bump to v2; a proposal built against v1 must be rejected.
    client.register_template_version(&admin, &ProposalTemplate::FeeChange, &2);
    client.create_proposal_from_template(
        &proposer,
        &ProposalTemplate::FeeChange,
        &1,
        &500,
        &0,
        &None,
        &String::from_str(&env, "stale"),
        &3600,
    );
}

// ---------------------------------------------------------------------------
// End-to-end template proposal
// ---------------------------------------------------------------------------

#[test]
fn test_create_and_pass_add_asset_template() {
    let (env, gov_id, at_id, ec_id, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);

    let proposer = Address::generate(&env);
    let voter = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);
    mint(&env, &at_id, &ec_id, &voter, 150);

    // AddAsset template: param_u64 is the asset id to approve.
    let pid = client.create_proposal_from_template(
        &proposer,
        &ProposalTemplate::AddAsset,
        &1,
        &77,
        &0,
        &None,
        &String::from_str(&env, "list asset 77"),
        &3600,
    );

    // Template metadata is recorded alongside the proposal.
    let meta = client.get_template_proposal(&pid).unwrap();
    assert_eq!(meta.template, ProposalTemplate::AddAsset);
    assert_eq!(meta.param_u64, 77);
    assert_eq!(meta.version, 1);

    // Vote it through; on tally the asset id is approved.
    client.vote(&voter, &pid, &true);
    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.tally_execute(&pid);

    let proposal = client.get_proposal(&pid).unwrap();
    assert_eq!(proposal.status, ProposalStatus::Passed);
    assert!(client.is_approved(&77));
}

#[test]
fn test_text_proposal_template() {
    let (env, gov_id, at_id, ec_id, _admin) = setup();
    let client = GovernanceClient::new(&env, &gov_id);

    let proposer = Address::generate(&env);
    mint(&env, &at_id, &ec_id, &proposer, 200);

    let pid = client.create_proposal_from_template(
        &proposer,
        &ProposalTemplate::TextProposal,
        &1,
        &0,
        &0,
        &None,
        &String::from_str(&env, "signal: thanks to contributors"),
        &3600,
    );

    let meta = client.get_template_proposal(&pid).unwrap();
    assert_eq!(meta.template, ProposalTemplate::TextProposal);
}
