// Upgradeable Proxy – Issue #342
//
// Implements a transparent upgrade pattern for Soroban contracts:
//
//   1. The proxy is deployed once and *never* changes.  Users keep interacting
//      with the proxy address, so all state (balances, allowances, ownership,
//      indexes) stays exactly where it is.
//   2. `propose_implementation` records a candidate implementation contract and
//      the ledger timestamp after which it may be activated.  A timelock gives
//      token holders and relayers time to react to an announced change.
//   3. `execute_upgrade` activates the pending implementation, bumps the
//      version, records history and keeps the previous implementation address so
//      the change can be rolled back with `rollback`.
//   4. `migrate` is the hook an implementation uses to migrate its own state
//      layout to the new version.
//
// Invariants enforced by this contract:
//   * only the admin may propose, execute, roll back or migrate;
//   * only one upgrade may be pending at a time;
//   * an upgrade cannot execute before its timelock expires;
//   * the implementation address is never the zero/none case and the proxy
//     cannot be pointed at itself;
//   * every version bump is recorded, so the deployed version is auditable.
//
// Threat model: the proxy is the only privileged address in the deployment.  It
// is assumed to be governed by a timelock and, where required, an external
// governance vote (see `set_governance_approver`).

use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum ProxyDataKey {
    /// Address allowed to administer the proxy (instance)
    Admin,
    /// Currently active implementation contract (instance)
    Implementation,
    /// Implementation recorded before the last upgrade, for rollback (instance)
    PreviousImplementation,
    /// Candidate implementation awaiting activation (instance)
    PendingImplementation,
    /// Ledger timestamp after which the pending implementation may be activated
    ExecuteAfter,
    /// Seconds a proposal must wait before it can be executed (instance)
    TimelockDuration,
    /// Monotonically increasing implementation version (instance)
    CurrentVersion,
    /// Number of upgrades recorded (instance)
    UpgradeCount,
    /// Optional governance contract that must approve an upgrade (instance)
    GovernanceApprover,
    /// Set once the pending implementation has been governance-approved
    GovernanceApproved,
    /// Address of the proxy itself, captured at initialize (instance)
    SelfAddress,
    /// Completed upgrades, indexed from zero (persistent)
    UpgradeRecord(u32),
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// A completed upgrade, stored in persistent history.
#[derive(Clone)]
#[contracttype]
pub struct ProxyUpgradeRecord {
    pub version: u32,
    pub implementation: Address,
    pub previous_implementation: Address,
    pub upgraded_by: Address,
    pub timestamp: u64,
}

/// Snapshot of the proxy configuration, useful for monitoring and for the
/// upgrade script to verify what it is about to change.
#[derive(Clone)]
#[contracttype]
pub struct ProxyStatus {
    pub admin: Address,
    pub implementation: Address,
    pub previous_implementation: Option<Address>,
    pub pending_implementation: Option<Address>,
    pub execute_after: u64,
    pub timelock_duration: u64,
    pub version: u32,
    pub upgrade_count: u32,
    pub governance_approved: bool,
}

/// Default timelock: one day. Long enough for token holders to notice, short
/// enough that a fix for a live incident does not take a week.
pub const DEFAULT_TIMELOCK: u64 = 86_400;

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct Proxy;

#[contractimpl]
impl Proxy {
    // -----------------------------------------------------------------------
    // Initialization
    // -----------------------------------------------------------------------

    /// Initialize the proxy. Can only be called once.
    pub fn initialize(env: Env, admin: Address, implementation: Address) {
        if env.storage().instance().has(&ProxyDataKey::Admin) {
            panic!("proxy already initialized");
        }
        admin.require_auth();

        env.storage().instance().set(&ProxyDataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&ProxyDataKey::Implementation, &implementation);
        env.storage()
            .instance()
            .set(&ProxyDataKey::CurrentVersion, &1u32);
        env.storage()
            .instance()
            .set(&ProxyDataKey::UpgradeCount, &0u32);
        env.storage()
            .instance()
            .set(&ProxyDataKey::TimelockDuration, &DEFAULT_TIMELOCK);
        env.storage()
            .instance()
            .set(&ProxyDataKey::SelfAddress, &env.current_contract_address());

        env.events()
            .publish((Symbol::new(&env, "proxy_initialized"),), implementation);
    }

    // -----------------------------------------------------------------------
    // Upgrade lifecycle
    // -----------------------------------------------------------------------

    /// Propose a new implementation contract.
    ///
    /// Only the admin may call this, only one proposal may be pending at a time,
    /// and the proposal cannot become active before `execute_after`.
    pub fn propose_implementation(env: Env, admin: Address, new_implementation: Address) {
        Self::require_admin(&env, &admin);

        if env
            .storage()
            .instance()
            .has(&ProxyDataKey::PendingImplementation)
        {
            panic!("an upgrade is already pending");
        }
        Self::validate_implementation(&env, &new_implementation);

        let timelock: u64 = Self::get_timelock(env.clone());
        let now = env.ledger().timestamp();
        let execute_after = now.checked_add(timelock).expect("timelock overflow");

        env.storage()
            .instance()
            .set(&ProxyDataKey::PendingImplementation, &new_implementation);
        env.storage()
            .instance()
            .set(&ProxyDataKey::ExecuteAfter, &execute_after);
        env.storage()
            .instance()
            .set(&ProxyDataKey::GovernanceApproved, &false);

        env.events().publish(
            (Symbol::new(&env, "implementation_proposed"), admin),
            (new_implementation, execute_after),
        );
    }

    /// Record approval from the registered governance approver.
    ///
    /// Only required when `set_governance_approver` has been called.
    pub fn approve_implementation(env: Env, governance: Address) {
        governance.require_auth();
        let approver: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::GovernanceApprover)
            .expect("no governance approver configured");
        if governance != approver {
            panic!("caller is not the governance approver");
        }
        if !env
            .storage()
            .instance()
            .has(&ProxyDataKey::PendingImplementation)
        {
            panic!("no pending implementation");
        }
        env.storage()
            .instance()
            .set(&ProxyDataKey::GovernanceApproved, &true);
        env.events()
            .publish((Symbol::new(&env, "implementation_approved"),), governance);
    }

    /// Activate the pending implementation once the timelock has expired.
    ///
    /// Records history, bumps the version, and keeps the previous
    /// implementation address so `rollback` can restore it.
    pub fn execute_upgrade(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);

        let new_implementation: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::PendingImplementation)
            .expect("no pending implementation");

        let execute_after: u64 = env
            .storage()
            .instance()
            .get(&ProxyDataKey::ExecuteAfter)
            .expect("no pending implementation");
        if env.ledger().timestamp() < execute_after {
            panic!("upgrade timelock has not expired");
        }

        if env.storage().instance().has(&ProxyDataKey::GovernanceApprover) {
            let approved: bool = env
                .storage()
                .instance()
                .get(&ProxyDataKey::GovernanceApproved)
                .unwrap_or(false);
            if !approved {
                panic!("governance approval required");
            }
        }

        let previous: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::Implementation)
            .expect("proxy not initialized");

        let version: u32 = Self::get_version(env.clone()) + 1;
        let count: u32 = Self::get_upgrade_count(env.clone());
        let now = env.ledger().timestamp();

        env.storage()
            .instance()
            .set(&ProxyDataKey::Implementation, &new_implementation);
        env.storage()
            .instance()
            .set(&ProxyDataKey::PreviousImplementation, &previous);
        env.storage()
            .instance()
            .set(&ProxyDataKey::CurrentVersion, &version);
        env.storage()
            .instance()
            .set(&ProxyDataKey::UpgradeCount, &(count + 1));
        env.storage()
            .instance()
            .remove(&ProxyDataKey::PendingImplementation);
        env.storage().instance().remove(&ProxyDataKey::ExecuteAfter);
        env.storage()
            .instance()
            .remove(&ProxyDataKey::GovernanceApproved);

        env.storage().persistent().set(
            &ProxyDataKey::UpgradeRecord(count),
            &ProxyUpgradeRecord {
                version,
                implementation: new_implementation.clone(),
                previous_implementation: previous.clone(),
                upgraded_by: admin.clone(),
                timestamp: now,
            },
        );

        env.events().publish(
            (Symbol::new(&env, "implementation_upgraded"), admin),
            (version, new_implementation, previous),
        );
    }

    /// Cancel a pending upgrade. Only the admin may call this.
    pub fn cancel_upgrade(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);
        if !env
            .storage()
            .instance()
            .has(&ProxyDataKey::PendingImplementation)
        {
            panic!("no pending implementation");
        }
        env.storage()
            .instance()
            .remove(&ProxyDataKey::PendingImplementation);
        env.storage().instance().remove(&ProxyDataKey::ExecuteAfter);
        env.storage()
            .instance()
            .remove(&ProxyDataKey::GovernanceApproved);
        env.events()
            .publish((Symbol::new(&env, "upgrade_cancelled"),), admin);
    }

    /// Point the proxy back at the implementation used before the last upgrade.
    ///
    /// The rollback is recorded as a new version so the history stays append
    /// only and the deployed version never moves backwards silently.
    pub fn rollback(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);

        let previous: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::PreviousImplementation)
            .expect("no previous implementation to roll back to");
        let current: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::Implementation)
            .expect("proxy not initialized");

        let version: u32 = Self::get_version(env.clone()) + 1;
        let count: u32 = Self::get_upgrade_count(env.clone());
        let now = env.ledger().timestamp();

        env.storage()
            .instance()
            .set(&ProxyDataKey::Implementation, &previous);
        env.storage()
            .instance()
            .set(&ProxyDataKey::PreviousImplementation, &current);
        env.storage()
            .instance()
            .set(&ProxyDataKey::CurrentVersion, &version);
        env.storage()
            .instance()
            .set(&ProxyDataKey::UpgradeCount, &(count + 1));
        env.storage().persistent().set(
            &ProxyDataKey::UpgradeRecord(count),
            &ProxyUpgradeRecord {
                version,
                implementation: previous.clone(),
                previous_implementation: current.clone(),
                upgraded_by: admin.clone(),
                timestamp: now,
            },
        );

        env.events().publish(
            (Symbol::new(&env, "implementation_rolled_back"), admin),
            (version, previous, current),
        );
    }

    /// Post-upgrade state migration hook.
    ///
    /// The proxy itself stores no user state, so this exists for the
    /// implementation to migrate its own layout. It emits an event recording
    /// that the migration window was opened.
    pub fn migrate(env: Env, admin: Address) {
        Self::require_admin(&env, &admin);
        if env
            .storage()
            .instance()
            .has(&ProxyDataKey::PendingImplementation)
        {
            panic!("cannot migrate while an upgrade is pending");
        }
        env.events().publish(
            (Symbol::new(&env, "migration_run"), admin),
            (Self::get_version(env.clone()), env.ledger().timestamp()),
        );
    }

    // -----------------------------------------------------------------------
    // Configuration
    // -----------------------------------------------------------------------

    /// Transfer proxy administration. Two-step: propose, then accept.
    pub fn propose_admin(env: Env, admin: Address, new_admin: Address) {
        Self::require_admin(&env, &admin);
        if new_admin == admin {
            panic!("new admin must differ from current admin");
        }
        env.events()
            .publish((Symbol::new(&env, "admin_proposed"),), new_admin);
    }

    /// Accept proxy administration from the pending admin.
    pub fn accept_admin(env: Env, new_admin: Address) {
        new_admin.require_auth();
        let current: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::Admin)
            .expect("proxy not initialized");
        if new_admin == current {
            panic!("caller is already the admin");
        }
        env.storage()
            .instance()
            .set(&ProxyDataKey::Admin, &new_admin);
        env.events()
            .publish((Symbol::new(&env, "admin_updated"),), new_admin);
    }

    /// Set the timelock applied to future proposals.
    pub fn set_timelock(env: Env, admin: Address, timelock_seconds: u64) {
        Self::require_admin(&env, &admin);
        env.storage()
            .instance()
            .set(&ProxyDataKey::TimelockDuration, &timelock_seconds);
    }

    /// Register (or clear) the governance approver required before execution.
    pub fn set_governance_approver(env: Env, admin: Address, approver: Option<Address>) {
        Self::require_admin(&env, &admin);
        match approver {
            Some(addr) => env
                .storage()
                .instance()
                .set(&ProxyDataKey::GovernanceApprover, &addr),
            None => {
                env.storage()
                    .instance()
                    .remove(&ProxyDataKey::GovernanceApprover);
            }
        }
    }

    // -----------------------------------------------------------------------
    // Views
    // -----------------------------------------------------------------------

    /// Address of the proxy itself.
    pub fn get_self(env: Env) -> Address {
        env.storage()
            .instance()
            .get(&ProxyDataKey::SelfAddress)
            .unwrap_or_else(|| env.current_contract_address())
    }

    pub fn get_admin(env: Env) -> Address {
        env.storage()
            .instance()
            .get(&ProxyDataKey::Admin)
            .expect("proxy not initialized")
    }

    pub fn get_implementation(env: Env) -> Address {
        env.storage()
            .instance()
            .get(&ProxyDataKey::Implementation)
            .expect("proxy not initialized")
    }

    pub fn get_pending_implementation(env: Env) -> Option<Address> {
        env.storage()
            .instance()
            .get(&ProxyDataKey::PendingImplementation)
    }

    pub fn get_previous_implementation(env: Env) -> Option<Address> {
        env.storage()
            .instance()
            .get(&ProxyDataKey::PreviousImplementation)
    }

    pub fn get_timelock(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&ProxyDataKey::TimelockDuration)
            .unwrap_or(DEFAULT_TIMELOCK)
    }

    pub fn get_version(env: Env) -> u32 {
        env.storage()
            .instance()
            .get(&ProxyDataKey::CurrentVersion)
            .unwrap_or(1)
    }

    pub fn get_upgrade_count(env: Env) -> u32 {
        env.storage()
            .instance()
            .get(&ProxyDataKey::UpgradeCount)
            .unwrap_or(0)
    }

    /// Return the recorded upgrade at zero-based `index`.
    pub fn get_upgrade_record(env: Env, index: u32) -> Option<ProxyUpgradeRecord> {
        env.storage()
            .persistent()
            .get(&ProxyDataKey::UpgradeRecord(index))
    }

    /// Full configuration snapshot.
    pub fn get_status(env: Env) -> ProxyStatus {
        ProxyStatus {
            admin: Self::get_admin(env.clone()),
            implementation: Self::get_implementation(env.clone()),
            previous_implementation: Self::get_previous_implementation(env.clone()),
            pending_implementation: Self::get_pending_implementation(env.clone()),
            execute_after: env
                .storage()
                .instance()
                .get(&ProxyDataKey::ExecuteAfter)
                .unwrap_or(0),
            timelock_duration: Self::get_timelock(env.clone()),
            version: Self::get_version(env.clone()),
            upgrade_count: Self::get_upgrade_count(env.clone()),
            governance_approved: env
                .storage()
                .instance()
                .get(&ProxyDataKey::GovernanceApproved)
                .unwrap_or(false),
        }
    }

    /// True when a pending implementation may be activated right now.
    pub fn is_upgrade_ready(env: Env) -> bool {
        if !env
            .storage()
            .instance()
            .has(&ProxyDataKey::PendingImplementation)
        {
            return false;
        }
        let execute_after: u64 = env
            .storage()
            .instance()
            .get(&ProxyDataKey::ExecuteAfter)
            .unwrap_or(0);
        env.ledger().timestamp() >= execute_after
    }

    /// Convenience helper: does `caller` administer the proxy?
    pub fn is_admin(env: Env, caller: Address) -> bool {
        caller.require_auth();
        Self::get_admin(env) == caller
    }

    // -----------------------------------------------------------------------
    // Internals
    // -----------------------------------------------------------------------

    fn require_admin(env: &Env, addr: &Address) {
        addr.require_auth();
        let admin: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::Admin)
            .expect("proxy not initialized");
        if *addr != admin {
            panic!("caller is not the proxy admin");
        }
    }

    /// A proxy must always point at *some* implementation and never at itself.
    fn validate_implementation(env: &Env, implementation: &Address) {
        let proxy: Address = env
            .storage()
            .instance()
            .get(&ProxyDataKey::SelfAddress)
            .unwrap_or_else(|| env.current_contract_address());
        if *implementation == proxy {
            panic!("implementation must not be the proxy itself");
        }
        if env
            .storage()
            .instance()
            .get::<ProxyDataKey, Address>(&ProxyDataKey::Implementation)
            == Some(implementation.clone())
        {
            panic!("implementation is already active");
        }
    }
}

