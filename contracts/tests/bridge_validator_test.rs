#[cfg(test)]
mod tests {
    use super::super::*;
    use soroban_sdk::testutils::{Address as AddressTestUtils, Env as EnvTestUtils};
    use soroban_sdk::{Address, Env, BytesN};

    #[test]
    fn test_bridge_validator_initialize() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator1 = Address::generate(&env);
        let validator2 = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator1.clone());
        validators.push_back(validator2.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100, // 1%
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(
            env.clone(),
            admin.clone(),
            validators,
            2,
            fee_config.clone(),
        );

        assert!(BridgeValidator::is_enabled(env.clone()));
    }

    #[test]
    fn test_add_validator() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator1 = Address::generate(&env);
        let validator2 = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator1.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(
            env.clone(),
            admin.clone(),
            validators,
            1,
            fee_config.clone(),
        );

        // Add new validator
        BridgeValidator::add_validator(env.clone(), admin.clone(), validator2.clone(), 5000);

        let val_info = BridgeValidator::get_validator(env.clone(), validator2.clone());
        assert!(val_info.is_some());
        assert_eq!(val_info.unwrap().stake, 5000);
    }

    #[test]
    fn test_approve_transfer() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(env.clone(), admin.clone(), validators, 1, fee_config);

        // Create a transfer (this would be done in a real scenario)
        let transfer_id = soroban_sdk::BytesN::<32>::from_array([1u8; 32]);

        // Note: In a real implementation, we would need to create a transfer first
        // This test is simplified for demonstration
    }

    #[test]
    fn test_slash_validator() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(env.clone(), admin.clone(), validators, 1, fee_config);

        BridgeValidator::add_validator(env.clone(), admin.clone(), validator.clone(), 10000);
        BridgeValidator::slash_validator(env.clone(), admin.clone(), validator.clone(), 2000);

        let val_info = BridgeValidator::get_validator(env.clone(), validator.clone());
        assert!(val_info.is_some());
        assert_eq!(val_info.unwrap().stake, 8000);
    }

    #[test]
    fn test_fee_config_update() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(
            env.clone(),
            admin.clone(),
            validators,
            1,
            fee_config.clone(),
        );

        let new_fee_config = BridgeFeeConfig {
            base_fee: 2000,
            fee_percentage: 150,
            max_fee_cap: 150000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::update_fee_config(env.clone(), admin.clone(), new_fee_config.clone());

        let retrieved_config = BridgeValidator::get_fee_config(env.clone());
        assert!(retrieved_config.is_some());
        assert_eq!(retrieved_config.unwrap().base_fee, 2000);
    }

    #[test]
    fn test_bridge_enable_disable() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(env.clone(), admin.clone(), validators, 1, fee_config);

        assert!(BridgeValidator::is_enabled(env.clone()));

        BridgeValidator::set_enabled(env.clone(), admin.clone(), false);
        assert!(!BridgeValidator::is_enabled(env.clone()));

        BridgeValidator::set_enabled(env.clone(), admin.clone(), true);
        assert!(BridgeValidator::is_enabled(env.clone()));
    }

    #[test]
    fn test_supported_chains() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(env.clone(), admin.clone(), validators, 1, fee_config);

        // Initial chains: stellar, ethereum, bsc
        BridgeValidator::add_supported_chain(env.clone(), admin.clone(), "polygon".to_string());
    }

    #[test]
    fn test_validator_reputation() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let validator = Address::generate(&env);

        let mut validators = soroban_sdk::Vec::new(&env);
        validators.push_back(validator.clone());

        let fee_config = BridgeFeeConfig {
            base_fee: 1000,
            fee_percentage: 100,
            max_fee_cap: 100000,
            fee_token: Address::generate(&env),
        };

        BridgeValidator::initialize(env.clone(), admin.clone(), validators, 1, fee_config);

        let val_info = BridgeValidator::get_validator(env.clone(), validator.clone());
        assert!(val_info.is_some());
        let info = val_info.unwrap();
        assert_eq!(info.reputation_score, 50); // Initial reputation
    }
}
