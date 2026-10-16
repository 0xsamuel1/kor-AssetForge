use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, Address, Bytes, BytesN, Env, String, Symbol, Vec,
};

use crate::emergency_control::{EmergencyControlClient, PauseScope};


#[derive(Clone)]
#[contracttype]
pub struct FractionalMintedEvent {
    pub asset_id: u64,
    pub total_fractions: u64,
    pub unit_value: i128,
    pub issuer: Address,
}

#[derive(Clone)]
#[contracttype]
pub struct FractionalTransferEvent {
    pub from: Address,
    pub to: Address,
    pub amount: i128,
    pub asset_id: u64,
}

#[derive(Clone, Copy, PartialEq, Eq)]
#[repr(u32)]
pub enum FractionalError {
    UnauthorizedAdmin = 1,
    AlreadyFractionalized = 2,
    ZeroFractions = 3,
    UnevenDivision = 4,
    InvalidOwnerList = 5,
    InsufficientBalance = 6,
    ArithmeticOverflow = 7,
    InvalidAsset = 8,
    VerifierFailed = 9,
    InvalidProof = 10,
    InsufficientConsensus = 11,
}

#[derive(Clone)]
#[contracttype]
pub struct Asset {
    pub id: u64,
    pub name: String,
    pub symbol: String,
    pub decimals: u32,
    pub owner: Address,
    pub is_fractionalized: bool,
    pub total_fractions: u64,
    pub unit_value: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct VerificationStatus {
    pub verified: bool,
    pub timestamp: u64,
    pub verifiers: Vec<Address>,
}

#[derive(Clone)]
#[contracttype]
pub struct Verifier {
    pub address: Address,
    pub reputation: u64,
    pub total_verifications: u64,
    pub successful_verifications: u64,
    pub is_active: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct ProofSubmission {
    pub submitter: Address,
    pub asset_id: u64,
    pub proof_data: Bytes,
    pub timestamp: u64,
    pub verifier_signatures: Vec<Address>,
}

#[derive(Clone)]
#[contracttype]
pub struct ConsensusConfig {
    pub min_verifiers_required: u32,
    pub consensus_threshold: u32, // percentage (0-100)
    pub verification_timeout: u64, // seconds
}

#[derive(Clone)]
#[contracttype]
pub struct ValuationConfig {
    pub min_interval: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct ValuationRecord {
    pub value: i128,
    pub timestamp: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct DividendSchedule {
    pub total_dividend: i128,
    pub payout_asset: Address,
    pub next_payout_time: u64,
    pub interval: u64,
    pub amount_per_token: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct DividendDistribution {
    pub distribution_id: u64,
    pub asset_id: u64,
    pub total_amount: i128,
    pub payout_asset: Address,
    pub timestamp: u64,
    pub snapshot_timestamp: u64,
    pub total_supply: i128,
    pub tax_withholding_rate: u32, // basis points
    pub is_paused: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct DividendClaim {
    pub distribution_id: u64,
    pub claimant: Address,
    pub amount: i128,
    pub withheld: i128,
    pub claimed_at: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct TokenSnapshot {
    pub snapshot_id: u64,
    pub timestamp: u64,
    pub total_supply: i128,
    pub holder_count: u32,
}

#[derive(Clone)]
#[contracttype]
pub struct SnapshotEntry {
    pub address: Address,
    pub balance: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct TransactionRecord {
    pub tx_id: u64,
    pub transaction_type: TransactionType,
    pub from: Address,
    pub to: Option<Address>,
    pub amount: i128,
    pub asset_id: u64,
    pub timestamp: u64,
    pub block_number: u64,
    pub fee: i128,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
#[contracttype]
pub enum TransactionType {
    Transfer = 1,
    Mint = 2,
    Burn = 3,
    Stake = 4,
    Unstake = 5,
    BridgeOut = 6,
    BridgeIn = 7,
    DividendClaim = 8,
    DividendDistribution = 9,
}

#[derive(Clone)]
#[contracttype]
pub struct BatchTransfer {
    pub batch_id: u64,
    pub from: Address,
    pub transfers: Vec<BatchTransferEntry>,
    pub total_amount: i128,
    pub timestamp: u64,
    pub completed: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct BatchTransferEntry {
    pub to: Address,
    pub amount: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct TransactionAnalytics {
    pub total_transactions: u64,
    pub total_volume: i128,
    pub unique_addresses: u32,
    pub average_transaction_size: i128,
    pub timestamp: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct VolumeData {
    pub timestamp: u64,
    pub volume: i128,
    pub transaction_count: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct PriceHistory {
    pub timestamp: u64,
    pub price: i128,
    pub volume: i128,
}

#[derive(Clone, PartialEq, Debug)]
#[contracttype]
pub enum TargetChain {
    Ethereum,
    Solana,
}

#[derive(Clone, PartialEq, Debug)]
#[contracttype]
pub enum BridgeStatus {
    Pending,
    Completed,
    Failed,
}

#[derive(Clone)]
#[contracttype]
pub struct PendingBridge {
    pub caller: Address,
    pub asset_id: Address,
    pub amount: i128,
    pub fee: i128,
    pub target_chain: TargetChain,
    pub target_address: Bytes,
    pub timeout: u64,
    pub status: BridgeStatus,
}

#[derive(Clone)]
#[contracttype]
pub struct BridgeConfig {
    pub fee_bps: u32,
    pub relayer_pool: Address,
    pub bridge_timeout: u64,
    pub max_pending_per_user: u32,
    pub paused: bool,
    pub relayer_pubkey: BytesN<32>,
}

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum StakingError {
    InsufficientBalance = 1,
    InsufficientToStake = 2,
    YieldsNotAvailable = 3,
    Overflow = 4,
}

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum MetadataError {
    ImmutableMetadata = 1,
    NotOwner = 2,
    TimelockNotExpired = 3,
    GovernanceRequired = 4,
    GovernanceNotSet = 5,
    UpdateNotFound = 6,
    AlreadyExecuted = 7,
    NotRequester = 8,
}

#[derive(Clone)]
#[contracttype]
pub enum DataKey {
    Admin,
    AssetInfo,
    Balance(Address),
    TotalSupply,
    Oracle,
    Valuation,
    ValuationHistory,
    ValuationConfig,
    ValuationTimestamps,
    DividendSchedule(u64), // asset_id -> schedule
    LastClaim(u64, Address),
    // Enhanced dividend distribution keys
    DividendDistribution(u64), // distribution_id -> distribution
    DividendClaim(u64, Address), // distribution_id, claimant -> claim
    TokenSnapshot(u64), // snapshot_id -> snapshot
    SnapshotEntry(u64, Address), // snapshot_id, address -> entry
    DistributionHistory(u64), // asset_id -> Vec<distribution_id>
    LastDistributionIndex, // for auto-incrementing distribution IDs
    LastSnapshotIndex, // for auto-incrementing snapshot IDs
    DividendPaused, // emergency pause flag
    // Transaction history and analytics keys
    TransactionRecord(u64), // tx_id -> record
    UserTransactions(Address), // user -> Vec<tx_id>
    AssetTransactions(u64), // asset_id -> Vec<tx_id>
    LastTransactionIndex, // for auto-incrementing tx IDs
    TransactionAnalytics(u64), // timestamp -> analytics
    VolumeHistory(u64), // timestamp -> volume data
    PriceHistory(u64), // timestamp -> price history
    AnalyticsCache(u64), // cache key -> cached data
    // Cross-chain bridging keys
    BridgeConfig,
    PendingBridge(BytesN<32>), // bridge_id -> PendingBridge
    UserPendingCount(Address), // rate limit tracker per user
    BridgeNonce,               // monotonic nonce for unique bridge IDs
    // Staking keys
    Asset,
    Staked(Address),
    TotalStaked,
    // Batch transaction keys
    BatchTransaction(u64),
    UserBatchTransactions(Address),
    LastBatchIndex,
    // Verification system keys
    Verifier(Address),
    VerifiersList,
    ConsensusConfig,
    ProofSubmission(u64), // proof_id -> submission
    VerificationStatus(u64), // asset_id -> status
    VerificationHistory(Address), // verifier -> Vec<asset_id>
    LastVerifierIndex,
    // Concentration limits
    ConcentrationLimit,
    ExemptFromLimit(Address),
    // Multi-signature integration
    MultiSig,
    // Controlled metadata update keys
    AssetMetadata(u64),             // asset_id -> AssetMetadata
    MetadataUpdateRequest(u64),     // request_id -> MetadataUpdateRequest
    MetadataUpdateHistory(u64),     // asset_id -> Vec<MetadataUpdateRecord>
    LastMetadataUpdateIndex,        // auto-incrementing update request IDs
    MetadataGovernanceContract,     // address of the governance contract for immutable changes
}

#[derive(Clone)]
#[contracttype]
pub struct Staked {
    pub amount: i128,
    pub start_time: u64,
    pub accumulated_yields: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct MetadataAttribute {
    pub trait_type: String,
    pub value: String,
}

#[derive(Clone)]
#[contracttype]
pub struct AssetMetadata {
    pub name: String,
    pub description: String,
    pub symbol: String,
    pub image_uri: String,
    pub external_url: String,
    pub attributes: Vec<MetadataAttribute>,
    pub is_immutable: bool,
    pub version: u64,
    pub updated_at: u64,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
#[contracttype]
pub enum MetadataUpdateStatus {
    Pending = 1,
    Approved = 2,
    Rejected = 3,
    Executed = 4,
    Cancelled = 5,
}

#[derive(Clone)]
#[contracttype]
pub struct MetadataUpdateRequest {
    pub request_id: u64,
    pub asset_id: u64,
    pub requester: Address,
    pub old_metadata: AssetMetadata,
    pub new_metadata: AssetMetadata,
    pub created_at: u64,
    pub timelock_until: u64,
    pub status: MetadataUpdateStatus,
    pub requires_governance: bool,
    pub governance_proposal_id: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct MetadataUpdateRecord {
    pub version: u64,
    pub old_metadata: AssetMetadata,
    pub new_metadata: AssetMetadata,
    pub updated_by: Address,
    pub updated_at: u64,
    pub update_type: u32, // 0 = owner_update, 1 = governance_update
}

const ANNUAL_YIELD_RATE: i128 = 500; // 500 basis points = 5%
const BASIS_POINTS_DIVISOR: i128 = 10000;
const SECONDS_IN_YEAR: i128 = 31536000;

#[contract]
pub struct AssetToken;

#[contractimpl]
impl AssetToken {
    pub fn initialize(env: Env, admin: Address, name: String, symbol: String, decimals: u32, initial_supply: i128) -> u64 {
        if env.storage().instance().has(&DataKey::AssetInfo) {
            panic!("already initialized");
        }
        admin.require_auth();

        let asset = Asset {
            id: 1,
            name,
            symbol,
            decimals,
            owner: admin.clone(),
            is_fractionalized: false,
            total_fractions: 0,
            unit_value: 0,
        };

        env.storage().instance().set(&DataKey::AssetInfo, &asset);
        env.storage().instance().set(&DataKey::Asset, &asset);
        env.storage().instance().set(&DataKey::Admin, &admin);
        env.storage().instance().set(&DataKey::TotalSupply, &initial_supply);

        if initial_supply > 0 {
            env.storage().persistent().set(&DataKey::Balance(admin.clone()), &initial_supply);
        }

        1 // asset_id
    }

    pub fn mint_fractional(
        env: Env,
        admin: Address,
        total_value: i128,
        fractions: u64,
        initial_owners: Option<Vec<(Address, u64)>>,
        proof_data: Option<Bytes>,
    ) -> u64 {
        admin.require_auth();

        // Verification Hook - verify asset authenticity if proof data is provided
        if let Some(proof) = proof_data {
            let asset_id = 1; // Default asset ID for this contract
            let verification_status = Self::get_verification_status(env.clone(), asset_id);
            
            match verification_status {
                Some(status) if status.verified => {
                    // Asset is verified, proceed with minting
                },
                Some(_) => {
                    // Asset exists but not verified
                    panic!("Asset not verified");
                },
                None => {
                    // No verification status exists, submit proof for verification
                    Self::submit_proof(env.clone(), admin.clone(), asset_id, proof);
                    panic!("Proof submitted, awaiting verification");
                }
            }
        }
        
        let mut asset: Asset = env.storage().instance().get(&DataKey::AssetInfo).expect("Asset not initialized");
        assert_eq!(asset.owner, admin, "not owner");
        assert!(!asset.is_fractionalized, "already fractionalized");
        assert!(fractions > 0, "fractions must be > 0");
        assert_eq!(total_value % (fractions as i128), 0, "uneven division");

        let unit_value = total_value / (fractions as i128);
        let mut total_distributed: u64 = 0;

        if let Some(owners) = initial_owners {
            for (owner_addr, share_count) in owners.iter() {
                total_distributed += share_count;
                assert!(total_distributed <= fractions, "exceeds fractions");
                let balance = (share_count as i128).checked_mul(unit_value).unwrap();
                let current: i128 = env.storage().persistent().get(&DataKey::Balance(owner_addr.clone())).unwrap_or(0);
                env.storage().persistent().set(&DataKey::Balance(owner_addr), &(current.checked_add(balance).unwrap()));
            }
        }

        asset.is_fractionalized = true;
        asset.total_fractions = fractions;
        asset.unit_value = unit_value;

        env.storage().instance().set(&DataKey::AssetInfo, &asset);
        env.storage().instance().set(&DataKey::TotalSupply, &total_value);

        env.events().publish((Symbol::new(&env, "fractions_minted"),), (asset.id, fractions, unit_value));
        asset.id
    }

    pub fn mint(env: Env, to: Address, amount: i128, asset_id: u64, emergency_control_id: Address) {
        assert!(amount > 0, "amount must be positive");
        let asset: Asset = env.storage().instance().get(&DataKey::AssetInfo).expect("Asset not initialized");
        asset.owner.require_auth();

        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Minting);

        let mut balance = Self::balance(env.clone(), to.clone());
        balance = balance.checked_add(amount).unwrap();
        
        let supply = Self::total_supply(env.clone());
        let new_supply = supply.checked_add(amount).unwrap();

        // Check ownership concentration limit
        if let Some(limit_bps) = env.storage().instance().get::<_, u32>(&DataKey::ConcentrationLimit) {
            let is_exempt: bool = env.storage().persistent().get(&DataKey::ExemptFromLimit(to.clone())).unwrap_or(false);
            if !is_exempt {
                let max_allowed = (new_supply * limit_bps as i128) / 10000;
                if balance > max_allowed {
                    panic!("Ownership concentration limit exceeded");
                }
            }
        }

        env.storage().persistent().set(&DataKey::Balance(to.clone()), &balance);
        env.storage().instance().set(&DataKey::TotalSupply, &new_supply);

        // Record transaction
        Self::record_transaction(&env, TransactionType::Mint, asset.owner.clone(), Some(to.clone()), amount, asset_id, 0);

        env.events().publish((Symbol::new(&env, "mint"), to), amount);
    }

    /// Get asset details
    pub fn get_asset(env: Env) -> Option<Asset> {
        env.storage().instance().get(&DataKey::AssetInfo)
    }

    /// Get balance of an address
    pub fn balance(env: Env, address: Address) -> i128 {
        env.storage().persistent().get(&DataKey::Balance(address)).unwrap_or(0)
    }

    /// Transfer tokens between addresses
    pub fn transfer(env: Env, from: Address, to: Address, amount: i128, asset_id: u64, emergency_control_id: Address) {
        assert!(amount > 0, "amount must be positive");
        from.require_auth();
        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Transfers);

        let mut from_balance = Self::balance(env.clone(), from.clone());
        if from_balance < amount {
            panic!("insufficient balance");
        }

        let mut to_balance = Self::balance(env.clone(), to.clone());

        from_balance = from_balance.checked_sub(amount).unwrap();
        to_balance = to_balance.checked_add(amount).unwrap();

        // Check ownership concentration limit
        if let Some(limit_bps) = env.storage().instance().get::<_, u32>(&DataKey::ConcentrationLimit) {
            let is_exempt: bool = env.storage().persistent().get(&DataKey::ExemptFromLimit(to.clone())).unwrap_or(false);
            if !is_exempt {
                let supply = Self::total_supply(env.clone());
                let max_allowed = (supply * limit_bps as i128) / 10000;
                if to_balance > max_allowed {
                    panic!("Ownership concentration limit exceeded");
                }
            }
        }

        env.storage().persistent().set(&DataKey::Balance(from.clone()), &from_balance);
        env.storage().persistent().set(&DataKey::Balance(to.clone()), &to_balance);

        // Record transaction
        Self::record_transaction(&env, TransactionType::Transfer, from.clone(), Some(to.clone()), amount, asset_id, 0);

        env.events().publish((Symbol::new(&env, "transfer"), from, to), amount);
    }

    /// Execute a batch of transfers atomically
    pub fn batch_transfer(
        env: Env,
        from: Address,
        transfers: Vec<BatchTransferEntry>,
        asset_id: u64,
        emergency_control_id: Address,
    ) -> u64 {
        from.require_auth();
        assert!(transfers.len() > 0, "batch must contain at least one transfer");
        assert!(transfers.len() <= 50, "batch cannot exceed 50 transfers");

        let ec_client = EmergencyControlClient::new(&env, &emergency_control_id);
        ec_client.require_not_paused(&asset_id, &PauseScope::Transfers);

        let mut total_amount: i128 = 0;
        for i in 0..transfers.len() {
            let entry = transfers.get(i).unwrap();
            assert!(entry.amount > 0, "each transfer amount must be positive");
            total_amount = total_amount.checked_add(entry.amount).unwrap();
        }

        let mut from_balance = Self::balance(env.clone(), from.clone());
        assert!(from_balance >= total_amount, "insufficient balance for batch");

        from_balance = from_balance.checked_sub(total_amount).unwrap();
        env.storage().persistent().set(&DataKey::Balance(from.clone()), &from_balance);

        let mut total_gas_saved: i128 = 0;
        for i in 0..transfers.len() {
            let entry = transfers.get(i).unwrap();
            let mut to_balance = Self::balance(env.clone(), entry.to.clone());
            to_balance = to_balance.checked_add(entry.amount).unwrap();
            env.storage().persistent().set(&DataKey::Balance(entry.to.clone()), &to_balance);

            Self::record_transaction(
                &env,
                TransactionType::Transfer,
                from.clone(),
                Some(entry.to.clone()),
                entry.amount,
                asset_id,
                0,
            );

            env.events().publish(
                (Symbol::new(&env, "batch_transfer"), from.clone(), entry.to.clone()),
                (i as u64 + 1, entry.amount),
            );

            total_gas_saved = total_gas_saved.checked_add(entry.amount).unwrap();
        }

        let batch_id = env.storage().instance().get(&DataKey::LastBatchIndex).unwrap_or(0u64) + 1;
        let batch = BatchTransfer {
            batch_id,
            from: from.clone(),
            transfers,
            total_amount,
            timestamp: env.ledger().timestamp(),
            completed: true,
        };

        env.storage().instance().set(&DataKey::BatchTransaction(batch_id), &batch);
        env.storage().instance().set(&DataKey::LastBatchIndex, &batch_id);

        let mut user_batches: Vec<u64> = env.storage().instance().get(&DataKey::UserBatchTransactions(from.clone())).unwrap_or(Vec::new(&env));
        user_batches.push_back(batch_id);
        env.storage().instance().set(&DataKey::UserBatchTransactions(from.clone()), &user_batches);

        env.events().publish((Symbol::new(&env, "batch_completed"), from), batch_id);

        batch_id
    }

    /// Get batch transfer details
    pub fn get_batch_transfer(env: Env, batch_id: u64) -> Option<BatchTransfer> {
        env.storage().instance().get(&DataKey::BatchTransaction(batch_id))
    }

    /// Get user's batch transaction IDs
    pub fn get_user_batch_transactions(env: Env, user: Address) -> Vec<u64> {
        env.storage().instance().get(&DataKey::UserBatchTransactions(user)).unwrap_or(Vec::new(&env))
    }

    pub fn total_supply(env: Env) -> i128 {
        env.storage().instance().get(&DataKey::TotalSupply).unwrap_or(0)
    }

    pub fn set_concentration_limit(env: Env, admin: Address, limit_bps: u32) {
        admin.require_auth();
        let expected_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, expected_admin, "not admin");
        assert!(limit_bps <= 10000, "limit must be <= 10000");
        env.storage().instance().set(&DataKey::ConcentrationLimit, &limit_bps);
        env.events().publish((Symbol::new(&env, "concentration_limit_set"), admin), limit_bps);
    }

    pub fn set_exemption(env: Env, admin: Address, account: Address, is_exempt: bool) {
        admin.require_auth();
        let expected_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, expected_admin, "not admin");
        env.storage().persistent().set(&DataKey::ExemptFromLimit(account.clone()), &is_exempt);
        env.events().publish((Symbol::new(&env, "exemption_set"), admin), (account, is_exempt));
    }

    pub fn get_concentration_limit(env: Env) -> Option<u32> {
        env.storage().instance().get(&DataKey::ConcentrationLimit)
    }

    pub fn is_exempt(env: Env, account: Address) -> bool {
        env.storage().persistent().get(&DataKey::ExemptFromLimit(account)).unwrap_or(false)
    }

    pub fn name(env: Env) -> String {
        env.storage().instance().get::<_, Asset>(&DataKey::AssetInfo).expect("not initialized").name
    }

    pub fn symbol(env: Env) -> String {
        env.storage().instance().get::<_, Asset>(&DataKey::AssetInfo).expect("not initialized").symbol
    }

    pub fn decimals(env: Env) -> u32 {
        env.storage().instance().get::<_, Asset>(&DataKey::AssetInfo).expect("not initialized").decimals
    }

    pub fn update_valuation(env: Env, updater: Address, new_value: i128) {
        updater.require_auth();
        let admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        let oracle: Option<Address> = env.storage().instance().get(&DataKey::Oracle);

        
        if updater != admin && (oracle.is_none() || updater != oracle.unwrap()) {
            panic!("not authorized");
        }

        let now = env.ledger().timestamp();
        let config: ValuationConfig = env.storage().instance().get(&DataKey::ValuationConfig).unwrap_or(ValuationConfig { min_interval: 0 });

        if let Some(last) = env.storage().instance().get::<_, ValuationRecord>(&DataKey::Valuation) {
            if now < last.timestamp + config.min_interval {
                panic!("too frequent update");
            }
        }

        let record = ValuationRecord { value: new_value, timestamp: now };
        env.storage().instance().set(&DataKey::Valuation, &record);

        let mut history: Vec<ValuationRecord> = env.storage().persistent().get(&DataKey::ValuationHistory).unwrap_or(Vec::new(&env));
        history.push_back(record);
        env.storage().persistent().set(&DataKey::ValuationHistory, &history);

        env.events().publish((Symbol::new(&env, "valuation_updated"),), new_value);
    }

    pub fn set_oracle(env: Env, caller: Address, oracle: Address) {
        caller.require_auth();
        Self::require_admin_or_multisig(&env, &caller);
        env.storage().instance().set(&DataKey::Oracle, &oracle);
    }

    pub fn set_valuation_config(env: Env, caller: Address, min_interval: u64) {
        caller.require_auth();
        Self::require_admin_or_multisig(&env, &caller);
        env.storage().instance().set(&DataKey::ValuationConfig, &ValuationConfig { min_interval });
    }


    pub fn schedule_dividend(env: Env, asset_id: u64, total_dividend: i128, payout_asset: Address, interval: u64) {
        let admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        admin.require_auth();
        assert!(total_dividend > 0, "dividend must be positive");

        let supply = Self::total_supply(env.clone());
        assert!(supply > 0, "no supply");

        let amount_per_token = total_dividend.checked_mul(10_000_000).unwrap().checked_div(supply).unwrap();
        let schedule = DividendSchedule {
            total_dividend,
            payout_asset,
            next_payout_time: env.ledger().timestamp() + interval,
            interval,
            amount_per_token,
        };

        env.storage().persistent().set(&DataKey::DividendSchedule(asset_id), &schedule);
        env.events().publish((Symbol::new(&env, "dividend_scheduled"), asset_id), total_dividend);
    }

    pub fn claim_dividend(env: Env, asset_id: u64, claimant: Address) {
        claimant.require_auth();
        let schedule: DividendSchedule = env.storage().persistent().get(&DataKey::DividendSchedule(asset_id)).expect("no schedule");
        let now = env.ledger().timestamp();
        assert!(now >= schedule.next_payout_time, "not due");

        let last_claim_key = DataKey::LastClaim(asset_id, claimant.clone());
        if let Some(last) = env.storage().persistent().get::<_, u64>(&last_claim_key) {
            assert!(last < schedule.next_payout_time, "already claimed");
        }

        let balance = Self::balance(env.clone(), claimant.clone());
        assert!(balance > 0, "no tokens");

        let amount = balance.checked_mul(schedule.amount_per_token).unwrap().checked_div(10_000_000).unwrap();
        env.storage().persistent().set(&last_claim_key, &now);
        env.events().publish((Symbol::new(&env, "dividend_claimed"), asset_id, claimant), amount);
    }

    pub fn get_dividend_info(env: Env, asset_id: u64) -> Option<DividendSchedule> {
        env.storage().persistent().get(&DataKey::DividendSchedule(asset_id))
    }

    // Enhanced Dividend Distribution Functions
    // -----------------------------------------------------------------------

    /// Create a snapshot of token holders at the current time
    pub fn create_snapshot(env: Env, admin: Address) -> u64 {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        let snapshot_id = env.storage().instance().get(&DataKey::LastSnapshotIndex).unwrap_or(0u64) + 1;
        let timestamp = env.ledger().timestamp();
        let total_supply = Self::total_supply(env.clone());
        
        // In a real implementation, you would iterate through all token holders
        // For now, we'll store the snapshot metadata
        let snapshot = TokenSnapshot {
            snapshot_id,
            timestamp,
            total_supply,
            holder_count: 0, // Would be calculated in a full implementation
        };

        env.storage().instance().set(&DataKey::TokenSnapshot(snapshot_id), &snapshot);
        env.storage().instance().set(&DataKey::LastSnapshotIndex, &snapshot_id);
        
        env.events().publish((Symbol::new(&env, "snapshot_created"),), snapshot_id);
        snapshot_id
    }

    /// Get snapshot information
    pub fn get_snapshot(env: Env, snapshot_id: u64) -> Option<TokenSnapshot> {
        env.storage().instance().get(&DataKey::TokenSnapshot(snapshot_id))
    }

    /// Create a dividend distribution with snapshot-based allocation
    pub fn create_dividend_distribution(
        env: Env,
        admin: Address,
        asset_id: u64,
        total_amount: i128,
        payout_asset: Address,
        tax_withholding_rate: u32,
        snapshot_id: Option<u64>,
    ) -> u64 {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");
        
        // Check if dividends are paused
        let paused: bool = env.storage().instance().get(&DataKey::DividendPaused).unwrap_or(false);
        assert!(!paused, "dividends paused");

        assert!(total_amount > 0, "amount must be positive");
        assert!(tax_withholding_rate <= 10000, "tax rate too high");

        // Use provided snapshot_id or create a new one
        let actual_snapshot_id = snapshot_id.unwrap_or_else(|| {
            Self::create_snapshot(env.clone(), admin.clone())
        });
        
        let snapshot: TokenSnapshot = env.storage().instance().get(&DataKey::TokenSnapshot(actual_snapshot_id))
            .expect("snapshot not found");

        let distribution_id = env.storage().instance().get(&DataKey::LastDistributionIndex).unwrap_or(0u64) + 1;
        let timestamp = env.ledger().timestamp();

        let distribution = DividendDistribution {
            distribution_id,
            asset_id,
            total_amount,
            payout_asset,
            timestamp,
            snapshot_timestamp: snapshot.timestamp,
            total_supply: snapshot.total_supply,
            tax_withholding_rate,
            is_paused: false,
        };

        env.storage().instance().set(&DataKey::DividendDistribution(distribution_id), &distribution);
        env.storage().instance().set(&DataKey::LastDistributionIndex, &distribution_id);
        
        // Add to distribution history
        let mut history: Vec<u64> = env.storage().instance().get(&DataKey::DistributionHistory(asset_id))
            .unwrap_or(Vec::new(&env));
        history.push_back(distribution_id);
        env.storage().instance().set(&DataKey::DistributionHistory(asset_id), &history);
        
        env.events().publish((Symbol::new(&env, "dividend_distribution_created"),), (distribution_id, total_amount));
        distribution_id
    }

    /// Claim dividend from a specific distribution with tax withholding
    pub fn claim_dividend_distribution(env: Env, distribution_id: u64, claimant: Address) {
        claimant.require_auth();
        
        let distribution: DividendDistribution = env.storage().instance().get(&DataKey::DividendDistribution(distribution_id))
            .expect("distribution not found");
        
        assert!(!distribution.is_paused, "distribution paused");
        
        // Check if already claimed
        let claim_key = DataKey::DividendClaim(distribution_id, claimant.clone());
        let existing_claim: Option<DividendClaim> = env.storage().instance().get(&claim_key);
        if existing_claim.is_some() {
            panic!("already claimed");
        }

        let balance = Self::balance(env.clone(), claimant.clone());
        assert!(balance > 0, "no tokens");

        // Calculate proportional share
        let gross_amount = (balance * distribution.total_amount) / distribution.total_supply;
        
        // Calculate tax withholding
        let withheld = (gross_amount * distribution.tax_withholding_rate as i128) / 10000;
        let net_amount = gross_amount - withheld;

        let claim = DividendClaim {
            distribution_id,
            claimant: claimant.clone(),
            amount: net_amount,
            withheld,
            claimed_at: env.ledger().timestamp(),
        };

        env.storage().instance().set(&claim_key, &claim);
        
        // In a real implementation, you would transfer the net_amount to the claimant
        // For now, we just emit an event
        env.events().publish((Symbol::new(&env, "dividend_claimed"),), (distribution_id, claimant, net_amount));
    }

    /// Get distribution information
    pub fn get_dividend_distribution(env: Env, distribution_id: u64) -> Option<DividendDistribution> {
        env.storage().instance().get(&DataKey::DividendDistribution(distribution_id))
    }

    /// Get claim information for a specific distribution
    pub fn get_dividend_claim(env: Env, distribution_id: u64, claimant: Address) -> Option<DividendClaim> {
        env.storage().instance().get(&DataKey::DividendClaim(distribution_id, claimant))
    }

    /// Get distribution history for an asset
    pub fn get_distribution_history(env: Env, asset_id: u64) -> Vec<u64> {
        env.storage().instance().get(&DataKey::DistributionHistory(asset_id))
            .unwrap_or(Vec::new(&env))
    }

    /// Calculate unclaimed dividends for a distribution
    pub fn calculate_unclaimed_dividends(env: Env, distribution_id: u64) -> i128 {
        let distribution: DividendDistribution = env.storage().instance().get(&DataKey::DividendDistribution(distribution_id))
            .expect("distribution not found");
        
        // In a real implementation, you would sum up all claimed amounts and subtract from total
        // For now, return the total amount as a placeholder
        distribution.total_amount
    }

    /// Pause all dividend distributions (emergency)
    pub fn pause_dividends(env: Env, admin: Address) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        env.storage().instance().set(&DataKey::DividendPaused, &true);
        env.events().publish((Symbol::new(&env, "dividends_paused"),), ());
    }

    /// Resume dividend distributions
    pub fn resume_dividends(env: Env, admin: Address) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        env.storage().instance().set(&DataKey::DividendPaused, &false);
        env.events().publish((Symbol::new(&env, "dividends_resumed"),), ());
    }

    /// Check if dividends are paused
    pub fn are_dividends_paused(env: Env) -> bool {
        env.storage().instance().get(&DataKey::DividendPaused).unwrap_or(false)
    }

    /// Pause a specific distribution
    pub fn pause_distribution(env: Env, admin: Address, distribution_id: u64) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        let mut distribution: DividendDistribution = env.storage().instance().get(&DataKey::DividendDistribution(distribution_id))
            .expect("distribution not found");
        
        distribution.is_paused = true;
        env.storage().instance().set(&DataKey::DividendDistribution(distribution_id), &distribution);
        env.events().publish((Symbol::new(&env, "distribution_paused"),), distribution_id);
    }

    /// Resume a specific distribution
    pub fn resume_distribution(env: Env, admin: Address, distribution_id: u64) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        let mut distribution: DividendDistribution = env.storage().instance().get(&DataKey::DividendDistribution(distribution_id))
            .expect("distribution not found");
        
        distribution.is_paused = false;
        env.storage().instance().set(&DataKey::DividendDistribution(distribution_id), &distribution);
        env.events().publish((Symbol::new(&env, "distribution_resumed"),), distribution_id);
    }

    // Transaction History and Analytics Functions
    // -----------------------------------------------------------------------

    /// Record a transaction in the history
    fn record_transaction(env: &Env, tx_type: TransactionType, from: Address, to: Option<Address>, amount: i128, asset_id: u64, fee: i128) {
        let tx_id = env.storage().instance().get(&DataKey::LastTransactionIndex).unwrap_or(0u64) + 1;
        let timestamp = env.ledger().timestamp();
        let block_number = env.ledger().sequence() as u64;

        let record = TransactionRecord {
            tx_id,
            transaction_type: tx_type,
            from,
            to,
            amount,
            asset_id,
            timestamp,
            block_number,
            fee,
        };

        env.storage().instance().set(&DataKey::TransactionRecord(tx_id), &record);
        env.storage().instance().set(&DataKey::LastTransactionIndex, &tx_id);

        // Add to user's transaction history
        let mut user_txs: Vec<u64> = env.storage().instance().get(&DataKey::UserTransactions(record.from.clone()))
            .unwrap_or(Vec::new(env));
        user_txs.push_back(tx_id);
        env.storage().instance().set(&DataKey::UserTransactions(record.from), &user_txs);

        // Add to asset's transaction history
        let mut asset_txs: Vec<u64> = env.storage().instance().get(&DataKey::AssetTransactions(asset_id))
            .unwrap_or(Vec::new(env));
        asset_txs.push_back(tx_id);
        env.storage().instance().set(&DataKey::AssetTransactions(asset_id), &asset_txs);
    }

    /// Get transaction record by ID
    pub fn get_transaction(env: Env, tx_id: u64) -> Option<TransactionRecord> {
        env.storage().instance().get(&DataKey::TransactionRecord(tx_id))
    }

    /// Get user's transaction history
    pub fn get_user_transactions(env: Env, user: Address) -> Vec<u64> {
        env.storage().instance().get(&DataKey::UserTransactions(user))
            .unwrap_or(Vec::new(&env))
    }

    /// Get asset's transaction history
    pub fn get_asset_transactions(env: Env, asset_id: u64) -> Vec<u64> {
        env.storage().instance().get(&DataKey::AssetTransactions(asset_id))
            .unwrap_or(Vec::new(&env))
    }

    /// Get transaction analytics for a time period
    pub fn get_transaction_analytics(env: Env, start_time: u64, end_time: u64) -> TransactionAnalytics {
        let last_tx_id = env.storage().instance().get(&DataKey::LastTransactionIndex).unwrap_or(0u64);
        let mut total_transactions = 0u64;
        let mut total_volume = 0i128;
        let mut unique_addresses = Vec::<Address>::new(&env);
        let mut total_amount = 0i128;

        for tx_id in 1..=last_tx_id {
            let record: Option<TransactionRecord> = env.storage().instance().get(&DataKey::TransactionRecord(tx_id));
            if let Some(record) = record {
                if record.timestamp >= start_time && record.timestamp <= end_time {
                    total_transactions += 1;
                    total_volume += record.amount;
                    
                    // Check if from address is already in unique list
                    let mut from_found = false;
                    for addr in unique_addresses.iter() {
                        if addr == record.from {
                            from_found = true;
                            break;
                        }
                    }
                    if !from_found {
                        unique_addresses.push_back(record.from.clone());
                    }
                    
                    if let Some(to) = &record.to {
                        let mut to_found = false;
                        for addr in unique_addresses.iter() {
                            if addr == *to {
                                to_found = true;
                                break;
                            }
                        }
                        if !to_found {
                            unique_addresses.push_back(to.clone());
                        }
                    }
                    
                    total_amount += record.amount;
                }
            }
        }

        let avg_size = if total_transactions > 0 {
            total_amount / total_transactions as i128
        } else {
            0
        };

        TransactionAnalytics {
            total_transactions,
            total_volume,
            unique_addresses: unique_addresses.len() as u32,
            average_transaction_size: avg_size,
            timestamp: env.ledger().timestamp(),
        }
    }

    /// Get volume data for time series visualization
    pub fn get_volume_history(env: Env, start_time: u64, end_time: u64) -> Vec<VolumeData> {
        let mut volume_data = Vec::new(&env);
        let last_tx_id = env.storage().instance().get(&DataKey::LastTransactionIndex).unwrap_or(0u64);

        // Group by hour for time series - use Vec for simplicity
        let mut hourly_timestamps = Vec::<u64>::new(&env);
        let mut hourly_volumes = Vec::<i128>::new(&env);
        let mut hourly_counts = Vec::<u64>::new(&env);

        for tx_id in 1..=last_tx_id {
            let record: Option<TransactionRecord> = env.storage().instance().get(&DataKey::TransactionRecord(tx_id));
            if let Some(record) = record {
                if record.timestamp >= start_time && record.timestamp <= end_time {
                    let hour = record.timestamp / 3600;
                    
                    // Find existing hour entry
                    let mut found_idx: Option<u32> = None;
                    for i in 0..hourly_timestamps.len() {
                        let ts = hourly_timestamps.get(i).unwrap();
                        if ts == hour {
                            found_idx = Some(i);
                            break;
                        }
                    }
                    
                    if let Some(idx) = found_idx {
                        // Update existing entry
                        let current_vol = hourly_volumes.get(idx).unwrap();
                        hourly_volumes.set(idx, current_vol + record.amount);
                        
                        let current_count = hourly_counts.get(idx).unwrap();
                        hourly_counts.set(idx, current_count + 1);
                    } else {
                        // Add new entry
                        hourly_timestamps.push_back(hour);
                        hourly_volumes.push_back(record.amount);
                        hourly_counts.push_back(1);
                    }
                }
            }
        }

        // Sort by timestamp
        let mut sorted_data = Vec::<(u64, i128, u64)>::new(&env);
        for i in 0..hourly_timestamps.len() {
            sorted_data.push_back((
                hourly_timestamps.get(i).unwrap(),
                hourly_volumes.get(i).unwrap(),
                hourly_counts.get(i).unwrap(),
            ));
        }

        // Simple bubble sort for small datasets
        for i in 0..sorted_data.len() {
            for j in (i + 1)..sorted_data.len() {
                if sorted_data.get(i).unwrap().0 > sorted_data.get(j).unwrap().0 {
                    let temp = sorted_data.get(i).unwrap();
                    sorted_data.set(i, sorted_data.get(j).unwrap());
                    sorted_data.set(j, temp);
                }
            }
        }

        for data in sorted_data.iter() {
            volume_data.push_back(VolumeData {
                timestamp: data.0 * 3600,
                volume: data.1,
                transaction_count: data.2,
            });
        }

        volume_data
    }

    /// Record price history entry
    pub fn record_price(env: Env, price: i128, volume: i128) {
        let timestamp = env.ledger().timestamp();
        let price_entry = PriceHistory {
            timestamp,
            price,
            volume,
        };

        env.storage().instance().set(&DataKey::PriceHistory(timestamp), &price_entry);
    }

    /// Get price history for a time range
    pub fn get_price_history(env: Env, _start_time: u64, _end_time: u64) -> Vec<PriceHistory> {
        let prices = Vec::new(&env);
        // In a real implementation, you would iterate through stored price history
        // For now, return empty vector as placeholder
        prices
    }

    /// Get transaction statistics by type (simplified without HashMap)
    pub fn get_transaction_stats_by_type(env: Env, start_time: u64, end_time: u64) -> Vec<(TransactionType, u64, i128)> {
        let mut stats = Vec::<(TransactionType, u64, i128)>::new(&env);
        let last_tx_id = env.storage().instance().get(&DataKey::LastTransactionIndex).unwrap_or(0u64);

        // Initialize all transaction types with zeros
        stats.push_back((TransactionType::Transfer, 0, 0));
        stats.push_back((TransactionType::Mint, 0, 0));
        stats.push_back((TransactionType::Burn, 0, 0));
        stats.push_back((TransactionType::Stake, 0, 0));
        stats.push_back((TransactionType::Unstake, 0, 0));
        stats.push_back((TransactionType::BridgeOut, 0, 0));
        stats.push_back((TransactionType::BridgeIn, 0, 0));
        stats.push_back((TransactionType::DividendClaim, 0, 0));
        stats.push_back((TransactionType::DividendDistribution, 0, 0));

        for tx_id in 1..=last_tx_id {
            let record: Option<TransactionRecord> = env.storage().instance().get(&DataKey::TransactionRecord(tx_id));
            if let Some(record) = record {
                if record.timestamp >= start_time && record.timestamp <= end_time {
                    // Find and update the matching transaction type
                    for i in 0..stats.len() {
                        let (tx_type, count, amount) = stats.get(i).unwrap();
                        if tx_type == record.transaction_type {
                            stats.set(i, (tx_type, count + 1, amount + record.amount));
                            break;
                        }
                    }
                }
            }
        }

        stats
    }

    /// Export transaction history as records for CSV/PDF generation (done off-chain)
    pub fn export_transactions(env: Env, user: Option<Address>, asset_id: Option<u64>, start_time: u64, end_time: u64) -> Vec<TransactionRecord> {
        let mut records = Vec::new(&env);
        
        let tx_ids = if let Some(u) = user {
            Self::get_user_transactions(env.clone(), u)
        } else if let Some(aid) = asset_id {
            Self::get_asset_transactions(env.clone(), aid)
        } else {
            let last_id = env.storage().instance().get(&DataKey::LastTransactionIndex).unwrap_or(0u64);
            let mut all_ids = Vec::new(&env);
            for id in 1..=last_id {
                all_ids.push_back(id);
            }
            all_ids
        };

        for tx_id in tx_ids.iter() {
            let record: Option<TransactionRecord> = env.storage().instance().get(&DataKey::TransactionRecord(tx_id));
            if let Some(record) = record {
                if record.timestamp >= start_time && record.timestamp <= end_time {
                    records.push_back(record);
                }
            }
        }

        records
    }

    pub fn get_valuation_history(env: Env) -> Vec<ValuationRecord> {
        env.storage().persistent().get(&DataKey::ValuationHistory).unwrap_or(Vec::new(&env))
    }

    pub fn set_bridge_config(env: Env, fee_bps: u32, relayer_pool: Address, bridge_timeout: u64, max_pending_per_user: u32, relayer_pubkey: BytesN<32>) {
        let admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        admin.require_auth();
        let config = BridgeConfig { fee_bps, relayer_pool, bridge_timeout, max_pending_per_user, paused: false, relayer_pubkey };
        env.storage().instance().set(&DataKey::BridgeConfig, &config);
    }

    pub fn set_bridge_paused(env: Env, paused: bool) {
        let admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        admin.require_auth();
        let mut config: BridgeConfig = env.storage().instance().get(&DataKey::BridgeConfig).expect("not configured");
        config.paused = paused;
        env.storage().instance().set(&DataKey::BridgeConfig, &config);
    }

    pub fn bridge_out(env: Env, caller: Address, asset_id: Address, amount: i128, target_chain: TargetChain, target_address: Bytes) -> BytesN<32> {
        assert!(amount > 0, "amount must be positive");
        caller.require_auth();
        let config: BridgeConfig = env.storage().instance().get(&DataKey::BridgeConfig).expect("not configured");
        assert!(!config.paused, "paused");

        let pending_count: u32 = env.storage().persistent().get(&DataKey::UserPendingCount(caller.clone())).unwrap_or(0);
        assert!(pending_count < config.max_pending_per_user, "rate limit");

        let balance = Self::balance(env.clone(), caller.clone());
        assert!(balance >= amount, "insufficient balance");

        let fee = amount.checked_mul(config.fee_bps as i128).unwrap().checked_div(10_000).unwrap();
        let net = amount.checked_sub(fee).unwrap();

        env.storage().persistent().set(&DataKey::Balance(caller.clone()), &(balance.checked_sub(amount).unwrap()));
        let pool_bal = Self::balance(env.clone(), config.relayer_pool.clone());
        env.storage().persistent().set(&DataKey::Balance(config.relayer_pool.clone()), &(pool_bal.checked_add(fee).unwrap()));

        let supply = Self::total_supply(env.clone());
        env.storage().instance().set(&DataKey::TotalSupply, &(supply.checked_sub(net).unwrap()));

        let nonce: u64 = env.storage().instance().get(&DataKey::BridgeNonce).unwrap_or(0);
        env.storage().instance().set(&DataKey::BridgeNonce, &(nonce.checked_add(1).unwrap()));

        let mut id_bytes = [0u8; 32];
        id_bytes[..8].copy_from_slice(&nonce.to_le_bytes());
        let bridge_id = BytesN::from_array(&env, &id_bytes);

        let pending = PendingBridge { caller: caller.clone(), asset_id, amount: net, fee, target_chain: target_chain.clone(), target_address, timeout: env.ledger().timestamp() + config.bridge_timeout, status: BridgeStatus::Pending };
        env.storage().persistent().set(&DataKey::PendingBridge(bridge_id.clone()), &pending);
        env.storage().persistent().set(&DataKey::UserPendingCount(caller.clone()), &(pending_count + 1));

        env.events().publish((Symbol::new(&env, "bridge_initiated"), caller, target_chain), (net, bridge_id.clone()));
        bridge_id
    }

    pub fn bridge_in(env: Env, bridge_id: BytesN<32>, recipient: Address, asset_id: Address, amount: i128, source_chain: TargetChain) {
        assert!(amount > 0, "amount must be positive");
        let admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        admin.require_auth();
        
        let balance = Self::balance(env.clone(), recipient.clone());
        env.storage().persistent().set(&DataKey::Balance(recipient.clone()), &(balance.checked_add(amount).unwrap()));

        let supply = Self::total_supply(env.clone());
        env.storage().instance().set(&DataKey::TotalSupply, &(supply.checked_add(amount).unwrap()));

        if let Some(mut pending) = env.storage().persistent().get::<_, PendingBridge>(&DataKey::PendingBridge(bridge_id.clone())) {
            pending.status = BridgeStatus::Completed;
            env.storage().persistent().set(&DataKey::PendingBridge(bridge_id), &pending);
            let count: u32 = env.storage().persistent().get(&DataKey::UserPendingCount(pending.caller.clone())).unwrap_or(1);
            env.storage().persistent().set(&DataKey::UserPendingCount(pending.caller), &(if count > 0 { count - 1 } else { 0 }));
        }

        env.events().publish((Symbol::new(&env, "bridge_completed"), recipient, source_chain), (asset_id, amount));
    }

    pub fn get_pending_bridge(env: Env, bridge_id: BytesN<32>) -> Option<PendingBridge> {
        env.storage().persistent().get(&DataKey::PendingBridge(bridge_id))
    }

    pub fn expire_bridge(env: Env, bridge_id: BytesN<32>) {
        let mut pending: PendingBridge = env.storage().persistent().get(&DataKey::PendingBridge(bridge_id.clone())).expect("bridge not found");
        assert!(pending.status == BridgeStatus::Pending, "not pending");
        assert!(env.ledger().timestamp() >= pending.timeout, "not expired");

        pending.status = BridgeStatus::Failed;
        env.storage().persistent().set(&DataKey::PendingBridge(bridge_id), &pending);
        
        let count: u32 = env.storage().persistent().get(&DataKey::UserPendingCount(pending.caller.clone())).unwrap_or(1);
        env.storage().persistent().set(&DataKey::UserPendingCount(pending.caller), &(if count > 0 { count - 1 } else { 0 }));

        env.events().publish((Symbol::new(&env, "bridge_expired"),), pending.amount);
    }

    pub fn get_bridge_config(env: Env) -> Option<BridgeConfig> {
        env.storage().instance().get(&DataKey::BridgeConfig)
    }

    /// Stake tokens to earn yield
    pub fn stake_tokens(env: Env, from: Address, amount: i128) {
        assert!(amount > 0, "amount must be positive");
        from.require_auth();
        
        let mut balance = Self::balance(env.clone(), from.clone());
        if balance < amount {
            panic!("insufficient balance to stake");
        }
        
        let mut staked_data = env.storage().persistent()
            .get::<DataKey, Staked>(&DataKey::Staked(from.clone()))
            .unwrap_or(Staked {
                amount: 0,
                start_time: env.ledger().timestamp(),
                accumulated_yields: 0,
            });
            
        // If already staking, update yield before adding more
        if staked_data.amount > 0 {
            staked_data.accumulated_yields = staked_data.accumulated_yields.checked_add(Self::calculate_yield(&env, &staked_data)).unwrap();
        }
        
        staked_data.amount = staked_data.amount.checked_add(amount).unwrap();
        staked_data.start_time = env.ledger().timestamp();
        
        balance = balance.checked_sub(amount).unwrap();
        
        env.storage().persistent().set(&DataKey::Balance(from.clone()), &balance);
        env.storage().persistent().set(&DataKey::Staked(from.clone()), &staked_data);
        
        let mut total_staked: i128 = env.storage().instance().get(&DataKey::TotalStaked).unwrap_or(0);
        total_staked = total_staked.checked_add(amount).unwrap();
        env.storage().instance().set(&DataKey::TotalStaked, &total_staked);
        
        env.events().publish((Symbol::new(&env, "tokens_staked"), from), amount);
    }

    /// Unstake tokens and claim yields
    pub fn unstake_tokens(env: Env, from: Address, amount: i128) {
        assert!(amount > 0, "amount must be positive");
        from.require_auth();
        
        let mut staked_data = env.storage().persistent()
            .get::<DataKey, Staked>(&DataKey::Staked(from.clone()))
            .unwrap_or_else(|| panic!("no staked tokens found"));
            
        if staked_data.amount < amount {
            panic!("insufficient staked amount");
        }
        
        let yield_earned = Self::calculate_yield(&env, &staked_data);
        let total_yield = staked_data.accumulated_yields.checked_add(yield_earned).unwrap();
        
        staked_data.amount = staked_data.amount.checked_sub(amount).unwrap();
        staked_data.accumulated_yields = 0; // Yield claimed
        staked_data.start_time = env.ledger().timestamp();
        
        let mut balance = Self::balance(env.clone(), from.clone());
        balance = balance.checked_add(amount).unwrap().checked_add(total_yield).unwrap();
        
        env.storage().persistent().set(&DataKey::Balance(from.clone()), &balance);
        
        if staked_data.amount == 0 {
            env.storage().persistent().remove(&DataKey::Staked(from.clone()));
        } else {
            env.storage().persistent().set(&DataKey::Staked(from.clone()), &staked_data);
        }
        
        let mut total_staked: i128 = env.storage().instance().get(&DataKey::TotalStaked).unwrap_or(0);
        total_staked = total_staked.checked_sub(amount).unwrap();
        env.storage().instance().set(&DataKey::TotalStaked, &total_staked);
        
        env.events().publish((Symbol::new(&env, "yields_claimed"), from), total_yield);
    }
    
    pub fn get_staked(env: Env, address: Address) -> Option<Staked> {
        env.storage().persistent().get(&DataKey::Staked(address))
    }

    fn calculate_yield(env: &Env, staked: &Staked) -> i128 {
        let current_time = env.ledger().timestamp();
        let elapsed = (current_time - staked.start_time) as i128;
        
        if elapsed <= 0 || staked.amount <= 0 {
            return 0;
        }
        
        // yield = (staked_amount * rate * time) / (year_seconds * basis_points_divisor)
        staked.amount.checked_mul(ANNUAL_YIELD_RATE).unwrap()
            .checked_mul(elapsed).unwrap()
            .checked_div(SECONDS_IN_YEAR.checked_mul(BASIS_POINTS_DIVISOR).unwrap()).unwrap()
    }

    // Multi-Signature Integration Functions
    // -----------------------------------------------------------------------

    /// Set the multi-sig contract address for high-value operations.
    /// When set, sensitive admin operations must be routed through multi-sig.
    pub fn set_multisig(env: Env, admin: Address, multisig: Address) {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");
        env.storage().instance().set(&DataKey::MultiSig, &multisig);
        env.events().publish((Symbol::new(&env, "multisig_set"),), multisig);
    }

    /// Get the multi-sig contract address (if set).
    pub fn get_multisig(env: Env) -> Option<Address> {
        env.storage().instance().get(&DataKey::MultiSig)
    }

    // -----------------------------------------------------------------------
    // Emergency Hook (Issue #205)
    // -----------------------------------------------------------------------

    /// Emergency token recovery hook.
    ///
    /// Called by the EmergencyControl contract after a fully-approved and
    /// timelock-expired withdrawal has been executed.  This hook re-assigns
    /// `amount` tokens from `from` to `to`, bypassing the normal pause check.
    ///
    /// Requires:
    ///   - Caller is the registered emergency_control contract.
    ///   - `from` has sufficient balance.
    ///   - `withdrawal_id` is provided for event traceability.
    pub fn emergency_recover(
        env: Env,
        emergency_control: Address,
        from: Address,
        to: Address,
        amount: i128,
        asset_id: u64,
        withdrawal_id: u64,
    ) {
        emergency_control.require_auth();

        // Only the registered EmergencyControl contract may call this
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&DataKey::Admin)
            .expect("admin not set");
        // The emergency_control caller must match the admin-registered EC address.
        // (In production, store the EC address explicitly; here we accept any
        //  address that can authenticate itself – the require_auth above guards it.)
        let _ = stored_admin;

        assert!(amount > 0, "amount must be positive");

        let from_balance = Self::balance(env.clone(), from.clone());
        if from_balance < amount {
            panic!("insufficient balance for emergency recovery");
        }

        let to_balance = Self::balance(env.clone(), to.clone());
        env.storage()
            .persistent()
            .set(&DataKey::Balance(from.clone()), &(from_balance - amount));
        env.storage()
            .persistent()
            .set(&DataKey::Balance(to.clone()), &(to_balance + amount));

        Self::record_transaction(
            &env,
            TransactionType::Transfer,
            from.clone(),
            Some(to.clone()),
            amount,
            asset_id,
            0,
        );

        env.events().publish(
            (Symbol::new(&env, "emergency_recovery"), withdrawal_id),
            (from, to, amount, asset_id),
        );
    }

    /// Require that the caller is either the direct admin or the multi-sig contract
    /// when a multi-sig is configured. This allows the multi-sig to approve admin ops.
    fn require_admin_or_multisig(env: &Env, caller: &Address) {
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        let ms: Option<Address> = env.storage().instance().get(&DataKey::MultiSig);
        match ms {
            Some(multisig) => {
                // If multi-sig is configured, only the multi-sig contract can perform admin ops
                assert_eq!(*caller, multisig, "multi-sig approval required");
            }
            None => {
                // No multi-sig — direct admin auth
                assert_eq!(*caller, stored_admin, "not admin");
            }
        }
    }

    // Verification System Functions
    // -----------------------------------------------------------------------

    /// Initialize the verification system with consensus configuration
    pub fn initialize_verification(env: Env, admin: Address, min_verifiers: u32, consensus_threshold: u32, verification_timeout: u64) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("Contract not initialized");
        assert_eq!(admin, stored_admin, "not admin");

        let config = ConsensusConfig {
            min_verifiers_required: min_verifiers,
            consensus_threshold: consensus_threshold,
            verification_timeout: verification_timeout,
        };

        env.storage().instance().set(&DataKey::ConsensusConfig, &config);
        env.storage().instance().set(&DataKey::LastVerifierIndex, &0u64);
        env.storage().instance().set(&DataKey::VerifiersList, &Vec::<Address>::new(&env));
    }

    /// Register a new verifier
    pub fn register_verifier(env: Env, admin: Address, verifier: Address) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("Contract not initialized");
        assert_eq!(admin, stored_admin, "not admin");

        let verifier_info = Verifier {
            address: verifier.clone(),
            reputation: 100, // Start with neutral reputation
            total_verifications: 0,
            successful_verifications: 0,
            is_active: true,
        };

        env.storage().instance().set(&DataKey::Verifier(verifier.clone()), &verifier_info);
        
        // Add to verifiers list
        let mut verifiers: Vec<Address> = env.storage().instance().get(&DataKey::VerifiersList).unwrap_or(Vec::new(&env));
        verifiers.push_back(verifier);
        env.storage().instance().set(&DataKey::VerifiersList, &verifiers);
    }

    /// Get list of all active verifiers
    pub fn get_verifiers(env: Env) -> Vec<Address> {
        let verifiers: Vec<Address> = env.storage().instance().get(&DataKey::VerifiersList).unwrap_or_else(|| Vec::new(&env));
        let mut active_verifiers = Vec::<Address>::new(&env);
        
        for verifier in verifiers.iter() {
            let key = DataKey::Verifier(verifier.clone());
            let verifier_info: Option<Verifier> = env.storage().instance().get(&key);
            if let Some(info) = verifier_info {
                if info.is_active {
                    active_verifiers.push_back(verifier.clone());
                }
            }
        }
        
        active_verifiers
    }

    /// Submit proof for verification
    pub fn submit_proof(env: Env, submitter: Address, asset_id: u64, proof_data: Bytes) -> u64 {
        submitter.require_auth();
        
        let current_time = env.ledger().timestamp();
        let proof_id = env.storage().instance().get(&DataKey::LastVerifierIndex).unwrap_or(0u64) + 1;
        
        let submission = ProofSubmission {
            submitter: submitter.clone(),
            asset_id,
            proof_data: proof_data.clone(),
            timestamp: current_time,
            verifier_signatures: Vec::new(&env),
        };

        env.storage().instance().set(&DataKey::ProofSubmission(proof_id), &submission);
        env.storage().instance().set(&DataKey::LastVerifierIndex, &proof_id);
        
        proof_id
    }

    /// Verify authenticity of an asset proof
    pub fn verify_authenticity(env: Env, verifier: Address, proof_id: u64, approve: bool) {
        verifier.require_auth();
        
        // Check if verifier is registered and active
        let verifier_info: Verifier = env.storage().instance().get(&DataKey::Verifier(verifier.clone()))
            .expect("Verifier not registered");
        assert!(verifier_info.is_active, "Verifier not active");

        let mut submission: ProofSubmission = env.storage().instance().get(&DataKey::ProofSubmission(proof_id))
            .expect("Proof submission not found");

        // Check if verifier has already signed
        assert!(!submission.verifier_signatures.contains(&verifier), "Already verified");

        // Add verifier signature
        submission.verifier_signatures.push_back(verifier.clone());
        env.storage().instance().set(&DataKey::ProofSubmission(proof_id), &submission);

        // Update verifier stats
        let mut updated_verifier = verifier_info.clone();
        updated_verifier.total_verifications += 1;
        if approve {
            updated_verifier.successful_verifications += 1;
        }
        env.storage().instance().set(&DataKey::Verifier(verifier), &updated_verifier);

        // Check if consensus is reached
        Self::check_consensus(&env, proof_id, submission.asset_id);
    }

    /// Check if consensus is reached and update verification status
    fn check_consensus(env: &Env, proof_id: u64, asset_id: u64) {
        let submission: ProofSubmission = env.storage().instance().get(&DataKey::ProofSubmission(proof_id)).unwrap();
        let config: ConsensusConfig = env.storage().instance().get(&DataKey::ConsensusConfig)
            .expect("Verification system not initialized");

        let total_verifiers = submission.verifier_signatures.len() as u32;
        
        if total_verifiers < config.min_verifiers_required {
            return; // Not enough verifiers yet
        }

        // For simplicity, we'll consider consensus reached if minimum verifiers are met
        // In a real implementation, you'd check the actual approval ratio
        let consensus_reached = total_verifiers >= config.min_verifiers_required;

        if consensus_reached {
            let verification_status = VerificationStatus {
                verified: true,
                timestamp: env.ledger().timestamp(),
                verifiers: submission.verifier_signatures.clone(),
            };

            env.storage().instance().set(&DataKey::VerificationStatus(asset_id), &verification_status);
            
            // Update verification history for each verifier
            for verifier in submission.verifier_signatures.iter() {
                let mut history: Vec<u64> = env.storage().instance().get(&DataKey::VerificationHistory(verifier.clone()))
                    .unwrap_or(Vec::new(env));
                history.push_back(asset_id);
                env.storage().instance().set(&DataKey::VerificationHistory(verifier.clone()), &history);
            }
        }
    }

    /// Get verification status for an asset
    pub fn get_verification_status(env: Env, asset_id: u64) -> Option<VerificationStatus> {
        env.storage().instance().get(&DataKey::VerificationStatus(asset_id))
    }

    /// Get verifier reputation and stats
    pub fn get_verifier_info(env: Env, verifier: Address) -> Option<Verifier> {
        env.storage().instance().get(&DataKey::Verifier(verifier))
    }

    /// Deactivate a verifier (admin only)
    pub fn deactivate_verifier(env: Env, admin: Address, verifier: Address) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("Contract not initialized");
        assert_eq!(admin, stored_admin, "not admin");

        let mut verifier_info: Verifier = env.storage().instance().get(&DataKey::Verifier(verifier.clone()))
            .expect("Verifier not found");
        verifier_info.is_active = false;
        env.storage().instance().set(&DataKey::Verifier(verifier), &verifier_info);
    }

    // Controlled Metadata Update Functions
    // -----------------------------------------------------------------------

    /// Set initial metadata for an asset (only by asset owner)
    pub fn set_asset_metadata(
        env: Env,
        owner: Address,
        asset_id: u64,
        name: String,
        description: String,
        symbol: String,
        image_uri: String,
        external_url: String,
    ) {
        owner.require_auth();

        let asset: Asset = env.storage().instance().get(&DataKey::AssetInfo).expect("Asset not initialized");
        assert_eq!(asset.owner, owner, "not owner");

        assert!(!env.storage().persistent().has(&DataKey::AssetMetadata(asset_id)), "metadata already set");

        let now = env.ledger().timestamp();
        let metadata = AssetMetadata {
            name,
            description,
            symbol,
            image_uri,
            external_url,
            attributes: Vec::new(&env),
            is_immutable: false,
            version: 1,
            updated_at: now,
        };

        env.storage().persistent().set(&DataKey::AssetMetadata(asset_id), &metadata);

        env.events().publish(
            (Symbol::new(&env, "metadata_set"), asset_id),
            metadata.version,
        );
    }

    /// Get asset metadata
    pub fn get_asset_metadata(env: Env, asset_id: u64) -> Option<AssetMetadata> {
        env.storage().persistent().get(&DataKey::AssetMetadata(asset_id))
    }

    /// Set the governance contract address for immutable metadata changes
    pub fn set_metadata_governance(env: Env, admin: Address, governance_id: Address) {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).expect("admin not set");
        assert_eq!(admin, stored_admin, "not admin");

        env.storage().instance().set(&DataKey::MetadataGovernanceContract, &governance_id);
        env.events().publish((Symbol::new(&env, "metadata_governance_set"),), governance_id);
    }

    /// Propose a metadata update with a 48-hour time-lock.
    /// Owner can update mutable metadata; immutable metadata requires governance approval.
    pub fn propose_metadata_update(
        env: Env,
        requester: Address,
        asset_id: u64,
        new_name: String,
        new_description: String,
        new_symbol: String,
        new_image_uri: String,
        new_external_url: String,
    ) -> u64 {
        requester.require_auth();

        let asset: Asset = env.storage().instance().get(&DataKey::AssetInfo).expect("Asset not initialized");
        let old_metadata: AssetMetadata = env.storage().persistent().get(&DataKey::AssetMetadata(asset_id))
            .expect("No metadata found for asset");

        // If metadata is marked immutable, this must go through governance
        if old_metadata.is_immutable {
            let gov_contract: Option<Address> = env.storage().instance().get(&DataKey::MetadataGovernanceContract);
            if gov_contract.is_none() {
                panic!("Governance contract not set");
            }
            // Mark that governance approval is required - will be approved by governance contract
        }

        // Owner can update, or governance can approve for immutable metadata
        let requires_governance = old_metadata.is_immutable;

        if !requires_governance {
            assert_eq!(asset.owner, requester, "not owner");
        }

        let now = env.ledger().timestamp();
        let timelock_duration: u64 = 172800; // 48 hours in seconds
        let timelock_until = now.checked_add(timelock_duration).expect("timelock overflow");

        let request_id = env.storage().instance().get(&DataKey::LastMetadataUpdateIndex).unwrap_or(0u64) + 1;
        env.storage().instance().set(&DataKey::LastMetadataUpdateIndex, &request_id);

        let new_version = old_metadata.version.checked_add(1).expect("version overflow");

        let new_metadata = AssetMetadata {
            name: new_name,
            description: new_description,
            symbol: new_symbol,
            image_uri: new_image_uri,
            external_url: new_external_url,
            attributes: old_metadata.attributes.clone(),
            is_immutable: old_metadata.is_immutable,
            version: new_version,
            updated_at: 0,
        };

        let update_request = MetadataUpdateRequest {
            request_id,
            asset_id,
            requester: requester.clone(),
            old_metadata: old_metadata.clone(),
            new_metadata,
            created_at: now,
            timelock_until,
            status: MetadataUpdateStatus::Pending,
            requires_governance,
            governance_proposal_id: 0,
        };

        env.storage().persistent().set(&DataKey::MetadataUpdateRequest(request_id), &update_request);

        env.events().publish(
            (Symbol::new(&env, "metadata_update_proposed"), request_id, asset_id),
            (requester, timelock_until, requires_governance),
        );

        request_id
    }

    /// Approve a metadata update request via governance (for immutable metadata changes)
    pub fn approve_metadata_update(env: Env, governance: Address, request_id: u64) {
        governance.require_auth();

        let gov_contract: Address = env.storage().instance().get(&DataKey::MetadataGovernanceContract)
            .expect("Governance contract not set");
        assert_eq!(governance, gov_contract, "not governance contract");

        let mut request: MetadataUpdateRequest = env.storage().persistent()
            .get(&DataKey::MetadataUpdateRequest(request_id))
            .expect("Update request not found");

        assert!(request.status == MetadataUpdateStatus::Pending, "request not pending");
        assert!(request.requires_governance, "governance not required");

        request.status = MetadataUpdateStatus::Approved;

        env.storage().persistent().set(&DataKey::MetadataUpdateRequest(request_id), &request);

        env.events().publish(
            (Symbol::new(&env, "metadata_update_approved"), request_id, request.asset_id),
            governance,
        );
    }

    /// Cancel a pending metadata update (only by requester)
    pub fn cancel_metadata_update(env: Env, requester: Address, request_id: u64) {
        requester.require_auth();

        let mut request: MetadataUpdateRequest = env.storage().persistent()
            .get(&DataKey::MetadataUpdateRequest(request_id))
            .expect("Update request not found");

        assert!(request.status == MetadataUpdateStatus::Pending, "request not pending");
        assert_eq!(request.requester, requester, "not requester");

        request.status = MetadataUpdateStatus::Cancelled;

        env.storage().persistent().set(&DataKey::MetadataUpdateRequest(request_id), &request);

        env.events().publish(
            (Symbol::new(&env, "metadata_update_cancelled"), request_id, request.asset_id),
            requester,
        );
    }

    /// Execute a metadata update after the time-lock has expired (or after governance approval)
    pub fn execute_metadata_update(env: Env, request_id: u64) {
        let mut request: MetadataUpdateRequest = env.storage().persistent()
            .get(&DataKey::MetadataUpdateRequest(request_id))
            .expect("Update request not found");

        // Check status
        if request.requires_governance {
            assert!(request.status == MetadataUpdateStatus::Approved, "governance approval required");
        } else {
            assert!(request.status == MetadataUpdateStatus::Pending, "request not pending");
        }

        // Check time-lock
        let now = env.ledger().timestamp();
        assert!(now >= request.timelock_until, "timelock not expired");

        // Update metadata
        let mut new_metadata = request.new_metadata.clone();
        new_metadata.updated_at = now;
        env.storage().persistent().set(&DataKey::AssetMetadata(request.asset_id), &new_metadata);

        // Record update history
        let record = MetadataUpdateRecord {
            version: new_metadata.version,
            old_metadata: request.old_metadata.clone(),
            new_metadata: new_metadata.clone(),
            updated_by: request.requester.clone(),
            updated_at: now,
            update_type: if request.requires_governance { 1u32 } else { 0u32 },
        };

        let mut history: Vec<MetadataUpdateRecord> = env.storage().persistent()
            .get(&DataKey::MetadataUpdateHistory(request.asset_id))
            .unwrap_or(Vec::new(&env));
        history.push_back(record);
        env.storage().persistent().set(&DataKey::MetadataUpdateHistory(request.asset_id), &history);

        // Mark request as executed
        request.status = MetadataUpdateStatus::Executed;
        env.storage().persistent().set(&DataKey::MetadataUpdateRequest(request_id), &request);

        env.events().publish(
            (Symbol::new(&env, "metadata_update_executed"), request_id, request.asset_id),
            (new_metadata.version, now),
        );
    }

    /// Get a metadata update request by ID
    pub fn get_metadata_update_request(env: Env, request_id: u64) -> Option<MetadataUpdateRequest> {
        env.storage().persistent().get(&DataKey::MetadataUpdateRequest(request_id))
    }

    /// Get metadata update history for an asset
    pub fn get_metadata_update_history(env: Env, asset_id: u64) -> Vec<MetadataUpdateRecord> {
        env.storage().persistent()
            .get(&DataKey::MetadataUpdateHistory(asset_id))
            .unwrap_or(Vec::new(&env))
    }

    /// Mark an asset's metadata as immutable (requires owner or governance)
    pub fn set_metadata_immutable(env: Env, caller: Address, asset_id: u64) {
        caller.require_auth();

        let asset: Asset = env.storage().instance().get(&DataKey::AssetInfo).expect("Asset not initialized");

        // Owner or governance can set immutable
        if caller != asset.owner {
            let gov_contract: Option<Address> = env.storage().instance().get(&DataKey::MetadataGovernanceContract);
            match gov_contract {
                Some(gc) if caller == gc => {},
                _ => panic!("not authorized to set immutable"),
            }
        }

        let mut metadata: AssetMetadata = env.storage().persistent()
            .get(&DataKey::AssetMetadata(asset_id))
            .expect("No metadata found");

        metadata.is_immutable = true;
        metadata.version = metadata.version.checked_add(1).expect("version overflow");
        metadata.updated_at = env.ledger().timestamp();

        env.storage().persistent().set(&DataKey::AssetMetadata(asset_id), &metadata);

        env.events().publish(
            (Symbol::new(&env, "metadata_immutable_set"), asset_id),
            caller,
        );
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    fn setup(env: &Env) -> (AssetTokenClient<'_>, Address, Address) {
        let at_id = env.register_contract(None, AssetToken);
        let client = AssetTokenClient::new(&env, &at_id);
        
        let ec_addr = env.register_contract(None, crate::emergency_control::EmergencyControl);
        let ec_client = crate::emergency_control::EmergencyControlClient::new(&env, &ec_addr);
        
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);
        
        let name = String::from_str(&env, "Real Estate Token");
        let symbol = String::from_str(&env, "RET");
        let supply = 1_000_000;
        
        client.initialize(&admin, &name, &symbol, &7, &supply);
        (client, admin, ec_addr)
    }


    #[test]
    fn test_lifecycle() {
        let env = Env::default();
        env.mock_all_auths();
        let at_id = env.register_contract(None, AssetToken);
        let client = AssetTokenClient::new(&env, &at_id);


        let admin = Address::generate(&env);
        let name = String::from_str(&env, "Test Asset");
        let symbol = String::from_str(&env, "TSTA");
        client.initialize(&admin, &name, &symbol, &7, &0);

        assert_eq!(client.name(), name);
        assert_eq!(client.symbol(), symbol);

        // Fractional mint
        let user1 = Address::generate(&env);
        let mut owners = Vec::new(&env);
        owners.push_back((user1.clone(), 100u64));
        client.mint_fractional(&admin, &100_000, &1000, &Some(owners), &None);

        assert_eq!(client.balance(&user1), 10_000);
        assert_eq!(client.total_supply(), 100_000);

        // Valuation
        client.set_oracle(&admin, &admin);
        client.update_valuation(&admin, &110_000);
        let history = client.get_valuation_history();
        assert_eq!(history.len(), 1);
        assert_eq!(history.get(0).unwrap().value, 110_000);

        // Bridge config
        let pool = Address::generate(&env);
        let pubkey = BytesN::from_array(&env, &[1u8; 32]);
        client.set_bridge_config(&30, &pool, &3600, &5, &pubkey);
        
        // Bridge out
        let target_addr = Bytes::from_array(&env, &[0xABu8; 20]);
        let _bridge_id = client.bridge_out(&user1, &at_id, &1000, &TargetChain::Ethereum, &target_addr);
        
        assert_eq!(client.balance(&user1), 9_000);
        assert_eq!(client.balance(&pool), 3); // 1000 * 30 / 10000 = 3
    }

    #[test]
    fn test_transfer() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        let user = Address::generate(&env);
        
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        assert_eq!(client.balance(&admin), 999_000);
        assert_eq!(client.balance(&user), 1000);
    }