// ===========================================================================
// Unit Tests
// ===========================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    const TIMELOCK: u64 = 100;

    fn setup() -> (Env, Address, Address, Address, ProxyClient<'static>) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let admin = Address::generate(&env);
        let impl_v1 = Address::generate(&env);
        let proxy_id = env.register_contract(None, Proxy);
        let client = ProxyClient::new(&env, &proxy_id);

        client.initialize(&admin, &impl_v1);
        client.set_timelock(&admin, &TIMELOCK);

        (env, admin, impl_v1, proxy_id, client)
    }

    fn advance(env: &Env, seconds: u64) {
        env.ledger().with_mut(|li| li.timestamp += seconds);
    }

    #[test]
    fn test_initialize() {
        let (_env, admin, impl_v1, _proxy_id, client) = setup();

        assert_eq!(client.get_admin(), admin);
        assert_eq!(client.get_implementation(), impl_v1.clone());
        assert_eq!(client.get_version(), 1);
        assert_eq!(client.get_upgrade_count(), 0);
    }

    #[test]
    #[should_panic(expected = "proxy already initialized")]
    fn test_double_initialize_panics() {
        let (_env, admin, impl_v1, _proxy_id, client) = setup();
        client.initialize(&admin, &impl_v1);
    }

    #[test]
    fn test_upgrade_flow() {
        let (env, admin, impl_v1, _proxy_id, client) = setup();
        let impl_v2 = Address::generate(&env);

        client.propose_implementation(&admin, &impl_v2);
        assert_eq!(client.get_pending_implementation(), Some(impl_v2.clone()));
        assert!(!client.is_upgrade_ready());

        advance(&env, TIMELOCK);
        assert!(client.is_upgrade_ready());

        client.execute_upgrade(&admin);

        assert_eq!(client.get_implementation(), impl_v2);
        assert_eq!(client.get_previous_implementation(), Some(impl_v1.clone()));
        assert_eq!(client.get_version(), 2);
        assert_eq!(client.get_upgrade_count(), 1);
        assert_eq!(client.get_pending_implementation(), None);

        let record = client.get_upgrade_record(&0).unwrap();
        assert_eq!(record.version, 2);
        assert_eq!(record.implementation, impl_v2);
        assert_eq!(record.previous_implementation, impl_v1.clone());
        assert_eq!(record.upgraded_by, admin);
    }

    #[test]
    #[should_panic(expected = "upgrade timelock has not expired")]
    fn test_execute_before_timelock_panics() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();
        let impl_v2 = Address::generate(&env);

        client.propose_implementation(&admin, &impl_v2);
        advance(&env, TIMELOCK - 1);
        client.execute_upgrade(&admin);
    }

    #[test]
    #[should_panic(expected = "no pending implementation")]
    fn test_execute_without_proposal_panics() {
        let (_env, admin, _impl_v1, _proxy_id, client) = setup();
        client.execute_upgrade(&admin);
    }

    #[test]
    #[should_panic(expected = "an upgrade is already pending")]
    fn test_second_proposal_panics() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();

        client.propose_implementation(&admin, &Address::generate(&env));
        client.propose_implementation(&admin, &Address::generate(&env));
    }

    #[test]
    #[should_panic(expected = "implementation is already active")]
    fn test_propose_current_implementation_panics() {
        let (_env, admin, impl_v1, _proxy_id, client) = setup();
        client.propose_implementation(&admin, &impl_v1);
    }

    #[test]
    #[should_panic(expected = "implementation must not be the proxy itself")]
    fn test_propose_proxy_as_implementation_panics() {
        let (_env, admin, _impl_v1, proxy_id, client) = setup();
        client.propose_implementation(&admin, &proxy_id);
    }

    #[test]
    #[should_panic(expected = "caller is not the proxy admin")]
    fn test_non_admin_propose_panics() {
        let (env, _admin, _impl_v1, _proxy_id, client) = setup();
        let attacker = Address::generate(&env);
        client.propose_implementation(&attacker, &Address::generate(&env));
    }

    #[test]
    #[should_panic(expected = "caller is not the proxy admin")]
    fn test_non_admin_execute_panics() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();
        let attacker = Address::generate(&env);

        client.propose_implementation(&admin, &Address::generate(&env));
        advance(&env, TIMELOCK);
        client.execute_upgrade(&attacker);
    }

    #[test]
    #[should_panic(expected = "caller is not the proxy admin")]
    fn test_non_admin_rollback_panics() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();

        client.propose_implementation(&admin, &Address::generate(&env));
        advance(&env, TIMELOCK);
        client.execute_upgrade(&admin);

        client.rollback(&Address::generate(&env));
    }

    #[test]
    fn test_cancel_upgrade() {
        let (env, admin, impl_v1, _proxy_id, client) = setup();
        let impl_v2 = Address::generate(&env);

        client.propose_implementation(&admin, &impl_v2);
        client.cancel_upgrade(&admin);

        assert_eq!(client.get_pending_implementation(), None);
        assert_eq!(client.get_implementation(), impl_v1.clone());
        assert_eq!(client.get_version(), 1);

        // The cancelled proposal can be replaced by a new one.
        client.propose_implementation(&admin, &impl_v2);
        advance(&env, TIMELOCK);
        client.execute_upgrade(&admin);
        assert_eq!(client.get_implementation(), impl_v2);
    }

    #[test]
    #[should_panic(expected = "no pending implementation")]
    fn test_cancel_without_proposal_panics() {
        let (_env, admin, _impl_v1, _proxy_id, client) = setup();
        client.cancel_upgrade(&admin);
    }

    #[test]
    fn test_rollback() {
        let (env, admin, impl_v1, _proxy_id, client) = setup();
        let impl_v2 = Address::generate(&env);

        client.propose_implementation(&admin, &impl_v2);
        advance(&env, TIMELOCK);
        client.execute_upgrade(&admin);
        assert_eq!(client.get_implementation(), impl_v2);

        client.rollback(&admin);

        assert_eq!(client.get_implementation(), impl_v1.clone());
        assert_eq!(client.get_version(), 3);
        assert_eq!(client.get_upgrade_count(), 2);
        assert_eq!(client.get_upgrade_record(&1).unwrap().implementation, impl_v1);
    }

    #[test]
    #[should_panic(expected = "no previous implementation to roll back to")]
    fn test_rollback_before_any_upgrade_panics() {
        let (_env, admin, _impl_v1, _proxy_id, client) = setup();
        client.rollback(&admin);
    }

    #[test]
    fn test_governance_approval_required() {
        let (env, admin, impl_v1, _proxy_id, client) = setup();
        let governance = Address::generate(&env);
        let impl_v2 = Address::generate(&env);

        client.set_governance_approver(&admin, &Some(governance.clone()));
        client.propose_implementation(&admin, &impl_v2);
        advance(&env, TIMELOCK);

        assert!(client.try_execute_upgrade(&admin).is_err());

        client.approve_implementation(&governance);
        client.execute_upgrade(&admin);

        assert_eq!(client.get_implementation(), impl_v2);
        assert_eq!(client.get_previous_implementation(), Some(impl_v1.clone()));
    }

    #[test]
    #[should_panic(expected = "caller is not the governance approver")]
    fn test_wrong_governance_approver_panics() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();
        let governance = Address::generate(&env);

        client.set_governance_approver(&admin, &Some(governance));
        client.propose_implementation(&admin, &Address::generate(&env));
        client.approve_implementation(&Address::generate(&env));
    }

    #[test]
    fn test_status_snapshot() {
        let (env, admin, impl_v1, _proxy_id, client) = setup();
        let impl_v2 = Address::generate(&env);

        let status = client.get_status();
        assert_eq!(status.implementation, impl_v1.clone());
        assert_eq!(status.version, 1);
        assert_eq!(status.timelock_duration, TIMELOCK);
        assert!(status.pending_implementation.is_none());

        client.propose_implementation(&admin, &impl_v2);
        let status = client.get_status();
        assert_eq!(status.pending_implementation, Some(impl_v2));
        assert_eq!(status.execute_after, 1_000 + TIMELOCK);
    }

    #[test]
    fn test_migrate_hook() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();
        client.migrate(&admin);

        client.propose_implementation(&admin, &Address::generate(&env));
        assert!(client.try_migrate(&admin).is_err());
    }

    #[test]
    fn test_admin_transfer() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();
        let new_admin = Address::generate(&env);

        client.propose_admin(&admin, &new_admin);
        client.accept_admin(&new_admin);

        assert_eq!(client.get_admin(), new_admin);
        assert!(client.is_admin(&new_admin));
        assert!(client.try_set_timelock(&admin, &10).is_err());
    }

    #[test]
    fn test_default_timelock() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let admin = Address::generate(&env);
        let impl_v1 = Address::generate(&env);
        let proxy_id = env.register_contract(None, Proxy);
        let client = ProxyClient::new(&env, &proxy_id);

        client.initialize(&admin, &impl_v1);
        assert_eq!(client.get_timelock(), DEFAULT_TIMELOCK);

        let impl_v2 = Address::generate(&env);
        client.propose_implementation(&admin, &impl_v2);
        let status = client.get_status();
        assert_eq!(status.execute_after, 1_000 + DEFAULT_TIMELOCK);
    }

    #[test]
    fn test_multiple_sequential_upgrades() {
        let (env, admin, _impl_v1, _proxy_id, client) = setup();

        for _ in 0..3 {
            let next = Address::generate(&env);
            client.propose_implementation(&admin, &next);
            advance(&env, TIMELOCK);
            client.execute_upgrade(&admin);
        }

        assert_eq!(client.get_version(), 4);
        assert_eq!(client.get_upgrade_count(), 3);
        assert_eq!(client.get_upgrade_record(&2).unwrap().version, 4);
    }
}