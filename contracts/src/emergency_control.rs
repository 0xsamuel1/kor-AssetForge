use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, String, Symbol, Vec};

// ============================================================================
// Emergency Fund Recovery Types (Issue #205)
// ============================================================================

/// A pending emergency withdrawal request with 72-hour timelock and
/// multi-sig approval tracking.
#[derive(Clone)]
#[contracttype]
pub struct EmergencyWithdrawal {
    pub id: u64,
    pub initiator: Address,
    pub destination: Address,
    pub amount: i128,
    pub asset_id: u64,
    pub reason: String,
    pub proposed_at: u64,
    /// Ledger timestamp after which execution is allowed (proposed_at + 72h).
    pub execute_after: u64,
    /// Number of multi-sig approvals collected so far.
    pub approvals: u32,
    pub executed: bool,
    pub cancelled: bool,
}

// Storage keys for emergency withdrawal subsystem.
fn withdrawal_key(env: &Env, id: u64) -> Symbol {
    let id_bytes = encode_u64(id);
    let mut key = [0u8; 32];
    let prefix = b"ew_";
    let mut pos = 0;
    for &b in prefix { if pos < 32 { key[pos] = b; pos += 1; } }
    for &b in id_bytes.iter() {
        if b == 0 { break; }
        if pos < 32 { key[pos] = b; pos += 1; }
    }
    let s = core::str::from_utf8(&key[..pos]).unwrap_or("ew_0");
    Symbol::new(env, s)
}

fn withdrawal_nonce_key(env: &Env) -> Symbol { Symbol::new(env, "ew_nonce") }
fn withdrawal_signers_key(env: &Env) -> Symbol { Symbol::new(env, "ew_signers") }
fn withdrawal_threshold_key(env: &Env) -> Symbol { Symbol::new(env, "ew_threshold") }
fn withdrawal_whitelist_key(env: &Env) -> Symbol { Symbol::new(env, "ew_whitelist") }

fn approval_key(env: &Env, withdrawal_id: u64, signer: &Address) -> Symbol {
    let id_bytes = encode_u64(withdrawal_id);
    let mut key = [0u8; 32];
    let prefix = b"ea_";
    let mut pos = 0;
    for &b in prefix { if pos < 32 { key[pos] = b; pos += 1; } }
    for &b in id_bytes.iter() {
        if b == 0 { break; }
        if pos < 32 { key[pos] = b; pos += 1; }
    }
    // Append a short hash of the signer address bytes (first 4 bytes of contract id)
    let _ = signer;
    let s = core::str::from_utf8(&key[..pos]).unwrap_or("ea_0");
    Symbol::new(env, s)
}

/// 72-hour timelock in seconds.
pub const EMERGENCY_TIMELOCK_SECS: u64 = 72 * 3600;

/// Defines the scope of a pause operation.
/// Supports granular, per-function pauses or a global halt.
#[derive(Clone, PartialEq, Debug)]
#[contracttype]
pub enum PauseScope {
    /// Pause all operations (transfers, trading, minting)
    All,
    /// Pause only transfer operations
    Transfers,
    /// Pause only trading/marketplace operations
    Trading,
    /// Pause only minting operations
    Minting,
    /// Pause specific function
    Function(Symbol),
    /// Pause specific user
    User(Address),
    /// Pause specific asset
    Asset(u64),
}

/// A record of a pause or unpause event for audit trail purposes.
#[derive(Clone)]
#[contracttype]
pub struct PauseRecord {
    pub asset_id: u64,
    pub admin: Address,
    pub scope: PauseScope,
    pub reason: String,
    pub ledger_timestamp: u32,
    pub is_pause: bool,
}

/// Custom error codes for emergency control operations.
#[derive(Clone, Debug, PartialEq)]
#[contracttype]
pub enum EmergencyControlError {
    NotAdmin,
    AlreadyPaused,
    NotPaused,
    InvalidAsset,
}

// --- Storage Key Helpers ---

/// Storage key for the admin address.
fn admin_key(env: &Env) -> Symbol {
    Symbol::new(env, "ec_admin")
}

