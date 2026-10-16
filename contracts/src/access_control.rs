use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Vec};

// ---------------------------------------------------------------------------
// Role Definitions
// ---------------------------------------------------------------------------

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
pub enum Role {
    /// Full system control
    Admin = 0,
    /// Operator: Can perform operational functions (minting, transfers)
    Operator = 1,
    /// Moderator: Can moderate content and disputes
    Moderator = 2,
    /// Auditor: Read-only access for auditing
    Auditor = 3,
    /// Fee Manager: Can manage fees and rates
    FeeManager = 4,
    /// Emergency Responder: Can trigger emergency controls
    EmergencyResponder = 5,
    /// Validator: Can validate transactions and proposals
    Validator = 6,
    /// Trader: Can perform trading operations
    Trader = 7,
    /// Liquidity Provider: Can manage liquidity pools
    LiquidityProvider = 8,
    /// User: Basic user with limited permissions
    User = 9,
}

impl Role {
    /// Convert u32 to Role
    pub fn from_u32(val: u32) -> Option<Self> {
        match val {
            0 => Some(Role::Admin),
            1 => Some(Role::Operator),
            2 => Some(Role::Moderator),
            3 => Some(Role::Auditor),
            4 => Some(Role::FeeManager),
            5 => Some(Role::EmergencyResponder),
            6 => Some(Role::Validator),
            7 => Some(Role::Trader),
            8 => Some(Role::LiquidityProvider),
            9 => Some(Role::User),
            _ => None,
        }
    }

    /// Get the hierarchy level (higher = more permissions)
    pub fn hierarchy_level(&self) -> u32 {
        match self {
            Role::Admin => 100,
            Role::EmergencyResponder => 80,
            Role::Operator => 60,
            Role::FeeManager => 55,
            Role::Validator => 50,
            Role::Moderator => 40,
            Role::LiquidityProvider => 30,
            Role::Trader => 20,
            Role::Auditor => 15,
            Role::User => 10,
        }
    }

    /// Check if this role can perform an action requiring a minimum level
    pub fn can_perform(&self, required_level: u32) -> bool {
        self.hierarchy_level() >= required_level
    }

    /// Check if this role has permission to grant other roles
    pub fn can_grant_roles(&self) -> bool {
        matches!(self, Role::Admin | Role::EmergencyResponder)
    }

    /// Check if this role can revoke other roles
    pub fn can_revoke_roles(&self) -> bool {
        matches!(self, Role::Admin | Role::EmergencyResponder)
    }
}

