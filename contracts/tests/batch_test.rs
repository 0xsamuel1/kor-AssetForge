#[cfg(test)]
mod test {
    use soroban_sdk::testutils::{Address as _, Ledger as _};
    use soroban_sdk::{Address, Env, Vec};

    use kor_assetforge_contracts::emergency_control::{
        EmergencyControl, EmergencyControlClient, PauseScope,
    };
    use kor_assetforge_contracts::marketplace::{
        BatchListingInput, BatchPurchaseInput, Marketplace, MarketplaceClient,
    };

    fn setup() -> (Env, MarketplaceClient<'static>, Address, Address) {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        (env, mp_client, ec_id, admin)
    }

    #[test]
    fn test_batch_create_listing_success() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
            BatchListingInput { asset_id: 2, amount: 200, price: 2000 },
        ]);

        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 2);
        assert_eq!(ids.get(0).unwrap(), 1);
        assert_eq!(ids.get(1).unwrap(), 2);

        let listing1 = mp.get_listing(&1).unwrap();
        assert_eq!(listing1.asset_id, 1);
        assert_eq!(listing1.seller, seller);
        assert!(listing1.active);

        let listing2 = mp.get_listing(&2).unwrap();
        assert_eq!(listing2.asset_id, 2);
        assert_eq!(listing2.amount, 200);
    }

    #[test]
    fn test_batch_create_listing_single_item() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 50, price: 500 },
        ]);

        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 1);
        assert_eq!(ids.get(0).unwrap(), 1);

        let listing = mp.get_listing(&1).unwrap();
        assert_eq!(listing.amount, 50);
        assert_eq!(listing.price, 500);
    }

    #[test]
    fn test_batch_create_listing_max_items() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);

        let mut listings = Vec::new(&env);
        for i in 0u64..20 {
            listings.push_back(BatchListingInput { asset_id: i, amount: 100, price: 1000 });
        }

        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 20);
    }

    #[test]
    #[should_panic(expected = "batch must not be empty")]
    fn test_batch_create_listing_empty() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);
        let listings = Vec::new(&env);
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);
    }

    #[test]
    #[should_panic(expected = "batch size exceeds maximum of 20")]
    fn test_batch_create_listing_exceeds_max() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);

        let mut listings = Vec::new(&env);
        for i in 0u64..21 {
            listings.push_back(BatchListingInput { asset_id: i, amount: 100, price: 1000 });
        }
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);
    }

    #[test]
    #[should_panic(expected = "operation blocked: asset is paused")]
    fn test_batch_create_listing_blocked_when_trading_paused() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        let reason = soroban_sdk::String::from_str(&env, "security");
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.pause_asset(&admin, &1, &PauseScope::Trading, &reason, &0);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
        ]);
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);
    }

    #[test]
    #[should_panic(expected = "asset is deprecated")]
    fn test_batch_create_listing_deprecated_asset() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        let metadata = soroban_sdk::String::from_str(&env, "test");
        mp.register_asset(&admin, &1, &metadata, &false, &0);
        mp.deprecate_asset(&admin, &1, &true);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
        ]);
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);
    }

    #[test]
    fn test_batch_create_listing_tracks_volume() {
        let (env, mp, ec_id, _admin) = setup();
        let seller = Address::generate(&env);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
            BatchListingInput { asset_id: 1, amount: 50, price: 2000 },
        ]);

        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 2);

        let analytics = mp.get_asset_analytics(&1);
        assert_eq!(analytics.listing_count, 2);
        assert_eq!(analytics.volume, 100 * 1000 + 50 * 2000);
    }

    #[test]
    fn test_batch_purchase_success() {
        let (env, mp, ec_id, _admin) = setup();
        let buyer = Address::generate(&env);

        let purchases = Vec::from_array(&env, [
            BatchPurchaseInput { listing_id: 1, amount: 50, asset_id: 1 },
            BatchPurchaseInput { listing_id: 2, amount: 75, asset_id: 1 },
        ]);

        let result = mp.batch_purchase(&buyer, &purchases, &ec_id);
        assert!(result);
    }

    #[test]
    fn test_batch_purchase_single_item() {
        let (env, mp, ec_id, _admin) = setup();
        let buyer = Address::generate(&env);

        let purchases = Vec::from_array(&env, [
            BatchPurchaseInput { listing_id: 1, amount: 30, asset_id: 1 },
        ]);

        assert!(mp.batch_purchase(&buyer, &purchases, &ec_id));
    }

    #[test]
    fn test_batch_purchase_max_items() {
        let (env, mp, ec_id, _admin) = setup();
        let buyer = Address::generate(&env);

        let mut purchases = Vec::new(&env);
        for i in 0u64..20 {
            purchases.push_back(BatchPurchaseInput { listing_id: i, amount: 10, asset_id: i });
        }

        assert!(mp.batch_purchase(&buyer, &purchases, &ec_id));
    }

    #[test]
    #[should_panic(expected = "batch must not be empty")]
    fn test_batch_purchase_empty() {
        let (env, mp, ec_id, _admin) = setup();
        let buyer = Address::generate(&env);
        let purchases = Vec::new(&env);
        mp.batch_purchase(&buyer, &purchases, &ec_id);
    }

    #[test]
    #[should_panic(expected = "batch size exceeds maximum of 20")]
    fn test_batch_purchase_exceeds_max() {
        let (env, mp, ec_id, _admin) = setup();
        let buyer = Address::generate(&env);

        let mut purchases = Vec::new(&env);
        for i in 0u64..21 {
            purchases.push_back(BatchPurchaseInput { listing_id: i, amount: 50, asset_id: i });
        }
        mp.batch_purchase(&buyer, &purchases, &ec_id);
    }

    #[test]
    #[should_panic(expected = "operation blocked: asset is paused")]
    fn test_batch_purchase_blocked_when_trading_paused() {
        let (env, mp, ec_id, admin) = setup();
        let buyer = Address::generate(&env);

        let reason = soroban_sdk::String::from_str(&env, "halt");
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.pause_asset(&admin, &1, &PauseScope::Trading, &reason, &0);

        let purchases = Vec::from_array(&env, [
            BatchPurchaseInput { listing_id: 1, amount: 50, asset_id: 1 },
        ]);
        mp.batch_purchase(&buyer, &purchases, &ec_id);
    }

    #[test]
    fn test_batch_purchase_allowed_when_different_scope_paused() {
        let (env, mp, ec_id, admin) = setup();
        let buyer = Address::generate(&env);

        let reason = soroban_sdk::String::from_str(&env, "minting halt");
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.pause_asset(&admin, &1, &PauseScope::Minting, &reason, &0);

        let purchases = Vec::from_array(&env, [
            BatchPurchaseInput { listing_id: 1, amount: 50, asset_id: 1 },
        ]);
        assert!(mp.batch_purchase(&buyer, &purchases, &ec_id));
    }

    #[test]
    fn test_batch_create_listing_whitelisted_private_asset() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        mp.set_asset_privacy(&admin, &1, &true);
        mp.add_to_whitelist(&admin, &1, &seller);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
        ]);
        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 1);
    }

    #[test]
    #[should_panic(expected = "user not whitelisted for private asset")]
    fn test_batch_create_listing_private_asset_not_whitelisted() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        mp.set_asset_privacy(&admin, &1, &true);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
        ]);
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);
    }

    #[test]
    fn test_batch_multiple_assets_private_mixed() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        mp.set_asset_privacy(&admin, &1, &true);
        mp.add_to_whitelist(&admin, &1, &seller);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
            BatchListingInput { asset_id: 2, amount: 200, price: 2000 },
        ]);
        let ids = mp.batch_create_listing(&seller, &listings, &ec_id, &None);
        assert_eq!(ids.len(), 2);
    }

    #[test]
    fn test_batch_create_listing_updates_metrics() {
        let (env, mp, ec_id, admin) = setup();
        let seller = Address::generate(&env);

        let metadata = soroban_sdk::String::from_str(&env, "test");
        mp.register_asset(&admin, &1, &metadata, &false, &0);

        let listings = Vec::from_array(&env, [
            BatchListingInput { asset_id: 1, amount: 100, price: 1000 },
        ]);
        mp.batch_create_listing(&seller, &listings, &ec_id, &None);

        let analytics = mp.get_asset_analytics(&1);
        assert_eq!(analytics.listing_count, 1);
        assert_eq!(analytics.volume, 100000);
    }

    #[test]
    fn test_batch_purchase_with_fees_and_referral() {
        let (env, mp, ec_id, admin) = setup();
        let buyer = Address::generate(&env);

        mp.initialize_buyback(
            &admin, &10_000, &50_000, &5_000, &30, &false,
        );

        mp.initialize_referral(
            &admin, &admin, &500, &0,
        );

        let referrer = Address::generate(&env);
        mp.refer_user(&buyer, &referrer);

        let purchases = Vec::from_array(&env, [
            BatchPurchaseInput { listing_id: 1, amount: 100_000, asset_id: 1 },
            BatchPurchaseInput { listing_id: 2, amount: 200_000, asset_id: 1 },
        ]);

        let result = mp.batch_purchase(&buyer, &purchases, &ec_id);
        assert!(result);

        let treasury = mp.get_treasury_balance();
        assert!(treasury > 0);

        let info = mp.get_referral_info(&referrer);
        let (_referrer_addr, reward, _count) = info;
        assert!(reward > 0);
    }
}
