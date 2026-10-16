use soroban_sdk::{contract, contracterror, contractimpl, contracttype, Address, Env, String, Symbol, Vec};

use crate::emergency_control::{EmergencyControlClient, PauseScope};
use crate::governance::GovernanceClient;
use crate::oracle::{OracleClient, AggregatedPrice};
use crate::reputation::ReputationContractClient;
use crate::whitelist::WhitelistClient;


#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum ComplianceError {
    InvalidTimeRange = 1,
    Unauthorized = 2,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TimeRange {
    pub start_timestamp: u64,
    pub end_timestamp: u64,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ComplianceMetrics {
    pub total_value: i128,
    pub volume: i128,
    pub holders: u32,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ReportFormat {
    Json,
    Csv,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AssetConfig {
    pub asset_id: u64,
    pub metadata: String,
    pub governance_required: bool,
    pub deprecated: bool,
    pub min_trade_amount: i128,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TransferRestrictionConfig {
    pub whitelist_required: bool,
    pub min_holding_seconds: u64,
    pub lockup_seconds: u64,
    pub max_transfer_amount: i128,
    pub approval_required: bool,
    pub emergency_override_enabled: bool,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TransferApprovalRequest {
    pub id: u64,
    pub asset_id: u64,
    pub requester: Address,
    pub beneficiary: Address,
    pub amount: i128,
    pub reason: String,
    pub approved: bool,
    pub reviewed: bool,
    pub created_at: u64,
    pub reviewed_at: u64,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AssetComplianceSnapshot {
    pub asset_id: u64,
    pub timestamp: u64,
    pub transaction_count: u64,
    pub volume: i128,
    pub taxable_volume: i128,
    pub holders: u32,
    pub restricted_transfer_count: u64,
    pub compliance_failures: u64,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AuditTrailEntry {
    pub timestamp: u64,
    pub actor: Address,
    pub action: Symbol,
    pub asset_id: u64,
    pub amount: i128,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ReportSchedule {
    pub id: u64,
    pub asset_id: Option<u64>,
    pub cadence_seconds: u64,
    pub next_run_at: u64,
    pub format: ReportFormat,
    pub enabled: bool,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RegulatoryReport {
    pub id: u64,
    pub generated_at: u64,
    pub asset_id: Option<u64>,
    pub format: ReportFormat,
    pub total_value: i128,
    pub volume: i128,
    pub taxable_volume: i128,
    pub holders: u32,
    pub restricted_transfer_count: u64,
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ReportExport {
    Json(RegulatoryReport),
    Csv(RegulatoryReport),
}

#[contracttype]
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AssetAnalytics {
    pub asset_id: u64,
    pub listing_count: u64,
    pub transaction_count: u64,
    pub volume: i128,
    pub holders: u32,
}


#[derive(Clone)]
#[contracttype]
pub struct Listing {
    pub asset_id: u64,
    pub seller: Address,
    pub price: i128,
    pub amount: i128,
    pub active: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct BundleListing {
    pub listing_id: u64,
    pub bundle_id: u64,
    pub bundle_contract: Address,
    pub seller: Address,
    pub price: i128,
    pub discount_bps: u32,
    pub active: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct BatchListingInput {
    pub asset_id: u64,
    pub amount: i128,
    pub price: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct BatchPurchaseInput {
    pub listing_id: u64,
    pub amount: i128,
    pub asset_id: u64,
}

#[derive(Clone)]
#[contracttype]
pub enum MarketplaceDataKey {
    MarketplaceAdmin,
    AssetPrivate(u64),
    Whitelisted(u64, Address),
    ListingNonce,
    Listing(u64),
    RegisteredAssets,
    AssetConfig(u64),
    ListingCount(u64),
    TransactionCount(u64),
    Volume(u64),
    TaxableVolume(u64),
    HolderCount(u64),
    HolderSeen(u64, Address),
    FirstHeldAt(u64, Address),
    LockupUntil(u64, Address),
    DailyTransfer(u64, Address, u64),
    RestrictionConfig(u64),
    ComplianceVerified(u64, Address),
    TransferRequestNonce,
    TransferRequest(u64),
    TransferApproved(u64),
    RestrictedTransferCount(u64),
    ComplianceFailures(u64),
    AuditTrail,
    ComplianceHistory(u64),
    ReportScheduleNonce,
    ReportSchedule(u64),
    ReportNonce,
    GeneratedReport(u64),
    Oracle,
    BundleListings,
    BundleListing(u64),
    // --- Flash-loan protection (Issue #210) ---
    /// Flash-loan guard configuration (instance-level).
    FlashLoanGuard,
    /// Per-buyer purchase counter for a given ledger (block): (buyer, ledger_seq) -> u32.
    BlockTrades(Address, u32),
}

/// Storage keys for buy-back and burn system.
#[derive(Clone)]
#[contracttype]
pub enum BuyBackDataKey {
    BuyBackAdminKey,
    BuyBackConfigKey,
    TreasuryBalanceKey,
    TotalBurnedKey,
    HistoryKey,
    GovernanceContractKey,
    ReputationContractKey,

}

/// Storage keys for the referral system.
#[derive(Clone)]
#[contracttype]
pub enum ReferralDataKey {
    ReferralConfigKey,
    ReferrerKey(Address),
    RewardBalanceKey(Address),
    ReferralCountKey(Address),
}

/// Configuration for the referral system.
#[derive(Clone)]
#[contracttype]
pub struct ReferralConfig {
    /// Reward percentage in basis points (e.g., 500 = 5% of trade fee)
    pub reward_bps: u32,
    /// Address of the treasury from which rewards are paid
    pub treasury: Address,
    /// Minimum trade activity (in native asset) requested for referral validity
    pub min_activity: i128,
}

/// Configuration for the buy-back and burn system.
#[derive(Clone)]
#[contracttype]
pub struct BuyBackConfig {
    /// Maximum tokens that can be burned in a single operation
    pub burn_cap: i128,
    /// Treasury balance threshold that triggers auto buy-back
    pub auto_threshold: i128,
    /// Amount to buy back when auto-triggered
    pub auto_buyback_amount: i128,
    /// Whether governance approval is required for burns
    pub require_governance: bool,
    /// Fee percentage (basis points) collected from marketplace trades
    pub fee_bps: u32,
    /// Whether the buy-back system is paused
    pub paused: bool,
}

/// Record of a buy-back or burn operation for audit trail.
#[derive(Clone)]
#[contracttype]
pub struct BuyBackRecord {
    /// Amount of tokens bought back
    pub amount: i128,
    /// Amount of tokens burned
    pub burned: i128,
    /// Source of funds used (treasury)
    pub source_funds: i128,
    /// Ledger timestamp when the operation occurred
    pub timestamp: u64,
    /// Address that triggered the operation
    pub executor: Address,
    /// Whether this was an auto-triggered buy-back
    pub auto_triggered: bool,
}

/// Flash-loan protection configuration for the marketplace (Issue #210).
///
/// Both limits default to the permissive value (`0`) so existing behaviour is
/// unchanged until an admin opts in via `configure_flash_loan_guard`.
#[derive(Clone)]
#[contracttype]
pub struct FlashLoanGuardConfig {
    /// Maximum number of purchases a single buyer may perform within one ledger
    /// (block). `0` disables the limit. Caps the atomic, same-block churn that
    /// flash-loan style attacks rely on.
    pub max_trades_per_block: u32,
    /// Maximum allowed deviation (bps) between a listing price and the oracle
    /// price on a guarded purchase. `0` disables the on-chain price check.
    pub max_price_deviation_bps: u32,
}

// ============================================================================
// CONTRACT
// ============================================================================

#[contract]
pub struct Marketplace;

#[contractimpl]
impl Marketplace {
    // -----------------------------------------------------------------------
    // Marketplace Operations (existing)
    // -----------------------------------------------------------------------

    /// List an asset for sale.
    /// Blocked if the asset is paused for Trading scope.
    /// Requires the asset to have been approved via governance.
    pub fn create_listing(
        env: Env,
        seller: Address,
        asset_id: u64,
        amount: i128,
        price: i128,
        emergency_control_id: Address,
        governance_id: Option<Address>,
    ) -> u64 {
        assert!(amount > 0, "amount must be positive");
        assert!(price > 0, "price must be positive");
        seller.require_auth();

        // Enforce pause check for trading operations
        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Trading);

        // Enforce governance approval if governance contract is provided
        if let Some(gov_addr) = governance_id {
            let gov_client = GovernanceClient::new(&env, &gov_addr);
            gov_client.require_approved(&asset_id);
        }

        // Enforce whitelisting if asset is private
        Self::require_whitelisted_if_private(&env, asset_id, &seller);
        // Enforce accredited investor check if whitelist contract is configured
        Self::require_accredited_investor(&env, &seller);

        // Block new listings for deprecated assets.
        if let Some(cfg) = env.storage().persistent().get::<_, AssetConfig>(&MarketplaceDataKey::AssetConfig(asset_id)) {
            if cfg.deprecated {
                panic!("asset is deprecated");
            }
        }

        Self::register_asset_if_missing(&env, asset_id);

        // Generate listing ID
        let listing_id: u64 = env
            .storage()
            .instance()
            .get(&MarketplaceDataKey::ListingNonce)
            .unwrap_or(0)
            + 1;
        env.storage().instance().set(&MarketplaceDataKey::ListingNonce, &listing_id);

        let listing = Listing {
            asset_id,
            seller: seller.clone(),
            price,
            amount,
            active: true,
        };

        env.storage().persistent().set(&MarketplaceDataKey::Listing(listing_id), &listing);

        let listing_count: u64 = env.storage().instance().get(&MarketplaceDataKey::ListingCount(asset_id)).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::ListingCount(asset_id), &listing_count);

        let total_value = price.checked_mul(amount).unwrap_or(0);
        let existing_total: i128 = env.storage().instance().get(&MarketplaceDataKey::Volume(asset_id)).unwrap_or(0);
        env.storage().instance().set(&MarketplaceDataKey::Volume(asset_id), &(existing_total + total_value));

        Self::append_audit_entry(&env, seller, Symbol::new(&env, "listing_created"), asset_id, total_value);

        listing_id
    }

    /// Create a listing with oracle price validation.
    /// The listing price must be within `max_deviation_bps` of the oracle price.
    pub fn create_listing_with_oracle(
        env: Env,
        seller: Address,
        asset_id: u64,
        amount: i128,
        price: i128,
        emergency_control_id: Address,
        governance_id: Option<Address>,
        max_deviation_bps: u32,
    ) -> u64 {
        assert!(
            Self::validate_listing_price(env.clone(), asset_id, price, max_deviation_bps),
            "price deviates too much from oracle"
        );
        Self::create_listing(env, seller, asset_id, amount, price, emergency_control_id, governance_id)
    }

    /// Purchase a listed asset.
    /// Blocked if the asset is paused for Trading scope.
    pub fn purchase(
        env: Env,
        buyer: Address,
        _listing_id: u64,
        amount: i128,
        asset_id: u64,
        emergency_control_id: Address,
    ) -> bool {
        assert!(amount > 0, "amount must be positive");
        buyer.require_auth();

        // Flash-loan guard: cap how many purchases a buyer can make per block.
        Self::enforce_block_trade_limit(&env, &buyer);

        // Enforce pause check for trading operations
        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Trading);

        // Enforce whitelisting if asset is private
        Self::require_whitelisted_if_private(&env, asset_id, &buyer);
        // Enforce accredited investor check if whitelist contract is configured
        Self::require_accredited_investor(&env, &buyer);
        // Enforce the asset's full TransferRestrictionConfig (max amount,
        // compliance verification, lock-up, minimum holding period, and
        // approval requirement) exactly as `purchase_internal` /
        // `purchase_with_approval` already do -- previously this was the
        // only transfer path that skipped it entirely, so an asset
        // configured with e.g. a lock-up or a transfer cap was enforceable
        // only if buyers happened to go through the approval flow (#380).
        // `approval_id: None` correctly rejects a restricted asset with
        // `approval_required = true` here, same as any other direct
        // (non-pre-approved) transfer attempt.
        Self::enforce_transfer_restrictions(&env, &buyer, asset_id, amount, None);

        // Collect fee and credit referral reward
        if env.storage().instance().has(&BuyBackDataKey::BuyBackConfigKey) {
            let fee = Self::collect_fee(env.clone(), amount);
            Self::credit_referral_reward(&env, &buyer, fee);
        }

        // Record trade completion in reputation system if configured
        if let Some(rep_addr) = env
            .storage()
            .instance()
            .get::<_, Address>(&BuyBackDataKey::ReputationContractKey)
        {
            let admin: Address = env
                .storage()
                .instance()
                .get(&BuyBackDataKey::BuyBackAdminKey)
                .unwrap_or(buyer.clone());
            let rep_client = ReputationContractClient::new(&env, &rep_addr);
            rep_client.record_trade_completion(&admin, &buyer);
        }
        true
    }

    /// Purchase a listed asset with oracle price validation.
    /// The purchase price must be within `max_deviation_bps` of the oracle price.
    pub fn purchase_with_oracle_price(
        env: Env,
        buyer: Address,
        listing_id: u64,
        amount: i128,
        asset_id: u64,
        emergency_control_id: Address,
        max_deviation_bps: u32,
    ) -> bool {
        let listing = Self::get_listing(env.clone(), listing_id)
            .expect("listing not found");
        assert!(
            Self::validate_listing_price(env.clone(), asset_id, listing.price, max_deviation_bps),
            "price deviates too much from oracle"
        );
        Self::purchase(env, buyer, listing_id, amount, asset_id, emergency_control_id)
    }

    /// Batch create listings for gas optimization.
    /// Validates all inputs upfront, then creates all listings atomically.
    /// Panics on any failure to ensure atomicity, rolling back the entire batch.
    /// Maximum 20 listings per batch.
    pub fn batch_create_listing(
        env: Env,
        seller: Address,
        listings: Vec<BatchListingInput>,
        emergency_control_id: Address,
        governance_id: Option<Address>,
    ) -> Vec<u64> {
        let count = listings.len();
        assert!(count > 0, "batch must not be empty");
        assert!(count <= 20, "batch size exceeds maximum of 20");

        seller.require_auth();

        for input in listings.iter() {
            assert!(input.amount > 0, "amount must be positive");
            assert!(input.price > 0, "price must be positive");

            let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
            ec_client.require_not_paused(&input.asset_id, &PauseScope::Trading);

            if let Some(ref gov_addr) = governance_id {
                let gov_client = GovernanceClient::new(&env, gov_addr);
                gov_client.require_approved(&input.asset_id);
            }

            Self::require_whitelisted_if_private(&env, input.asset_id, &seller);

            if let Some(cfg) = env.storage().persistent().get::<_, AssetConfig>(&MarketplaceDataKey::AssetConfig(input.asset_id)) {
                if cfg.deprecated {
                    panic!("asset is deprecated");
                }
            }
        }

        for input in listings.iter() {
            Self::register_asset_if_missing(&env, input.asset_id);
        }

        let mut result_ids: Vec<u64> = Vec::new(&env);
        let mut total_value: i128 = 0;

        for input in listings.iter() {
            let listing_id: u64 = env
                .storage()
                .instance()
                .get(&MarketplaceDataKey::ListingNonce)
                .unwrap_or(0)
                + 1;
            env.storage().instance().set(&MarketplaceDataKey::ListingNonce, &listing_id);

            let listing = Listing {
                asset_id: input.asset_id,
                seller: seller.clone(),
                price: input.price,
                amount: input.amount,
                active: true,
            };

            env.storage().persistent().set(&MarketplaceDataKey::Listing(listing_id), &listing);

            let listing_count: u64 = env.storage().instance().get(&MarketplaceDataKey::ListingCount(input.asset_id)).unwrap_or(0) + 1;
            env.storage().instance().set(&MarketplaceDataKey::ListingCount(input.asset_id), &listing_count);

            let value = input.price.checked_mul(input.amount).unwrap_or(0);
            total_value = total_value.checked_add(value).expect("total value overflow");
            let existing_total: i128 = env.storage().instance().get(&MarketplaceDataKey::Volume(input.asset_id)).unwrap_or(0);
            env.storage().instance().set(&MarketplaceDataKey::Volume(input.asset_id), &(existing_total + value));

            result_ids.push_back(listing_id);
        }

        Self::append_audit_entry(&env, seller.clone(), Symbol::new(&env, "batch_listing_created"), 0, total_value);

        env.events().publish(
            (Symbol::new(&env, "batch_listing_created"), seller),
            result_ids.clone(),
        );

        result_ids
    }

    /// Batch purchase listings for gas optimization.
    /// Validates all inputs upfront, then executes all purchases atomically.
    /// Panics on any failure to ensure atomicity, rolling back the entire batch.
    /// Maximum 20 purchases per batch.
    pub fn batch_purchase(
        env: Env,
        buyer: Address,
        purchases: Vec<BatchPurchaseInput>,
        emergency_control_id: Address,
    ) -> bool {
        let count = purchases.len();
        assert!(count > 0, "batch must not be empty");
        assert!(count <= 20, "batch size exceeds maximum of 20");

        buyer.require_auth();

        for input in purchases.iter() {
            assert!(input.amount > 0, "amount must be positive");

            let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
            ec_client.require_not_paused(&input.asset_id, &PauseScope::Trading);

            Self::require_whitelisted_if_private(&env, input.asset_id, &buyer);
        }

        let mut total_amount: i128 = 0;
        for input in purchases.iter() {
            if env.storage().instance().has(&BuyBackDataKey::BuyBackConfigKey) {
                let fee = Self::collect_fee(env.clone(), input.amount);
                Self::credit_referral_reward(&env, &buyer, fee);
            }

            if let Some(rep_addr) = env
                .storage()
                .instance()
                .get::<_, Address>(&BuyBackDataKey::ReputationContractKey)
            {
                let admin: Address = env
                    .storage()
                    .instance()
                    .get(&BuyBackDataKey::BuyBackAdminKey)
                    .unwrap_or(buyer.clone());
                let rep_client = ReputationContractClient::new(&env, &rep_addr);
                rep_client.record_trade_completion(&admin, &buyer);
            }

            total_amount = total_amount.checked_add(input.amount).expect("total amount overflow");
        }

        Self::append_audit_entry(&env, buyer.clone(), Symbol::new(&env, "batch_purchase"), 0, total_amount);

        env.events().publish(
            (Symbol::new(&env, "batch_purchase_completed"), buyer),
            purchases.len(),
        );

        true
    }

    pub fn cancel_listing(
        env: Env,
        seller: Address,
        listing_id: u64,
        asset_id: u64,
        emergency_control_id: Address,
    ) -> bool {
        seller.require_auth();

        // Enforce pause check for trading operations
        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Trading);

        if let Some(mut listing) = env.storage().persistent().get::<_, Listing>(&MarketplaceDataKey::Listing(listing_id)) {
            if listing.seller != seller {
                panic!("only listing owner can cancel");
            }
            listing.active = false;
            env.storage().persistent().set(&MarketplaceDataKey::Listing(listing_id), &listing);
        }

        Self::append_audit_entry(&env, seller, Symbol::new(&env, "listing_canceled"), asset_id, 0);

        true
    }

    /// Get listing details
    pub fn get_listing(env: Env, listing_id: u64) -> Option<Listing> {
        env.storage().persistent().get(&MarketplaceDataKey::Listing(listing_id))
    }

    // -----------------------------------------------------------------------
    // Whitelisting – Admin & Privacy Management
    // -----------------------------------------------------------------------

    /// Initialize the marketplace with an admin
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&MarketplaceDataKey::MarketplaceAdmin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&MarketplaceDataKey::MarketplaceAdmin, &admin);
    }

    /// Set an asset as private or public (Admin only)
    pub fn set_asset_privacy(env: Env, admin: Address, asset_id: u64, private: bool) {
        Self::require_admin(&env, &admin);
        env.storage().persistent().set(&MarketplaceDataKey::AssetPrivate(asset_id), &private);
        env.events().publish((Symbol::new(&env, "asset_privacy_updated"), asset_id), private);
    }

    /// Add a user to an asset's whitelist (Admin only)
    pub fn add_to_whitelist(env: Env, admin: Address, asset_id: u64, user: Address) {
        Self::require_admin(&env, &admin);
        env.storage().persistent().set(&MarketplaceDataKey::Whitelisted(asset_id, user.clone()), &true);
        env.events().publish((Symbol::new(&env, "user_whitelisted"), asset_id), user);
    }

    /// Remove a user from an asset's whitelist (Admin only)
    pub fn remove_from_whitelist(env: Env, admin: Address, asset_id: u64, user: Address) {
        Self::require_admin(&env, &admin);
        env.storage().persistent().set(&MarketplaceDataKey::Whitelisted(asset_id, user.clone()), &false);
        env.events().publish((Symbol::new(&env, "user_removed"), asset_id), user);
    }

    /// Bulk add users to an asset's whitelist (Admin only)
    pub fn bulk_add_to_whitelist(env: Env, admin: Address, asset_id: u64, users: Vec<Address>) {
        Self::require_admin(&env, &admin);
        for user in users.iter() {
            env.storage().persistent().set(&MarketplaceDataKey::Whitelisted(asset_id, user.clone()), &true);
        }
        env.events().publish((Symbol::new(&env, "bulk_whitelist"), asset_id), users.len());
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "bulk_whitelist"), asset_id, users.len() as i128);
    }

    /// Check if a user is whitelisted for an asset
    pub fn is_whitelisted(env: Env, asset_id: u64, user: Address) -> bool {
        env.storage().persistent().get(&MarketplaceDataKey::Whitelisted(asset_id, user)).unwrap_or(false)
    }

    /// Check if an asset is private
    pub fn is_private(env: Env, asset_id: u64) -> bool {
        env.storage().persistent().get(&MarketplaceDataKey::AssetPrivate(asset_id)).unwrap_or(false)
    }

    /// Register an asset for isolated per-asset configuration.
    pub fn register_asset(
        env: Env,
        admin: Address,
        asset_id: u64,
        metadata: String,
        governance_required: bool,
        min_trade_amount: i128,
    ) {
        Self::require_admin(&env, &admin);
        if min_trade_amount < 0 {
            panic!("min trade amount must be non-negative");
        }

        if env.storage().persistent().has(&MarketplaceDataKey::AssetConfig(asset_id)) {
            panic!("asset already registered");
        }

        let cfg = AssetConfig {
            asset_id,
            metadata,
            governance_required,
            deprecated: false,
            min_trade_amount,
        };

        env.storage().persistent().set(&MarketplaceDataKey::AssetConfig(asset_id), &cfg);
        Self::append_registered_asset(&env, asset_id);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "asset_registered"), asset_id, 0);
    }

    pub fn update_asset_config(
        env: Env,
        admin: Address,
        asset_id: u64,
        metadata: String,
        governance_required: bool,
        min_trade_amount: i128,
    ) {
        Self::require_admin(&env, &admin);
        let mut cfg: AssetConfig = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::AssetConfig(asset_id))
            .expect("asset not registered");

        cfg.metadata = metadata;
        cfg.governance_required = governance_required;
        cfg.min_trade_amount = min_trade_amount;

        env.storage().persistent().set(&MarketplaceDataKey::AssetConfig(asset_id), &cfg);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "asset_updated"), asset_id, 0);
    }

    pub fn deprecate_asset(env: Env, admin: Address, asset_id: u64, deprecated: bool) {
        Self::require_admin(&env, &admin);
        let mut cfg: AssetConfig = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::AssetConfig(asset_id))
            .expect("asset not registered");
        cfg.deprecated = deprecated;
        env.storage().persistent().set(&MarketplaceDataKey::AssetConfig(asset_id), &cfg);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "asset_deprecated"), asset_id, if deprecated { 1 } else { 0 });
    }

    pub fn migrate_asset(env: Env, admin: Address, old_asset_id: u64, new_asset_id: u64) {
        Self::require_admin(&env, &admin);
        let old_cfg: AssetConfig = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::AssetConfig(old_asset_id))
            .expect("source asset not registered");
        if env.storage().persistent().has(&MarketplaceDataKey::AssetConfig(new_asset_id)) {
            panic!("target asset already registered");
        }

        let new_cfg = AssetConfig {
            asset_id: new_asset_id,
            metadata: old_cfg.metadata,
            governance_required: old_cfg.governance_required,
            deprecated: false,
            min_trade_amount: old_cfg.min_trade_amount,
        };

        env.storage().persistent().set(&MarketplaceDataKey::AssetConfig(new_asset_id), &new_cfg);
        Self::append_registered_asset(&env, new_asset_id);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "asset_migrated"), new_asset_id, old_asset_id as i128);
    }

    pub fn get_asset_config(env: Env, asset_id: u64) -> Option<AssetConfig> {
        env.storage().persistent().get(&MarketplaceDataKey::AssetConfig(asset_id))
    }

    pub fn list_registered_assets(env: Env) -> Vec<u64> {
        env.storage()
            .instance()
            .get(&MarketplaceDataKey::RegisteredAssets)
            .unwrap_or(Vec::new(&env))
    }

    pub fn get_asset_analytics(env: Env, asset_id: u64) -> AssetAnalytics {
        AssetAnalytics {
            asset_id,
            listing_count: env.storage().instance().get(&MarketplaceDataKey::ListingCount(asset_id)).unwrap_or(0),
            transaction_count: env.storage().instance().get(&MarketplaceDataKey::TransactionCount(asset_id)).unwrap_or(0),
            volume: env.storage().instance().get(&MarketplaceDataKey::Volume(asset_id)).unwrap_or(0),
            holders: env.storage().instance().get(&MarketplaceDataKey::HolderCount(asset_id)).unwrap_or(0),
        }
    }

    pub fn configure_transfer_restrictions(
        env: Env,
        admin: Address,
        asset_id: u64,
        whitelist_required: bool,
        min_holding_seconds: u64,
        lockup_seconds: u64,
        max_transfer_amount: i128,
        approval_required: bool,
        emergency_override_enabled: bool,
    ) {
        Self::require_admin(&env, &admin);
        if max_transfer_amount <= 0 {
            panic!("max transfer amount must be positive");
        }

        let cfg = TransferRestrictionConfig {
            whitelist_required,
            min_holding_seconds,
            lockup_seconds,
            max_transfer_amount,
            approval_required,
            emergency_override_enabled,
        };

        env.storage().persistent().set(&MarketplaceDataKey::RestrictionConfig(asset_id), &cfg);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "restrictions_configured"), asset_id, max_transfer_amount);
    }

    pub fn set_compliance_verified(env: Env, admin: Address, asset_id: u64, user: Address, verified: bool) {
        Self::require_admin(&env, &admin);
        env.storage().persistent().set(&MarketplaceDataKey::ComplianceVerified(asset_id, user.clone()), &verified);
        env.events().publish((Symbol::new(&env, "compliance_verified"), asset_id), (user, verified));
    }

    pub fn set_lockup_until(env: Env, admin: Address, asset_id: u64, user: Address, lockup_until: u64) {
        Self::require_admin(&env, &admin);
        env.storage().persistent().set(&MarketplaceDataKey::LockupUntil(asset_id, user), &lockup_until);
    }

    pub fn request_transfer_approval(
        env: Env,
        requester: Address,
        beneficiary: Address,
        asset_id: u64,
        amount: i128,
        reason: String,
    ) -> u64 {
        requester.require_auth();
        if amount <= 0 {
            panic!("amount must be positive");
        }

        let id: u64 = env.storage().instance().get(&MarketplaceDataKey::TransferRequestNonce).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::TransferRequestNonce, &id);

        let req = TransferApprovalRequest {
            id,
            asset_id,
            requester: requester.clone(),
            beneficiary,
            amount,
            reason,
            approved: false,
            reviewed: false,
            created_at: env.ledger().timestamp(),
            reviewed_at: 0,
        };
        env.storage().persistent().set(&MarketplaceDataKey::TransferRequest(id), &req);
        Self::append_audit_entry(&env, requester, Symbol::new(&env, "approval_requested"), asset_id, amount);
        id
    }

    pub fn review_transfer_approval(env: Env, admin: Address, request_id: u64, approved: bool) {
        Self::require_admin(&env, &admin);
        let mut req: TransferApprovalRequest = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::TransferRequest(request_id))
            .expect("request not found");
        req.approved = approved;
        req.reviewed = true;
        req.reviewed_at = env.ledger().timestamp();
        env.storage().persistent().set(&MarketplaceDataKey::TransferRequest(request_id), &req);
        env.storage().persistent().set(&MarketplaceDataKey::TransferApproved(request_id), &approved);
        Self::append_audit_entry(&env, admin, Symbol::new(&env, "approval_reviewed"), req.asset_id, req.amount);
    }

    pub fn get_transfer_approval(env: Env, request_id: u64) -> Option<TransferApprovalRequest> {
        env.storage().persistent().get(&MarketplaceDataKey::TransferRequest(request_id))
    }

    pub fn get_restricted_transfer_count(env: Env, asset_id: u64) -> u64 {
        env.storage().instance().get(&MarketplaceDataKey::RestrictedTransferCount(asset_id)).unwrap_or(0)
    }

    pub fn purchase_with_approval(
        env: Env,
        buyer: Address,
        listing_id: u64,
        amount: i128,
        asset_id: u64,
        request_id: u64,
        emergency_control_id: Address,
    ) -> bool {
        let approved: bool = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::TransferApproved(request_id))
            .unwrap_or(false);
        if !approved {
            panic!("transfer approval required");
        }
        let ok = Self::purchase_internal(
            env.clone(),
            buyer,
            listing_id,
            amount,
            asset_id,
            emergency_control_id,
            Some(request_id),
        );
        if ok {
            env.storage().persistent().remove(&MarketplaceDataKey::TransferApproved(request_id));
        }
        ok
    }

    // Helper: Require admin authorization
    fn require_admin(env: &Env, admin: &Address) {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&MarketplaceDataKey::MarketplaceAdmin).expect("admin not set");
        if admin != &stored_admin {
            panic!("not authorized: admin only");
        }
    }

    // Helper: Require user whitelisted if asset is private
    fn require_whitelisted_if_private(env: &Env, asset_id: u64, user: &Address) {
        if Self::is_private(env.clone(), asset_id) {
            if !Self::is_whitelisted(env.clone(), asset_id, user.clone()) {
                panic!("user not whitelisted for private asset");
            }
        }
    }

    // -----------------------------------------------------------------------
    // Flash-loan protection (Issue #210)
    // -----------------------------------------------------------------------

    /// Configure the marketplace flash-loan guard. Admin only.
    ///
    /// * `max_trades_per_block` – per-buyer purchase cap per ledger (0 = off).
    /// * `max_price_deviation_bps` – max listing-vs-oracle deviation enforced by
    ///   [`Marketplace::purchase_guarded`] (0 = off).
    pub fn configure_flash_loan_guard(
        env: Env,
        admin: Address,
        max_trades_per_block: u32,
        max_price_deviation_bps: u32,
    ) {
        Self::require_admin(&env, &admin);
        let cfg = FlashLoanGuardConfig {
            max_trades_per_block,
            max_price_deviation_bps,
        };
        env.storage()
            .instance()
            .set(&MarketplaceDataKey::FlashLoanGuard, &cfg);
        env.events().publish(
            (Symbol::new(&env, "flash_loan_guard_configured"),),
            (max_trades_per_block, max_price_deviation_bps),
        );
    }

    /// Return the active flash-loan guard configuration (defaults to permissive).
    pub fn get_flash_loan_guard(env: Env) -> FlashLoanGuardConfig {
        Self::flash_loan_guard(&env)
    }

    /// Purchase with both the per-block trade limit and an oracle price-deviation
    /// check applied (Issue #210). Reverts if the listing price strays beyond the
    /// configured deviation from the oracle, blocking price-manipulation exits.
    pub fn purchase_guarded(
        env: Env,
        buyer: Address,
        listing_id: u64,
        amount: i128,
        asset_id: u64,
        emergency_control_id: Address,
    ) -> bool {
        let cfg = Self::flash_loan_guard(&env);
        if cfg.max_price_deviation_bps > 0 {
            let listing = Self::get_listing(env.clone(), listing_id).expect("listing not found");
            assert!(
                Self::validate_listing_price(
                    env.clone(),
                    asset_id,
                    listing.price,
                    cfg.max_price_deviation_bps
                ),
                "price deviates too much from oracle"
            );
        }
        Self::purchase(env, buyer, listing_id, amount, asset_id, emergency_control_id)
    }

    fn flash_loan_guard(env: &Env) -> FlashLoanGuardConfig {
        env.storage()
            .instance()
            .get(&MarketplaceDataKey::FlashLoanGuard)
            .unwrap_or(FlashLoanGuardConfig {
                max_trades_per_block: 0,
                max_price_deviation_bps: 0,
            })
    }

    fn enforce_block_trade_limit(env: &Env, buyer: &Address) {
        let cfg = Self::flash_loan_guard(env);
        if cfg.max_trades_per_block == 0 {
            return;
        }
        let seq = env.ledger().sequence();
        let key = MarketplaceDataKey::BlockTrades(buyer.clone(), seq);
        let count: u32 = env.storage().persistent().get(&key).unwrap_or(0);
        if count >= cfg.max_trades_per_block {
            panic!("per-block trade limit exceeded");
        }
        env.storage().persistent().set(&key, &(count + 1));
    }

    // -----------------------------------------------------------------------
    // Oracle Price Validation
    // -----------------------------------------------------------------------

    /// Set the oracle contract address for price validation.
    pub fn set_marketplace_oracle(env: Env, admin: Address, oracle: Address) {
        Self::require_admin(&env, &admin);
        env.storage().instance().set(&MarketplaceDataKey::Oracle, &oracle);
        env.events().publish((Symbol::new(&env, "oracle_set"),), oracle);
    }

    /// Get the oracle contract address.
    pub fn get_marketplace_oracle(env: Env) -> Option<Address> {
        env.storage().instance().get(&MarketplaceDataKey::Oracle)
    }

    /// Validate a price against the oracle.
    /// Returns `true` if the price is within `max_deviation_bps` of the oracle price.
    pub fn validate_listing_price(env: Env, asset_id: u64, price: i128, max_deviation_bps: u32) -> bool {
        let oracle_addr: Address = env.storage().instance().get(&MarketplaceDataKey::Oracle)
            .expect("oracle not configured");
        let oracle = OracleClient::new(&env, &oracle_addr);
        let oracle_price: AggregatedPrice = oracle.get_price(&asset_id);
        if oracle_price.price == 0 {
            return true;
        }
        let deviation_bps = if price >= oracle_price.price {
            ((price - oracle_price.price) * 10_000 / oracle_price.price) as u32
        } else {
            ((oracle_price.price - price) * 10_000 / oracle_price.price) as u32
        };
        deviation_bps <= max_deviation_bps
    }

    // -----------------------------------------------------------------------
    // Bundle Marketplace Integration
    // -----------------------------------------------------------------------

    /// List a bundle on the marketplace with an optional discount.
    pub fn create_bundle_listing(
        env: Env,
        seller: Address,
        bundle_id: u64,
        bundle_contract: Address,
        price: i128,
        discount_bps: u32,
        emergency_control_id: Address,
    ) -> u64 {
        seller.require_auth();
        assert!(price > 0, "price must be positive");
        assert!(discount_bps <= 10000, "discount must be <= 10000");

        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&bundle_id, &PauseScope::Trading);

        // Verify bundle exists via the bundle contract
        let bundle_client = crate::asset_bundle::AssetBundleContractClient::new(&env, &bundle_contract);
        let bundle = bundle_client.get_bundle(&bundle_id);
        if bundle.is_none() {
            panic!("bundle not found");
        }
        let bundle_data = bundle.unwrap();
        if bundle_data.creator != seller {
            panic!("only bundle creator can list");
        }

        let listing_id: u64 = env
            .storage()
            .instance()
            .get(&MarketplaceDataKey::ListingNonce)
            .unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::ListingNonce, &listing_id);

        let listing = BundleListing {
            listing_id,
            bundle_id,
            bundle_contract,
            seller: seller.clone(),
            price,
            discount_bps,
            active: true,
        };

        env.storage().persistent().set(&MarketplaceDataKey::BundleListing(listing_id), &listing);

        let mut list: Vec<u64> = env
            .storage()
            .instance()
            .get(&MarketplaceDataKey::BundleListings)
            .unwrap_or(Vec::new(&env));
        list.push_back(listing_id);
        env.storage().instance().set(&MarketplaceDataKey::BundleListings, &list);

        Self::append_audit_entry(&env, seller, Symbol::new(&env, "bundle_listed"), bundle_id, price);
        listing_id
    }

    /// Get a bundle listing by ID.
    pub fn get_bundle_listing(env: Env, listing_id: u64) -> Option<BundleListing> {
        env.storage().persistent().get(&MarketplaceDataKey::BundleListing(listing_id))
    }

    /// List all active bundle listing IDs.
    pub fn get_bundle_listings(env: Env) -> Vec<u64> {
        env.storage().instance().get(&MarketplaceDataKey::BundleListings)
            .unwrap_or(Vec::new(&env))
    }

    /// Purchase a bundle listing through the marketplace.
    /// The marketplace collects its fee, then completes the bundle purchase via the bundle contract.
    pub fn buy_bundle_listing(
        env: Env,
        buyer: Address,
        listing_id: u64,
        emergency_control_id: Address,
        token: Address,
    ) -> bool {
        buyer.require_auth();

        let mut listing: BundleListing = env.storage().persistent()
            .get(&MarketplaceDataKey::BundleListing(listing_id))
            .expect("bundle listing not found");
        assert!(listing.active, "listing not active");

        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&listing.bundle_id, &PauseScope::Trading);

        let fee = if env.storage().instance().has(&BuyBackDataKey::BuyBackConfigKey) {
            Self::collect_fee(env.clone(), listing.price)
        } else {
            0
        };

        // Transfer tokens from buyer to seller (minus marketplace fee)
        let token_client = soroban_sdk::token::Client::new(&env, &token);
        let net_price = listing.price - fee;
        token_client.transfer(&buyer, &listing.seller, &net_price);

        // Complete the bundle purchase through the bundle contract
        let bundle_client = crate::asset_bundle::AssetBundleContractClient::new(&env, &listing.bundle_contract);
        bundle_client.buy_bundle(&buyer, &listing.bundle_id, &token);

        // Mark the bundle listing as sold
        listing.active = false;
        env.storage().persistent().set(&MarketplaceDataKey::BundleListing(listing_id), &listing);

        Self::append_audit_entry(&env, buyer.clone(), Symbol::new(&env, "bundle_purchased"), listing.bundle_id, listing.price);
        Self::credit_referral_reward(&env, &buyer, fee);

        true
    }

    /// Cancel an active bundle listing.
    pub fn cancel_bundle_listing(env: Env, seller: Address, listing_id: u64) {
        seller.require_auth();
        let mut listing: BundleListing = env.storage().persistent()
            .get(&MarketplaceDataKey::BundleListing(listing_id))
            .expect("bundle listing not found");
        assert_eq!(listing.seller, seller, "only seller can cancel");
        assert!(listing.active, "listing not active");
        listing.active = false;
        env.storage().persistent().set(&MarketplaceDataKey::BundleListing(listing_id), &listing);
        Self::append_audit_entry(&env, seller, Symbol::new(&env, "bundle_canceled"), listing.bundle_id, 0);
    }

    /// Update the price or discount on an active bundle listing.
    pub fn update_bundle_listing(
        env: Env,
        seller: Address,
        listing_id: u64,
        new_price: i128,
        new_discount_bps: u32,
    ) {
        seller.require_auth();
        assert!(new_price > 0, "price must be positive");
        assert!(new_discount_bps <= 10000, "discount must be <= 10000");

        let mut listing: BundleListing = env.storage().persistent()
            .get(&MarketplaceDataKey::BundleListing(listing_id))
            .expect("bundle listing not found");
        assert_eq!(listing.seller, seller, "only seller can update listing");
        assert!(listing.active, "listing not active");

        listing.price = new_price;
        listing.discount_bps = new_discount_bps;
        env.storage().persistent().set(&MarketplaceDataKey::BundleListing(listing_id), &listing);

        Self::append_audit_entry(&env, seller, Symbol::new(&env, "bundle_listing_updated"), listing.bundle_id, new_price);
    }

    /// Return the effective (post-discount) price for a bundle listing.
    pub fn get_discounted_bundle_price(env: Env, listing_id: u64) -> i128 {
        let listing: BundleListing = env.storage().persistent()
            .get(&MarketplaceDataKey::BundleListing(listing_id))
            .expect("bundle listing not found");
        if listing.discount_bps == 0 {
            return listing.price;
        }
        let discount = listing.price
            .checked_mul(listing.discount_bps as i128)
            .expect("discount overflow")
            / 10_000;
        listing.price - discount
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – Initialization
    // -----------------------------------------------------------------------

    /// Initialize the buy-back and burn system.
    ///
    /// # Arguments
    /// * `admin` – Admin address authorized for buy-back operations.
    /// * `burn_cap` – Maximum tokens that can be burned per operation.
    /// * `auto_threshold` – Treasury balance threshold to trigger auto buy-back.
    /// * `auto_buyback_amount` – Amount to buy-back when auto-triggered.
    /// * `fee_bps` – Marketplace fee in basis points (e.g., 30 = 0.30%).
    /// * `require_governance` – Whether governance approval is required for burns.
    pub fn initialize_buyback(
        env: Env,
        admin: Address,
        burn_cap: i128,
        auto_threshold: i128,
        auto_buyback_amount: i128,
        fee_bps: u32,
        require_governance: bool,
    ) {
        admin.require_auth();

        if env.storage().instance().has(&BuyBackDataKey::BuyBackAdminKey) {
            panic!("buy-back system already initialized");
        }

        if burn_cap <= 0 {
            panic!("burn cap must be positive");
        }
        if auto_threshold < 0 {
            panic!("auto threshold must be non-negative");
        }
        if auto_buyback_amount < 0 {
            panic!("auto buyback amount must be non-negative");
        }
        if fee_bps > 10_000 {
            panic!("fee basis points must not exceed 10000");
        }

        env.storage().instance().set(&BuyBackDataKey::BuyBackAdminKey, &admin);

        let config = BuyBackConfig {
            burn_cap,
            auto_threshold,
            auto_buyback_amount,
            require_governance,
            fee_bps,
            paused: false,
        };

        env.storage()
            .instance()
            .set(&BuyBackDataKey::BuyBackConfigKey, &config);
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TreasuryBalanceKey, &0i128);
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TotalBurnedKey, &0i128);

        let history: Vec<BuyBackRecord> = Vec::new(&env);
        env.storage()
            .persistent()
            .set(&BuyBackDataKey::HistoryKey, &history);

        env.events().publish(
            (Symbol::new(&env, "buyback_initialized"),),
            (admin, burn_cap, fee_bps),
        );
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – Treasury Management
    // -----------------------------------------------------------------------

    /// Deposit funds into the buy-back treasury.
    /// Typically called when marketplace fees are collected.
    ///
    /// # Arguments
    /// * `depositor` – The address depositing funds.
    /// * `amount` – Amount to deposit into the treasury.
    pub fn deposit_to_treasury(env: Env, depositor: Address, amount: i128) {
        depositor.require_auth();

        if amount <= 0 {
            panic!("deposit amount must be positive");
        }

        Self::require_buyback_initialized(&env);

        let current: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TreasuryBalanceKey)
            .unwrap_or(0);

        let new_balance = current
            .checked_add(amount)
            .expect("treasury balance overflow");

        env.storage()
            .instance()
            .set(&BuyBackDataKey::TreasuryBalanceKey, &new_balance);

        env.events()
            .publish((Symbol::new(&env, "treasury_deposit"), depositor), amount);
    }

    /// Collect a fee from a trade amount and deposit it into the treasury.
    /// Called internally during marketplace `purchase` operations.
    ///
    /// # Arguments
    /// * `trade_amount` – The total trade amount to calculate fee from.
    ///
    /// # Returns
    /// The fee amount collected.
    pub fn collect_fee(env: Env, trade_amount: i128) -> i128 {
        Self::require_buyback_initialized(&env);

        let config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        let fee = trade_amount
            .checked_mul(config.fee_bps as i128)
            .expect("fee calculation overflow")
            .checked_div(10_000)
            .expect("fee calculation div error");

        if fee > 0 {
            let current: i128 = env
                .storage()
                .instance()
                .get(&BuyBackDataKey::TreasuryBalanceKey)
                .unwrap_or(0);

            let new_balance = current.checked_add(fee).expect("treasury balance overflow");

            env.storage()
                .instance()
                .set(&BuyBackDataKey::TreasuryBalanceKey, &new_balance);

            env.events()
                .publish((Symbol::new(&env, "fee_collected"),), (trade_amount, fee));
        }

        fee
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – Core Operations
    // -----------------------------------------------------------------------

    /// Buy back tokens using treasury funds.
    ///
    /// Uses accumulated treasury funds to buy back tokens from the market.
    /// In production, this would integrate with Stellar's path payments / DEX.
    /// Currently simulates the purchase and removes tokens from circulation.
    ///
    /// # Arguments
    /// * `admin` – Must be the buy-back admin.
    /// * `amount` – Amount of tokens to buy back.
    /// * `source_funds` – Treasury funds to use for the buy-back.
    /// * `governance_id` – Optional governance contract for approval check.
    pub fn buy_back_tokens(
        env: Env,
        admin: Address,
        amount: i128,
        source_funds: i128,
        governance_id: Option<Address>,
    ) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);
        Self::require_buyback_not_paused(&env);

        if amount <= 0 {
            panic!("buy-back amount must be positive");
        }
        if source_funds <= 0 {
            panic!("source funds must be positive");
        }

        let config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        // Enforce burn cap
        if amount > config.burn_cap {
            panic!("amount exceeds burn cap");
        }

        // Check treasury has sufficient funds
        let treasury: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TreasuryBalanceKey)
            .unwrap_or(0);

        if treasury < source_funds {
            panic!("insufficient treasury funds");
        }

        // Enforce governance approval if required
        if config.require_governance {
            if let Some(gov_addr) = governance_id {
                let gov_client = GovernanceClient::new(&env, &gov_addr);
                // Use asset_id 0 as a sentinel for buy-back governance proposals
                gov_client.require_approved(&0);
            } else {
                panic!("governance approval required but no governance contract provided");
            }
        }

        // Deduct from treasury
        let new_treasury = treasury
            .checked_sub(source_funds)
            .expect("treasury underflow");
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TreasuryBalanceKey, &new_treasury);

        // Update total burned
        let total_burned: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TotalBurnedKey)
            .unwrap_or(0);
        let new_total_burned = total_burned
            .checked_add(amount)
            .expect("total burned overflow");
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TotalBurnedKey, &new_total_burned);

        // Record in history
        let record = BuyBackRecord {
            amount,
            burned: amount,
            source_funds,
            timestamp: env.ledger().timestamp(),
            executor: admin.clone(),
            auto_triggered: false,
        };
        Self::append_buyback_history(&env, record);

        // Emit buy-back event
        env.events().publish(
            (Symbol::new(&env, "buy_back_executed"), admin),
            (amount, source_funds),
        );

        // Emit burn event
        env.events().publish(
            (Symbol::new(&env, "tokens_burned"),),
            (amount, new_total_burned),
        );
    }

    /// Burn tokens directly from the treasury allocation.
    ///
    /// Reduces total supply by removing tokens permanently.
    /// Subject to burn cap and optional governance approval.
    ///
    /// # Arguments
    /// * `admin` – Must be the buy-back admin.
    /// * `amount` – Amount of tokens to burn.
    /// * `governance_id` – Optional governance contract for approval check.
    pub fn burn_tokens(env: Env, admin: Address, amount: i128, governance_id: Option<Address>) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);
        Self::require_buyback_not_paused(&env);

        if amount <= 0 {
            panic!("burn amount must be positive");
        }

        let config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        // Enforce burn cap
        if amount > config.burn_cap {
            panic!("burn amount exceeds burn cap");
        }

        // Enforce governance approval if required
        if config.require_governance {
            if let Some(gov_addr) = governance_id {
                let gov_client = GovernanceClient::new(&env, &gov_addr);
                gov_client.require_approved(&0);
            } else {
                panic!("governance approval required but no governance contract provided");
            }
        }

        // Update total burned
        let total_burned: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TotalBurnedKey)
            .unwrap_or(0);
        let new_total_burned = total_burned
            .checked_add(amount)
            .expect("total burned overflow");
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TotalBurnedKey, &new_total_burned);

        // Record in history
        let record = BuyBackRecord {
            amount: 0,
            burned: amount,
            source_funds: 0,
            timestamp: env.ledger().timestamp(),
            executor: admin.clone(),
            auto_triggered: false,
        };
        Self::append_buyback_history(&env, record);

        // Emit burn event
        env.events().publish(
            (Symbol::new(&env, "tokens_burned"),),
            (amount, new_total_burned),
        );
    }

    /// Auto buy-back triggered when treasury balance exceeds the threshold.
    ///
    /// Anyone can call this function, but it only executes if:
    /// - The treasury balance exceeds `auto_threshold`
    /// - The buy-back system is not paused
    /// - The auto-buyback amount does not exceed the burn cap
    pub fn auto_buy_back(env: Env) {
        Self::require_buyback_initialized(&env);
        Self::require_buyback_not_paused(&env);

        let config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        if config.auto_threshold <= 0 || config.auto_buyback_amount <= 0 {
            panic!("auto buy-back not configured");
        }

        let treasury: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TreasuryBalanceKey)
            .unwrap_or(0);

        if treasury < config.auto_threshold {
            panic!("treasury below auto threshold");
        }

        // Enforce burn cap on auto amount
        let buyback_amount = if config.auto_buyback_amount > config.burn_cap {
            config.burn_cap
        } else {
            config.auto_buyback_amount
        };

        // Deduct from treasury (use buyback_amount as proxy for cost)
        let new_treasury = treasury
            .checked_sub(buyback_amount)
            .expect("treasury underflow");
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TreasuryBalanceKey, &new_treasury);

        // Update total burned
        let total_burned: i128 = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::TotalBurnedKey)
            .unwrap_or(0);
        let new_total_burned = total_burned
            .checked_add(buyback_amount)
            .expect("total burned overflow");
        env.storage()
            .instance()
            .set(&BuyBackDataKey::TotalBurnedKey, &new_total_burned);

        // Record in history
        let admin: Address = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackAdminKey)
            .expect("admin not set");

        let record = BuyBackRecord {
            amount: buyback_amount,
            burned: buyback_amount,
            source_funds: buyback_amount,
            timestamp: env.ledger().timestamp(),
            executor: admin,
            auto_triggered: true,
        };
        Self::append_buyback_history(&env, record);

        // Emit events
        env.events().publish(
            (Symbol::new(&env, "buy_back_executed"),),
            (buyback_amount, true), // (amount, auto_triggered)
        );

        env.events().publish(
            (Symbol::new(&env, "tokens_burned"),),
            (buyback_amount, new_total_burned),
        );
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – BBConfiguration Management
    // -----------------------------------------------------------------------

    /// Update the buy-back configuration. BBAdmin only.
    ///
    /// # Arguments
    /// * `admin` – Must be the buy-back admin.
    /// * `burn_cap` – New maximum burn per operation.
    /// * `auto_threshold` – New auto-trigger threshold.
    /// * `auto_buyback_amount` – New auto buy-back amount.
    /// * `fee_bps` – New fee in basis points.
    /// * `require_governance` – Whether governance is required.
    pub fn update_buyback_config(
        env: Env,
        admin: Address,
        burn_cap: i128,
        auto_threshold: i128,
        auto_buyback_amount: i128,
        fee_bps: u32,
        require_governance: bool,
    ) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);

        if burn_cap <= 0 {
            panic!("burn cap must be positive");
        }
        if fee_bps > 10_000 {
            panic!("fee basis points must not exceed 10000");
        }

        let config = BuyBackConfig {
            burn_cap,
            auto_threshold,
            auto_buyback_amount,
            require_governance,
            fee_bps,
            paused: false,
        };

        env.storage()
            .instance()
            .set(&BuyBackDataKey::BuyBackConfigKey, &config);

        env.events().publish(
            (Symbol::new(&env, "buyback_config_updated"), admin),
            (burn_cap, auto_threshold, fee_bps),
        );
    }

    /// Set the burn cap. BBAdmin only.
    /// Provides a dedicated function for governance to adjust the burn cap.
    pub fn set_burn_cap(env: Env, admin: Address, new_cap: i128) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);

        if new_cap <= 0 {
            panic!("burn cap must be positive");
        }

        let mut config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        config.burn_cap = new_cap;

        env.storage()
            .instance()
            .set(&BuyBackDataKey::BuyBackConfigKey, &config);

        env.events()
            .publish((Symbol::new(&env, "burn_cap_updated"),), new_cap);
    }

    /// Set or update the reputation contract address. BBAdmin only.
    pub fn set_reputation_contract(env: Env, admin: Address, reputation_contract: Address) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);
        env.storage()
            .instance()
            .set(&BuyBackDataKey::ReputationContractKey, &reputation_contract);
    }


    /// Pause or unpause the buy-back system. BBAdmin only.
    pub fn set_buyback_paused(env: Env, admin: Address, paused: bool) {
        admin.require_auth();
        Self::require_buyback_admin(&env, &admin);

        let mut config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");

        config.paused = paused;

        env.storage()
            .instance()
            .set(&BuyBackDataKey::BuyBackConfigKey, &config);

        env.events()
            .publish((Symbol::new(&env, "buyback_paused"),), paused);
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – Query Functions
    // -----------------------------------------------------------------------

    /// Get the current treasury balance.
    pub fn get_treasury_balance(env: Env) -> i128 {
        env.storage()
            .instance()
            .get(&BuyBackDataKey::TreasuryBalanceKey)
            .unwrap_or(0)
    }

    /// Get the total tokens burned across all operations.
    pub fn get_total_burned(env: Env) -> i128 {
        env.storage()
            .instance()
            .get(&BuyBackDataKey::TotalBurnedKey)
            .unwrap_or(0)
    }

    /// Get the current buy-back configuration.
    pub fn get_buyback_config(env: Env) -> Option<BuyBackConfig> {
        env.storage().instance().get(&BuyBackDataKey::BuyBackConfigKey)
    }

    /// Get the buy-back operation history.
    pub fn get_buyback_history(env: Env) -> Vec<BuyBackRecord> {
        env.storage()
            .persistent()
            .get(&BuyBackDataKey::HistoryKey)
            .unwrap_or(Vec::new(&env))
    }

    /// Check whether the auto buy-back threshold has been reached.
    pub fn is_auto_buyback_ready(env: Env) -> bool {
        let config: Option<BuyBackConfig> = env.storage().instance().get(&BuyBackDataKey::BuyBackConfigKey);

        if let Some(c) = config {
            if c.paused || c.auto_threshold <= 0 {
                return false;
            }
            let treasury: i128 = env
                .storage()
                .instance()
                .get(&BuyBackDataKey::TreasuryBalanceKey)
                .unwrap_or(0);
            treasury >= c.auto_threshold
        } else {
            false
        }
    }

    // -----------------------------------------------------------------------
    // Buy-Back & Burn System – Internal Helpers
    // -----------------------------------------------------------------------

    /// Verify that the buy-back system has been initialized.
    fn require_buyback_initialized(env: &Env) {
        if !env.storage().instance().has(&BuyBackDataKey::BuyBackAdminKey) {
            panic!("buy-back system not initialized");
        }
    }

    /// Verify the caller is the buy-back admin.
    fn require_buyback_admin(env: &Env, caller: &Address) {
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackAdminKey)
            .expect("buy-back system not initialized");
        if *caller != stored_admin {
            panic!("caller is not buy-back admin");
        }
    }

    /// Verify the buy-back system is not paused.
    fn require_buyback_not_paused(env: &Env) {
        let config: BuyBackConfig = env
            .storage()
            .instance()
            .get(&BuyBackDataKey::BuyBackConfigKey)
            .expect("buy-back not configured");
        if config.paused {
            panic!("buy-back system is paused");
        }
    }

    /// Append a BuyBackRecord to the history.
    fn append_buyback_history(env: &Env, record: BuyBackRecord) {
        let mut history: Vec<BuyBackRecord> = env
            .storage()
            .persistent()
            .get(&BuyBackDataKey::HistoryKey)
            .unwrap_or(Vec::new(env));
        history.push_back(record);
        env.storage()
            .persistent()
            .set(&BuyBackDataKey::HistoryKey, &history);
    }

    // -----------------------------------------------------------------------
    // Referral System Operations
    // -----------------------------------------------------------------------

    /// Initialize the referral reward system.
    pub fn initialize_referral(
        env: Env,
        admin: Address,
        treasury: Address,
        reward_bps: u32,
        min_activity: i128,
    ) {
        Self::require_admin(&env, &admin);

        if env.storage().instance().has(&ReferralDataKey::ReferralConfigKey) {
            panic!("referral system already initialized");
        }

        if reward_bps > 10_000 {
            panic!("reward basis points must not exceed 10000");
        }

        let config = ReferralConfig {
            reward_bps,
            treasury,
            min_activity,
        };

        env.storage().instance().set(&ReferralDataKey::ReferralConfigKey, &config);
        env.events().publish((Symbol::new(&env, "referral_initialized"),), (reward_bps, min_activity));
    }

    /// Link a new user to a referrer.
    pub fn refer_user(env: Env, referred: Address, referrer: Address) {
        referred.require_auth();

        if referred == referrer {
            panic!("self-referral not allowed");
        }

        if env.storage().persistent().has(&ReferralDataKey::ReferrerKey(referred.clone())) {
            panic!("user already referred");
        }

        env.storage().persistent().set(&ReferralDataKey::ReferrerKey(referred.clone()), &referrer);
        
        let count: u32 = env.storage().persistent().get(&ReferralDataKey::ReferralCountKey(referrer.clone())).unwrap_or(0);
        env.storage().persistent().set(&ReferralDataKey::ReferralCountKey(referrer.clone()), &(count + 1));

        env.events().publish((Symbol::new(&env, "referral_set"), referred), referrer);
    }

    /// Claim accumulated referral rewards.
    pub fn claim_referral_reward(env: Env, referrer: Address) {
        referrer.require_auth();

        let reward: i128 = env.storage().persistent().get(&ReferralDataKey::RewardBalanceKey(referrer.clone())).unwrap_or(0);
        if reward <= 0 {
            panic!("no reward to claim");
        }

        let _config: ReferralConfig = env.storage().instance().get(&ReferralDataKey::ReferralConfigKey).expect("referral system not initialized");

        // reset balance before transfer for security
        env.storage().persistent().set(&ReferralDataKey::RewardBalanceKey(referrer.clone()), &0i128);

        // In a real implementation, we would use a token client to transfer reward from config.treasury to referrer.
        // For this phase, we emit the event and simulate the state change.
        env.events().publish((Symbol::new(&env, "reward_claimed"), referrer), reward);
    }

    /// Query referral status for a user.
    pub fn get_referral_info(env: Env, user: Address) -> (Option<Address>, i128, u32) {
        let referrer: Option<Address> = env.storage().persistent().get(&ReferralDataKey::ReferrerKey(user.clone()));
        let reward: i128 = env.storage().persistent().get(&ReferralDataKey::RewardBalanceKey(user.clone())).unwrap_or(0);
        let count: u32 = env.storage().persistent().get(&ReferralDataKey::ReferralCountKey(user.clone())).unwrap_or(0);
        (referrer, reward, count)
    }

    // Helper to credit referral rewards
    fn credit_referral_reward(env: &Env, referred: &Address, fee_amount: i128) {
        if let Some(config) = env.storage().instance().get::<_, ReferralConfig>(&ReferralDataKey::ReferralConfigKey) {
            if let Some(referrer) = env.storage().persistent().get::<_, Address>(&ReferralDataKey::ReferrerKey(referred.clone())) {
                let reward = fee_amount
                    .checked_mul(config.reward_bps as i128)
                    .expect("reward calculation overflow")
                    / 10_000;

                if reward > 0 {
                    let current: i128 = env.storage().persistent().get(&ReferralDataKey::RewardBalanceKey(referrer.clone())).unwrap_or(0);
                    env.storage().persistent().set(&ReferralDataKey::RewardBalanceKey(referrer.clone()), &(current + reward));
                    env.events().publish((Symbol::new(env, "referral_reward_credited"), referrer), reward);
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // Compliance Reporting Queries
    // -----------------------------------------------------------------------
    /// Get total tokenized value
    pub fn get_total_tokenized_value(env: Env, asset_id: Option<u64>, time_range: Option<TimeRange>) -> Result<i128, ComplianceError> {
        Self::validate_time_range(&time_range)?;
        Ok(Self::aggregate_metric_i128(&env, asset_id, |id| {
            env.storage().instance().get(&MarketplaceDataKey::Volume(id)).unwrap_or(0)
        }))
    }

    /// Get transaction volume
    pub fn get_transaction_volume(env: Env, asset_id: Option<u64>, time_range: Option<TimeRange>) -> Result<i128, ComplianceError> {
        Self::validate_time_range(&time_range)?;
        Ok(Self::aggregate_metric_i128(&env, asset_id, |id| {
            env.storage().instance().get(&MarketplaceDataKey::Volume(id)).unwrap_or(0)
        }))
    }

    /// Get unique holder count
    pub fn get_holder_count(env: Env, asset_id: Option<u64>, time_range: Option<TimeRange>) -> Result<u32, ComplianceError> {
        Self::validate_time_range(&time_range)?;
        Ok(Self::aggregate_metric_u32(&env, asset_id, |id| {
            env.storage().instance().get(&MarketplaceDataKey::HolderCount(id)).unwrap_or(0)
        }))
    }

    /// Get taxable volume for tax-focused reporting.
    pub fn get_taxable_volume(env: Env, asset_id: Option<u64>, time_range: Option<TimeRange>) -> Result<i128, ComplianceError> {
        Self::validate_time_range(&time_range)?;
        Ok(Self::aggregate_metric_i128(&env, asset_id, |id| {
            env.storage().instance().get(&MarketplaceDataKey::TaxableVolume(id)).unwrap_or(0)
        }))
    }

    /// Run regulatory checks and return pass/fail counters.
    /// Tuple format: (checked_assets, failed_assets, restricted_transfers)
    pub fn run_regulatory_checks(env: Env) -> (u32, u32, u64) {
        let assets = Self::list_registered_assets(env.clone());
        let mut checked: u32 = 0;
        let mut failed: u32 = 0;
        let mut restricted: u64 = 0;

        for id in assets.iter() {
            checked += 1;
            let cfg = env.storage().persistent().get::<_, TransferRestrictionConfig>(&MarketplaceDataKey::RestrictionConfig(id));
            let failures: u64 = env.storage().instance().get(&MarketplaceDataKey::ComplianceFailures(id)).unwrap_or(0);
            restricted += env.storage().instance().get::<_, u64>(&MarketplaceDataKey::RestrictedTransferCount(id)).unwrap_or(0);

            if failures > 0 {
                failed += 1;
                continue;
            }

            if let Some(c) = cfg {
                if c.max_transfer_amount <= 0 {
                    failed += 1;
                }
            }
        }

        (checked, failed, restricted)
    }

    /// Generate and persist a regulatory report snapshot.
    pub fn generate_regulatory_report(
        env: Env,
        admin: Address,
        asset_id: Option<u64>,
        time_range: Option<TimeRange>,
        format: ReportFormat,
    ) -> Result<u64, ComplianceError> {
        Self::require_admin_for_result(&env, &admin)?;
        Self::build_and_store_report(&env, asset_id, time_range, format)
    }

    /// Export a generated report in the selected format.
    pub fn export_report(env: Env, report_id: u64) -> ReportExport {
        let report: RegulatoryReport = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::GeneratedReport(report_id))
            .expect("report not found");

        match report.format {
            ReportFormat::Json => ReportExport::Json(report),
            ReportFormat::Csv => ReportExport::Csv(report),
        }
    }

    /// Schedule a recurring regulatory report.
    pub fn schedule_report(
        env: Env,
        admin: Address,
        asset_id: Option<u64>,
        cadence_seconds: u64,
        format: ReportFormat,
    ) -> Result<u64, ComplianceError> {
        Self::require_admin_for_result(&env, &admin)?;
        if cadence_seconds == 0 {
            panic!("cadence must be positive");
        }

        let id: u64 = env.storage().instance().get(&MarketplaceDataKey::ReportScheduleNonce).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::ReportScheduleNonce, &id);

        let schedule = ReportSchedule {
            id,
            asset_id,
            cadence_seconds,
            next_run_at: env.ledger().timestamp() + cadence_seconds,
            format,
            enabled: true,
        };
        env.storage().persistent().set(&MarketplaceDataKey::ReportSchedule(id), &schedule);
        Ok(id)
    }

    /// Execute all due schedules and emit reports.
    pub fn run_due_reports(env: Env, admin: Address) -> Result<u32, ComplianceError> {
        Self::require_admin_for_result(&env, &admin)?;

        let now = env.ledger().timestamp();
        let max_id: u64 = env.storage().instance().get(&MarketplaceDataKey::ReportScheduleNonce).unwrap_or(0);
        let mut generated = 0;

        for i in 1..=max_id {
            if let Some(mut schedule) = env.storage().persistent().get::<_, ReportSchedule>(&MarketplaceDataKey::ReportSchedule(i)) {
                if !schedule.enabled || schedule.next_run_at > now {
                    continue;
                }

                let _ = Self::build_and_store_report(
                    &env,
                    schedule.asset_id,
                    Some(TimeRange {
                        start_timestamp: now.saturating_sub(schedule.cadence_seconds),
                        end_timestamp: now,
                    }),
                    schedule.format.clone(),
                )?;

                schedule.next_run_at = now + schedule.cadence_seconds;
                env.storage().persistent().set(&MarketplaceDataKey::ReportSchedule(i), &schedule);
                generated += 1;
            }
        }

        Ok(generated)
    }

    /// Returns historical compliance snapshots for an asset.
    pub fn get_historical_compliance(env: Env, asset_id: u64, time_range: Option<TimeRange>) -> Result<Vec<AssetComplianceSnapshot>, ComplianceError> {
        Self::validate_time_range(&time_range)?;
        let history: Vec<AssetComplianceSnapshot> = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::ComplianceHistory(asset_id))
            .unwrap_or(Vec::new(&env));

        if time_range.is_none() {
            return Ok(history);
        }

        let range = time_range.expect("checked");
        let mut filtered = Vec::new(&env);
        for snap in history.iter() {
            if snap.timestamp >= range.start_timestamp && snap.timestamp <= range.end_timestamp {
                filtered.push_back(snap);
            }
        }
        Ok(filtered)
    }

    /// Real-time compliance snapshot for a single asset.
    pub fn get_real_time_compliance(env: Env, asset_id: u64) -> AssetComplianceSnapshot {
        Self::build_snapshot(&env, asset_id)
    }

    fn register_asset_if_missing(env: &Env, asset_id: u64) {
        if env.storage().persistent().has(&MarketplaceDataKey::AssetConfig(asset_id)) {
            return;
        }
        let cfg = AssetConfig {
            asset_id,
            metadata: String::from_str(env, "auto-registered"),
            governance_required: false,
            deprecated: false,
            min_trade_amount: 0,
        };
        env.storage().persistent().set(&MarketplaceDataKey::AssetConfig(asset_id), &cfg);
        Self::append_registered_asset(env, asset_id);
    }

    fn append_registered_asset(env: &Env, asset_id: u64) {
        let mut assets: Vec<u64> = env
            .storage()
            .instance()
            .get(&MarketplaceDataKey::RegisteredAssets)
            .unwrap_or(Vec::new(env));
        for existing in assets.iter() {
            if existing == asset_id {
                return;
            }
        }
        assets.push_back(asset_id);
        env.storage().instance().set(&MarketplaceDataKey::RegisteredAssets, &assets);
    }

    fn append_audit_entry(env: &Env, actor: Address, action: Symbol, asset_id: u64, amount: i128) {
        let mut entries: Vec<AuditTrailEntry> = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::AuditTrail)
            .unwrap_or(Vec::new(env));
        entries.push_back(AuditTrailEntry {
            timestamp: env.ledger().timestamp(),
            actor,
            action,
            asset_id,
            amount,
        });
        env.storage().persistent().set(&MarketplaceDataKey::AuditTrail, &entries);
    }

    fn enforce_transfer_restrictions(
        env: &Env,
        user: &Address,
        asset_id: u64,
        amount: i128,
        approval_id: Option<u64>,
    ) {
        let cfg = env
            .storage()
            .persistent()
            .get::<_, TransferRestrictionConfig>(&MarketplaceDataKey::RestrictionConfig(asset_id));
        if cfg.is_none() {
            Self::track_holder(env, asset_id, user);
            return;
        }
        let cfg = cfg.expect("restriction config exists");

        if amount > cfg.max_transfer_amount {
            Self::increment_failure(env, asset_id);
            panic!("transfer exceeds limit");
        }

        if cfg.whitelist_required && !Self::is_whitelisted(env.clone(), asset_id, user.clone()) {
            Self::increment_failure(env, asset_id);
            panic!("user not whitelisted for restricted transfer");
        }

        let verified: bool = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::ComplianceVerified(asset_id, user.clone()))
            .unwrap_or(false);
        if !verified {
            Self::increment_failure(env, asset_id);
            panic!("compliance verification required");
        }

        let now = env.ledger().timestamp();
        let lockup_until: u64 = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::LockupUntil(asset_id, user.clone()))
            .unwrap_or(0);
        if lockup_until > now {
            Self::increment_failure(env, asset_id);
            panic!("lock-up period active");
        }

        let first_held: u64 = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::FirstHeldAt(asset_id, user.clone()))
            .unwrap_or(now);
        if now < first_held + cfg.min_holding_seconds {
            Self::increment_failure(env, asset_id);
            panic!("minimum holding period not met");
        }

        if cfg.approval_required {
            if approval_id.is_none() {
                Self::increment_failure(env, asset_id);
                panic!("transfer approval required");
            }
            let approved: bool = env
                .storage()
                .persistent()
                .get(&MarketplaceDataKey::TransferApproved(approval_id.expect("checked")))
                .unwrap_or(false);
            if !approved {
                Self::increment_failure(env, asset_id);
                panic!("transfer approval required");
            }
        }

        let day_bucket = now / 86_400;
        let transferred_today: i128 = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::DailyTransfer(asset_id, user.clone(), day_bucket))
            .unwrap_or(0);
        let new_transferred = transferred_today + amount;
        env.storage().persistent().set(
            &MarketplaceDataKey::DailyTransfer(asset_id, user.clone(), day_bucket),
            &new_transferred,
        );

        let restricted_count: u64 = env.storage().instance().get(&MarketplaceDataKey::RestrictedTransferCount(asset_id)).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::RestrictedTransferCount(asset_id), &restricted_count);
        Self::track_holder(env, asset_id, user);
    }

    fn purchase_internal(
        env: Env,
        buyer: Address,
        _listing_id: u64,
        amount: i128,
        asset_id: u64,
        emergency_control_id: Address,
        approval_id: Option<u64>,
    ) -> bool {
        buyer.require_auth();

        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Trading);

        Self::require_whitelisted_if_private(&env, asset_id, &buyer);
        Self::enforce_transfer_restrictions(&env, &buyer, asset_id, amount, approval_id);
        Self::record_trade_metrics(&env, asset_id, &buyer, amount);

        if env.storage().instance().has(&BuyBackDataKey::BuyBackConfigKey) {
            let fee = Self::collect_fee(env.clone(), amount);
            Self::credit_referral_reward(&env, &buyer, fee);
        }

        Self::append_audit_entry(&env, buyer, Symbol::new(&env, "purchase"), asset_id, amount);
        true
    }

    fn track_holder(env: &Env, asset_id: u64, holder: &Address) {
        if !env
            .storage()
            .persistent()
            .get::<_, bool>(&MarketplaceDataKey::HolderSeen(asset_id, holder.clone()))
            .unwrap_or(false)
        {
            env.storage()
                .persistent()
                .set(&MarketplaceDataKey::HolderSeen(asset_id, holder.clone()), &true);
            let holders: u32 = env.storage().instance().get(&MarketplaceDataKey::HolderCount(asset_id)).unwrap_or(0) + 1;
            env.storage().instance().set(&MarketplaceDataKey::HolderCount(asset_id), &holders);
            env.storage()
                .persistent()
                .set(&MarketplaceDataKey::FirstHeldAt(asset_id, holder.clone()), &env.ledger().timestamp());
        }
    }

    fn record_trade_metrics(env: &Env, asset_id: u64, buyer: &Address, amount: i128) {
        let tx_count: u64 = env.storage().instance().get(&MarketplaceDataKey::TransactionCount(asset_id)).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::TransactionCount(asset_id), &tx_count);

        let volume: i128 = env.storage().instance().get(&MarketplaceDataKey::Volume(asset_id)).unwrap_or(0) + amount;
        env.storage().instance().set(&MarketplaceDataKey::Volume(asset_id), &volume);

        let taxable_volume: i128 = env.storage().instance().get(&MarketplaceDataKey::TaxableVolume(asset_id)).unwrap_or(0) + amount;
        env.storage().instance().set(&MarketplaceDataKey::TaxableVolume(asset_id), &taxable_volume);

        Self::track_holder(env, asset_id, buyer);
        Self::snapshot_compliance(env, asset_id);
    }

    fn build_snapshot(env: &Env, asset_id: u64) -> AssetComplianceSnapshot {
        AssetComplianceSnapshot {
            asset_id,
            timestamp: env.ledger().timestamp(),
            transaction_count: env.storage().instance().get(&MarketplaceDataKey::TransactionCount(asset_id)).unwrap_or(0),
            volume: env.storage().instance().get(&MarketplaceDataKey::Volume(asset_id)).unwrap_or(0),
            taxable_volume: env.storage().instance().get(&MarketplaceDataKey::TaxableVolume(asset_id)).unwrap_or(0),
            holders: env.storage().instance().get(&MarketplaceDataKey::HolderCount(asset_id)).unwrap_or(0),
            restricted_transfer_count: env.storage().instance().get(&MarketplaceDataKey::RestrictedTransferCount(asset_id)).unwrap_or(0),
            compliance_failures: env.storage().instance().get(&MarketplaceDataKey::ComplianceFailures(asset_id)).unwrap_or(0),
        }
    }

    fn snapshot_compliance(env: &Env, asset_id: u64) {
        let snapshot = Self::build_snapshot(env, asset_id);
        let mut history: Vec<AssetComplianceSnapshot> = env
            .storage()
            .persistent()
            .get(&MarketplaceDataKey::ComplianceHistory(asset_id))
            .unwrap_or(Vec::new(env));
        history.push_back(snapshot);
        env.storage().persistent().set(&MarketplaceDataKey::ComplianceHistory(asset_id), &history);
    }

    fn increment_failure(env: &Env, asset_id: u64) {
        let failures: u64 = env.storage().instance().get(&MarketplaceDataKey::ComplianceFailures(asset_id)).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::ComplianceFailures(asset_id), &failures);
    }

    fn aggregate_metric_i128<F>(env: &Env, asset_id: Option<u64>, mut read: F) -> i128
    where
        F: FnMut(u64) -> i128,
    {
        if let Some(id) = asset_id {
            return read(id);
        }
        let assets: Vec<u64> = env.storage().instance().get(&MarketplaceDataKey::RegisteredAssets).unwrap_or(Vec::new(env));
        let mut total: i128 = 0;
        for id in assets.iter() {
            total += read(id);
        }
        total
    }

    fn aggregate_metric_u32<F>(env: &Env, asset_id: Option<u64>, mut read: F) -> u32
    where
        F: FnMut(u64) -> u32,
    {
        if let Some(id) = asset_id {
            return read(id);
        }
        let assets: Vec<u64> = env.storage().instance().get(&MarketplaceDataKey::RegisteredAssets).unwrap_or(Vec::new(env));
        let mut total: u32 = 0;
        for id in assets.iter() {
            total += read(id);
        }
        total
    }

    fn aggregate_metric_u64<F>(env: &Env, asset_id: Option<u64>, mut read: F) -> u64
    where
        F: FnMut(u64) -> u64,
    {
        if let Some(id) = asset_id {
            return read(id);
        }
        let assets: Vec<u64> = env.storage().instance().get(&MarketplaceDataKey::RegisteredAssets).unwrap_or(Vec::new(env));
        let mut total: u64 = 0;
        for id in assets.iter() {
            total += read(id);
        }
        total
    }

    fn validate_time_range(time_range: &Option<TimeRange>) -> Result<(), ComplianceError> {
        if let Some(range) = time_range {
            if range.start_timestamp > range.end_timestamp {
                return Err(ComplianceError::InvalidTimeRange);
            }
        }
        Ok(())
    }

    fn build_and_store_report(
        env: &Env,
        asset_id: Option<u64>,
        time_range: Option<TimeRange>,
        format: ReportFormat,
    ) -> Result<u64, ComplianceError> {
        Self::validate_time_range(&time_range)?;

        let total_value = Self::get_total_tokenized_value(env.clone(), asset_id, time_range.clone())?;
        let volume = Self::get_transaction_volume(env.clone(), asset_id, time_range.clone())?;
        let taxable_volume = Self::get_taxable_volume(env.clone(), asset_id, time_range.clone())?;
        let holders = Self::get_holder_count(env.clone(), asset_id, time_range)?;

        let restricted_transfer_count = Self::aggregate_metric_u64(env, asset_id, |id| {
            env.storage().instance().get(&MarketplaceDataKey::RestrictedTransferCount(id)).unwrap_or(0)
        });

        let report_id: u64 = env.storage().instance().get(&MarketplaceDataKey::ReportNonce).unwrap_or(0) + 1;
        env.storage().instance().set(&MarketplaceDataKey::ReportNonce, &report_id);

        let report = RegulatoryReport {
            id: report_id,
            generated_at: env.ledger().timestamp(),
            asset_id,
            format,
            total_value,
            volume,
            taxable_volume,
            holders,
            restricted_transfer_count,
        };

        env.storage().persistent().set(&MarketplaceDataKey::GeneratedReport(report_id), &report);
        env.events().publish((Symbol::new(env, "report_generated"), report_id), report.generated_at);
        Ok(report_id)
    }

    fn require_admin_for_result(env: &Env, admin: &Address) -> Result<(), ComplianceError> {
        admin.require_auth();
        let stored_admin: Address = match env
            .storage()
            .instance()
            .get(&MarketplaceDataKey::MarketplaceAdmin)
        {
            Some(v) => v,
            None => return Err(ComplianceError::Unauthorized),
        };
        if *admin != stored_admin {
            return Err(ComplianceError::Unauthorized);
        }
        Ok(())
    }
}