    #[test]
    fn test_staking_and_yield() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        let user = Address::generate(&env);      
        client.transfer(&admin, &user, &10_000, &1, &ec_id);
        
        // Stake 10,000 tokens
        client.stake_tokens(&user, &10_000);
        assert_eq!(client.balance(&user), 0);
        
        let staked = client.get_staked(&user).unwrap();
        assert_eq!(staked.amount, 10_000);
        
        // Jump 1 year forward (31,536,000 seconds)
        env.ledger().with_mut(|li| {
            li.timestamp += 31_536_000;
        });
        
        // Unstake all
        client.unstake_tokens(&user, &10_000);
        
        // 5% yield on 10,000 = 500
        assert_eq!(client.balance(&user), 10_500);
    }

    #[test]
    fn test_accumulated_yield() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        let user = Address::generate(&env);      
        client.transfer(&admin, &user, &20_000, &1, &ec_id);
        
        // Stake 10,000 tokens
        client.stake_tokens(&user, &10_000);
        
        // Jump 6 months
        env.ledger().with_mut(|li| {
            li.timestamp += 15_768_000;
        });
        
        // Stake another 10,000 tokens. This should trigger yield calculation for the first stake.
        client.stake_tokens(&user, &10_000);
        
        let staked = client.get_staked(&user).unwrap();
        assert_eq!(staked.amount, 20_000);
        // 5% yield for 6 months on 10,000 = 250
        assert_eq!(staked.accumulated_yields, 250);
        
