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
        if threshold == 0 || threshold > owners.len() {
            panic!("invalid threshold");
        }
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

        if new_threshold == 0 || new_threshold > owners.len() {
            panic!("invalid threshold");
        }

        env.storage()
            .instance()
            .set(&MultiSigKey::Threshold, &new_threshold);
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
}