#[cfg(test)]
mod test {
    use super::*;
    use crate::asset_token::{AssetToken, AssetTokenClient};
    use crate::emergency_control::{EmergencyControl, EmergencyControlClient, PauseScope};
    use crate::governance::{Governance, GovernanceClient};
    use soroban_sdk::testutils::{Address as _, Ledger};
    use soroban_sdk::{Address, Env, Vec, String};

    #[test]
    fn test_create_listing_when_not_paused() {
        let env = Env::default();
        env.mock_all_auths();
        // Deploy emergency control contract
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Deploy marketplace contract
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin

        let seller = Address::generate(&env);
        let asset_id = 1;
        let amount = 100;
        let price = 1000;

        // No governance gate (None)
        let listing_id =
            mp_client.create_listing(&seller, &asset_id, &amount, &price, &ec_id, &None);
        assert_eq!(listing_id, 1);
    }

    #[test]
    #[should_panic(expected = "operation blocked: asset is paused")]
    fn test_create_listing_blocked_when_trading_paused() {
        let env = Env::default();
        env.mock_all_auths();

        // Deploy and initialize emergency control
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Pause trading
        let reason = String::from_str(&env, "security");
        ec_client.pause_asset(&admin, &1, &PauseScope::Trading, &reason, &0);

        // Deploy marketplace
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin

        let seller = Address::generate(&env);
        // This should panic because trading is paused
        mp_client.create_listing(&seller, &1, &100, &1000, &ec_id, &None);
    }

