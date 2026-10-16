use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, String, Vec};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum MultiSigKey {
    Admin,
    Owners,
    Threshold,
    NextProposalId,
    Proposal(u64),
    Signature(u64, Address),
    /// Duration in seconds after which a proposal expires
    ProposalExpiry,
}

// ---------------------------------------------------------------------------
// Proposal Struct
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct MultiSigProposal {
    pub id: u64,
    pub to: Address,
    pub amount: i128,
    pub sign_count: u32,
    pub executed: bool,
    pub created_at: u64,
    /// Description for the proposal action
    pub description: String,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct MultiSigWalletContract;

#[contractimpl]
impl MultiSigWalletContract {
    pub fn initialize(env: Env, admin: Address, owners: Vec<Address>, threshold: u32) {
        if env.storage().instance().has(&MultiSigKey::Admin) {
            panic!("already initialized");
        }
        Self::require_distinct_owners(&owners);
        Self::validate_threshold(threshold, owners.len());
        admin.require_auth();

        env.storage().instance().set(&MultiSigKey::Admin, &admin);
        env.storage().instance().set(&MultiSigKey::Owners, &owners);
        env.storage().instance().set(&MultiSigKey::Threshold, &threshold);
        env.storage().instance().set(&MultiSigKey::NextProposalId, &1u64);
    }

    pub fn submit_proposal(env: Env, proposer: Address, to: Address, amount: i128, description: String) -> u64 {
        proposer.require_auth();
        Self::require_owner(&env, &proposer);

        let proposal_id: u64 = env
            .storage()
            .instance()
            .get(&MultiSigKey::NextProposalId)
            .unwrap_or(1);
        env.storage()
            .instance()
            .set(&MultiSigKey::NextProposalId, &(proposal_id.checked_add(1).unwrap()));

        let proposal = MultiSigProposal {
            id: proposal_id,
            to,
            amount,
            sign_count: 0,
            executed: false,
            created_at: env.ledger().timestamp(),
            description,
        };

        env.storage()
            .persistent()
            .set(&MultiSigKey::Proposal(proposal_id), &proposal);

        proposal_id
    }

    pub fn sign_proposal(env: Env, signer: Address, proposal_id: u64) {
        signer.require_auth();
        Self::require_owner(&env, &signer);

        if env
            .storage()
            .persistent()
            .has(&MultiSigKey::Signature(proposal_id, signer.clone()))
        {
            panic!("already signed");
        }

        let mut proposal: MultiSigProposal = env
            .storage()
            .persistent()
            .get(&MultiSigKey::Proposal(proposal_id))
            .expect("proposal not found");

        if proposal.executed {
            panic!("proposal already executed");
        }

        env.storage()
            .persistent()
            .set(&MultiSigKey::Signature(proposal_id, signer), &true);

        proposal.sign_count = proposal.sign_count.checked_add(1).unwrap();
        env.storage()
            .persistent()
            .set(&MultiSigKey::Proposal(proposal_id), &proposal);
    }

    pub fn revoke_signature(env: Env, signer: Address, proposal_id: u64) {
        signer.require_auth();
        Self::require_owner(&env, &signer);

        if !env
            .storage()
            .persistent()
            .has(&MultiSigKey::Signature(proposal_id, signer.clone()))
        {
            panic!("not signed");
        }

        let mut proposal: MultiSigProposal = env
            .storage()
            .persistent()
            .get(&MultiSigKey::Proposal(proposal_id))
            .expect("proposal not found");

        if proposal.executed {
            panic!("proposal already executed");
        }

        env.storage()
            .persistent()
            .remove(&MultiSigKey::Signature(proposal_id, signer));

        proposal.sign_count = proposal.sign_count.checked_sub(1).unwrap();
        env.storage()
            .persistent()
            .set(&MultiSigKey::Proposal(proposal_id), &proposal);
    }

    pub fn execute_proposal(env: Env, executor: Address, proposal_id: u64) {
        executor.require_auth();
        Self::require_owner(&env, &executor);

        let mut proposal: MultiSigProposal = env
            .storage()
            .persistent()
            .get(&MultiSigKey::Proposal(proposal_id))
            .expect("proposal not found");

        if proposal.executed {
            panic!("proposal already executed");
        }

        // Check proposal expiry
        let expiry_duration: u64 = env
            .storage()
            .instance()
            .get(&MultiSigKey::ProposalExpiry)
            .unwrap_or(0);
        if expiry_duration > 0 {
            let now = env.ledger().timestamp();
            if now > proposal.created_at + expiry_duration {
                panic!("proposal has expired");
            }
        }

        let threshold: u32 = env
            .storage()
            .instance()
            .get(&MultiSigKey::Threshold)
            .expect("not initialized");

        if proposal.sign_count < threshold {
            panic!("insufficient signatures");
        }

        proposal.executed = true;
        env.storage()
            .persistent()
            .set(&MultiSigKey::Proposal(proposal_id), &proposal);
    }

    /// Set the proposal expiry duration in seconds (0 means no expiry).
    pub fn set_proposal_expiry(env: Env, admin: Address, expiry_seconds: u64) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&MultiSigKey::Admin)
            .expect("not initialized");
        if admin != stored_admin {
            panic!("admin only");
        }
        env.storage()
            .instance()
            .set(&MultiSigKey::ProposalExpiry, &expiry_seconds);
    }

    /// Get the proposal expiry duration.
    pub fn get_proposal_expiry(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&MultiSigKey::ProposalExpiry)
            .unwrap_or(0)
    }

    pub fn get_proposal(env: Env, proposal_id: u64) -> Option<MultiSigProposal> {
        env.storage()
            .persistent()
            .get(&MultiSigKey::Proposal(proposal_id))
    }

    pub fn get_threshold(env: Env) -> u32 {
        env.storage()
            .instance()
            .get(&MultiSigKey::Threshold)
            .expect("not initialized")
    }

    pub fn get_owners(env: Env) -> Vec<Address> {
        env.storage()
            .instance()
            .get(&MultiSigKey::Owners)
            .expect("not initialized")
    }

    pub fn has_signed(env: Env, proposal_id: u64, signer: Address) -> bool {
        env.storage()
            .persistent()
            .has(&MultiSigKey::Signature(proposal_id, signer))
    }

    pub fn update_threshold(env: Env, admin: Address, new_threshold: u32) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&MultiSigKey::Admin)
            .expect("not initialized");
        if admin != stored_admin {
            panic!("admin only");
        }

        let owners: Vec<Address> = env
            .storage()
            .instance()
            .get(&MultiSigKey::Owners)
            .expect("not initialized");

        Self::validate_threshold(new_threshold, owners.len());

        env.storage()
            .instance()
            .set(&MultiSigKey::Threshold, &new_threshold);
    }

    /// Add a new owner. Only the admin may call this.
    ///
    /// Adding owners can only make the current threshold easier to satisfy, so
    /// the threshold itself is left untouched.
    pub fn add_owner(env: Env, admin: Address, new_owner: Address) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let mut owners: Vec<Address> = env
            .storage()
            .instance()
            .get(&MultiSigKey::Owners)
            .expect("not initialized");

        if owners.contains(&new_owner) {
            panic!("owner already exists");
        }

        owners.push_back(new_owner);
        env.storage()
            .instance()
            .set(&MultiSigKey::Owners, &owners);
    }

    /// Remove an owner. Only the admin may call this.
    ///
    /// The owner set can never shrink below the configured threshold, otherwise
    /// the wallet would become permanently unable to execute a proposal.
    pub fn remove_owner(env: Env, admin: Address, owner: Address) {
        admin.require_auth();
        Self::require_admin(&env, &admin);

        let owners: Vec<Address> = env
            .storage()
            .instance()
            .get(&MultiSigKey::Owners)
            .expect("not initialized");
        let threshold: u32 = env
            .storage()
            .instance()
            .get(&MultiSigKey::Threshold)
            .expect("not initialized");

        if !owners.contains(&owner) {
            panic!("owner not found");
        }

        if owners.len() <= threshold {
            panic!("cannot remove owner below threshold");
        }

        let mut updated: Vec<Address> = Vec::new(&env);
        for i in 0..owners.len() {
            let current = owners.get(i).unwrap();
            if current != owner {
                updated.push_back(current);
            }
        }

        Self::validate_threshold(threshold, updated.len());

        env.storage()
            .instance()
            .set(&MultiSigKey::Owners, &updated);
    }

    /// Validate a threshold against the current owner set size.
    ///
    /// Panics with a specific message so integrators can distinguish between the
    /// two failure modes:
    /// * `threshold must be greater than zero`
    /// * `threshold cannot exceed number of owners`
    fn validate_threshold(threshold: u32, owner_count: u32) {
        if threshold == 0 {
            panic!("threshold must be greater than zero");
        }
        if threshold > owner_count {
            panic!("threshold cannot exceed number of owners");
        }
    }

    /// Reject an owner list that contains the same address more than once.
    /// Duplicates would silently inflate the owner count and allow a threshold
    /// that looks safe but is satisfiable by a single address.
    fn require_distinct_owners(owners: &Vec<Address>) {
        for i in 0..owners.len() {
            for j in (i + 1)..owners.len() {
                if owners.get(i).unwrap() == owners.get(j).unwrap() {
                    panic!("duplicate owner");
                }
            }
        }
    }

    fn require_admin(env: &Env, addr: &Address) {
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&MultiSigKey::Admin)
            .expect("not initialized");
        if *addr != stored_admin {
            panic!("admin only");
        }
    }

    fn require_owner(env: &Env, addr: &Address) {
        let owners: Vec<Address> = env
            .storage()
            .instance()
            .get(&MultiSigKey::Owners)
            .expect("not initialized");
        for owner in owners.iter() {
            if owner == *addr {
                return;
            }
        }
        panic!("not an owner");
    }
}

