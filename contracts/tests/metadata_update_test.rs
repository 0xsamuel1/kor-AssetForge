#[cfg(test)]
mod tests {
    use kor_assetforge_contracts::asset_token::{
        AssetToken, AssetTokenClient, MetadataUpdateStatus,
    };
    use kor_assetforge_contracts::governance::{Governance, GovernanceClient, ProposalStatus};
    use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};
    use soroban_sdk::testutils::{Address as _, Ledger};
    use soroban_sdk::{Address, Env, String, Vec};

    fn setup_asset_token(env: &Env) -> (AssetTokenClient<'_>, Address, Address) {
        let at_id = env.register_contract(None, AssetToken);
        let client = AssetTokenClient::new(env, &at_id);

        let ec_addr = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(env, &ec_addr);

        let admin = Address::generate(env);
        ec_client.initialize(&admin);

        let name = String::from_str(env, "Real Estate Token");
        let symbol = String::from_str(env, "RET");
        let supply: i128 = 1_000_000;

        client.initialize(&admin, &name, &symbol, &7, &supply);
        (client, admin, ec_addr)
    }

    fn setup_governance(env: &Env, at_id: &Address) -> (GovernanceClient<'_>, Address) {
        let gov_id = env.register_contract(None, Governance);
        let client = GovernanceClient::new(env, &gov_id);
        let admin = Address::generate(env);
        client.initialize(&admin, at_id, &100, &50);
        (client, admin)
    }

    #[test]
    fn test_set_initial_metadata() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Tokenized Villa #1");
        let description = String::from_str(&env, "A luxury villa in Miami");
        let symbol = String::from_str(&env, "VIL001");
        let image_uri = String::from_str(&env, "ipfs://QmAsset123");
        let external_url = String::from_str(&env, "https://example.com/villa/1");

        client.set_asset_metadata(
            &admin,
            &1,
            &name,
            &description,
            &symbol,
            &image_uri,
            &external_url,
        );

        let metadata = client.get_asset_metadata(&1).unwrap();
        assert_eq!(metadata.name, name);
        assert_eq!(metadata.description, description);
        assert_eq!(metadata.symbol, symbol);
        assert_eq!(metadata.image_uri, image_uri);
        assert_eq!(metadata.external_url, external_url);
        assert_eq!(metadata.version, 1);
        assert!(!metadata.is_immutable);
    }

    #[test]
    #[should_panic(expected = "metadata already set")]
    fn test_cannot_set_metadata_twice() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Test");
        let description = String::from_str(&env, "Desc");
        let symbol = String::from_str(&env, "TST");
        let image_uri = String::from_str(&env, "ipfs://test");
        let external_url = String::from_str(&env, "https://test.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);
        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);
    }

    #[test]
    fn test_propose_metadata_update() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Tokenized Villa #1");
        let description = String::from_str(&env, "A luxury villa in Miami");
        let symbol = String::from_str(&env, "VIL001");
        let image_uri = String::from_str(&env, "ipfs://QmAsset123");
        let external_url = String::from_str(&env, "https://example.com/villa/1");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        let new_name = String::from_str(&env, "Updated Villa #1");
        let new_description = String::from_str(&env, "Renovated luxury villa");
        let new_symbol = String::from_str(&env, "VIL001R");
        let new_image = String::from_str(&env, "ipfs://QmAsset456");
        let new_url = String::from_str(&env, "https://example.com/villa/1-v2");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        assert_eq!(request_id, 1);

        let request = client.get_metadata_update_request(&request_id).unwrap();
        assert_eq!(request.asset_id, 1);
        assert_eq!(request.requester, admin);
        assert!(!request.requires_governance);
        assert_eq!(request.status, MetadataUpdateStatus::Pending);
        assert!(request.timelock_until > env.ledger().timestamp());
    }

    #[test]
    fn test_execute_metadata_update_after_timelock() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Original Name");
        let description = String::from_str(&env, "Original Desc");
        let symbol = String::from_str(&env, "ORG");
        let image_uri = String::from_str(&env, "ipfs://original");
        let external_url = String::from_str(&env, "https://original.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        let new_name = String::from_str(&env, "Updated Name");
        let new_description = String::from_str(&env, "Updated Desc");
        let new_symbol = String::from_str(&env, "UPD");
        let new_image = String::from_str(&env, "ipfs://updated");
        let new_url = String::from_str(&env, "https://updated.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        // Advance past the 48-hour timelock
        env.ledger().with_mut(|li| {
            li.timestamp += 172800;
        });

        client.execute_metadata_update(&request_id);

        let metadata = client.get_asset_metadata(&1).unwrap();
        assert_eq!(metadata.name, new_name);
        assert_eq!(metadata.description, new_description);
        assert_eq!(metadata.symbol, new_symbol);
        assert_eq!(metadata.image_uri, new_image);
        assert_eq!(metadata.external_url, new_url);
        assert_eq!(metadata.version, 2);

        let request = client.get_metadata_update_request(&request_id).unwrap();
        assert_eq!(request.status, MetadataUpdateStatus::Executed);

        let history = client.get_metadata_update_history(&1);
        assert_eq!(history.len(), 1);
        assert_eq!(history.get(0).unwrap().version, 2);
    }

    #[test]
    #[should_panic(expected = "timelock not expired")]
    fn test_cannot_execute_before_timelock() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Original Name");
        let description = String::from_str(&env, "Original Desc");
        let symbol = String::from_str(&env, "ORG");
        let image_uri = String::from_str(&env, "ipfs://original");
        let external_url = String::from_str(&env, "https://original.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        let new_name = String::from_str(&env, "Updated Name");
        let new_description = String::from_str(&env, "Updated Desc");
        let new_symbol = String::from_str(&env, "UPD");
        let new_image = String::from_str(&env, "ipfs://updated");
        let new_url = String::from_str(&env, "https://updated.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        // Try to execute immediately without waiting for timelock
        client.execute_metadata_update(&request_id);
    }

    #[test]
    fn test_cancel_pending_update() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Original Name");
        let description = String::from_str(&env, "Original Desc");
        let symbol = String::from_str(&env, "ORG");
        let image_uri = String::from_str(&env, "ipfs://original");
        let external_url = String::from_str(&env, "https://original.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        let new_name = String::from_str(&env, "Updated Name");
        let new_description = String::from_str(&env, "Updated Desc");
        let new_symbol = String::from_str(&env, "UPD");
        let new_image = String::from_str(&env, "ipfs://updated");
        let new_url = String::from_str(&env, "https://updated.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        client.cancel_metadata_update(&admin, &request_id);

        let request = client.get_metadata_update_request(&request_id).unwrap();
        assert_eq!(request.status, MetadataUpdateStatus::Cancelled);

        // Metadata should remain unchanged
        let metadata = client.get_asset_metadata(&1).unwrap();
        assert_eq!(metadata.name, name);
        assert_eq!(metadata.version, 1);
    }

    #[test]
    #[should_panic(expected = "not requester")]
    fn test_only_requester_can_cancel() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Original Name");
        let description = String::from_str(&env, "Original Desc");
        let symbol = String::from_str(&env, "ORG");
        let image_uri = String::from_str(&env, "ipfs://original");
        let external_url = String::from_str(&env, "https://original.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        let new_name = String::from_str(&env, "Updated Name");
        let new_description = String::from_str(&env, "Updated Desc");
        let new_symbol = String::from_str(&env, "UPD");
        let new_image = String::from_str(&env, "ipfs://updated");
        let new_url = String::from_str(&env, "https://updated.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        let other = Address::generate(&env);
        client.cancel_metadata_update(&other, &request_id);
    }

    #[test]
    fn test_set_metadata_immutable() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Immutable Asset");
        let description = String::from_str(&env, "Cannot be changed");
        let symbol = String::from_str(&env, "IMM");
        let image_uri = String::from_str(&env, "ipfs://immutable");
        let external_url = String::from_str(&env, "https://immutable.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        client.set_metadata_immutable(&admin, &1);

        let metadata = client.get_asset_metadata(&1).unwrap();
        assert!(metadata.is_immutable);
    }

    #[test]
    fn test_immutable_metadata_requires_governance() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Immutable Asset");
        let description = String::from_str(&env, "Cannot be changed");
        let symbol = String::from_str(&env, "IMM");
        let image_uri = String::from_str(&env, "ipfs://immutable");
        let external_url = String::from_str(&env, "https://immutable.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);
        client.set_metadata_immutable(&admin, &1);

        let new_name = String::from_str(&env, "New Name");
        let new_description = String::from_str(&env, "New Desc");
        let new_symbol = String::from_str(&env, "NEW");
        let new_image = String::from_str(&env, "ipfs://new");
        let new_url = String::from_str(&env, "https://new.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        let request = client.get_metadata_update_request(&request_id).unwrap();
        assert!(request.requires_governance);
    }

    #[test]
    fn test_governance_approval_for_immutable_metadata() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Immutable Asset");
        let description = String::from_str(&env, "Needs governance");
        let symbol = String::from_str(&env, "IMM");
        let image_uri = String::from_str(&env, "ipfs://immutable");
        let external_url = String::from_str(&env, "https://immutable.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);
        client.set_metadata_immutable(&admin, &1);

        // Set governance contract
        let gov_addr = Address::generate(&env);
        client.set_metadata_governance(&admin, &gov_addr);

        let new_name = String::from_str(&env, "New Name");
        let new_description = String::from_str(&env, "New Desc");
        let new_symbol = String::from_str(&env, "NEW");
        let new_image = String::from_str(&env, "ipfs://new");
        let new_url = String::from_str(&env, "https://new.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        let request = client.get_metadata_update_request(&request_id).unwrap();
        assert!(request.requires_governance);

        // Governance approves the update
        client.approve_metadata_update(&gov_addr, &request_id);

        // Advance past timelock and execute
        env.ledger().with_mut(|li| {
            li.timestamp += 172800;
        });

        client.execute_metadata_update(&request_id);

        let metadata = client.get_asset_metadata(&1).unwrap();
        assert_eq!(metadata.name, new_name);
        assert_eq!(metadata.description, new_description);
        assert_eq!(metadata.version, 3); // initial + immutable + update
    }

    #[test]
    #[should_panic(expected = "not governance contract")]
    fn test_only_governance_can_approve() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Immutable Asset");
        let description = String::from_str(&env, "Needs governance");
        let symbol = String::from_str(&env, "IMM");
        let image_uri = String::from_str(&env, "ipfs://immutable");
        let external_url = String::from_str(&env, "https://immutable.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);
        client.set_metadata_immutable(&admin, &1);

        let gov_addr = Address::generate(&env);
        client.set_metadata_governance(&admin, &gov_addr);

        let new_name = String::from_str(&env, "New Name");
        let new_description = String::from_str(&env, "New Desc");
        let new_symbol = String::from_str(&env, "NEW");
        let new_image = String::from_str(&env, "ipfs://new");
        let new_url = String::from_str(&env, "https://new.com");

        let request_id = client.propose_metadata_update(
            &admin,
            &1,
            &new_name,
            &new_description,
            &new_symbol,
            &new_image,
            &new_url,
        );

        let fake_governance = Address::generate(&env);
        client.approve_metadata_update(&fake_governance, &request_id);
    }

    #[test]
    fn test_metadata_update_history_tracking() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup_asset_token(&env);

        let name = String::from_str(&env, "Version 1");
        let description = String::from_str(&env, "Description v1");
        let symbol = String::from_str(&env, "V1");
        let image_uri = String::from_str(&env, "ipfs://v1");
        let external_url = String::from_str(&env, "https://v1.com");

        client.set_asset_metadata(&admin, &1, &name, &description, &symbol, &image_uri, &external_url);

        // First update
        let new_name1 = String::from_str(&env, "Version 2");
        let new_desc1 = String::from_str(&env, "Description v2");
        let new_sym1 = String::from_str(&env, "V2");
        let new_img1 = String::from_str(&env, "ipfs://v2");
        let new_url1 = String::from_str(&env, "https://v2.com");

        let rid1 = client.propose_metadata_update(
            &admin, &1, &new_name1, &new_desc1, &new_sym1, &new_img1, &new_url1,
        );

        env.ledger().with_mut(|li| { li.timestamp += 172800; });
        client.execute_metadata_update(&rid1);

        // Second update
        let new_name2 = String::from_str(&env, "Version 3");
        let new_desc2 = String::from_str(&env, "Description v3");
        let new_sym2 = String::from_str(&env, "V3");
        let new_img2 = String::from_str(&env, "ipfs://v3");
        let new_url2 = String::from_str(&env, "https://v3.com");

        let rid2 = client.propose_metadata_update(
            &admin, &1, &new_name2, &new_desc2, &new_sym2, &new_img2, &new_url2,
        );

        env.ledger().with_mut(|li| { li.timestamp += 172800; });
        client.execute_metadata_update(&rid2);

        let history = client.get_metadata_update_history(&1);
        assert_eq!(history.len(), 2);

        let first_record = history.get(0).unwrap();
        assert_eq!(first_record.version, 2);
        assert_eq!(first_record.old_metadata.name, name);

        let second_record = history.get(1).unwrap();
        assert_eq!(second_record.version, 3);
        assert_eq!(second_record.old_metadata.name, new_name1);
        assert_eq!(second_record.new_metadata.name, new_name2);
    }

    #[test]
    fn test_governance_metadata_proposal_workflow() {
        let env = Env::default();
        env.mock_all_auths();

        // Setup
        let at_id = env.register_contract(None, AssetToken);
        let at_client = AssetTokenClient::new(&env, &at_id);

        let ec_addr = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_addr);

        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let name = String::from_str(&env, "Governance Token");
        let symbol = String::from_str(&env, "GOV");
        at_client.initialize(&admin, &name, &symbol, &7, &1_000_000);

        // Setup governance
        let gov_id = env.register_contract(None, Governance);
        let gov_client = GovernanceClient::new(&env, &gov_id);
        gov_client.initialize(&admin, &at_id, &100, &50);

        // Set metadata
        let meta_name = String::from_str(&env, "Original Name");
        let meta_desc = String::from_str(&env, "Original Desc");
        let meta_sym = String::from_str(&env, "ORG");
        let meta_img = String::from_str(&env, "ipfs://original");
        let meta_url = String::from_str(&env, "https://original.com");

        at_client.set_asset_metadata(&admin, &1, &meta_name, &meta_desc, &meta_sym, &meta_img, &meta_url);
        at_client.set_metadata_immutable(&admin, &1);
        at_client.set_metadata_governance(&admin, &gov_id);

        // Propose immutable metadata update
        let new_name = String::from_str(&env, "Gov Approved Name");
        let new_desc = String::from_str(&env, "Gov Approved Desc");
        let new_sym = String::from_str(&env, "GAP");
        let new_img = String::from_str(&env, "ipfs://gov-approved");
        let new_url = String::from_str(&env, "https://gov-approved.com");

        let request_id = at_client.propose_metadata_update(
            &admin, &1, &new_name, &new_desc, &new_sym, &new_img, &new_url,
        );

        // Create governance proposal for metadata update
        let voter = Address::generate(&env);
        at_client.mint(&voter, &200, &1, &ec_addr);

        gov_client.delegate(&voter, &voter); // delegate to self to avoid delegation block

        let proposal_desc = String::from_str(&env, "Approve metadata update for asset #1");
        let proposal_id = gov_client.create_metadata_update_proposal(
            &voter, &1, &request_id, &proposal_desc, &3600,
        );

        assert_eq!(proposal_id, 1);

        // Check linked request
        let linked_request = gov_client.get_metadata_request_id(&proposal_id).unwrap();
        assert_eq!(linked_request, request_id);

        // Vote
        gov_client.vote(&voter, &proposal_id, &true);

        // Advance time and execute
        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        gov_client.tally_execute_metadata(&proposal_id, &at_id);

        let p = gov_client.get_proposal(&proposal_id).unwrap();
        assert_eq!(p.status, ProposalStatus::Passed);

        // Now execute the metadata update
        env.ledger().with_mut(|li| {
            li.timestamp += 172801; // past timelock
        });

        at_client.execute_metadata_update(&request_id);

        let metadata = at_client.get_asset_metadata(&1).unwrap();
        assert_eq!(metadata.name, new_name);
        assert_eq!(metadata.version, 3);
    }
}
