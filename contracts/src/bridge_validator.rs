use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Vec, BytesN, Bytes, String};

// ---------------------------------------------------------------------------
// Validator Status
// ---------------------------------------------------------------------------

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[repr(u32)]
pub enum ValidatorStatus {
    Active = 0,
    Inactive = 1,
    Slashed = 2,
    Pending = 3,
}

impl ValidatorStatus {
    pub fn from_u32(val: u32) -> Option<Self> {
        match val {
            0 => Some(ValidatorStatus::Active),
            1 => Some(ValidatorStatus::Inactive),
            2 => Some(ValidatorStatus::Slashed),
            3 => Some(ValidatorStatus::Pending),
            _ => None,
        }
    }
}

// ---------------------------------------------------------------------------
// Validator Info
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct ValidatorInfo {
    pub address: Address,
    pub status: u32, // ValidatorStatus
    pub stake: i128,
    pub signed_count: u64,
    pub slash_count: u64,
    pub joined_at: u64,
    pub last_active: u64,
    pub reputation_score: u32, // 0-100
}

// ---------------------------------------------------------------------------
// Cross-Chain Bridge Transfer
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct BridgeTransfer {
    pub id: BytesN<32>,
    pub asset_id: u64,
    pub source_chain: String,
    pub dest_chain: String,
    pub amount: i128,
    pub sender: Address,
    pub recipient: Address,
    pub timestamp: u64,
    pub status: u32, // 0=pending, 1=approved, 2=executed, 3=failed
    pub approvals: u32,
    pub signatures: Vec<Address>,
    pub fraud_submitted: bool,
}

// ---------------------------------------------------------------------------
// Fraud Proof
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct FraudProof {
    pub transfer_id: BytesN<32>,
    pub proof_data: Bytes,
    pub submitter: Address,
    pub timestamp: u64,
    pub verified: bool,
}