// ===========================================================================
// Unit Tests
// ===========================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    fn setup_three(env: &Env) -> (Address, Address, Address, Address, Address) {
        let admin = Address::generate(env);
        let owner1 = Address::generate(env);
        let owner2 = Address::generate(env);
        let owner3 = Address::generate(env);
        let contract_id = env.register_contract(None, MultiSigWalletContract);

        let client = MultiSigWalletContractClient::new(env, &contract_id);
        let mut owners = Vec::new(env);
        owners.push_back(owner1.clone());
        owners.push_back(owner2.clone());
        owners.push_back(owner3.clone());
        client.initialize(&admin, &owners, &2);

        (contract_id, admin, owner1, owner2, owner3)
    }

    #[test]
    fn test_initialize() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        assert_eq!(client.get_threshold(), 2);
        let owners = client.get_owners();
        assert_eq!(owners.len(), 3);
        assert!(owners.contains(&owner1));
        assert!(owners.contains(&owner2));
    }

    #[test]
    #[should_panic(expected = "already initialized")]
    fn test_double_init_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let mut owners = Vec::new(&env);
        owners.push_back(owner1.clone());
        client.initialize(&admin, &owners, &1);
    }

    #[test]
    fn test_submit_sign_execute() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test transfer");
        let pid = client.submit_proposal(&owner1, &recipient, &1000, &desc);
        assert_eq!(pid, 1);

        let proposal = client.get_proposal(&pid).unwrap();
        assert_eq!(proposal.sign_count, 0);
        assert!(!proposal.executed);

        client.sign_proposal(&owner1, &pid);
        client.sign_proposal(&owner2, &pid);

        let proposal = client.get_proposal(&pid).unwrap();
        assert_eq!(proposal.sign_count, 2);

        client.execute_proposal(&owner1, &pid);

        let proposal = client.get_proposal(&pid).unwrap();
        assert!(proposal.executed);
    }

    #[test]
    #[should_panic(expected = "already signed")]
    fn test_double_sign_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test");
        let pid = client.submit_proposal(&owner1, &recipient, &500, &desc);

        client.sign_proposal(&owner1, &pid);
        client.sign_proposal(&owner1, &pid);
    }

    #[test]
    #[should_panic(expected = "insufficient signatures")]
    fn test_execute_insufficient_signatures_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test");
        let pid = client.submit_proposal(&owner1, &recipient, &500, &desc);

        client.sign_proposal(&owner1, &pid);
        client.execute_proposal(&owner1, &pid);
    }

    #[test]
    #[should_panic(expected = "not an owner")]
    fn test_non_owner_submit_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, _owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let outsider = Address::generate(&env);
        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test");
        client.submit_proposal(&outsider, &recipient, &500, &desc);
    }

    #[test]
    #[should_panic(expected = "not an owner")]
    fn test_non_owner_sign_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let outsider = Address::generate(&env);
        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test");
        let pid = client.submit_proposal(&owner1, &recipient, &500, &desc);
        client.sign_proposal(&outsider, &pid);
    }

    #[test]
    fn test_has_signed() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "test");
        let pid = client.submit_proposal(&owner1, &recipient, &500, &desc);

        assert!(!client.has_signed(&pid, &owner1));
        client.sign_proposal(&owner1, &pid);
        assert!(client.has_signed(&pid, &owner1));
        assert!(!client.has_signed(&pid, &owner2));
    }

    #[test]
    fn test_update_threshold() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        assert_eq!(client.get_threshold(), 2);
        client.update_threshold(&admin, &3);
        assert_eq!(client.get_threshold(), 3);
    }

    #[test]
    #[should_panic(expected = "admin only")]
    fn test_update_threshold_non_admin_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.update_threshold(&owner1, &1);
    }

    #[test]
    fn test_revoke_signature() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "revoke test");
        let pid = client.submit_proposal(&owner1, &recipient, &1000, &desc);

        client.sign_proposal(&owner1, &pid);
        assert!(client.has_signed(&pid, &owner1));

        client.revoke_signature(&owner1, &pid);
        assert!(!client.has_signed(&pid, &owner1));
    }

    #[test]
    #[should_panic(expected = "proposal has expired")]
    fn test_execute_expired_proposal_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owner1, owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        // Set expiry to 100 seconds
        client.set_proposal_expiry(&admin, &100);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "expiry test");
        let pid = client.submit_proposal(&owner1, &recipient, &500, &desc);

        client.sign_proposal(&owner1, &pid);
        client.sign_proposal(&owner2, &pid);

        // Advance time beyond expiry
        env.ledger().with_mut(|li| {
            li.timestamp += 200;
        });

        client.execute_proposal(&owner1, &pid);
    }

    #[test]
    fn test_proposal_expiry_set_and_get() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owner1, _owner2, _owner3) = setup_three(&env);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        assert_eq!(client.get_proposal_expiry(), 0);
        client.set_proposal_expiry(&admin, &3600);
        assert_eq!(client.get_proposal_expiry(), 3600);
    }

    // -----------------------------------------------------------------------
    // Threshold validation (#345)
    // -----------------------------------------------------------------------

    fn setup_with(env: &Env, owner_count: u32, threshold: u32) -> (Address, Address, Vec<Address>) {
        let admin = Address::generate(env);
        let contract_id = env.register_contract(None, MultiSigWalletContract);
        let client = MultiSigWalletContractClient::new(env, &contract_id);

        let mut owners = Vec::new(env);
        for _ in 0..owner_count {
            owners.push_back(Address::generate(env));
        }
        client.initialize(&admin, &owners, &threshold);

        (contract_id, admin, owners)
    }

    #[test]
    #[should_panic(expected = "threshold must be greater than zero")]
    fn test_initialize_zero_threshold_panics() {
        let env = Env::default();
        env.mock_all_auths();
        setup_with(&env, 3, 0);
    }

    #[test]
    #[should_panic(expected = "threshold cannot exceed number of owners")]
    fn test_initialize_threshold_above_owners_panics() {
        let env = Env::default();
        env.mock_all_auths();
        setup_with(&env, 2, 3);
    }

    #[test]
    #[should_panic(expected = "threshold cannot exceed number of owners")]
    fn test_initialize_no_owners_panics() {
        let env = Env::default();
        env.mock_all_auths();
        setup_with(&env, 0, 1);
    }

    #[test]
    #[should_panic(expected = "duplicate owner")]
    fn test_initialize_duplicate_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);
        let owner = Address::generate(&env);
        let contract_id = env.register_contract(None, MultiSigWalletContract);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let mut owners = Vec::new(&env);
        owners.push_back(owner.clone());
        owners.push_back(owner);
        client.initialize(&admin, &owners, &2);
    }

    #[test]
    fn test_initialize_threshold_equal_to_owner_count() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owners) = setup_with(&env, 2, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        assert_eq!(client.get_threshold(), 2);
        assert_eq!(client.get_owners().len(), 2);
        assert_eq!(owners.len(), 2);
    }

    #[test]
    #[should_panic(expected = "threshold must be greater than zero")]
    fn test_update_threshold_zero_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.update_threshold(&admin, &0);
    }

    #[test]
    #[should_panic(expected = "threshold cannot exceed number of owners")]
    fn test_update_threshold_above_owners_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.update_threshold(&admin, &4);
    }

    #[test]
    fn test_update_threshold_rejects_invalid_without_mutating_state() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        assert!(client.try_update_threshold(&admin, &0).is_err());
        assert_eq!(client.get_threshold(), 2);

        assert!(client.try_update_threshold(&admin, &9).is_err());
        assert_eq!(client.get_threshold(), 2);

        client.update_threshold(&admin, &3);
        assert_eq!(client.get_threshold(), 3);
    }

    // -----------------------------------------------------------------------
    // Owner management with threshold invariants (#345)
    // -----------------------------------------------------------------------

    #[test]
    fn test_add_owner() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owners) = setup_with(&env, 3, 3);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let new_owner = Address::generate(&env);
        client.add_owner(&admin, &new_owner);

        assert_eq!(client.get_owners().len(), 4);
        assert!(client.get_owners().contains(&new_owner));
        assert_eq!(client.get_threshold(), 3);
    }

    #[test]
    #[should_panic(expected = "owner already exists")]
    fn test_add_duplicate_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.add_owner(&admin, &owners.get(0).unwrap());
    }

    #[test]
    #[should_panic(expected = "admin only")]
    fn test_add_owner_non_admin_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.add_owner(&owners.get(0).unwrap(), &Address::generate(&env));
    }

    #[test]
    #[should_panic(expected = "admin only")]
    fn test_remove_owner_non_admin_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, _admin, owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&owners.get(0).unwrap(), &owners.get(1).unwrap());
    }

    #[test]
    #[should_panic(expected = "owner not found")]
    fn test_remove_unknown_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, _owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&admin, &Address::generate(&env));
    }

    #[test]
    #[should_panic(expected = "cannot remove owner below threshold")]
    fn test_remove_owner_below_threshold_panics() {
        let env = Env::default();
        env.mock_all_auths();

        // 3 owners, threshold 2 -> removing one owner is fine, the second is not.
        let (contract_id, admin, owners) = setup_with(&env, 3, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&admin, &owners.get(0).unwrap());
        assert_eq!(client.get_owners().len(), 2);

        client.remove_owner(&admin, &owners.get(1).unwrap());
    }

    #[test]
    #[should_panic(expected = "cannot remove owner below threshold")]
    fn test_remove_owner_when_all_owners_required_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 3, 3);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&admin, &owners.get(0).unwrap());
    }

    #[test]
    #[should_panic(expected = "cannot remove owner below threshold")]
    fn test_remove_last_owner_panics() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 1, 1);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&admin, &owners.get(0).unwrap());
    }

    #[test]
    fn test_remove_owner_then_raise_threshold() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 4, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        client.remove_owner(&admin, &owners.get(0).unwrap());
        assert_eq!(client.get_owners().len(), 3);

        // Threshold may still be raised, but not above the remaining owners.
        client.update_threshold(&admin, &3);
        assert_eq!(client.get_threshold(), 3);

        assert!(client.try_update_threshold(&admin, &4).is_err());
        assert_eq!(client.get_threshold(), 3);
    }

    #[test]
    fn test_removed_owner_loses_authority() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 4, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let removed = owners.get(0).unwrap();
        let remaining = owners.get(1).unwrap();
        client.remove_owner(&admin, &removed);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "after removal");
        assert!(client.try_submit_proposal(&removed, &recipient, &100, &desc).is_err());

        let pid = client.submit_proposal(&remaining, &recipient, &100, &desc);
        assert_eq!(pid, 1);
    }

    #[test]
    fn test_added_owner_can_sign() {
        let env = Env::default();
        env.mock_all_auths();

        let (contract_id, admin, owners) = setup_with(&env, 2, 2);
        let client = MultiSigWalletContractClient::new(&env, &contract_id);

        let new_owner = Address::generate(&env);
        client.add_owner(&admin, &new_owner);

        let recipient = Address::generate(&env);
        let desc = String::from_str(&env, "new owner signs");
        let pid = client.submit_proposal(&owners.get(0).unwrap(), &recipient, &250, &desc);

        client.sign_proposal(&owners.get(0).unwrap(), &pid);
        client.sign_proposal(&new_owner, &pid);
        client.execute_proposal(&owners.get(0).unwrap(), &pid);

        assert!(client.get_proposal(&pid).unwrap().executed);
    }
}