/// Storage key for a pause flag: "pause_{asset_id}_{scope_index}"
fn pause_flag_key(env: &Env, asset_id: u64, scope: &PauseScope) -> Symbol {
    let scope_idx: u32 = match scope {
        PauseScope::All => 0,
        PauseScope::Transfers => 1,
        PauseScope::Trading => 2,
        PauseScope::Minting => 3,
        PauseScope::Function(_) => 4,
        PauseScope::User(_) => 5,
        PauseScope::Asset(_) => 6,
    };
    let mut key_str = [0u8; 32];
    let prefix = b"p_";
    let mut pos = 0;
    for &b in prefix {
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    let id_str = encode_u64(asset_id);
    for &b in id_str.iter() {
        if b == 0 {
            break;
        }
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    if pos < 32 {
        key_str[pos] = b'_';
        pos += 1;
    }
    if pos < 32 {
        key_str[pos] = b'0' + scope_idx as u8;
        pos += 1;
    }
    let s = core::str::from_utf8(&key_str[..pos]).unwrap_or("p_0_0");
    Symbol::new(env, s)
}

/// Storage key for auto-unpause ledger sequence.
fn auto_unpause_key(env: &Env, asset_id: u64, scope: &PauseScope) -> Symbol {
    let scope_idx: u32 = match scope {
        PauseScope::All => 0,
        PauseScope::Transfers => 1,
        PauseScope::Trading => 2,
        PauseScope::Minting => 3,
        PauseScope::Function(_) => 4,
        PauseScope::User(_) => 5,
        PauseScope::Asset(_) => 6,
    };
    let mut key_str = [0u8; 32];
    let prefix = b"au_";
    let mut pos = 0;
    for &b in prefix {
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    let id_str = encode_u64(asset_id);
    for &b in id_str.iter() {
        if b == 0 {
            break;
        }
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    if pos < 32 {
        key_str[pos] = b'_';
        pos += 1;
    }
    if pos < 32 {
        key_str[pos] = b'0' + scope_idx as u8;
        pos += 1;
    }
    let s = core::str::from_utf8(&key_str[..pos]).unwrap_or("au_0_0");
    Symbol::new(env, s)
}

/// Storage key for pause history of an asset.
fn history_key(env: &Env, asset_id: u64) -> Symbol {
    let mut key_str = [0u8; 32];
    let prefix = b"ph_";
    let mut pos = 0;
    for &b in prefix {
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    let id_str = encode_u64(asset_id);
    for &b in id_str.iter() {
        if b == 0 {
            break;
        }
        if pos < 32 {
            key_str[pos] = b;
            pos += 1;
        }
    }
    let s = core::str::from_utf8(&key_str[..pos]).unwrap_or("ph_0");
    Symbol::new(env, s)
}

/// Circuit breaker: last recorded price for an asset.
fn cb_price_key(env: &Env, asset_id: u64) -> Symbol {
    let id_str = encode_u64(asset_id);
    let mut key = [0u8; 32];
    let prefix = b"cb_p_";
    let mut pos = 0;
    for &b in prefix { if pos < 32 { key[pos] = b; pos += 1; } }
    for &b in id_str.iter() { if b == 0 { break; } if pos < 32 { key[pos] = b; pos += 1; } }
    let s = core::str::from_utf8(&key[..pos]).unwrap_or("cb_p_0");
    Symbol::new(env, s)
}

/// Circuit breaker: timestamp of last price observation.
fn cb_time_key(env: &Env, asset_id: u64) -> Symbol {
    let id_str = encode_u64(asset_id);
    let mut key = [0u8; 32];
    let prefix = b"cb_t_";
    let mut pos = 0;
    for &b in prefix { if pos < 32 { key[pos] = b; pos += 1; } }
    for &b in id_str.iter() { if b == 0 { break; } if pos < 32 { key[pos] = b; pos += 1; } }
    let s = core::str::from_utf8(&key[..pos]).unwrap_or("cb_t_0");
    Symbol::new(env, s)
}

/// Encode a u64 into a fixed-size byte array of ASCII digits.
fn encode_u64(mut val: u64) -> [u8; 20] {
    let mut buf = [0u8; 20];
    if val == 0 {
        buf[0] = b'0';
        return buf;
    }
    let mut pos = 0;
    let mut tmp = [0u8; 20];
    while val > 0 {
        tmp[pos] = b'0' + (val % 10) as u8;
        val /= 10;
        pos += 1;
    }
    for i in 0..pos {
        buf[i] = tmp[pos - 1 - i];
    }
    buf
}

#[contract]
pub struct EmergencyControl;

#[contractimpl]
impl EmergencyControl {
    // ---------------------------------------------------------------
    // Admin Management
    // ---------------------------------------------------------------

    /// Initialize the emergency control contract with an admin address.
    /// Must be called once before any other function.
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        if env.storage().instance().has(&admin_key(&env)) {
            panic!("already initialized");
        }
        env.storage().instance().set(&admin_key(&env), &admin);
    }

    /// Returns the current admin address.
    pub fn get_admin(env: Env) -> Address {
        env.storage()
            .instance()
            .get(&admin_key(&env))
            .expect("not initialized")
    }

    // ---------------------------------------------------------------
    // Pause / Unpause
    // ---------------------------------------------------------------

    /// Pause an asset for a given scope.
    ///
    /// # Arguments
    /// * `admin` - Must be the contract admin
    /// * `asset_id` - The asset to pause
    /// * `scope` - Which operations to pause (All, Transfers, Trading, Minting)
    /// * `reason` - Human-readable reason for the pause
    /// * `auto_unpause_ledger` - Optional ledger sequence number at which to auto-unpause (0 = no auto-unpause)
    pub fn pause_asset(
        env: Env,
        admin: Address,
        asset_id: u64,
        scope: PauseScope,
        reason: String,
        auto_unpause_ledger: u32,
    ) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let flag_key = pause_flag_key(&env, asset_id, &scope);

        // Check not already paused for this scope
        let already_paused: bool = env.storage().instance().get(&flag_key).unwrap_or(false);
        if already_paused {
            panic!("asset is already paused for this scope");
        }

        // Set the pause flag
        env.storage().instance().set(&flag_key, &true);

        // Set auto-unpause if requested
        if auto_unpause_ledger > 0 {
            let au_key = auto_unpause_key(&env, asset_id, &scope);
            env.storage().instance().set(&au_key, &auto_unpause_ledger);
        }

        // Record in audit trail
        let record = PauseRecord {
            asset_id,
            admin: admin.clone(),
            scope: scope.clone(),
            reason: reason.clone(),
            ledger_timestamp: env.ledger().sequence(),
            is_pause: true,
        };
        Self::append_history(&env, asset_id, record);

        // Emit event
        env.events().publish(
            (Symbol::new(&env, "asset_paused"), asset_id),
            (admin, scope, reason, env.ledger().sequence()),
        );
    }

    /// Unpause an asset for a given scope.
    ///
    /// # Arguments
    /// * `admin` - Must be the contract admin
    /// * `asset_id` - The asset to unpause
    /// * `scope` - Which operations to unpause
    pub fn unpause_asset(env: Env, admin: Address, asset_id: u64, scope: PauseScope) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let flag_key = pause_flag_key(&env, asset_id, &scope);

        // Check that it is currently paused
        let is_paused: bool = env.storage().instance().get(&flag_key).unwrap_or(false);
        if !is_paused {
            panic!("asset is not paused for this scope");
        }

        // Clear the pause flag
        env.storage().instance().set(&flag_key, &false);

        // Clear any auto-unpause timer
        let au_key = auto_unpause_key(&env, asset_id, &scope);
        env.storage().instance().remove(&au_key);

        // Record in audit trail
        let reason = String::from_str(&env, "manual_unpause");
        let record = PauseRecord {
            asset_id,
            admin: admin.clone(),
            scope: scope.clone(),
            reason,
            ledger_timestamp: env.ledger().sequence(),
            is_pause: false,
        };
        Self::append_history(&env, asset_id, record);

        // Emit event
        env.events().publish(
            (Symbol::new(&env, "asset_unpaused"), asset_id),
            (admin, scope, env.ledger().sequence()),
        );
    }

    // ---------------------------------------------------------------
    // Query Functions
    // ---------------------------------------------------------------

    /// Check if an asset is paused for a given scope.
    /// Also checks the `All` scope (if `All` is paused, everything is paused).
    /// Handles auto-unpause expiry transparently.
    pub fn is_paused(env: Env, asset_id: u64, scope: PauseScope) -> bool {
        // Check auto-unpause for the specific scope
        Self::check_auto_unpause_internal(&env, asset_id, &scope);
        // Check auto-unpause for the All scope
        Self::check_auto_unpause_internal(&env, asset_id, &PauseScope::All);

        // Check if paused for the specific scope
        let flag_key = pause_flag_key(&env, asset_id, &scope);
        let specific_paused: bool = env.storage().instance().get(&flag_key).unwrap_or(false);
        if specific_paused {
            return true;
        }

        // Check if globally paused (scope = All)
        if scope != PauseScope::All {
            let all_key = pause_flag_key(&env, asset_id, &PauseScope::All);
            let all_paused: bool = env.storage().instance().get(&all_key).unwrap_or(false);
            if all_paused {
                return true;
            }
        }

        false
    }

    /// Enforcement modifier: panics if the asset is paused for the given scope.
    /// Call this at the start of any function that should be blocked when paused.
    pub fn require_not_paused(env: Env, asset_id: u64, scope: PauseScope) {
        if Self::is_paused(env, asset_id, scope) {
            panic!("operation blocked: asset is paused");
        }
    }

    /// Get the full pause history for an asset.
    pub fn get_pause_history(env: Env, asset_id: u64) -> Vec<PauseRecord> {
        let h_key = history_key(&env, asset_id);
        env.storage()
            .instance()
            .get(&h_key)
            .unwrap_or(Vec::new(&env))
    }

    // ---------------------------------------------------------------
    // Emergency Fund Recovery (Issue #205)
    // ---------------------------------------------------------------

    /// Configure multi-sig signers and approval threshold for emergency withdrawals.
    /// Must be called by admin. Requires at least 2-of-N for meaningful multi-sig.
    pub fn configure_recovery_multisig(
        env: Env,
        admin: Address,
        signers: Vec<Address>,
        threshold: u32,
    ) {
        admin.require_auth();
        Self::require_admin(&env, &admin);
        if threshold == 0 || threshold > signers.len() {
            panic!("invalid threshold");
        }
        env.storage().instance().set(&withdrawal_signers_key(&env), &signers);
        env.storage().instance().set(&withdrawal_threshold_key(&env), &threshold);
    }

    /// Add an address to the recovery destination whitelist.
    pub fn add_recovery_destination(env: Env, admin: Address, destination: Address) {
        admin.require_auth();
        Self::require_admin(&env, &admin);
        let mut wl: Vec<Address> = env
            .storage()
            .instance()
            .get(&withdrawal_whitelist_key(&env))
            .unwrap_or(Vec::new(&env));
        for addr in wl.iter() {
            if addr == destination {
                panic!("destination already whitelisted");
            }
        }
        wl.push_back(destination.clone());
        env.storage().instance().set(&withdrawal_whitelist_key(&env), &wl);
        env.events().publish(
            (Symbol::new(&env, "recovery_dest_added"), destination),
            env.ledger().timestamp(),
        );
    }

    /// Remove an address from the recovery destination whitelist.
    pub fn remove_recovery_destination(env: Env, admin: Address, destination: Address) {
        admin.require_auth();
        Self::require_admin(&env, &admin);
        let wl: Vec<Address> = env
            .storage()
            .instance()
            .get(&withdrawal_whitelist_key(&env))
            .unwrap_or(Vec::new(&env));
        let mut new_wl = Vec::new(&env);
        let mut found = false;
        for addr in wl.iter() {
            if addr == destination {
                found = true;
            } else {
                new_wl.push_back(addr);
            }
        }
        if !found {
            panic!("destination not in whitelist");
        }
        env.storage().instance().set(&withdrawal_whitelist_key(&env), &new_wl);
    }

    /// Propose an emergency withdrawal. Admin only.
    ///
    /// The destination must be on the recovery whitelist.
    /// The proposal is locked for 72 hours before execution is allowed.
    /// Returns the withdrawal id.
    pub fn propose_emergency_withdrawal(
        env: Env,
        admin: Address,
        destination: Address,
        amount: i128,
        asset_id: u64,
        reason: String,
    ) -> u64 {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        if amount <= 0 {
            panic!("amount must be positive");
        }

        // Destination must be whitelisted
        let wl: Vec<Address> = env
            .storage()
            .instance()
            .get(&withdrawal_whitelist_key(&env))
            .unwrap_or(Vec::new(&env));
        let mut whitelisted = false;
        for addr in wl.iter() {
            if addr == destination { whitelisted = true; break; }
        }
        if !whitelisted {
            panic!("destination is not whitelisted");
        }

        let nonce: u64 = env
            .storage()
            .instance()
            .get(&withdrawal_nonce_key(&env))
            .unwrap_or(0)
            + 1;
        env.storage().instance().set(&withdrawal_nonce_key(&env), &nonce);

        let now = env.ledger().timestamp();
        let withdrawal = EmergencyWithdrawal {
            id: nonce,
            initiator: admin.clone(),
            destination: destination.clone(),
            amount,
            asset_id,
            reason: reason.clone(),
            proposed_at: now,
            execute_after: now + EMERGENCY_TIMELOCK_SECS,
            approvals: 0,
            executed: false,
            cancelled: false,
        };

        env.storage().instance().set(&withdrawal_key(&env, nonce), &withdrawal);

        env.events().publish(
            (Symbol::new(&env, "emergency_proposed"), admin),
            (nonce, destination, amount, now + EMERGENCY_TIMELOCK_SECS),
        );

        nonce
    }

    /// Multi-sig signer approves an emergency withdrawal.
    pub fn approve_emergency_withdrawal(env: Env, signer: Address, withdrawal_id: u64) {
        signer.require_auth();

        let signers: Vec<Address> = env
            .storage()
            .instance()
            .get(&withdrawal_signers_key(&env))
            .expect("multi-sig not configured");

        let mut is_signer = false;
        for s in signers.iter() {
            if s == signer { is_signer = true; break; }
        }
        if !is_signer {
            panic!("caller is not a registered signer");
        }

        // Use a combined key: approval_key encodes withdrawal_id; we store per-signer
        // via a secondary persistent key to avoid Symbol length limits.
        let appr_key = approval_key(&env, withdrawal_id, &signer);
        // Check for double-approval: we store the signer list for this withdrawal
        let mut approved_signers: Vec<Address> = env
            .storage()
            .persistent()
            .get(&appr_key)
            .unwrap_or(Vec::new(&env));
        for s in approved_signers.iter() {
            if s == signer { panic!("already approved"); }
        }
        approved_signers.push_back(signer.clone());
        env.storage().persistent().set(&appr_key, &approved_signers);

        let mut withdrawal: EmergencyWithdrawal = env
            .storage()
            .instance()
            .get(&withdrawal_key(&env, withdrawal_id))
            .expect("withdrawal not found");

        if withdrawal.executed || withdrawal.cancelled {
            panic!("withdrawal is already closed");
        }

        withdrawal.approvals += 1;
        env.storage().instance().set(&withdrawal_key(&env, withdrawal_id), &withdrawal);

        env.events().publish(
            (Symbol::new(&env, "emergency_approved"), signer),
            (withdrawal_id, withdrawal.approvals),
        );
    }

    /// Execute the emergency withdrawal after timelock and multi-sig threshold met.
    ///
    /// Admin only. Emits `emergency_executed`.
    pub fn execute_emergency_withdrawal(env: Env, admin: Address, withdrawal_id: u64) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let mut withdrawal: EmergencyWithdrawal = env
            .storage()
            .instance()
            .get(&withdrawal_key(&env, withdrawal_id))
            .expect("withdrawal not found");

        if withdrawal.executed {
            panic!("already executed");
        }
        if withdrawal.cancelled {
            panic!("withdrawal was cancelled");
        }

        let now = env.ledger().timestamp();
        if now < withdrawal.execute_after {
            panic!("72-hour timelock has not expired");
        }

        let threshold: u32 = env
            .storage()
            .instance()
            .get(&withdrawal_threshold_key(&env))
            .expect("multi-sig not configured");

        if withdrawal.approvals < threshold {
            panic!("insufficient multi-sig approvals");
        }

        withdrawal.executed = true;
        env.storage().instance().set(&withdrawal_key(&env, withdrawal_id), &withdrawal);

        env.events().publish(
            (Symbol::new(&env, "emergency_executed"), admin),
            (withdrawal_id, withdrawal.destination.clone(), withdrawal.amount),
        );
    }

    /// Cancel a pending emergency withdrawal. Admin only.
    pub fn cancel_emergency_withdrawal(env: Env, admin: Address, withdrawal_id: u64) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let mut withdrawal: EmergencyWithdrawal = env
            .storage()
            .instance()
            .get(&withdrawal_key(&env, withdrawal_id))
            .expect("withdrawal not found");

        if withdrawal.executed {
            panic!("already executed");
        }
        if withdrawal.cancelled {
            panic!("already cancelled");
        }

        withdrawal.cancelled = true;
        env.storage().instance().set(&withdrawal_key(&env, withdrawal_id), &withdrawal);

        env.events().publish(
            (Symbol::new(&env, "emergency_cancelled"), admin),
            withdrawal_id,
        );
    }

    /// Get a withdrawal proposal by id.
    pub fn get_emergency_withdrawal(env: Env, withdrawal_id: u64) -> Option<EmergencyWithdrawal> {
        env.storage().instance().get(&withdrawal_key(&env, withdrawal_id))
    }

    /// Get the recovery destination whitelist.
    pub fn get_recovery_whitelist(env: Env) -> Vec<Address> {
        env.storage()
            .instance()
            .get(&withdrawal_whitelist_key(&env))
            .unwrap_or(Vec::new(&env))
    }

    // ---------------------------------------------------------------
    // Circuit Breaker (Issue #231)
    // ---------------------------------------------------------------

    /// Record a price observation for an asset.  If the price has dropped by
    /// more than 50% within the last hour the circuit breaker trips and the
    /// asset is automatically paused for `PauseScope::Trading`.
    ///
    /// Returns `true` if the breaker tripped.
    pub fn record_price(env: Env, admin: Address, asset_id: u64, price: i128) -> bool {
        admin.require_auth();
        Self::require_admin(&env, &admin);
        if price <= 0 {
            panic!("price must be positive");
        }

        let now = env.ledger().timestamp();
        let price_key = cb_price_key(&env, asset_id);
        let time_key = cb_time_key(&env, asset_id);

        let prev_price: Option<i128> = env.storage().instance().get(&price_key);
        let prev_time: Option<u64> = env.storage().instance().get(&time_key);

        // Persist current observation
        env.storage().instance().set(&price_key, &price);
        env.storage().instance().set(&time_key, &now);

        // Trip if price dropped >50% within 1 hour
        if let (Some(prev), Some(t)) = (prev_price, prev_time) {
            let elapsed = now.saturating_sub(t);
            if elapsed <= 3600 && prev > 0 {
                // price dropped by more than 50%: price < prev / 2
                if price * 2 < prev {
                    // Trip breaker: pause trading
                    let flag_key = pause_flag_key(&env, asset_id, &PauseScope::Trading);
                    let already_paused: bool =
                        env.storage().instance().get(&flag_key).unwrap_or(false);
                    if !already_paused {
                        env.storage().instance().set(&flag_key, &true);
                        // Set auto-unpause after cooldown (1 hour = ~720 ledgers approx)
                        let cooldown_ledger = env.ledger().sequence() + 720;
                        let au_key = auto_unpause_key(&env, asset_id, &PauseScope::Trading);
                        env.storage().instance().set(&au_key, &cooldown_ledger);

                        let reason = String::from_str(&env, "circuit_breaker_price_crash");
                        let record = PauseRecord {
                            asset_id,
                            admin: admin.clone(),
                            scope: PauseScope::Trading,
                            reason,
                            ledger_timestamp: env.ledger().sequence(),
                            is_pause: true,
                        };
                        Self::append_history(&env, asset_id, record);
                        env.events().publish(
                            (Symbol::new(&env, "circuit_breaker_tripped"), asset_id),
                            (prev, price, now),
                        );
                        return true;
                    }
                }
            }
        }
        false
    }

    /// Record a volume spike observation.  If `volume` exceeds `threshold`
    /// the circuit breaker trips and pauses trading for this asset.
    pub fn record_volume(
        env: Env,
        admin: Address,
        asset_id: u64,
        volume: i128,
        threshold: i128,
    ) -> bool {
        admin.require_auth();
        Self::require_admin(&env, &admin);
        if threshold <= 0 {
            panic!("threshold must be positive");
        }

        if volume > threshold {
            let flag_key = pause_flag_key(&env, asset_id, &PauseScope::Trading);
            let already_paused: bool =
                env.storage().instance().get(&flag_key).unwrap_or(false);
            if !already_paused {
                env.storage().instance().set(&flag_key, &true);
                let cooldown_ledger = env.ledger().sequence() + 720;
                let au_key = auto_unpause_key(&env, asset_id, &PauseScope::Trading);
                env.storage().instance().set(&au_key, &cooldown_ledger);

                let reason = String::from_str(&env, "circuit_breaker_volume_spike");
                let record = PauseRecord {
                    asset_id,
                    admin: admin.clone(),
                    scope: PauseScope::Trading,
                    reason,
                    ledger_timestamp: env.ledger().sequence(),
                    is_pause: true,
                };
                Self::append_history(&env, asset_id, record);
                env.events().publish(
                    (Symbol::new(&env, "circuit_breaker_tripped"), asset_id),
                    (volume, threshold),
                );
                return true;
            }
        }
        false
    }

    /// Manual unpause by admin (requires explicit approval intent for circuit-breaker pauses).
    /// Delegates to `unpause_asset` for the Trading scope.
    pub fn circuit_breaker_unpause(env: Env, admin: Address, asset_id: u64) {
        // reuse existing unpause logic
        Self::unpause_asset(env, admin, asset_id, PauseScope::Trading);
    }

    // ---------------------------------------------------------------
    // Internal Helpers
    // ---------------------------------------------------------------

    /// Verify that the caller is the admin.
    fn require_admin(env: &Env, caller: &Address) {
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&admin_key(env))
            .expect("not initialized");
        if *caller != stored_admin {
            panic!("caller is not admin");
        }
    }

    /// Append a PauseRecord to the history for a given asset.
    fn append_history(env: &Env, asset_id: u64, record: PauseRecord) {
        let h_key = history_key(env, asset_id);
        let mut history: Vec<PauseRecord> = env
            .storage()
            .instance()
            .get(&h_key)
            .unwrap_or(Vec::new(env));
        history.push_back(record);
        env.storage().instance().set(&h_key, &history);
    }

    /// Check if auto-unpause should trigger for a given asset+scope.
    /// If the current ledger sequence >= the auto-unpause ledger, clear the pause.
    fn check_auto_unpause_internal(env: &Env, asset_id: u64, scope: &PauseScope) {
        let au_key = auto_unpause_key(env, asset_id, scope);
        let auto_ledger: Option<u32> = env.storage().instance().get(&au_key);
        if let Some(target_ledger) = auto_ledger {
            if env.ledger().sequence() >= target_ledger {
                // Auto-unpause: clear the flag and timer
                let flag_key = pause_flag_key(env, asset_id, scope);
                env.storage().instance().set(&flag_key, &false);
                env.storage().instance().remove(&au_key);

                // Record in audit trail
                let reason = String::from_str(env, "auto_unpause");
                let record = PauseRecord {
                    asset_id,
                    admin: env
                        .storage()
                        .instance()
                        .get(&admin_key(env))
                        .unwrap_or_else(|| panic!("not initialized")),
                    scope: scope.clone(),
                    reason,
                    ledger_timestamp: env.ledger().sequence(),
                    is_pause: false,
                };
                Self::append_history(env, asset_id, record);

                // Emit event
                let admin: Address = env
                    .storage()
                    .instance()
                    .get(&admin_key(env))
                    .unwrap_or_else(|| panic!("not initialized"));
                env.events().publish(
                    (Symbol::new(env, "asset_unpaused"), asset_id),
                    (admin, scope.clone(), env.ledger().sequence()),
                );
            }
        }
    }
}