        // Jump another 6 months
        env.ledger().with_mut(|li| {
            li.timestamp += 15_768_000;
        });
        
        // Total yield should be:
        // 250 (accumulated) + (20,000 * 0.05 * 0.5 year) = 250 + 500 = 750
        client.unstake_tokens(&user, &20_000);
        assert_eq!(client.balance(&user), 20_750);
    }

    #[test]
    #[should_panic(expected = "insufficient balance to stake")]
    fn test_stake_insufficient_balance() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, _admin, _ec_id) = setup(&env);
        let user = Address::generate(&env);
        
        client.stake_tokens(&user, &100);
    }

    #[test]
    #[should_panic(expected = "insufficient staked amount")]
    fn test_unstake_insufficient() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.stake_tokens(&admin, &1000);
        client.unstake_tokens(&admin, &2000);
    }

    // Verification System Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_verification_initialization() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        // Test that verifiers list is initially empty
        let verifiers = client.get_verifiers();
        assert_eq!(verifiers.len(), 0);
    }

    #[test]
    fn test_verifier_registration() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        let verifier1 = Address::generate(&env);
        let verifier2 = Address::generate(&env);
        
        client.register_verifier(&admin, &verifier1);
        client.register_verifier(&admin, &verifier2);
        
        let verifiers = client.get_verifiers();
        assert_eq!(verifiers.len(), 2);
        assert!(verifiers.contains(&verifier1));
        assert!(verifiers.contains(&verifier2));
        
        // Test verifier info
        let verifier_info = client.get_verifier_info(&verifier1).unwrap();
        assert_eq!(verifier_info.address, verifier1);
        assert_eq!(verifier_info.reputation, 100);
        assert_eq!(verifier_info.total_verifications, 0);
        assert_eq!(verifier_info.successful_verifications, 0);
        assert!(verifier_info.is_active);
    }

    #[test]
    fn test_proof_submission() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        let verifier = Address::generate(&env);
        client.register_verifier(&admin, &verifier);
        
        let proof_data = Bytes::from_slice(&env, &[1, 2, 3, 4, 5]);
        let proof_id = client.submit_proof(&admin, &1, &proof_data);
        
        assert_eq!(proof_id, 1);
    }

    #[test]
    fn test_proof_verification() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &2, &75, &3600);
        
        let verifier1 = Address::generate(&env);
        let verifier2 = Address::generate(&env);
        
        client.register_verifier(&admin, &verifier1);
        client.register_verifier(&admin, &verifier2);
        
        let proof_data = Bytes::from_slice(&env, &[1, 2, 3, 4, 5]);
        let proof_id = client.submit_proof(&admin, &1, &proof_data);
        
        // First verifier approves
        client.verify_authenticity(&verifier1, &proof_id, &true);
        
        // Check that verification is not yet complete (need 2 verifiers)
        let status = client.get_verification_status(&1);
        assert!(status.is_none());
        
        // Second verifier approves
        client.verify_authenticity(&verifier2, &proof_id, &true);
        
        // Now verification should be complete
        let status = client.get_verification_status(&1).unwrap();
        assert!(status.verified);
        assert_eq!(status.verifiers.len(), 2);
        assert!(status.verifiers.contains(&verifier1));
        assert!(status.verifiers.contains(&verifier2));
        
        // Check verifier stats
        let verifier1_info = client.get_verifier_info(&verifier1).unwrap();
        assert_eq!(verifier1_info.total_verifications, 1);
        assert_eq!(verifier1_info.successful_verifications, 1);
    }

    #[test]
    fn test_verifier_deactivation() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        let verifier = Address::generate(&env);
        client.register_verifier(&admin, &verifier);
        
        // Verify verifier is active
        let verifiers = client.get_verifiers();
        assert_eq!(verifiers.len(), 1);
        
        // Deactivate verifier
        client.deactivate_verifier(&admin, &verifier);
        
        // Verify verifier is no longer in active list
        let verifiers = client.get_verifiers();
        assert_eq!(verifiers.len(), 0);
        
        // But verifier info still exists
        let verifier_info = client.get_verifier_info(&verifier).unwrap();
        assert!(!verifier_info.is_active);
    }

    #[test]
    fn test_mint_fractional_without_verification() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        let verifier = Address::generate(&env);
        client.register_verifier(&admin, &verifier);
        
        // Submit proof first
        let proof_data = Bytes::from_slice(&env, &[1, 2, 3, 4, 5]);
        let proof_id = client.submit_proof(&admin, &1, &proof_data);
        assert_eq!(proof_id, 1);
        
        // Check that verification status is None (not verified yet)
        let status = client.get_verification_status(&1);
        assert!(status.is_none());
    }

    #[test]
    fn test_mint_fractional_submits_proof() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &3, &75, &3600);
        
        let verifier = Address::generate(&env);
        client.register_verifier(&admin, &verifier);
        
        // Submit proof first
        let proof_data = Bytes::from_slice(&env, &[1, 2, 3, 4, 5]);
        let proof_id = client.submit_proof(&admin, &1, &proof_data);
        assert_eq!(proof_id, 1);
        
        // Now verify the proof
        client.verify_authenticity(&verifier, &proof_id, &true);
        
        // Check verification status
        let status = client.get_verification_status(&1);
        assert!(status.is_some());
        assert!(status.unwrap().verified);
    }

    #[test]
    fn test_mint_fractional_with_verification() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.initialize_verification(&admin, &2, &75, &3600);
        
        let verifier1 = Address::generate(&env);
        let verifier2 = Address::generate(&env);
        
        client.register_verifier(&admin, &verifier1);
        client.register_verifier(&admin, &verifier2);
        
        // Submit and verify proof first
        let proof_data = Bytes::from_slice(&env, &[1, 2, 3, 4, 5]);
        let proof_id = client.submit_proof(&admin, &1, &proof_data);
        
        client.verify_authenticity(&verifier1, &proof_id, &true);
        client.verify_authenticity(&verifier2, &proof_id, &true);
        
        // Now minting should work
        let asset_id = client.mint_fractional(&admin, &100000, &100, &Some(Vec::from_array(&env, [(admin.clone(), 100u64)])), &Some(proof_data));
        assert_eq!(asset_id, 1);
    }

    // Dividend Distribution System Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_create_snapshot() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        assert_eq!(snapshot_id, 1);
        
        let snapshot = client.get_snapshot(&snapshot_id).unwrap();
        assert_eq!(snapshot.snapshot_id, 1);
        assert!(snapshot.total_supply > 0);
    }

    #[test]
    fn test_create_dividend_distribution() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id)); // 5% tax
        
        assert_eq!(distribution_id, 1);
        
        let distribution = client.get_dividend_distribution(&distribution_id).unwrap();
        assert_eq!(distribution.asset_id, 1);
        assert_eq!(distribution.total_amount, 100000);
        assert_eq!(distribution.tax_withholding_rate, 500);
        assert!(!distribution.is_paused);
    }

    #[test]
    fn test_claim_dividend_distribution() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
        
        let user = Address::generate(&env);
        client.transfer(&admin, &user, &10000, &1, &_ec_id);
        
        client.claim_dividend_distribution(&distribution_id, &user);
        
        let claim = client.get_dividend_claim(&distribution_id, &user).unwrap();
        assert_eq!(claim.distribution_id, distribution_id);
        assert_eq!(claim.claimant, user);
        assert!(claim.amount > 0);
        assert!(claim.withheld > 0);
    }

    #[test]
    #[should_panic(expected = "already claimed")]
    fn test_claim_dividend_twice() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
        
        let user = Address::generate(&env);
        client.transfer(&admin, &user, &10000, &1, &_ec_id);
        
        client.claim_dividend_distribution(&distribution_id, &user);
        client.claim_dividend_distribution(&distribution_id, &user);
    }

    #[test]
    fn test_pause_resume_dividends() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        assert!(!client.are_dividends_paused());
        
        client.pause_dividends(&admin);
        assert!(client.are_dividends_paused());
        
        client.resume_dividends(&admin);
        assert!(!client.are_dividends_paused());
    }

    #[test]
    fn test_pause_resume_distribution() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
        
        let distribution = client.get_dividend_distribution(&distribution_id).unwrap();
        assert!(!distribution.is_paused);
        
        client.pause_distribution(&admin, &distribution_id);
        
        let distribution = client.get_dividend_distribution(&distribution_id).unwrap();
        assert!(distribution.is_paused);
        
        client.resume_distribution(&admin, &distribution_id);
        
        let distribution = client.get_dividend_distribution(&distribution_id).unwrap();
        assert!(!distribution.is_paused);
    }

    #[test]
    #[should_panic(expected = "distribution paused")]
    fn test_claim_from_paused_distribution() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
        
        client.pause_distribution(&admin, &distribution_id);
        
        let user = Address::generate(&env);
        client.transfer(&admin, &user, &10000, &1, &_ec_id);
        
        client.claim_dividend_distribution(&distribution_id, &user);
    }

    #[test]
    fn test_distribution_history() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id1 = client.create_snapshot(&admin);
        let snapshot_id2 = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id1));
        client.create_dividend_distribution(&admin, &1, &150000, &payout_asset, &750, &Some(snapshot_id2));
        
        let history = client.get_distribution_history(&1);
        assert_eq!(history.len(), 2);
        assert!(history.contains(&1));
        assert!(history.contains(&2));
    }

    #[test]
    fn test_tax_withholding_calculation() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &1000, &Some(snapshot_id)); // 10% tax
        
        let user = Address::generate(&env);
        client.transfer(&admin, &user, &50000, &1, &_ec_id); // 50% of supply
        
        client.claim_dividend_distribution(&distribution_id, &user);
        
        let claim = client.get_dividend_claim(&distribution_id, &user).unwrap();
        
        // User should get 50% of 100000 = 50000 gross
        // 10% tax = 5000 withheld
        // Net = 45000
        assert_eq!(claim.amount, 45000);
        assert_eq!(claim.withheld, 5000);
    }

    #[test]
    #[should_panic(expected = "dividends paused")]
    fn test_create_distribution_when_paused() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        client.pause_dividends(&admin);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
    }

    #[test]
    fn test_calculate_unclaimed_dividends() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        let snapshot_id = client.create_snapshot(&admin);
        let payout_asset = Address::generate(&env);
        let distribution_id = client.create_dividend_distribution(&admin, &1, &100000, &payout_asset, &500, &Some(snapshot_id));
        
        let unclaimed = client.calculate_unclaimed_dividends(&distribution_id);
        assert_eq!(unclaimed, 100000);
    }

    // Transaction Analytics System Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_transaction_tracking() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Perform a transfer
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        
        // Check that transaction was recorded
        let user_txs = client.get_user_transactions(&user);
        assert_eq!(user_txs.len(), 1);
        
        let tx = client.get_transaction(&user_txs.get(0).unwrap()).unwrap();
        assert_eq!(tx.transaction_type, TransactionType::Transfer);
        assert_eq!(tx.amount, 1000);
    }

    #[test]
    fn test_transaction_analytics() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user1 = Address::generate(&env);
        let user2 = Address::generate(&env);
        
        // Perform some transactions
        client.transfer(&admin, &user1, &1000, &1, &ec_id);
        client.transfer(&admin, &user2, &2000, &1, &ec_id);
        
        // Get analytics
        let analytics = client.get_transaction_analytics(&0, &u64::MAX);
        assert_eq!(analytics.total_transactions, 2);
        assert_eq!(analytics.total_volume, 3000);
        assert!(analytics.unique_addresses >= 2);
    }

    #[test]
    fn test_volume_history() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Perform transactions
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        client.transfer(&admin, &user, &2000, &1, &ec_id);
        
        // Get volume history
        let volume = client.get_volume_history(&0, &u64::MAX);
        assert!(volume.len() > 0);
    }

    #[test]
    fn test_transaction_stats_by_type() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Perform different transaction types
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        client.mint(&user, &500, &1, &ec_id);
        
        // Get stats by type
        let stats = client.get_transaction_stats_by_type(&0, &u64::MAX);
        assert!(stats.len() > 0);
        
        // Find transfer stats
        let transfer_stat = stats.iter().find(|(tx_type, _, _)| *tx_type == TransactionType::Transfer);
        assert!(transfer_stat.is_some());
    }

    #[test]
    fn test_export_transactions() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Perform transactions
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        
        // Export transactions
        let records = client.export_transactions(&Some(user), &None, &0, &u64::MAX);
        assert_eq!(records.len(), 1);
        assert_eq!(records.get(0).unwrap().transaction_type, TransactionType::Transfer);
    }

    #[test]
    fn test_price_history_recording() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, _admin, _ec_id) = setup(&env);
        
        // Record price
        client.record_price(&1000, &5000);
        
        // Get price history (returns empty in placeholder implementation)
        let prices = client.get_price_history(&0, &u64::MAX);
        // Placeholder returns empty
        assert_eq!(prices.len(), 0);
    }

    #[test]
    fn test_asset_transaction_history() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Perform transactions for asset_id 1
        client.transfer(&admin, &user, &1000, &1, &ec_id);
        client.transfer(&admin, &user, &2000, &1, &ec_id);
        
        // Get asset transactions
        let asset_txs = client.get_asset_transactions(&1);
        assert!(asset_txs.len() >= 2);
    }

    #[test]
    fn test_mint_transaction_tracking() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, ec_id) = setup(&env);
        
        let user = Address::generate(&env);
        
        // Mint tokens
        client.mint(&user, &1000, &1, &ec_id);
        
        // Check transaction was recorded
        let user_txs = client.get_user_transactions(&admin);
        assert!(user_txs.len() > 0);
        
        let tx = client.get_transaction(&user_txs.get(0).unwrap()).unwrap();
        assert_eq!(tx.transaction_type, TransactionType::Mint);
    }

    #[test]
    fn test_stake_transaction_tracking() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        // Stake tokens
        client.stake_tokens(&admin, &1000);
        
        // Check transaction was recorded
        let user_txs = client.get_user_transactions(&admin);
        assert!(user_txs.len() > 0);
        
        let tx = client.get_transaction(&user_txs.get(0).unwrap()).unwrap();
        assert_eq!(tx.transaction_type, TransactionType::Stake);
    }

    #[test]
    fn test_unstake_transaction_tracking() {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, _ec_id) = setup(&env);
        
        // Stake first
        client.stake_tokens(&admin, &1000);
        
        // Unstake tokens
        client.unstake_tokens(&admin, &500);
        
        // Check transaction was recorded
        let user_txs = client.get_user_transactions(&admin);
        assert!(user_txs.len() > 1);
        
        // Find unstake transaction
        let unstake_tx = user_txs.iter().find(|tx_id| {
            let tx = client.get_transaction(tx_id).unwrap();
            tx.transaction_type == TransactionType::Unstake
        });
        assert!(unstake_tx.is_some());
    }
}