// ---------------------------------------------------------------------------
// Role Grant Structure
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct RoleGrant {
    pub role: u32, // Stored as u32 for compatibility
    pub grantee: Address,
    pub granter: Address,
    pub granted_at: u64,
    pub expires_at: Option<u64>, // None = indefinite
    pub revoked: bool,
}

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum RbacKey {
    /// Admin address
    Admin,
    /// All roles granted to an address: Address -> Vec<RoleGrant>
    UserRoles(Address),
    /// All addresses holding a role: u32 -> Vec<Address>
    RoleMembers(u32),
    /// Role grant counter (for tracking)
    GrantCounter,
    /// Role hierarchy enabled flag
    HierarchyEnabled,
    /// Time-limited roles feature enabled
    TimeLimitedEnabled,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct AccessControl;

#[contractimpl]
impl AccessControl {
    /// Initialize the access control system
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();

        if env.storage().instance().has(&RbacKey::Admin) {
            panic!("already initialized");
        }

        env.storage().instance().set(&RbacKey::Admin, &admin);
        env.storage().instance().set(&RbacKey::GrantCounter, &0u64);
        env.storage().instance().set(&RbacKey::HierarchyEnabled, &true);
        env.storage().instance().set(&RbacKey::TimeLimitedEnabled, &true);

        // Grant admin role to the admin address
        let grant = RoleGrant {
            role: Role::Admin as u32,
            grantee: admin.clone(),
            granter: admin.clone(),
            granted_at: env.ledger().timestamp(),
            expires_at: None,
            revoked: false,
        };

        let mut roles: Vec<RoleGrant> = Vec::new(&env);
        roles.push_back(grant.clone());
        env.storage().instance().set(&RbacKey::UserRoles(admin.clone()), &roles);

        // Add to role members
        let mut members: Vec<Address> = Vec::new(&env);
        members.push_back(admin);
        env.storage().instance().set(&RbacKey::RoleMembers(Role::Admin as u32), &members);
    }

    /// Grant a role to an address (with optional expiration)
    pub fn grant_role(
        env: Env,
        granter: Address,
        grantee: Address,
        role: u32,
        expires_in_seconds: Option<u64>,
    ) {
        granter.require_auth();

        // Verify granter has permission
        let granter_level = Self::get_highest_role_level(&env, &granter);
        let role_enum = Role::from_u32(role).expect("invalid role");

        if !role_enum.can_grant_roles() {
            panic!("insufficient permissions to grant roles");
        }

        // Can't grant role higher than granter's own role
        if granter_level > 0 && granter_level < role_enum.hierarchy_level() {
            panic!("cannot grant role higher than own role");
        }

        let current_time = env.ledger().timestamp();
        let expires_at = expires_in_seconds.map(|seconds| current_time + seconds);

        let grant = RoleGrant {
            role,
            grantee: grantee.clone(),
            granter,
            granted_at: current_time,
            expires_at,
            revoked: false,
        };

        // Add to user roles
        let mut roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(grantee.clone()))
            .unwrap_or_else(|| Vec::new(&env));

        roles.push_back(grant);
        env.storage().instance().set(&RbacKey::UserRoles(grantee.clone()), &roles);

        // Add to role members
        let mut members: Vec<Address> = env
            .storage()
            .instance()
            .get(&RbacKey::RoleMembers(role))
            .unwrap_or_else(|| Vec::new(&env));

        let has_address = members.iter().any(|addr| addr == grantee);
        if !has_address {
            members.push_back(grantee);
            env.storage().instance().set(&RbacKey::RoleMembers(role), &members);
        }

        // Increment grant counter
        let counter: u64 = env.storage().instance().get(&RbacKey::GrantCounter).unwrap_or(0);
        env.storage().instance().set(&RbacKey::GrantCounter, &(counter + 1));
    }

    /// Revoke a specific role from an address
    pub fn revoke_role(env: Env, revoker: Address, grantee: Address, role: u32) {
        revoker.require_auth();

        // Verify revoker has permission
        let revoker_level = Self::get_highest_role_level(&env, &revoker);
        let role_enum = Role::from_u32(role).expect("invalid role");

        if !role_enum.can_revoke_roles() {
            panic!("insufficient permissions to revoke roles");
        }

        if revoker_level > 0 && revoker_level < role_enum.hierarchy_level() {
            panic!("cannot revoke role higher than own role");
        }

        // Mark role as revoked
        let mut roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(grantee.clone()))
            .unwrap_or_else(|| Vec::new(&env));

        for i in 0..roles.len() {
            if let Some(grant) = roles.get(i) {
                if grant.role == role {
                    let mut updated_grant = grant.clone();
                    updated_grant.revoked = true;
                    roles.set(i, updated_grant);
                    break;
                }
            }
        }

        env.storage().instance().set(&RbacKey::UserRoles(grantee), &roles);
    }

    /// Check if an address has a specific role (considering expiration)
    pub fn has_role(env: Env, user: Address, role: u32) -> bool {
        let roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(user))
            .unwrap_or_else(|| Vec::new(&env));

        let current_time = env.ledger().timestamp();

        for grant in roles.iter() {
            if grant.role == role && !grant.revoked {
                // Check if expired
                if let Some(expires_at) = grant.expires_at {
                    if current_time < expires_at {
                        return true;
                    }
                } else {
                    return true;
                }
            }
        }

        false
    }

    /// Get the highest role an address holds (helper method) - returns hierarchy level
    fn get_highest_role_level(env: &Env, user: &Address) -> u32 {
        let roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(user.clone()))
            .unwrap_or_else(|| Vec::new(env));

        let current_time = env.ledger().timestamp();
        let mut highest_level = 0u32;

        for grant in roles.iter() {
            if grant.revoked {
                continue;
            }

            // Check expiration
            if let Some(expires_at) = grant.expires_at {
                if current_time >= expires_at {
                    continue;
                }
            }

            if let Some(role) = Role::from_u32(grant.role) {
                let level = role.hierarchy_level();
                if level > highest_level {
                    highest_level = level;
                }
            }
        }

        highest_level
    }

    /// Get the highest role an address holds
    pub fn get_highest_role_value(env: Env, user: Address) -> u32 {
        Self::get_highest_role_level(&env, &user)
    }

    /// Get all active roles for a user
    pub fn get_user_roles(env: Env, user: Address) -> Vec<u32> {
        let roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(user))
            .unwrap_or_else(|| Vec::new(&env));

        let current_time = env.ledger().timestamp();
        let mut active_roles = Vec::new(&env);

        for grant in roles.iter() {
            if grant.revoked {
                continue;
            }

            // Check expiration
            if let Some(expires_at) = grant.expires_at {
                if current_time >= expires_at {
                    continue;
                }
            }

            active_roles.push_back(grant.role);
        }

        active_roles
    }

    /// Get all members with a specific role
    pub fn get_role_members(env: Env, role: u32) -> Vec<Address> {
        env.storage()
            .instance()
            .get(&RbacKey::RoleMembers(role))
            .unwrap_or_else(|| Vec::new(&env))
    }

    /// Require that caller has a minimum role level
    pub fn require_role(env: Env, caller: Address, required_level: u32) {
        caller.require_auth();

        let highest_level = Self::get_highest_role_level(&env, &caller);
        if highest_level == 0 {
            panic!("no role assigned");
        }
        
        if highest_level < required_level {
            panic!("insufficient role level");
        }
    }

    /// Check time-limited role expiration
    pub fn check_role_expiration(env: Env, user: Address, role: u32) -> Option<u64> {
        let roles: Vec<RoleGrant> = env
            .storage()
            .instance()
            .get(&RbacKey::UserRoles(user))
            .unwrap_or_else(|| Vec::new(&env));

        for grant in roles.iter() {
            if grant.role == role && !grant.revoked {
                return grant.expires_at;
            }
        }

        None
    }

    /// Enable/disable role hierarchy
    pub fn set_hierarchy_enabled(env: Env, admin: Address, enabled: bool) {
        admin.require_auth();
        Self::require_role(env.clone(), admin.clone(), Role::Admin.hierarchy_level());

        env.storage().instance().set(&RbacKey::HierarchyEnabled, &enabled);
    }

    /// Enable/disable time-limited roles feature
    pub fn set_time_limited_enabled(env: Env, admin: Address, enabled: bool) {
        admin.require_auth();
        Self::require_role(env.clone(), admin.clone(), Role::Admin.hierarchy_level());

        env.storage().instance().set(&RbacKey::TimeLimitedEnabled, &enabled);
    }

    /// Get total number of grants issued
    pub fn get_grant_count(env: Env) -> u64 {
        env.storage().instance().get(&RbacKey::GrantCounter).unwrap_or(0)
    }
}