    #[test]
    #[should_panic(expected = "operation blocked: asset is paused")]
    fn test_purchase_blocked_when_trading_paused() {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let reason = String::from_str(&env, "halt");
        ec_client.pause_asset(&admin, &1, &PauseScope::Trading, &reason, &0);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin

        let buyer = Address::generate(&env);
        mp_client.purchase(&buyer, &1, &50, &1, &ec_id);
    }

    #[test]
    fn test_purchase_allowed_when_different_scope_paused() {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Pause minting only - trading should still work
        let reason = String::from_str(&env, "minting halt");
        ec_client.pause_asset(&admin, &1, &PauseScope::Minting, &reason, &0);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin

        let buyer = Address::generate(&env);
        let result = mp_client.purchase(&buyer, &1, &50, &1, &ec_id);
        assert!(result);
    }

    // -----------------------------------------------------------------------
    // Governance-gated listing tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_create_listing_with_governance_approved() {
        let env = Env::default();
        env.mock_all_auths();

        // Emergency control
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Asset / governance token
        let at_id = env.register_contract(None, AssetToken);
        let at_client = AssetTokenClient::new(&env, &at_id);
        at_client.initialize(
            &admin,
            &String::from_str(&env, "GOV"),
            &String::from_str(&env, "GOV"),
            &7,
            &0,
        );