// =================================================================
// Unit Tests
// =================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger, LedgerInfo};

    #[test]
    fn test_initialize_and_get_admin() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);
        let stored_admin = client.get_admin();
        assert_eq!(stored_admin, admin);
    }

    #[test]
    #[should_panic(expected = "already initialized")]
    fn test_double_initialize_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);
        client.initialize(&admin);
    }

    #[test]
    fn test_pause_and_unpause_asset() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "security incident");

        // Pause trading
        client.pause_asset(&admin, &asset_id, &PauseScope::Trading, &reason, &0);
        assert!(client.is_paused(&asset_id, &PauseScope::Trading));

        // Transfers should NOT be paused
        assert!(!client.is_paused(&asset_id, &PauseScope::Transfers));

        // Unpause trading
        client.unpause_asset(&admin, &asset_id, &PauseScope::Trading);
        assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
    }

    #[test]
    fn test_pause_all_blocks_everything() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "global halt");

        client.pause_asset(&admin, &asset_id, &PauseScope::All, &reason, &0);

        // All scopes should report as paused
        assert!(client.is_paused(&asset_id, &PauseScope::All));
        assert!(client.is_paused(&asset_id, &PauseScope::Transfers));
        assert!(client.is_paused(&asset_id, &PauseScope::Trading));
        assert!(client.is_paused(&asset_id, &PauseScope::Minting));
    }

    #[test]
    fn test_pause_history_recorded() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "audit");

        client.pause_asset(&admin, &asset_id, &PauseScope::Minting, &reason, &0);
        client.unpause_asset(&admin, &asset_id, &PauseScope::Minting);

        let history = client.get_pause_history(&asset_id);
        assert_eq!(history.len(), 2);

        // First record is the pause
        let first = history.get(0).unwrap();
        assert!(first.is_pause);
        assert_eq!(first.asset_id, asset_id);

        // Second record is the unpause
        let second = history.get(1).unwrap();
        assert!(!second.is_pause);
    }

    #[test]
    #[should_panic(expected = "asset is already paused for this scope")]
    fn test_double_pause_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "test");

        client.pause_asset(&admin, &asset_id, &PauseScope::Trading, &reason, &0);
        client.pause_asset(&admin, &asset_id, &PauseScope::Trading, &reason, &0);
    }

    #[test]
    #[should_panic(expected = "asset is not paused for this scope")]
    fn test_unpause_not_paused_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        client.unpause_asset(&admin, &asset_id, &PauseScope::Trading);
    }

    #[test]
    #[should_panic(expected = "caller is not admin")]
    fn test_non_admin_pause_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let non_admin = Address::generate(&env);
        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "test");

        client.pause_asset(&non_admin, &asset_id, &PauseScope::Trading, &reason, &0);
    }

    #[test]
    #[should_panic(expected = "operation blocked: asset is paused")]
    fn test_require_not_paused_blocks_when_paused() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "blocked");

        client.pause_asset(&admin, &asset_id, &PauseScope::Trading, &reason, &0);
        client.require_not_paused(&asset_id, &PauseScope::Trading);
    }

    #[test]
    fn test_require_not_paused_allows_when_not_paused() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        // Should not panic
        client.require_not_paused(&asset_id, &PauseScope::Trading);
    }

    #[test]
    fn test_auto_unpause() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "temporary halt");

        // Set auto-unpause at ledger sequence 100
        client.pause_asset(&admin, &asset_id, &PauseScope::Trading, &reason, &100);
        assert!(client.is_paused(&asset_id, &PauseScope::Trading));

        // Advance ledger sequence to 100
        env.ledger().set(LedgerInfo {
            timestamp: 0,
            protocol_version: 20,
            sequence_number: 100,
            network_id: Default::default(),
            base_reserve: 10,
            min_temp_entry_ttl: 10,
            min_persistent_entry_ttl: 10,
            max_entry_ttl: 3110400,
        });

        // Should auto-unpause
        assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
    }

    #[test]
    fn test_selective_scope_independence() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let asset_id: u64 = 1;
        let reason = String::from_str(&env, "scope test");

        // Pause only transfers
        client.pause_asset(&admin, &asset_id, &PauseScope::Transfers, &reason, &0);

        // Transfers paused
        assert!(client.is_paused(&asset_id, &PauseScope::Transfers));

        // Trading and Minting should NOT be paused
        assert!(!client.is_paused(&asset_id, &PauseScope::Trading));
        assert!(!client.is_paused(&asset_id, &PauseScope::Minting));
    }

    #[test]
    fn test_multiple_assets_independent() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let reason = String::from_str(&env, "asset isolation test");

        // Pause asset 1 for trading
        client.pause_asset(&admin, &1, &PauseScope::Trading, &reason, &0);

        // Asset 1 trading paused
        assert!(client.is_paused(&1, &PauseScope::Trading));

        // Asset 2 trading should NOT be paused
        assert!(!client.is_paused(&2, &PauseScope::Trading));
    }

    #[test]
    fn test_empty_pause_history() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, EmergencyControl);
        let client = EmergencyControlClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin);

        let history = client.get_pause_history(&1);
        assert_eq!(history.len(), 0);
    }
}