// ---------------------------------------------------------------------------
// Bridge Fee Config
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct BridgeFeeConfig {
    pub base_fee: i128,
    pub fee_percentage: u32, // basis points (100 = 1%)
    pub max_fee_cap: i128,
    pub fee_token: Address,
}

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum BridgeValidatorKey {
    /// Admin address
    Admin,
    /// Minimum validators required for consensus
    MinValidators,
    /// Validators list
    Validators(u32), // index
    /// Validator info by address
    ValidatorInfo(Address),
    /// Validator count
    ValidatorCount,
    /// Bridge transfers
    Transfer(BytesN<32>),
    /// Transfer IDs
    TransferIds(u32), // index
    /// Transfer count
    TransferCount,
    /// Fraud proofs
    FraudProof(BytesN<32>),
    /// Bridge fee config
    FeeConfig,
    /// Total fees collected
    FeesCollected,
    /// Supported chains
    SupportedChains(u32), // index
    /// Supported chain count
    SupportedChainCount,
    /// Bridge enabled flag
    Enabled,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct BridgeValidator;

#[contractimpl]
impl BridgeValidator {
    /// Initialize the bridge validator contract
    pub fn initialize(
        env: Env,
        admin: Address,
        validators: Vec<Address>,
        min_validators: u32,
        fee_config: BridgeFeeConfig,
    ) {
        admin.require_auth();

        if env.storage().instance().has(&BridgeValidatorKey::Admin) {
            panic!("already initialized");
        }

        if min_validators == 0 || min_validators > (validators.len() as u32) {
            panic!("invalid minimum validators");
        }

        env.storage().instance().set(&BridgeValidatorKey::Admin, &admin);
        env.storage().instance().set(&BridgeValidatorKey::MinValidators, &min_validators);
        env.storage().instance().set(&BridgeValidatorKey::FeeConfig, &fee_config);
        env.storage().instance().set(&BridgeValidatorKey::ValidatorCount, &(validators.len() as u32));
        env.storage().instance().set(&BridgeValidatorKey::FeesCollected, &0i128);
        env.storage().instance().set(&BridgeValidatorKey::Enabled, &true);
        env.storage().instance().set(&BridgeValidatorKey::SupportedChainCount, &3u32);

        let current_time = env.ledger().timestamp();

        // Initialize validators
        for (i, validator) in validators.iter().enumerate() {
            let info = ValidatorInfo {
                address: validator.clone(),
                status: ValidatorStatus::Active as u32,
                stake: 0i128,
                signed_count: 0,
                slash_count: 0,
                joined_at: current_time,
                last_active: current_time,
                reputation_score: 50,
            };

            env.storage().instance().set(&BridgeValidatorKey::ValidatorInfo(validator.clone()), &info);
            env.storage().instance().set(&BridgeValidatorKey::Validators(i as u32), &validator);
        }
    }

    /// Add a validator to the validator set
    pub fn add_validator(env: Env, admin: Address, validator: Address, initial_stake: i128) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&BridgeValidatorKey::Admin).expect("not initialized");
        if admin != stored_admin {
            panic!("only admin can add validators");
        }

        if env.storage().instance().has(&BridgeValidatorKey::ValidatorInfo(validator.clone())) {
            panic!("validator already exists");
        }

        let info = ValidatorInfo {
            address: validator.clone(),
            status: ValidatorStatus::Pending as u32,
            stake: initial_stake,
            signed_count: 0,
            slash_count: 0,
            joined_at: env.ledger().timestamp(),
            last_active: env.ledger().timestamp(),
            reputation_score: 25,
        };

        let count: u32 = env.storage().instance().get(&BridgeValidatorKey::ValidatorCount).unwrap_or(0);
        env.storage().instance().set(&BridgeValidatorKey::Validators(count), &validator);
        env.storage().instance().set(&BridgeValidatorKey::ValidatorCount, &(count + 1));
        env.storage().instance().set(&BridgeValidatorKey::ValidatorInfo(validator), &info);
    }

    /// Approve a pending cross-chain transfer
    pub fn approve_transfer(env: Env, validator: Address, transfer_id: BytesN<32>) {
        validator.require_auth();

        let mut transfer: BridgeTransfer = env
            .storage()
            .persistent()
            .get(&BridgeValidatorKey::Transfer(transfer_id.clone()))
            .expect("transfer not found");

        let mut validator_info: ValidatorInfo = env
            .storage()
            .instance()
            .get(&BridgeValidatorKey::ValidatorInfo(validator.clone()))
            .expect("validator not found");

        if validator_info.status != ValidatorStatus::Active as u32 {
            panic!("validator not active");
        }

        // Check if already signed
        for sig in transfer.signatures.iter() {
            if sig == validator {
                panic!("already signed this transfer");
            }
        }

        transfer.signatures.push_back(validator.clone());
        transfer.approvals += 1;

        // Check if we have enough approvals
        let min_validators: u32 = env.storage().instance().get(&BridgeValidatorKey::MinValidators).unwrap_or(1);
        if transfer.approvals >= min_validators {
            transfer.status = 1; // approved
        }

        validator_info.signed_count += 1;
        validator_info.last_active = env.ledger().timestamp();

        env.storage().persistent().set(&BridgeValidatorKey::Transfer(transfer_id), &transfer);
        env.storage().instance().set(&BridgeValidatorKey::ValidatorInfo(validator), &validator_info);
    }

    /// Submit a fraud proof for a transfer (triggers slashing)
    pub fn submit_fraud_proof(
        env: Env,
        submitter: Address,
        transfer_id: BytesN<32>,
        proof_data: Bytes,
    ) {
        submitter.require_auth();

        let fraud_proof = FraudProof {
            transfer_id: transfer_id.clone(),
            proof_data: proof_data.clone(),
            submitter,
            timestamp: env.ledger().timestamp(),
            verified: false,
        };

        env.storage().persistent().set(&BridgeValidatorKey::FraudProof(transfer_id.clone()), &fraud_proof);

        // Mark transfer as failed
        let mut transfer: BridgeTransfer = env
            .storage()
            .persistent()
            .get(&BridgeValidatorKey::Transfer(transfer_id.clone()))
            .expect("transfer not found");

        transfer.status = 3; // failed
        transfer.fraud_submitted = true;

        env.storage().persistent().set(&BridgeValidatorKey::Transfer(transfer_id.clone()), &transfer);
    }

    /// Execute a validated transfer
    pub fn execute_transfer(env: Env, executor: Address, transfer_id: BytesN<32>) {
        executor.require_auth();

        let mut transfer: BridgeTransfer = env
            .storage()
            .persistent()
            .get(&BridgeValidatorKey::Transfer(transfer_id.clone()))
            .expect("transfer not found");

        if transfer.status != 1 {
            panic!("transfer not approved");
        }

        transfer.status = 2; // executed
        env.storage().persistent().set(&BridgeValidatorKey::Transfer(transfer_id), &transfer);
    }

    /// Get validator information
    pub fn get_validator(env: Env, validator: Address) -> Option<ValidatorInfo> {
        env.storage().instance().get(&BridgeValidatorKey::ValidatorInfo(validator))
    }

    /// Get transfer details
    pub fn get_transfer(env: Env, transfer_id: BytesN<32>) -> Option<BridgeTransfer> {
        env.storage().persistent().get(&BridgeValidatorKey::Transfer(transfer_id))
    }

    /// Get fraud proof for a transfer
    pub fn get_fraud_proof(env: Env, transfer_id: BytesN<32>) -> Option<FraudProof> {
        env.storage().persistent().get(&BridgeValidatorKey::FraudProof(transfer_id))
    }

    /// Slash a validator for misbehavior
    pub fn slash_validator(env: Env, admin: Address, validator: Address, slash_amount: i128) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&BridgeValidatorKey::Admin).expect("not initialized");
        if admin != stored_admin {
            panic!("only admin can slash validators");
        }

        let mut validator_info: ValidatorInfo = env
            .storage()
            .instance()
            .get(&BridgeValidatorKey::ValidatorInfo(validator.clone()))
            .expect("validator not found");

        validator_info.stake -= slash_amount;
        validator_info.slash_count += 1;

        // Deactivate if stake becomes zero
        if validator_info.stake <= 0 {
            validator_info.status = ValidatorStatus::Inactive as u32;
        }

        // Reduce reputation score
        if validator_info.reputation_score > 0 {
            validator_info.reputation_score = validator_info.reputation_score.saturating_sub(10);
        }

        env.storage().instance().set(&BridgeValidatorKey::ValidatorInfo(validator), &validator_info);
    }

    /// Add supported chain for cross-chain transfers
    pub fn add_supported_chain(env: Env, admin: Address, chain_name: String) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&BridgeValidatorKey::Admin).expect("not initialized");
        if admin != stored_admin {
            panic!("only admin can add chains");
        }

        let count: u32 = env.storage().instance().get(&BridgeValidatorKey::SupportedChainCount).unwrap_or(0);

        if count < 10 {
            // Max 10 chains
            env.storage().instance().set(&BridgeValidatorKey::SupportedChains(count), &chain_name);
            env.storage().instance().set(&BridgeValidatorKey::SupportedChainCount, &(count + 1));
        }
    }

    /// Get fee configuration
    pub fn get_fee_config(env: Env) -> Option<BridgeFeeConfig> {
        env.storage().instance().get(&BridgeValidatorKey::FeeConfig)
    }

    /// Update fee configuration
    pub fn update_fee_config(env: Env, admin: Address, fee_config: BridgeFeeConfig) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&BridgeValidatorKey::Admin).expect("not initialized");
        if admin != stored_admin {
            panic!("only admin can update fee config");
        }

        env.storage().instance().set(&BridgeValidatorKey::FeeConfig, &fee_config);
    }

    /// Enable/disable the bridge
    pub fn set_enabled(env: Env, admin: Address, enabled: bool) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&BridgeValidatorKey::Admin).expect("not initialized");
        if admin != stored_admin {
            panic!("only admin can toggle bridge");
        }

        env.storage().instance().set(&BridgeValidatorKey::Enabled, &enabled);
    }

    /// Get bridge enabled status
    pub fn is_enabled(env: Env) -> bool {
        env.storage().instance().get(&BridgeValidatorKey::Enabled).unwrap_or(false)
    }

    /// Get total fees collected
    pub fn get_total_fees(env: Env) -> i128 {
        env.storage().instance().get(&BridgeValidatorKey::FeesCollected).unwrap_or(0)
    }
}
