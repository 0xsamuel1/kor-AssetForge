use soroban_sdk::{contract, contracterror, contractimpl, contracttype, Address, Env, Symbol, Vec};

const ONE_YEAR: u64 = 31_536_000;

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
#[contracterror]
pub enum WhitelistError {
    AlreadyInitialized = 1,
    NotAuthorized = 2,
    AlreadyWhitelisted = 3,
    NotFound = 4,
    TierNotSupported = 5,
    AlreadyExpired = 6,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum AccreditedTier {
    Tier1,
    Tier2,
    Tier3,
}

#[derive(Clone)]
#[contracttype]
pub struct WhitelistEntry {
    pub user: Address,
    pub tier: AccreditedTier,
    pub added_at: u64,
    pub expires_at: u64,
    pub active: bool,
}

#[derive(Clone)]
#[contracttype]
pub enum WhitelistDataKey {
    Admin,
    Entry(Address),
    UserList,
}

#[contract]
pub struct Whitelist;

#[contractimpl]
impl Whitelist {
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&WhitelistDataKey::Admin) {
            panic!("already initialized");
        }
        admin.require_auth();
        env.storage().instance().set(&WhitelistDataKey::Admin, &admin);
        env.storage().instance().set(&WhitelistDataKey::UserList, &Vec::<Address>::new(&env));
    }

    pub fn add_entry(env: Env, admin: Address, user: Address, tier: AccreditedTier) {
        Self::require_admin(&env, &admin);
        if env.storage().persistent().has(&WhitelistDataKey::Entry(user.clone())) {
            let existing: WhitelistEntry = env.storage().persistent()
                .get(&WhitelistDataKey::Entry(user.clone()))
                .unwrap();
            if existing.active && existing.expires_at > env.ledger().timestamp() {
                panic!("user already whitelisted");
            }
        }
        let now = env.ledger().timestamp();
        let entry = WhitelistEntry {
            user: user.clone(),
            tier,
            added_at: now,
            expires_at: now + ONE_YEAR,
            active: true,
        };
        env.storage().persistent().set(&WhitelistDataKey::Entry(user.clone()), &entry);
        Self::add_to_user_list(&env, user.clone());
        env.events().publish(
            (Symbol::new(&env, "whitelist_entry_added"),),
            (user, tier, entry.expires_at),
        );
    }

    pub fn add_bulk_entries(env: Env, admin: Address, users: Vec<Address>, tiers: Vec<AccreditedTier>) {
        Self::require_admin(&env, &admin);
        assert!(users.len() == tiers.len(), "users and tiers length mismatch");
        let now = env.ledger().timestamp();
        for i in 0..users.len() {
            let user = users.get(i).unwrap();
            let tier = tiers.get(i).unwrap();
            if env.storage().persistent().has(&WhitelistDataKey::Entry(user.clone())) {
                let existing: WhitelistEntry = env.storage().persistent()
                    .get(&WhitelistDataKey::Entry(user.clone()))
                    .unwrap();
                if existing.active && existing.expires_at > now {
                    continue;
                }
            }
            let entry = WhitelistEntry {
                user: user.clone(),
                tier,
                added_at: now,
                expires_at: now + ONE_YEAR,
                active: true,
            };
            env.storage().persistent().set(&WhitelistDataKey::Entry(user.clone()), &entry);
            Self::add_to_user_list(&env, user.clone());
        }
        env.events().publish(
            (Symbol::new(&env, "whitelist_bulk_added"),),
            (users.len(),),
        );
    }

    pub fn remove_entry(env: Env, admin: Address, user: Address) {
        Self::require_admin(&env, &admin);
        let mut entry: WhitelistEntry = env.storage().persistent()
            .get(&WhitelistDataKey::Entry(user.clone()))
            .expect("entry not found");
        entry.active = false;
        env.storage().persistent().set(&WhitelistDataKey::Entry(user.clone()), &entry);
        env.events().publish(
            (Symbol::new(&env, "whitelist_entry_removed"),),
            (user,),
        );
    }

    pub fn remove_bulk_entries(env: Env, admin: Address, users: Vec<Address>) {
        Self::require_admin(&env, &admin);
        for i in 0..users.len() {
            let user = users.get(i).unwrap();
            if env.storage().persistent().has(&WhitelistDataKey::Entry(user.clone())) {
                let mut entry: WhitelistEntry = env.storage().persistent()
                    .get(&WhitelistDataKey::Entry(user.clone()))
                    .unwrap();
                entry.active = false;
                env.storage().persistent().set(&WhitelistDataKey::Entry(user.clone()), &entry);
            }
        }
        env.events().publish(
            (Symbol::new(&env, "whitelist_bulk_removed"),),
            (users.len(),),
        );
    }

    pub fn is_whitelisted(env: Env, user: Address) -> bool {
        if let Some(entry) = env.storage().persistent().get::<_, WhitelistEntry>(&WhitelistDataKey::Entry(user)) {
            entry.active && entry.expires_at > env.ledger().timestamp()
        } else {
            false
        }
    }

    pub fn get_entry(env: Env, user: Address) -> Option<WhitelistEntry> {
        env.storage().persistent().get(&WhitelistDataKey::Entry(user))
    }

    pub fn get_tier(env: Env, user: Address) -> Option<AccreditedTier> {
        if let Some(entry) = env.storage().persistent().get::<_, WhitelistEntry>(&WhitelistDataKey::Entry(user)) {
            if entry.active && entry.expires_at > env.ledger().timestamp() {
                return Some(entry.tier);
            }
        }
        None
    }

    pub fn get_user_count(env: Env) -> u32 {
        let list: Vec<Address> = env.storage().instance()
            .get(&WhitelistDataKey::UserList).unwrap_or(Vec::new(&env));
        list.len()
    }

    pub fn get_all_users(env: Env) -> Vec<Address> {
        env.storage().instance()
            .get(&WhitelistDataKey::UserList).unwrap_or(Vec::new(&env))
    }

    pub fn refresh_entry(env: Env, admin: Address, user: Address) {
        Self::require_admin(&env, &admin);
        let mut entry: WhitelistEntry = env.storage().persistent()
            .get(&WhitelistDataKey::Entry(user.clone()))
            .expect("entry not found");
        let now = env.ledger().timestamp();
        entry.expires_at = now + ONE_YEAR;
        entry.active = true;
        entry.added_at = now;
        env.storage().persistent().set(&WhitelistDataKey::Entry(user.clone()), &entry);
        env.events().publish(
            (Symbol::new(&env, "whitelist_entry_refreshed"),),
            (user, entry.expires_at),
        );
    }

    fn require_admin(env: &Env, admin: &Address) {
        admin.require_auth();
        let stored: Address = env.storage().instance()
            .get(&WhitelistDataKey::Admin).expect("not initialized");
        if admin != &stored {
            panic!("not authorized");
        }
    }

    fn add_to_user_list(env: &Env, user: Address) {
        let mut list: Vec<Address> = env.storage().instance()
            .get(&WhitelistDataKey::UserList).unwrap_or(Vec::<Address>::new(env));
        for existing in list.iter() {
            if existing == user {
                return;
            }
        }
        list.push_back(user);
        env.storage().instance().set(&WhitelistDataKey::UserList, &list);
    }
}
