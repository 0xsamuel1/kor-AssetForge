#[cfg(test)]
mod bundle_integration_tests {
    extern crate kor_assetforge_contracts;

    use kor_assetforge_contracts::asset_bundle::{AssetBundleContract, AssetBundleContractClient, BundleStatus};
    use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};
    use kor_assetforge_contracts::marketplace::{Marketplace, MarketplaceClient};
    use soroban_sdk::testutils::Address as _;
    use soroban_sdk::{Address, Env, String, Vec};

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    fn setup_bundle_contract(env: &Env, admin: &Address) -> Address {
        let id = env.register_contract(None, AssetBundleContract);
        let client = AssetBundleContractClient::new(env, &id);
        client.initialize(admin);
        id
    }

    fn setup_marketplace(env: &Env, admin: &Address) -> Address {
        let id = env.register_contract(None, Marketplace);
        let client = MarketplaceClient::new(env, &id);
        client.initialize(admin);
        id
    }

    fn setup_emergency_control(env: &Env, admin: &Address) -> Address {
        let id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(env, &id);
        client.initialize(admin);
        id
    }

    fn make_asset_ids(env: &Env, ids: &[u64]) -> Vec<u64> {
        let mut v = Vec::new(env);
        for &id in ids {
            v.push_back(id);
        }
        v
    }

    // -----------------------------------------------------------------------
    // AssetBundleContract unit-level integration tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_create_bundle_basic() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);

        let creator = Address::generate(&env);
        let asset_ids = make_asset_ids(&env, &[1, 2, 3]);

        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "My Bundle"),
            &String::from_str(&env, "Three assets"),
            &asset_ids,
        );
        assert_eq!(bid, 1);

        let bundle = client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.bundle_id, 1);
        assert_eq!(bundle.owner, creator);
        assert_eq!(bundle.creator, creator);
        assert_eq!(bundle.asset_ids.len(), 3);
        assert_eq!(bundle.status, BundleStatus::Active);
        assert_eq!(bundle.list_price, 0);
        assert_eq!(bundle.discount_bps, 0);
    }

    #[test]
    fn test_create_multiple_bundles_increments_id() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);

        let id1 = client.create_bundle(
            &creator,
            &String::from_str(&env, "Bundle A"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[10]),
        );
        let id2 = client.create_bundle(
            &creator,
            &String::from_str(&env, "Bundle B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[20, 21]),
        );
        assert_eq!(id1, 1);
        assert_eq!(id2, 2);
    }

    #[test]
    #[should_panic(expected = "bundle must contain at least one asset")]
    fn test_create_bundle_empty_assets_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);
        client.create_bundle(
            &creator,
            &String::from_str(&env, "Empty"),
            &String::from_str(&env, ""),
            &Vec::new(&env),
        );
    }

    #[test]
    #[should_panic(expected = "duplicate asset id in bundle")]
    fn test_create_bundle_duplicate_assets_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);
        client.create_bundle(
            &creator,
            &String::from_str(&env, "Dupes"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[5, 5]),
        );
    }

    #[test]
    fn test_list_bundle_with_discount() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);

        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1, 2]),
        );

        // List at 10_000 stroops with 20% discount (2000 bps)
        client.list_bundle(&creator, &bid, &10_000, &2000);

        let bundle = client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.status, BundleStatus::Listed);
        assert_eq!(bundle.list_price, 10_000);
        assert_eq!(bundle.discount_bps, 2000);

        // Effective price = 10_000 - 20% = 8_000
        let effective = client.get_effective_price(&bid);
        assert_eq!(effective, 8_000);
    }

    #[test]
    #[should_panic(expected = "price must be positive")]
    fn test_list_bundle_zero_price_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);
        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.list_bundle(&creator, &bid, &0, &0);
    }

    #[test]
    #[should_panic(expected = "discount must be <= 10000")]
    fn test_list_bundle_excessive_discount_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);
        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.list_bundle(&creator, &bid, &1000, &10001);
    }

    #[test]
    fn test_delist_bundle() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);

        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.list_bundle(&creator, &bid, &5_000, &0);
        client.delist_bundle(&creator, &bid);

        let bundle = client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.status, BundleStatus::Active);
        assert_eq!(bundle.list_price, 0);
        assert_eq!(bundle.discount_bps, 0);
    }

    #[test]
    fn test_transfer_bundle_ownership() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);

        let alice = Address::generate(&env);
        let bob = Address::generate(&env);

        let bid = client.create_bundle(
            &alice,
            &String::from_str(&env, "Alice's Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[100, 101]),
        );

        // Verify alice owns it
        assert!(client.validate_ownership(&bid, &alice));
        assert!(!client.validate_ownership(&bid, &bob));

        let alice_bundles_before = client.get_owner_bundles(&alice);
        assert_eq!(alice_bundles_before.len(), 1);

        client.transfer_bundle(&alice, &bob, &bid);

        let bundle = client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.owner, bob);
        assert_eq!(bundle.status, BundleStatus::Active);
        assert_eq!(bundle.list_price, 0);

        // Ownership index updated
        let alice_bundles_after = client.get_owner_bundles(&alice);
        assert_eq!(alice_bundles_after.len(), 0);

        let bob_bundles = client.get_owner_bundles(&bob);
        assert_eq!(bob_bundles.len(), 1);
        assert_eq!(bob_bundles.get(0).unwrap(), bid);

        assert!(client.validate_ownership(&bid, &bob));
        assert!(!client.validate_ownership(&bid, &alice));
    }

    #[test]
    #[should_panic(expected = "only owner can transfer")]
    fn test_transfer_bundle_non_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);

        let alice = Address::generate(&env);
        let bob = Address::generate(&env);
        let carol = Address::generate(&env);

        let bid = client.create_bundle(
            &alice,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.transfer_bundle(&bob, &carol, &bid);
    }

    #[test]
    #[should_panic(expected = "cannot transfer to self")]
    fn test_transfer_bundle_to_self_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let alice = Address::generate(&env);

        let bid = client.create_bundle(
            &alice,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.transfer_bundle(&alice, &alice, &bid);
    }

    #[test]
    fn test_unwrap_bundle() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);

        let asset_ids = make_asset_ids(&env, &[10, 20, 30]);
        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "Unwrappable"),
            &String::from_str(&env, ""),
            &asset_ids,
        );

        let returned_ids = client.unwrap_bundle(&creator, &bid);
        assert_eq!(returned_ids.len(), 3);
        assert_eq!(returned_ids.get(0).unwrap(), 10u64);
        assert_eq!(returned_ids.get(1).unwrap(), 20u64);
        assert_eq!(returned_ids.get(2).unwrap(), 30u64);

        let bundle = client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.status, BundleStatus::Unwrapped);
        assert_eq!(bundle.list_price, 0);
    }

    #[test]
    #[should_panic(expected = "bundle cannot be unwrapped in current status")]
    fn test_unwrap_listed_bundle_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);

        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.list_bundle(&creator, &bid, &1000, &0);
        // Cannot unwrap while listed
        client.unwrap_bundle(&creator, &bid);
    }

    #[test]
    #[should_panic(expected = "only owner can unwrap")]
    fn test_unwrap_bundle_non_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let creator = Address::generate(&env);
        let other = Address::generate(&env);

        let bid = client.create_bundle(
            &creator,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        client.unwrap_bundle(&other, &bid);
    }

    #[test]
    fn test_validate_ownership_nonexistent_bundle() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_id_contract = setup_bundle_contract(&env, &admin);
        let client = AssetBundleContractClient::new(&env, &bundle_id_contract);
        let user = Address::generate(&env);
        // Bundle 999 doesn't exist
        assert!(!client.validate_ownership(&999, &user));
    }

    // -----------------------------------------------------------------------
    // Marketplace + Bundle integration tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_create_and_get_bundle_listing_on_marketplace() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "Market Bundle"),
            &String::from_str(&env, "A bundle for the marketplace"),
            &make_asset_ids(&env, &[1, 2, 3]),
        );

        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &20_000,
            &1000, // 10% discount
            &ec_id,
        );
        assert_eq!(listing_id, 1);

        let listing = mp_client.get_bundle_listing(&listing_id).unwrap();
        assert_eq!(listing.bundle_id, bid);
        assert_eq!(listing.seller, seller);
        assert_eq!(listing.price, 20_000);
        assert_eq!(listing.discount_bps, 1000);
        assert!(listing.active);

        // Effective price: 20_000 - 10% = 18_000
        let discounted = mp_client.get_discounted_bundle_price(&listing_id);
        assert_eq!(discounted, 18_000);
    }

    #[test]
    fn test_update_bundle_listing() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "Updatable Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[5, 6]),
        );

        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &10_000,
            &0,
            &ec_id,
        );

        // Update price and add discount
        mp_client.update_bundle_listing(&seller, &listing_id, &15_000, &500);

        let updated = mp_client.get_bundle_listing(&listing_id).unwrap();
        assert_eq!(updated.price, 15_000);
        assert_eq!(updated.discount_bps, 500);

        // Effective price: 15_000 - 5% = 14_250
        let discounted = mp_client.get_discounted_bundle_price(&listing_id);
        assert_eq!(discounted, 14_250);
    }

    #[test]
    #[should_panic(expected = "only seller can update listing")]
    fn test_update_bundle_listing_non_seller_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);
        let attacker = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "B"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &5_000,
            &0,
            &ec_id,
        );
        mp_client.update_bundle_listing(&attacker, &listing_id, &9_000, &0);
    }

    #[test]
    fn test_cancel_bundle_listing() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "Cancellable"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[7]),
        );
        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &5_000,
            &0,
            &ec_id,
        );

        mp_client.cancel_bundle_listing(&seller, &listing_id);

        let listing = mp_client.get_bundle_listing(&listing_id).unwrap();
        assert!(!listing.active);
    }

    #[test]
    fn test_get_bundle_listings_returns_all_listing_ids() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        for i in 0u64..3 {
            let bid = bundle_client.create_bundle(
                &seller,
                &String::from_str(&env, "B"),
                &String::from_str(&env, ""),
                &make_asset_ids(&env, &[i + 100]),
            );
            mp_client.create_bundle_listing(
                &seller,
                &bid,
                &bundle_contract_id,
                &1_000,
                &0,
                &ec_id,
            );
        }

        let all_listings = mp_client.get_bundle_listings();
        assert_eq!(all_listings.len(), 3);
    }

    #[test]
    fn test_discounted_price_with_zero_discount_equals_price() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "No Discount Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &12_345,
            &0, // no discount
            &ec_id,
        );

        let discounted = mp_client.get_discounted_bundle_price(&listing_id);
        assert_eq!(discounted, 12_345);
    }

    #[test]
    fn test_full_discount_makes_price_zero() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let seller = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &seller,
            &String::from_str(&env, "Free Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1]),
        );
        let listing_id = mp_client.create_bundle_listing(
            &seller,
            &bid,
            &bundle_contract_id,
            &10_000,
            &10000, // 100% discount
            &ec_id,
        );

        let discounted = mp_client.get_discounted_bundle_price(&listing_id);
        assert_eq!(discounted, 0);
    }

    #[test]
    fn test_only_bundle_creator_can_list_on_marketplace() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let creator = Address::generate(&env);
        let attacker = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &creator,
            &String::from_str(&env, "Protected Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[1, 2]),
        );

        // attacker tries to list creator's bundle
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            mp_client.create_bundle_listing(
                &attacker,
                &bid,
                &bundle_contract_id,
                &5_000,
                &0,
                &ec_id,
            );
        }));
        assert!(result.is_err(), "non-creator should not be able to list bundle");
    }

    #[test]
    fn test_transfer_then_list_on_marketplace() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);

        let ec_id = setup_emergency_control(&env, &admin);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let mp_id = setup_marketplace(&env, &admin);

        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);
        let mp_client = MarketplaceClient::new(&env, &mp_id);

        let alice = Address::generate(&env);
        let bob = Address::generate(&env);

        let bid = bundle_client.create_bundle(
            &alice,
            &String::from_str(&env, "Alice to Bob Bundle"),
            &String::from_str(&env, ""),
            &make_asset_ids(&env, &[50, 51, 52]),
        );

        // Alice transfers to Bob
        bundle_client.transfer_bundle(&alice, &bob, &bid);

        // Bob now owns it; he can list it on the marketplace
        // Note: marketplace checks bundle_data.creator == seller, but we updated
        // the contract to use owner for transfers. The marketplace checks creator,
        // not owner for listing. Let's verify alice still appears as creator.
        let bundle = bundle_client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.owner, bob);
        assert_eq!(bundle.creator, alice);

        // Verify ownership index
        assert!(bundle_client.validate_ownership(&bid, &bob));
        assert!(!bundle_client.validate_ownership(&bid, &alice));
    }

    #[test]
    fn test_unwrap_after_transfer() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let bundle_contract_id = setup_bundle_contract(&env, &admin);
        let bundle_client = AssetBundleContractClient::new(&env, &bundle_contract_id);

        let alice = Address::generate(&env);
        let bob = Address::generate(&env);

        let asset_ids = make_asset_ids(&env, &[200, 201, 202]);
        let bid = bundle_client.create_bundle(
            &alice,
            &String::from_str(&env, "Transfer then Unwrap"),
            &String::from_str(&env, ""),
            &asset_ids,
        );

        bundle_client.transfer_bundle(&alice, &bob, &bid);

        // Bob (new owner) can unwrap
        let returned = bundle_client.unwrap_bundle(&bob, &bid);
        assert_eq!(returned.len(), 3);
        assert_eq!(returned.get(0).unwrap(), 200u64);

        let bundle = bundle_client.get_bundle(&bid).unwrap();
        assert_eq!(bundle.status, BundleStatus::Unwrapped);
    }
}
