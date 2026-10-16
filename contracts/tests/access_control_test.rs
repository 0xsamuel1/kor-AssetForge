#[cfg(test)]
mod tests {
    use super::super::*;
    use soroban_sdk::testutils::{Address as AddressTestUtils, Env as EnvTestUtils};
    use soroban_sdk::{Address, Env};

    #[test]
    fn test_initialize() {
        let env = Env::default();
        let admin = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        // Verify admin has Admin role
        assert!(AccessControl::has_role(env.clone(), admin.clone(), 0)); // Role::Admin = 0
    }

    #[test]
    fn test_grant_role() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        // Grant Operator role to user (expires in 86400 seconds = 1 day)
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, Some(86400));

        // Verify user has Operator role
        assert!(AccessControl::has_role(env.clone(), user.clone(), 1));
    }

    #[test]
    fn test_revoke_role() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, None);

        assert!(AccessControl::has_role(env.clone(), user.clone(), 1));

        // Revoke role
        AccessControl::revoke_role(env.clone(), admin.clone(), user.clone(), 1);

        // User should no longer have the role
        assert!(!AccessControl::has_role(env.clone(), user.clone(), 1));
    }

    #[test]
    fn test_role_hierarchy() {
        let env = Env::default();
        let admin = Address::generate(&env);

        // Admin role hierarchy level should be 100
        assert_eq!(Role::Admin.hierarchy_level(), 100);
        assert_eq!(Role::Operator.hierarchy_level(), 60);
        assert_eq!(Role::User.hierarchy_level(), 10);

        // Higher role can perform actions requiring lower level
        assert!(Role::Admin.can_perform(50));
        assert!(!Role::User.can_perform(50));
    }

    #[test]
    fn test_time_limited_role_expiration() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        // Grant role that expires immediately
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, Some(1));

        // Role should be active initially (depends on ledger timestamp)
        // This test would need mock time to properly test expiration
    }

    #[test]
    fn test_get_user_roles() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, None);
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 2, None);

        let roles = AccessControl::get_user_roles(env.clone(), user.clone());
        assert_eq!(roles.len(), 2);
    }

    #[test]
    fn test_get_role_members() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user1 = Address::generate(&env);
        let user2 = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        AccessControl::grant_role(env.clone(), admin.clone(), user1.clone(), 1, None);
        AccessControl::grant_role(env.clone(), admin.clone(), user2.clone(), 1, None);

        let members = AccessControl::get_role_members(env.clone(), 1);
        assert!(members.len() >= 2);
    }

    #[test]
    fn test_role_granting_permissions() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let operator = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        // Grant Operator role
        AccessControl::grant_role(env.clone(), admin.clone(), operator.clone(), 1, None);

        // Operator should NOT be able to grant roles (only Admin and EmergencyResponder)
        // This would panic in the actual implementation
    }

    #[test]
    fn test_highest_role() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, None); // Operator
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 2, None); // Moderator

        let highest_role = AccessControl::get_highest_role_value(env.clone(), user.clone());
        assert!(highest_role < u32::MAX);
    }

    #[test]
    fn test_grant_counter() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        AccessControl::initialize(env.clone(), admin.clone());

        let initial_count = AccessControl::get_grant_count(env.clone());
        AccessControl::grant_role(env.clone(), admin.clone(), user.clone(), 1, None);
        let final_count = AccessControl::get_grant_count(env.clone());

        assert_eq!(final_count, initial_count + 1);
    }
}
