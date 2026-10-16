#[cfg(test)]
mod test {
    use soroban_sdk::testutils::{Address as _, Ledger as _};
    use soroban_sdk::{Address, Env, Vec};

    use kor_assetforge_contracts::whitelist::{
        AccreditedTier, Whitelist, WhitelistClient,
    };

    fn setup() -> (Env, Address) {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        Whitelist::initialize(env.clone(), admin.clone());
        (env, admin)
    }

    #[test]
    fn test_initialize() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        Whitelist::initialize(env.clone(), admin.clone());
        assert_eq!(Whitelist::get_user_count(env.clone()), 0);
    }

    #[test]
    #[should_panic(expected = "already initialized")]
    fn test_initialize_twice_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        Whitelist::initialize(env.clone(), admin.clone());
        Whitelist::initialize(env, admin);
    }

    #[test]
    fn test_add_entry() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin, user.clone(), AccreditedTier::Tier1);
        assert!(Whitelist::is_whitelisted(env.clone(), user.clone()));
        assert_eq!(Whitelist::get_user_count(env.clone()), 1);
    }

    #[test]
    fn test_add_entry_all_tiers() {
        let (env, admin) = setup();
        let u1 = Address::generate(&env);
        let u2 = Address::generate(&env);
        let u3 = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), u1.clone(), AccreditedTier::Tier1);
        Whitelist::add_entry(env.clone(), admin.clone(), u2.clone(), AccreditedTier::Tier2);
        Whitelist::add_entry(env.clone(), admin.clone(), u3.clone(), AccreditedTier::Tier3);
        assert_eq!(
            Whitelist::get_tier(env.clone(), u1).unwrap(),
            AccreditedTier::Tier1
        );
        assert_eq!(
            Whitelist::get_tier(env.clone(), u2).unwrap(),
            AccreditedTier::Tier2
        );
        assert_eq!(
            Whitelist::get_tier(env.clone(), u3).unwrap(),
            AccreditedTier::Tier3
        );
    }

    #[test]
    fn test_get_entry() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier2);
        let entry = Whitelist::get_entry(env.clone(), user.clone()).unwrap();
        assert_eq!(entry.user, user);
        assert_eq!(entry.tier, AccreditedTier::Tier2);
        assert!(entry.active);
        assert!(entry.expires_at > entry.added_at);
    }

    #[test]
    fn test_get_entry_not_found() {
        let (env, _admin) = setup();
        let user = Address::generate(&env);
        let entry = Whitelist::get_entry(env.clone(), user);
        assert!(entry.is_none());
    }

    #[test]
    fn test_remove_entry() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        assert!(Whitelist::is_whitelisted(env.clone(), user.clone()));
        Whitelist::remove_entry(env.clone(), admin, user.clone());
        assert!(!Whitelist::is_whitelisted(env.clone(), user));
    }

    #[test]
    fn test_expiry_after_one_year() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        env.ledger().with_mut(|l| l.timestamp = 1_000_000);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        assert!(Whitelist::is_whitelisted(env.clone(), user.clone()));
        // Advance past 1 year
        env.ledger().with_mut(|l| l.timestamp = 1_000_000 + 31_536_001);
        assert!(!Whitelist::is_whitelisted(env.clone(), user));
    }

    #[test]
    fn test_expiry_boundary() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        env.ledger().with_mut(|l| l.timestamp = 100);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        // Exactly at expiry boundary - should be expired (not strictly greater)
        env.ledger().with_mut(|l| l.timestamp = 100 + 31_536_000);
        assert!(!Whitelist::is_whitelisted(env.clone(), user));
    }

    #[test]
    fn test_bulk_add_entries() {
        let (env, admin) = setup();
        let u1 = Address::generate(&env);
        let u2 = Address::generate(&env);
        let u3 = Address::generate(&env);
        let users = Vec::from_array(&env, [u1.clone(), u2.clone(), u3.clone()]);
        let tiers = Vec::from_array(
            &env,
            [
                AccreditedTier::Tier1,
                AccreditedTier::Tier2,
                AccreditedTier::Tier3,
            ],
        );
        Whitelist::add_bulk_entries(env.clone(), admin, users, tiers);
        assert!(Whitelist::is_whitelisted(env.clone(), u1));
        assert!(Whitelist::is_whitelisted(env.clone(), u2));
        assert!(Whitelist::is_whitelisted(env.clone(), u3));
        assert_eq!(Whitelist::get_user_count(env.clone()), 3);
    }

    #[test]
    fn test_remove_bulk_entries() {
        let (env, admin) = setup();
        let u1 = Address::generate(&env);
        let u2 = Address::generate(&env);
        let users = Vec::from_array(&env, [u1.clone(), u2.clone()]);
        let tiers = Vec::from_array(&env, [AccreditedTier::Tier1, AccreditedTier::Tier2]);
        Whitelist::add_bulk_entries(env.clone(), admin.clone(), users.clone(), tiers);
        assert!(Whitelist::is_whitelisted(env.clone(), u1.clone()));
        assert!(Whitelist::is_whitelisted(env.clone(), u2.clone()));
        Whitelist::remove_bulk_entries(env.clone(), admin, users);
        assert!(!Whitelist::is_whitelisted(env.clone(), u1));
        assert!(!Whitelist::is_whitelisted(env.clone(), u2));
    }

    #[test]
    fn test_bulk_add_skips_duplicates() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        let users = Vec::from_array(&env, [user.clone()]);
        let tiers = Vec::from_array(&env, [AccreditedTier::Tier2]);
        Whitelist::add_bulk_entries(env.clone(), admin.clone(), users, tiers);
        // Still only 1 user
        assert_eq!(Whitelist::get_user_count(env.clone()), 1);
    }

    #[test]
    fn test_refresh_entry() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        env.ledger().with_mut(|l| l.timestamp = 100);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        let entry_before = Whitelist::get_entry(env.clone(), user.clone()).unwrap();
        let original_expiry = entry_before.expires_at;
        // Advance time 6 months
        env.ledger().with_mut(|l| l.timestamp = 100 + 15_768_000);
        assert!(Whitelist::is_whitelisted(env.clone(), user.clone()));
        // Refresh
        Whitelist::refresh_entry(env.clone(), admin, user.clone());
        let entry_after = Whitelist::get_entry(env.clone(), user).unwrap();
        assert!(entry_after.expires_at > original_expiry);
    }

    #[test]
    fn test_get_tier_not_whitelisted() {
        let (env, _admin) = setup();
        let user = Address::generate(&env);
        let tier = Whitelist::get_tier(env.clone(), user);
        assert!(tier.is_none());
    }

    #[test]
    fn test_get_tier_expired() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        env.ledger().with_mut(|l| l.timestamp = 100);
        Whitelist::add_entry(env.clone(), admin, user.clone(), AccreditedTier::Tier2);
        env.ledger().with_mut(|l| l.timestamp = 100 + 31_536_001);
        let tier = Whitelist::get_tier(env.clone(), user);
        assert!(tier.is_none());
    }

    #[test]
    fn test_get_all_users() {
        let (env, admin) = setup();
        let u1 = Address::generate(&env);
        let u2 = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), u1.clone(), AccreditedTier::Tier1);
        Whitelist::add_entry(env.clone(), admin, u2.clone(), AccreditedTier::Tier2);
        let users = Whitelist::get_all_users(env.clone());
        assert_eq!(users.len(), 2);
    }

    #[test]
    fn test_user_count() {
        let (env, admin) = setup();
        assert_eq!(Whitelist::get_user_count(env.clone()), 0);
        let user = Address::generate(&env);
        Whitelist::add_entry(env.clone(), admin.clone(), user, AccreditedTier::Tier1);
        assert_eq!(Whitelist::get_user_count(env.clone()), 1);
    }

    #[test]
    fn test_add_same_user_after_expiry() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        env.ledger().with_mut(|l| l.timestamp = 100);
        Whitelist::add_entry(env.clone(), admin.clone(), user.clone(), AccreditedTier::Tier1);
        env.ledger().with_mut(|l| l.timestamp = 100 + 31_536_001);
        assert!(!Whitelist::is_whitelisted(env.clone(), user.clone()));
        // Re-add after expiry
        Whitelist::add_entry(env.clone(), admin, user.clone(), AccreditedTier::Tier2);
        assert!(Whitelist::is_whitelisted(env.clone(), user));
        assert_eq!(Whitelist::get_user_count(env.clone()), 1);
    }

    #[test]
    fn test_is_whitelisted_no_contract() {
        let (env, admin) = setup();
        let user = Address::generate(&env);
        let result = Whitelist::is_whitelisted(env.clone(), user);
        assert!(!result);
    }

    #[test]
    #[should_panic(expected = "not authorized")]
    fn test_non_admin_cannot_add() {
        let (env, _admin) = setup();
        let non_admin = Address::generate(&env);
        let user = Address::generate(&env);
        env.mock_all_auths();
        Whitelist::add_entry(env, non_admin, user, AccreditedTier::Tier1);
    }

    #[test]
    #[should_panic(expected = "not authorized")]
    fn test_non_admin_cannot_remove() {
        let (env, _admin) = setup();
        let non_admin = Address::generate(&env);
        let user = Address::generate(&env);
        env.mock_all_auths();
        Whitelist::remove_entry(env, non_admin, user);
    }

    #[test]
    fn test_marketplace_integration_deployed_together() {
        use kor_assetforge_contracts::marketplace::{Marketplace, MarketplaceClient};
        use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};

        let env = Env::default();
        env.mock_all_auths();

        // Deploy marketplace
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        mp_client.initialize(&admin);

        // Deploy whitelist contract
        let wl_id = env.register_contract(None, Whitelist);
        let wl_client = WhitelistClient::new(&env, &wl_id);
        wl_client.initialize(&admin);

        // Deploy emergency control
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.initialize(&admin);

        // Set whitelist contract on marketplace
        mp_client.set_whitelist_contract(&admin, &wl_id);

        // Add user to whitelist
        let seller = Address::generate(&env);
        wl_client.add_entry(&admin, &seller, &AccreditedTier::Tier1);
        assert!(wl_client.is_whitelisted(&seller));

        // Seller can create listing because they're whitelisted
        let lid = mp_client.create_listing(&seller, &1, &100, &1000, &ec_id, &None);
        assert_eq!(lid, 1);

        // Non-whitelisted buyer cannot purchase
        let buyer = Address::generate(&env);
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            mp_client.purchase(&buyer, &1, &50, &1, &ec_id);
        }));
        assert!(result.is_err());
    }

    #[test]
    fn test_marketplace_integration_whitelisted_buyer() {
        use kor_assetforge_contracts::marketplace::{Marketplace, MarketplaceClient};
        use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};

        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let wl_id = env.register_contract(None, Whitelist);
        let wl_client = WhitelistClient::new(&env, &wl_id);
        wl_client.initialize(&admin);

        mp_client.set_whitelist_contract(&admin, &wl_id);

        let seller = Address::generate(&env);
        let buyer = Address::generate(&env);

        wl_client.add_entry(&admin, &seller, &AccreditedTier::Tier2);
        wl_client.add_entry(&admin, &buyer, &AccreditedTier::Tier3);

        mp_client.create_listing(&seller, &10, &100, &500, &ec_id, &None);
        let result = mp_client.purchase(&buyer, &1, &50, &10, &ec_id);
        assert!(result);
    }

    #[test]
    fn test_marketplace_no_whitelist_contract_configured() {
        use kor_assetforge_contracts::marketplace::{Marketplace, MarketplaceClient};
        use kor_assetforge_contracts::emergency_control::{EmergencyControl, EmergencyControlClient};

        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        // No whitelist contract configured - trades should still work
        let seller = Address::generate(&env);
        let buyer = Address::generate(&env);

        mp_client.create_listing(&seller, &1, &100, &1000, &ec_id, &None);
        let result = mp_client.purchase(&buyer, &1, &50, &1, &ec_id);
        assert!(result);
    }
}
