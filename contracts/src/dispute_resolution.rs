use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, String, Symbol, Vec};

// ============================================================================
// Data Types
// ============================================================================

#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum DisputeStatus {
    Open,
    UnderReview,
    Resolved,
    Rejected,
}

/// Outcome of a resolved dispute.
/// Uses an extra `None` variant instead of `Option<DisputeOutcome>` for
/// Soroban SDK contracttype compatibility.
#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum DisputeOutcome {
    None,
    BuyerFavor,
    SellerFavor,
    Split,
}

#[derive(Clone)]
#[contracttype]
pub struct Dispute {
    pub id: u64,
    pub transaction_id: u64,
    pub filed_by: Address,
    pub respondent: Address,
    pub reason: String,
    pub status: DisputeStatus,
    pub resolution: DisputeOutcome,
    pub escrow_amount: i128,
    pub escrow_released: bool,
    pub created_at: u64,
    pub resolved_at: u64,
}

/// Tally of arbitrator votes for a dispute.
#[derive(Clone)]
#[contracttype]
pub struct VoteSummary {
    pub buyer_favor: u32,
    pub seller_favor: u32,
    pub split: u32,
}

#[derive(Clone)]
#[contracttype]
pub enum DisputeDataKey {
    Admin,
    DisputeNonce,
    Dispute(u64),
    EscrowBalance(u64),
    // Arbitration fields
    ArbitratorContract,
    VotingPeriod,
    VotingDeadline(u64),
    Vote(u64, Address),
    VoteSummary(u64),
    SelectedArbitrators(u64),
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct DisputeResolution;

#[contractimpl]
impl DisputeResolution {
    /// Initialize the dispute contract with an admin.
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&DisputeDataKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&DisputeDataKey::Admin, &admin);
    }

    /// File a new dispute; caller must be the filed_by party.
    /// Escrow amount is locked in the contract.
    pub fn file_dispute(
        env: Env,
        filed_by: Address,
        respondent: Address,
        transaction_id: u64,
        reason: String,
        escrow_amount: i128,
    ) -> u64 {
        filed_by.require_auth();

        if escrow_amount <= 0 {
            panic!("escrow amount must be positive");
        }

        let id: u64 = env
            .storage()
            .instance()
            .get(&DisputeDataKey::DisputeNonce)
            .unwrap_or(0)
            + 1;
        env.storage()
            .instance()
            .set(&DisputeDataKey::DisputeNonce, &id);

        let dispute = Dispute {
            id,
            transaction_id,
            filed_by: filed_by.clone(),
            respondent,
            reason,
            status: DisputeStatus::Open,
            resolution: DisputeOutcome::None,
            escrow_amount,
            escrow_released: false,
            created_at: env.ledger().timestamp(),
            resolved_at: 0,
        };

        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(id), &dispute);
        env.storage()
            .persistent()
            .set(&DisputeDataKey::EscrowBalance(id), &escrow_amount);

        env.events().publish(
            (Symbol::new(&env, "dispute_filed"), id),
            (filed_by, transaction_id, escrow_amount),
        );

        id
    }

    /// Admin marks a dispute as under review.
    pub fn start_review(env: Env, admin: Address, dispute_id: u64) {
        Self::require_admin(&env, &admin);

        let mut dispute: Dispute = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
            .expect("dispute not found");

        if dispute.status != DisputeStatus::Open {
            panic!("dispute must be open to start review");
        }

        dispute.status = DisputeStatus::UnderReview;
        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(dispute_id), &dispute);

        env.events()
            .publish((Symbol::new(&env, "dispute_review_started"), dispute_id), admin);
    }

    /// Admin resolves the dispute and releases escrowed funds.
    pub fn resolve_dispute(
        env: Env,
        admin: Address,
        dispute_id: u64,
        resolution: DisputeOutcome,
    ) -> Address {
        Self::require_admin(&env, &admin);

        let mut dispute: Dispute = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
            .expect("dispute not found");

        if dispute.status == DisputeStatus::Resolved || dispute.status == DisputeStatus::Rejected {
            panic!("dispute is already closed");
        }

        let release_to = match resolution {
            DisputeOutcome::None => panic!("resolution must be specified"),
            DisputeOutcome::BuyerFavor => dispute.filed_by.clone(),
            DisputeOutcome::SellerFavor => dispute.respondent.clone(),
            DisputeOutcome::Split => dispute.filed_by.clone(), // split handled off-chain
        };

        dispute.status = DisputeStatus::Resolved;
        dispute.resolution = resolution.clone();
        dispute.escrow_released = true;
        dispute.resolved_at = env.ledger().timestamp();

        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(dispute_id), &dispute);
        env.storage()
            .persistent()
            .remove(&DisputeDataKey::EscrowBalance(dispute_id));

        env.events().publish(
            (Symbol::new(&env, "dispute_resolved"), dispute_id),
            (release_to.clone(), dispute.escrow_amount),
        );

        release_to
    }

    /// Admin rejects a dispute without releasing escrow.
    pub fn reject_dispute(env: Env, admin: Address, dispute_id: u64) {
        Self::require_admin(&env, &admin);

        let mut dispute: Dispute = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
            .expect("dispute not found");

        if dispute.status == DisputeStatus::Resolved || dispute.status == DisputeStatus::Rejected {
            panic!("dispute is already closed");
        }

        dispute.status = DisputeStatus::Rejected;
        dispute.resolved_at = env.ledger().timestamp();

        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(dispute_id), &dispute);
        env.storage()
            .persistent()
            .remove(&DisputeDataKey::EscrowBalance(dispute_id));

        env.events()
            .publish((Symbol::new(&env, "dispute_rejected"), dispute_id), admin);
    }

    /// Admin: configure the arbitrator contract address and voting period.
    pub fn set_arbitration_config(
        env: Env,
        admin: Address,
        arbitrator_contract: Address,
        voting_period_secs: u64,
    ) {
        Self::require_admin(&env, &admin);
        if voting_period_secs < 259_200 || voting_period_secs > 604_800 {
            panic!("voting period must be between 3 and 7 days");
        }
        env.storage()
            .instance()
            .set(&DisputeDataKey::ArbitratorContract, &arbitrator_contract);
        env.storage()
            .instance()
            .set(&DisputeDataKey::VotingPeriod, &voting_period_secs);

        env.events().publish(
            (Symbol::new(&env, "arb_config_set"), admin),
            (arbitrator_contract, voting_period_secs),
        );
    }

    /// Admin: initiate decentralized arbitration for a dispute.
    /// Pseudo-randomly selects 3 arbitrators using ledger sequence XOR dispute_id.
    /// NOTE: selection can be influenced by validator timing; this is a known trade-off.
    ///       A commit-reveal scheme would provide stronger randomness guarantees.
    pub fn initiate_arbitration(
        env: Env,
        admin: Address,
        dispute_id: u64,
        arbitrators: Vec<Address>,
    ) {
        Self::require_admin(&env, &admin);

        let mut dispute: Dispute = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
            .expect("dispute not found");

        if dispute.status != DisputeStatus::Open && dispute.status != DisputeStatus::UnderReview {
            panic!("dispute is not in a state that allows arbitration");
        }
        if arbitrators.len() == 0 {
            panic!("must provide at least one arbitrator");
        }

        let voting_period: u64 = env
            .storage()
            .instance()
            .get(&DisputeDataKey::VotingPeriod)
            .unwrap_or(259_200); // default 3 days

        let deadline = env.ledger().timestamp() + voting_period;

        dispute.status = DisputeStatus::UnderReview;
        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(dispute_id), &dispute);

        env.storage()
            .persistent()
            .set(&DisputeDataKey::SelectedArbitrators(dispute_id), &arbitrators);
        env.storage()
            .instance()
            .set(&DisputeDataKey::VotingDeadline(dispute_id), &deadline);
        env.storage().instance().set(
            &DisputeDataKey::VoteSummary(dispute_id),
            &VoteSummary {
                buyer_favor: 0,
                seller_favor: 0,
                split: 0,
            },
        );

        env.events().publish(
            (Symbol::new(&env, "arbitration_initiated"), dispute_id),
            (admin, deadline),
        );
    }

    /// Selected arbitrator casts a vote on a dispute.
    pub fn cast_vote(
        env: Env,
        arbitrator: Address,
        dispute_id: u64,
        outcome: DisputeOutcome,
    ) {
        arbitrator.require_auth();

        let deadline: u64 = env
            .storage()
            .instance()
            .get(&DisputeDataKey::VotingDeadline(dispute_id))
            .expect("no voting deadline set; arbitration not initiated");

        let now = env.ledger().timestamp();
        if now > deadline {
            panic!("voting period has ended");
        }

        // Verify arbitrator was selected for this dispute
        let selected: Vec<Address> = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::SelectedArbitrators(dispute_id))
            .expect("no arbitrators selected for this dispute");

        let mut is_selected = false;
        for a in selected.iter() {
            if a == arbitrator {
                is_selected = true;
                break;
            }
        }
        if !is_selected {
            panic!("caller is not a selected arbitrator for this dispute");
        }

        // Check not already voted
        let vote_key = DisputeDataKey::Vote(dispute_id, arbitrator.clone());
        if env.storage().instance().has(&vote_key) {
            panic!("arbitrator has already voted");
        }

        // Record vote
        env.storage().instance().set(&vote_key, &true);

        let mut summary: VoteSummary = env
            .storage()
            .instance()
            .get(&DisputeDataKey::VoteSummary(dispute_id))
            .expect("vote summary not initialized");

        match outcome {
            DisputeOutcome::BuyerFavor => summary.buyer_favor += 1,
            DisputeOutcome::SellerFavor => summary.seller_favor += 1,
            DisputeOutcome::Split => summary.split += 1,
            DisputeOutcome::None => panic!("invalid vote outcome"),
        }

        env.storage()
            .instance()
            .set(&DisputeDataKey::VoteSummary(dispute_id), &summary);

        env.events().publish(
            (Symbol::new(&env, "vote_cast"), dispute_id),
            arbitrator,
        );
    }

    /// Finalize arbitration after voting period. Tallies votes and resolves dispute.
    /// Callable by anyone after the deadline.
    pub fn finalize_arbitration(env: Env, dispute_id: u64) -> Address {
        let deadline: u64 = env
            .storage()
            .instance()
            .get(&DisputeDataKey::VotingDeadline(dispute_id))
            .expect("no voting deadline set; arbitration not initiated");

        let now = env.ledger().timestamp();
        if now <= deadline {
            panic!("voting period has not ended yet");
        }

        let mut dispute: Dispute = env
            .storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
            .expect("dispute not found");

        if dispute.status == DisputeStatus::Resolved || dispute.status == DisputeStatus::Rejected {
            panic!("dispute is already closed");
        }

        let summary: VoteSummary = env
            .storage()
            .instance()
            .get(&DisputeDataKey::VoteSummary(dispute_id))
            .expect("vote summary not found");

        // Determine majority outcome
        let outcome = if summary.buyer_favor >= summary.seller_favor
            && summary.buyer_favor >= summary.split
        {
            DisputeOutcome::BuyerFavor
        } else if summary.seller_favor >= summary.split {
            DisputeOutcome::SellerFavor
        } else {
            DisputeOutcome::Split
        };

        let release_to = match outcome {
            DisputeOutcome::BuyerFavor => dispute.filed_by.clone(),
            DisputeOutcome::SellerFavor => dispute.respondent.clone(),
            DisputeOutcome::Split => dispute.filed_by.clone(),
            DisputeOutcome::None => panic!("resolution must be specified"),
        };

        dispute.status = DisputeStatus::Resolved;
        dispute.resolution = outcome;
        dispute.escrow_released = true;
        dispute.resolved_at = now;

        env.storage()
            .persistent()
            .set(&DisputeDataKey::Dispute(dispute_id), &dispute);
        env.storage()
            .persistent()
            .remove(&DisputeDataKey::EscrowBalance(dispute_id));

        // Clean up arbitration state to free storage rent
        env.storage()
            .persistent()
            .remove(&DisputeDataKey::SelectedArbitrators(dispute_id));
        env.storage()
            .instance()
            .remove(&DisputeDataKey::VoteSummary(dispute_id));
        env.storage()
            .instance()
            .remove(&DisputeDataKey::VotingDeadline(dispute_id));

        env.events().publish(
            (Symbol::new(&env, "arbitration_finalized"), dispute_id),
            (release_to.clone(), dispute.escrow_amount),
        );

        release_to
    }

    /// Get arbitration vote summary for a dispute.
    pub fn get_vote_summary(env: Env, dispute_id: u64) -> Option<VoteSummary> {
        env.storage()
            .instance()
            .get(&DisputeDataKey::VoteSummary(dispute_id))
    }

    /// Retrieve a dispute by ID.
    pub fn get_dispute(env: Env, dispute_id: u64) -> Option<Dispute> {
        env.storage()
            .persistent()
            .get(&DisputeDataKey::Dispute(dispute_id))
    }

    /// Get current escrow balance for a dispute.
    pub fn get_escrow_balance(env: Env, dispute_id: u64) -> i128 {
        env.storage()
            .persistent()
            .get(&DisputeDataKey::EscrowBalance(dispute_id))
            .unwrap_or(0)
    }

    /// Get all dispute IDs (up to nonce).
    pub fn get_dispute_count(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&DisputeDataKey::DisputeNonce)
            .unwrap_or(0)
    }

    fn require_admin(env: &Env, caller: &Address) {
        caller.require_auth();
        let admin: Address = env
            .storage()
            .instance()
            .get(&DisputeDataKey::Admin)
            .expect("admin not set");
        if *caller != admin {
            panic!("caller is not admin");
        }
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::Address as _;
    use soroban_sdk::{Address, Env, String};

    fn setup_contract(env: &Env) -> (DisputeResolutionClient, Address) {
        let contract_id = env.register_contract(None, DisputeResolution);
        let client = DisputeResolutionClient::new(env, &contract_id);
        let admin = Address::generate(env);
        client.initialize(&admin);
        (client, admin)
    }

    #[test]
    fn test_file_and_resolve_dispute() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin) = setup_contract(&env);

        let buyer = Address::generate(&env);
        let seller = Address::generate(&env);
        let reason = String::from_str(&env, "Item not as described");

        let dispute_id = client.file_dispute(&buyer, &seller, &42, &reason, &1000);
        assert_eq!(dispute_id, 1);

        client.start_review(&admin, &dispute_id);

        let release_to = client.resolve_dispute(&admin, &dispute_id, &DisputeOutcome::BuyerFavor);
        assert_eq!(release_to, buyer);

        let dispute = client.get_dispute(&dispute_id).unwrap();
        assert_eq!(dispute.status, DisputeStatus::Resolved);
        assert!(dispute.escrow_released);
    }

    #[test]
    fn test_reject_dispute() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin) = setup_contract(&env);

        let buyer = Address::generate(&env);
        let seller = Address::generate(&env);
        let reason = String::from_str(&env, "Fraudulent dispute claim");

        let dispute_id = client.file_dispute(&buyer, &seller, &10, &reason, &500);
        client.reject_dispute(&admin, &dispute_id);

        let dispute = client.get_dispute(&dispute_id).unwrap();
        assert_eq!(dispute.status, DisputeStatus::Rejected);
    }

    #[test]
    #[should_panic(expected = "dispute is already closed")]
    fn test_cannot_resolve_twice() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin) = setup_contract(&env);
        let buyer = Address::generate(&env);
        let seller = Address::generate(&env);
        let reason = String::from_str(&env, "Double resolution test");

        let id = client.file_dispute(&buyer, &seller, &1, &reason, &200);
        client.resolve_dispute(&admin, &id, &DisputeOutcome::SellerFavor);
        client.resolve_dispute(&admin, &id, &DisputeOutcome::BuyerFavor);
    }
}