        // Governance
        let gov_id = env.register_contract(None, Governance);
        let gov_client = GovernanceClient::new(&env, &gov_id);
        gov_client.initialize(&admin, &at_id, &100, &50);

        // Mint tokens and run governance flow
        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);
        at_client.mint(&proposer, &200, &1, &ec_id);
        at_client.mint(&voter, &150, &1, &ec_id);

        let asset_id: u64 = 1;
        let pid =
            gov_client.create_proposal(&proposer, &asset_id, &String::from_str(&env, "List"), &3600);
        gov_client.vote(&voter, &pid, &true);

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });
        gov_client.tally_execute(&pid);

        // Now listing should succeed with governance gate
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin
        let seller = Address::generate(&env);

        let lid = mp_client.create_listing(
            &seller,
            &asset_id,
            &100,
            &1000,
            &ec_id,
            &Some(gov_id),
        );
        assert_eq!(lid, 1);
    }

    #[test]
    #[should_panic(expected = "asset not approved by governance")]
    fn test_create_listing_blocked_without_governance_approval() {
        let env = Env::default();
        env.mock_all_auths();

        // Emergency control
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Asset / governance token
        let at_id = env.register_contract(None, AssetToken);
        let at_client = AssetTokenClient::new(&env, &at_id);
        at_client.initialize(
            &admin,
            &String::from_str(&env, "GOV"),
            &String::from_str(&env, "GOV"),
            &7,
            &0,
        );

        // Governance (no proposals passed)
        let gov_id = env.register_contract(None, Governance);
        let gov_client = GovernanceClient::new(&env, &gov_id);
        gov_client.initialize(&admin, &at_id, &100, &50);

        // Try listing with governance gate — should fail
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin); // Initialize marketplace admin
        let seller = Address::generate(&env);

        mp_client.create_listing(&seller, &1, &100, &1000, &ec_id, &Some(gov_id));
    }

    // -----------------------------------------------------------------------
    // Whitelisting tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_initialize_and_admin_auth() {
        let env = Env::default();
        let admin = Address::generate(&env);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);

        mp_client.initialize(&admin);
        
        let _not_admin = Address::generate(&env);
        env.mock_all_auths();
        
        // This should pass with admin auth
        mp_client.set_asset_privacy(&admin, &1, &true);
        assert!(mp_client.is_private(&1));
    }

    #[test]
    #[should_panic(expected = "not authorized: admin only")]
    fn test_set_privacy_unauthorized() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);

        mp_client.initialize(&admin);
        
        let not_admin = Address::generate(&env);
        mp_client.set_asset_privacy(&not_admin, &1, &true);
    }

    #[test]
    fn test_whitelisting_flow() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let user1 = Address::generate(&env);
        let user2 = Address::generate(&env);
        let asset_id = 1;

        assert!(!mp_client.is_whitelisted(&asset_id, &user1));

        // Add to whitelist
        mp_client.add_to_whitelist(&admin, &asset_id, &user1);
        assert!(mp_client.is_whitelisted(&asset_id, &user1));

        // Bulk add
        let users = Vec::from_array(&env, [user2.clone()]);
        mp_client.bulk_add_to_whitelist(&admin, &asset_id, &users);
        assert!(mp_client.is_whitelisted(&asset_id, &user2));

        // Remove from whitelist
        mp_client.remove_from_whitelist(&admin, &asset_id, &user1);
        assert!(!mp_client.is_whitelisted(&asset_id, &user1));
    }

    #[test]
    #[should_panic(expected = "user not whitelisted for private asset")]
    fn test_create_listing_private_asset_not_whitelisted() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let ec_id = env.register_contract(None, EmergencyControl);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let asset_id = 42;
        mp_client.set_asset_privacy(&admin, &asset_id, &true);

        let seller = Address::generate(&env);
        // Not whitelisted, should panic
        mp_client.create_listing(&seller, &asset_id, &100, &1000, &ec_id, &None);
    }

    #[test]
    fn test_create_listing_private_asset_whitelisted() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let ec_id = env.register_contract(None, EmergencyControl);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let asset_id = 42;
        mp_client.set_asset_privacy(&admin, &asset_id, &true);

        let seller = Address::generate(&env);
        mp_client.add_to_whitelist(&admin, &asset_id, &seller);

        // Whitelisted, should succeed
        let lid = mp_client.create_listing(&seller, &asset_id, &100, &1000, &ec_id, &None);
        assert_eq!(lid, 1);
    }

    #[test]
    #[should_panic(expected = "user not whitelisted for private asset")]
    fn test_purchase_private_asset_not_whitelisted() {
        let env = Env::default();
        env.mock_all_auths();
        let admin = Address::generate(&env);
        let ec_id = env.register_contract(None, EmergencyControl);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let asset_id = 42;
        mp_client.set_asset_privacy(&admin, &asset_id, &true);

        let buyer = Address::generate(&env);
        // Not whitelisted, should panic
        mp_client.purchase(&buyer, &1, &50, &asset_id, &ec_id);
    }

    // =======================================================================
    // Buy-Back & Burn System Tests
    // =======================================================================

    /// Helper: deploy marketplace and initialize buy-back system.
    fn setup_buyback() -> (Env, Address, MarketplaceClient<'static>, Address) {
        let env = Env::default();
        env.mock_all_auths();

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);

        mp_client.initialize_buyback(
            &admin, &10_000, // burn_cap
            &50_000, // auto_threshold
            &5_000,  // auto_buyback_amount
            &30,     // fee_bps (0.30%)
            &false,  // require_governance
        );

        (env, mp_id, mp_client, admin)
    }

    // -----------------------------------------------------------------------
    // Initialization tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_initialize_buyback_success() {
        let (_env, _mp_id, mp_client, _admin) = setup_buyback();

        assert_eq!(mp_client.get_treasury_balance(), 0);
        assert_eq!(mp_client.get_total_burned(), 0);

        let config = mp_client.get_buyback_config().unwrap();
        assert_eq!(config.burn_cap, 10_000);
        assert_eq!(config.auto_threshold, 50_000);
        assert_eq!(config.auto_buyback_amount, 5_000);
        assert_eq!(config.fee_bps, 30);
        assert!(!config.require_governance);
        assert!(!config.paused);
    }

    #[test]
    #[should_panic(expected = "buy-back system already initialized")]
    fn test_initialize_buyback_twice_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.initialize_buyback(&admin, &10_000, &50_000, &5_000, &30, &false);
    }

    #[test]
    #[should_panic(expected = "burn cap must be positive")]
    fn test_initialize_buyback_zero_burn_cap_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        mp_client.initialize_buyback(&admin, &0, &50_000, &5_000, &30, &false);
    }

    #[test]
    #[should_panic(expected = "fee basis points must not exceed 10000")]
    fn test_initialize_buyback_invalid_fee_panics() {
        let env = Env::default();
        env.mock_all_auths();
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        mp_client.initialize_buyback(&admin, &10_000, &50_000, &5_000, &10_001, &false);
    }

    // -----------------------------------------------------------------------
    // Treasury deposit tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_deposit_to_treasury() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        mp_client.deposit_to_treasury(&admin, &25_000);
        assert_eq!(mp_client.get_treasury_balance(), 25_000);

        mp_client.deposit_to_treasury(&admin, &10_000);
        assert_eq!(mp_client.get_treasury_balance(), 35_000);
    }

    #[test]
    #[should_panic(expected = "deposit amount must be positive")]
    fn test_deposit_to_treasury_zero_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &0);
    }

    #[test]
    #[should_panic(expected = "deposit amount must be positive")]
    fn test_deposit_to_treasury_negative_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &-100);
    }

    // -----------------------------------------------------------------------
    // Fee collection tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_collect_fee() {
        let (_env, _mp_id, mp_client, _admin) = setup_buyback();

        // 30 bps on 100_000 = 300
        let fee = mp_client.collect_fee(&100_000);
        assert_eq!(fee, 300);
        assert_eq!(mp_client.get_treasury_balance(), 300);
    }

    #[test]
    fn test_collect_fee_accumulates() {
        let (_env, _mp_id, mp_client, _admin) = setup_buyback();

        mp_client.collect_fee(&100_000); // 300
        mp_client.collect_fee(&200_000); // 600
        assert_eq!(mp_client.get_treasury_balance(), 900);
    }

    // -----------------------------------------------------------------------
    // Buy-back tokens tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_buy_back_tokens_success() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        // Fund treasury
        mp_client.deposit_to_treasury(&admin, &50_000);

        // Buy back 5000 tokens using 5000 from treasury
        mp_client.buy_back_tokens(&admin, &5_000, &5_000, &None);

        assert_eq!(mp_client.get_treasury_balance(), 45_000);
        assert_eq!(mp_client.get_total_burned(), 5_000);

        let history = mp_client.get_buyback_history();
        assert_eq!(history.len(), 1);
        let record = history.get(0).unwrap();
        assert_eq!(record.amount, 5_000);
        assert_eq!(record.burned, 5_000);
        assert_eq!(record.source_funds, 5_000);
        assert!(!record.auto_triggered);
    }

    #[test]
    fn test_buy_back_tokens_multiple_operations() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        mp_client.deposit_to_treasury(&admin, &50_000);

        mp_client.buy_back_tokens(&admin, &3_000, &3_000, &None);
        mp_client.buy_back_tokens(&admin, &2_000, &2_000, &None);

        assert_eq!(mp_client.get_treasury_balance(), 45_000);
        assert_eq!(mp_client.get_total_burned(), 5_000);

        let history = mp_client.get_buyback_history();
        assert_eq!(history.len(), 2);
    }

    #[test]
    #[should_panic(expected = "buy-back amount must be positive")]
    fn test_buy_back_tokens_zero_amount_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);
        mp_client.buy_back_tokens(&admin, &0, &5_000, &None);
    }

    #[test]
    #[should_panic(expected = "amount exceeds burn cap")]
    fn test_buy_back_tokens_exceeds_burn_cap_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);
        // burn_cap is 10_000, trying 15_000
        mp_client.buy_back_tokens(&admin, &15_000, &15_000, &None);
    }

    #[test]
    #[should_panic(expected = "insufficient treasury funds")]
    fn test_buy_back_tokens_insufficient_treasury_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &1_000);
        mp_client.buy_back_tokens(&admin, &5_000, &5_000, &None);
    }

    #[test]
    #[should_panic(expected = "caller is not buy-back admin")]
    fn test_buy_back_tokens_non_admin_panics() {
        let (env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);
        let non_admin = Address::generate(&env);
        mp_client.buy_back_tokens(&non_admin, &5_000, &5_000, &None);
    }

    // -----------------------------------------------------------------------
    // Burn tokens tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_burn_tokens_success() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        mp_client.burn_tokens(&admin, &5_000, &None);

        assert_eq!(mp_client.get_total_burned(), 5_000);

        let history = mp_client.get_buyback_history();
        assert_eq!(history.len(), 1);
        let record = history.get(0).unwrap();
        assert_eq!(record.burned, 5_000);
        assert_eq!(record.amount, 0); // direct burn, no buy-back
    }

    #[test]
    #[should_panic(expected = "burn amount must be positive")]
    fn test_burn_tokens_zero_amount_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.burn_tokens(&admin, &0, &None);
    }

    #[test]
    #[should_panic(expected = "burn amount exceeds burn cap")]
    fn test_burn_tokens_exceeds_cap_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.burn_tokens(&admin, &15_000, &None); // cap is 10_000
    }

    #[test]
    #[should_panic(expected = "caller is not buy-back admin")]
    fn test_burn_tokens_non_admin_panics() {
        let (env, _mp_id, mp_client, _admin) = setup_buyback();
        let non_admin = Address::generate(&env);
        mp_client.burn_tokens(&non_admin, &5_000, &None);
    }

    // -----------------------------------------------------------------------
    // Auto buy-back tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_auto_buy_back_triggers_above_threshold() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        // Fund treasury above auto_threshold (50_000)
        mp_client.deposit_to_treasury(&admin, &60_000);

        assert!(mp_client.is_auto_buyback_ready());

        mp_client.auto_buy_back();

        // auto_buyback_amount = 5_000 deducted
        assert_eq!(mp_client.get_treasury_balance(), 55_000);
        assert_eq!(mp_client.get_total_burned(), 5_000);

        let history = mp_client.get_buyback_history();
        assert_eq!(history.len(), 1);
        let record = history.get(0).unwrap();
        assert!(record.auto_triggered);
        assert_eq!(record.amount, 5_000);
    }

    #[test]
    #[should_panic(expected = "treasury below auto threshold")]
    fn test_auto_buy_back_below_threshold_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &10_000);
        assert!(!mp_client.is_auto_buyback_ready());
        mp_client.auto_buy_back();
    }

    #[test]
    fn test_auto_buy_back_respects_burn_cap() {
        let env = Env::default();
        env.mock_all_auths();

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);

        // Set auto_buyback_amount (20_000) > burn_cap (5_000)
        mp_client.initialize_buyback(
            &admin, &5_000,  // burn_cap
            &10_000, // auto_threshold
            &20_000, // auto_buyback_amount (exceeds cap)
            &30, &false,
        );

        mp_client.deposit_to_treasury(&admin, &50_000);
        mp_client.auto_buy_back();

        // Should burn only up to burn_cap (5_000)
        assert_eq!(mp_client.get_total_burned(), 5_000);
        assert_eq!(mp_client.get_treasury_balance(), 45_000);
    }

    // -----------------------------------------------------------------------
    // Pause tests
    // -----------------------------------------------------------------------

    #[test]
    #[should_panic(expected = "buy-back system is paused")]
    fn test_buy_back_tokens_paused_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);
        mp_client.set_buyback_paused(&admin, &true);
        mp_client.buy_back_tokens(&admin, &5_000, &5_000, &None);
    }

    #[test]
    #[should_panic(expected = "buy-back system is paused")]
    fn test_burn_tokens_paused_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.set_buyback_paused(&admin, &true);
        mp_client.burn_tokens(&admin, &5_000, &None);
    }

    #[test]
    #[should_panic(expected = "buy-back system is paused")]
    fn test_auto_buy_back_paused_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &60_000);
        mp_client.set_buyback_paused(&admin, &true);
        mp_client.auto_buy_back();
    }

    #[test]
    fn test_pause_unpause_cycle() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);

        // Pause
        mp_client.set_buyback_paused(&admin, &true);
        let config = mp_client.get_buyback_config().unwrap();
        assert!(config.paused);

        // Unpause
        mp_client.set_buyback_paused(&admin, &false);
        let config = mp_client.get_buyback_config().unwrap();
        assert!(!config.paused);

        // Operations work after unpause
        mp_client.buy_back_tokens(&admin, &5_000, &5_000, &None);
        assert_eq!(mp_client.get_total_burned(), 5_000);
    }

    // -----------------------------------------------------------------------
    // Configuration update tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_update_buyback_config() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        mp_client.update_buyback_config(
            &admin, &20_000,  // new burn_cap
            &100_000, // new auto_threshold
            &10_000,  // new auto_buyback_amount
            &50,      // new fee_bps
            &false,
        );

        let config = mp_client.get_buyback_config().unwrap();
        assert_eq!(config.burn_cap, 20_000);
        assert_eq!(config.auto_threshold, 100_000);
        assert_eq!(config.auto_buyback_amount, 10_000);
        assert_eq!(config.fee_bps, 50);
    }

    #[test]
    fn test_set_burn_cap() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        mp_client.set_burn_cap(&admin, &25_000);
        let config = mp_client.get_buyback_config().unwrap();
        assert_eq!(config.burn_cap, 25_000);
    }

    #[test]
    #[should_panic(expected = "burn cap must be positive")]
    fn test_set_burn_cap_zero_panics() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.set_burn_cap(&admin, &0);
    }

    // -----------------------------------------------------------------------
    // Reporting and history tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_buyback_history_tracks_operations() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();
        mp_client.deposit_to_treasury(&admin, &50_000);

        // Multiple operations
        mp_client.buy_back_tokens(&admin, &3_000, &3_000, &None);
        mp_client.burn_tokens(&admin, &2_000, &None);
        mp_client.buy_back_tokens(&admin, &1_000, &1_000, &None);

        let history = mp_client.get_buyback_history();
        assert_eq!(history.len(), 3);
        assert_eq!(mp_client.get_total_burned(), 6_000);
    }

    #[test]
    fn test_is_auto_buyback_ready_reflects_state() {
        let (_env, _mp_id, mp_client, admin) = setup_buyback();

        // Below threshold
        assert!(!mp_client.is_auto_buyback_ready());

        mp_client.deposit_to_treasury(&admin, &30_000);
        assert!(!mp_client.is_auto_buyback_ready());

        mp_client.deposit_to_treasury(&admin, &25_000); // Total 55,000 >= 50,000
        assert!(mp_client.is_auto_buyback_ready());

        // After auto buy-back drains below threshold
        mp_client.auto_buy_back(); // 55,000 - 5,000 = 50,000
        assert!(mp_client.is_auto_buyback_ready()); // 50_000 still >= 50_000

        // Pause makes it not ready
        mp_client.set_buyback_paused(&admin, &true);
        assert!(!mp_client.is_auto_buyback_ready());
    }

    // -----------------------------------------------------------------------
    // Governance-gated burn tests
    // -----------------------------------------------------------------------

    #[test]
    #[should_panic(expected = "governance approval required but no governance contract provided")]
    fn test_burn_tokens_governance_required_no_contract_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);

        // Initialize with require_governance = true
        mp_client.initialize_buyback(&admin, &10_000, &50_000, &5_000, &30, &true);

        mp_client.burn_tokens(&admin, &5_000, &None);
    }

    #[test]
    #[should_panic(expected = "governance approval required but no governance contract provided")]
    fn test_buy_back_governance_required_no_contract_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);

        mp_client.initialize_buyback(&admin, &10_000, &50_000, &5_000, &30, &true);
        mp_client.deposit_to_treasury(&admin, &50_000);

        mp_client.buy_back_tokens(&admin, &5_000, &5_000, &None);
    }

    // -----------------------------------------------------------------------
    // Referral System Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_referral_initialization() {
        let env = Env::default();
        env.mock_all_auths();
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        let treasury = Address::generate(&env);

        mp_client.initialize(&admin); // Initialize marketplace admin
        mp_client.initialize_referral(&admin, &treasury, &500, &1000); // 5% reward on fees

        let (referrer, reward, count) = mp_client.get_referral_info(&Address::generate(&env));
        assert!(referrer.is_none());
        assert_eq!(reward, 0);
        assert_eq!(count, 0);
    }

    #[test]
    fn test_refer_user_flow() {
        let env = Env::default();
        env.mock_all_auths();
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        let referrer = Address::generate(&env);
        let referred = Address::generate(&env);

        mp_client.initialize(&admin); // Initialize marketplace admin
        mp_client.refer_user(&referred, &referrer);

        let (stored_referrer, _reward, _count) = mp_client.get_referral_info(&referred);
        assert_eq!(stored_referrer.unwrap(), referrer);

        let (_, _, count) = mp_client.get_referral_info(&referrer);
        assert_eq!(count, 1);
    }

    #[test]
    #[should_panic(expected = "self-referral not allowed")]
    fn test_refer_self_fails() {
        let env = Env::default();
        env.mock_all_auths();
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let user = Address::generate(&env);
        let admin = Address::generate(&env); // Added for marketplace init
        mp_client.initialize(&admin); // Initialize marketplace admin

        mp_client.refer_user(&user, &user);
    }

    #[test]
    fn test_referral_reward_crediting() {
        let env = Env::default();
        env.mock_all_auths();
        
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let treasury = Address::generate(&env);
        let referrer = Address::generate(&env);
        let referred = Address::generate(&env);

        mp_client.initialize(&admin); // Initialize marketplace admin
        // 5% reward on fees (500 bps), 30 bps marketplace fee
        mp_client.initialize_buyback(&admin, &10000, &50000, &5000, &30, &false);
        mp_client.initialize_referral(&admin, &treasury, &500, &100);

        mp_client.refer_user(&referred, &referrer);

        // Trade of 100,000 -> 300 fee -> 5% of 300 = 15 reward
        mp_client.purchase(&referred, &1, &100_000, &1, &ec_id);

        let (_, reward, _) = mp_client.get_referral_info(&referrer);
        assert_eq!(reward, 15);
    }

    #[test]
    fn test_claim_referral_reward_flow() {
        let env = Env::default();
        env.mock_all_auths();
        
        // Setup same as above and then claim
        let ec_id = env.register_contract(None, EmergencyControl);
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        let treasury = Address::generate(&env);
        let referrer = Address::generate(&env);
        let referred = Address::generate(&env);

        mp_client.initialize(&admin); // Initialize marketplace admin
        mp_client.initialize_buyback(&admin, &10000, &50000, &5000, &30, &false);
        mp_client.initialize_referral(&admin, &treasury, &500, &100);

        mp_client.refer_user(&referred, &referrer);
        mp_client.purchase(&referred, &1, &100_000, &1, &ec_id);

        // Claim
        mp_client.claim_referral_reward(&referrer);
        
        let (_, reward, _) = mp_client.get_referral_info(&referrer);
        assert_eq!(reward, 0);
    }

    #[test]
    fn test_multi_asset_registration_and_isolation() {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        mp_client.register_asset(
            &admin,
            &101,
            &String::from_str(&env, "asset-a"),
            &false,
            &1,
        );
        mp_client.register_asset(
            &admin,
            &202,
            &String::from_str(&env, "asset-b"),
            &false,
            &1,
        );

        let seller_a = Address::generate(&env);
        let seller_b = Address::generate(&env);
        mp_client.create_listing(&seller_a, &101, &10, &50, &ec_id, &None);
        mp_client.create_listing(&seller_b, &202, &20, &70, &ec_id, &None);

        let a = mp_client.get_asset_analytics(&101);
        let b = mp_client.get_asset_analytics(&202);
        assert_eq!(a.asset_id, 101);
        assert_eq!(b.asset_id, 202);
        assert_eq!(a.listing_count, 1);
        assert_eq!(b.listing_count, 1);
        assert_ne!(a.volume, b.volume);
    }

    #[test]
    fn test_transfer_restrictions_with_approval_flow() {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let asset_id = 404;
        let buyer = Address::generate(&env);
        let seller = Address::generate(&env);

        mp_client.configure_transfer_restrictions(
            &admin,
            &asset_id,
            &true,
            &0,
            &0,
            &1_000,
            &true,
            &true,
        );
        mp_client.add_to_whitelist(&admin, &asset_id, &buyer);
        mp_client.set_compliance_verified(&admin, &asset_id, &buyer, &true);
        mp_client.create_listing(&seller, &asset_id, &10, &100, &ec_id, &None);

        let req = mp_client.request_transfer_approval(
            &buyer,
            &buyer,
            &asset_id,
            &500,
            &String::from_str(&env, "manual review"),
        );
        mp_client.review_transfer_approval(&admin, &req, &true);

        let ok = mp_client.purchase_with_approval(&buyer, &1, &500, &asset_id, &req, &ec_id);
        assert!(ok);
        assert_eq!(mp_client.get_restricted_transfer_count(&asset_id), 1);
    }

    #[test]
    fn test_compliance_reporting_export_and_scheduler() {
        let env = Env::default();
        env.mock_all_auths();

        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        mp_client.initialize(&admin);

        let asset_id = 777;
        mp_client.register_asset(
            &admin,
            &asset_id,
            &String::from_str(&env, "regulated-asset"),
            &false,
            &1,
        );

        let seller = Address::generate(&env);
        let buyer = Address::generate(&env);
        mp_client.create_listing(&seller, &asset_id, &10, &100, &ec_id, &None);
        mp_client.purchase(&buyer, &1, &250, &asset_id, &ec_id);

        let report_id = mp_client
            .generate_regulatory_report(&admin, &Some(asset_id), &None, &ReportFormat::Csv);
        let export = mp_client.export_report(&report_id);
        match export {
            ReportExport::Csv(r) => {
                assert_eq!(r.asset_id, Some(asset_id));
                assert!(r.volume >= 250);
            }
            _ => panic!("expected csv export"),
        }

        let schedule_id = mp_client
            .schedule_report(&admin, &Some(asset_id), &60, &ReportFormat::Json);
        assert_eq!(schedule_id, 1);

        env.ledger().with_mut(|li| {
            li.timestamp += 120;
        });

        let generated = mp_client.run_due_reports(&admin);
        assert!(generated >= 1);
    }

    // -----------------------------------------------------------------------
    // Oracle Integration Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_set_and_get_marketplace_oracle() {
        let env = Env::default();
        env.mock_all_auths();

        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let admin = Address::generate(&env);
        mp_client.initialize(&admin);

        let oracle_addr = Address::generate(&env);
        mp_client.set_marketplace_oracle(&admin, &oracle_addr);
        let stored = mp_client.get_marketplace_oracle().unwrap();
        assert_eq!(stored, oracle_addr);
    }

    #[test]
    fn test_oracle_set_and_get_on_marketplace() {
        use crate::oracle::{Oracle, OracleClient};

        let env = Env::default();
        env.mock_all_auths();

        // Deploy oracle
        let oracle_id = env.register_contract(None, Oracle);
        let oracle_client = OracleClient::new(&env, &oracle_id);
        let oracle_admin = Address::generate(&env);
        oracle_client.initialize(&oracle_admin, &500);

        // Deploy marketplace
        let mp_id = env.register_contract(None, Marketplace);
        let mp_client = MarketplaceClient::new(&env, &mp_id);
        let mp_admin = Address::generate(&env);
        mp_client.initialize(&mp_admin);

        // Set marketplace on oracle
        oracle_client.set_marketplace(&oracle_admin, &mp_id);
        let stored_mp = oracle_client.get_marketplace().unwrap();
        assert_eq!(stored_mp, mp_id);

        // Set oracle on marketplace
        mp_client.set_marketplace_oracle(&mp_admin, &oracle_id);
        let stored_oracle = mp_client.get_marketplace_oracle().unwrap();
        assert_eq!(stored_oracle, oracle_id);
    }

    #[test]
    fn test_compliance_queries_success() {
        let env = Env::default();
        let contract_id = env.register_contract(None, Marketplace);
        let client = MarketplaceClient::new(&env, &contract_id);

        // Test total tokenized value
        let total_val = client.get_total_tokenized_value(&None, &None);
        assert_eq!(total_val, 0);

        // Test transaction volume
        let volume = client.get_transaction_volume(&None, &None);
        assert_eq!(volume, 0);

        // Test holder count
        let holders = client.get_holder_count(&None, &None);
        assert_eq!(holders, 0);
    }

    #[test]
    #[should_panic(expected = "HostError: Error(Contract, #1)")]
    fn test_compliance_queries_invalid_time_range() {
        let env = Env::default();
        let contract_id = env.register_contract(None, Marketplace);
        let client = MarketplaceClient::new(&env, &contract_id);

        let invalid_range = TimeRange {
            start_timestamp: 100,
            end_timestamp: 50,
        };

        client.get_total_tokenized_value(&None, &Some(invalid_range));
    }
}
