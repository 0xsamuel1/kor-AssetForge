use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, String, Symbol, Vec};

use crate::asset_token::AssetTokenClient;
use crate::events::{
    DelegationEvent, ProposalCreatedEvent, ProposalFinalizedEvent, VoteCastEvent, EVENT_VERSION,
};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum GovDataKey {
    /// Admin address
    Admin,
    /// Address of the governance token (AssetToken) contract
    TokenContract,
    /// Auto-incrementing proposal counter
    NextProposalId,
    /// Proposal metadata by id
    Proposal(u64),
    /// Whether a voter has already voted on a proposal
    Voted(u64, Address),
    /// Snapshot of a voter's balance at proposal creation
    Snapshot(u64, Address),
    /// Set of approved asset ids (proposal passed)
    Approved(u64),
    /// Required deposit amount for creating proposals (anti-spam)
    DepositAmount,
    /// Depositor address per proposal (for refund)
    Depositor(u64),
    /// Quorum threshold (minimum total votes for a proposal to be valid)
    Quorum,
    /// Delegation: delegator address -> delegatee address
    Delegate(Address),
    /// Delegation: delegatee address -> Vec<Address> of all delegators
    DelegatorsOf(Address),
    /// Maximum number of delegators allowed per delegatee (instance-level setting)
    DelegationLimit,
    /// On-demand balance snapshot: snapshot_id -> taker address -> balance
    BalanceSnapshot(u64, Address),
    /// Snapshot metadata: snapshot_id -> expiry timestamp
    SnapshotExpiry(u64),
    /// Next snapshot id counter
    NextSnapshotId,
    /// Current registered version for a given proposal template (Issue #211)
    TemplateVersion(ProposalTemplate),
    /// Template metadata attached to a proposal id (Issue #211)
    TemplateMeta(u64),
}

// ---------------------------------------------------------------------------
// Governance Proposal Templates (Issue #211)
// ---------------------------------------------------------------------------

/// Pre-defined templates for common governance actions. Each template encodes
/// the *shape* of a proposal so the front-end and validators can reason about
/// the parameters without parsing free-form descriptions.
#[derive(Clone, Copy, PartialEq, Debug)]
#[contracttype]
pub enum ProposalTemplate {
    /// Change a protocol fee (param_u64 = new fee in bps).
    FeeChange,
    /// Update a generic numeric protocol parameter (param_i128 = new value).
    ParameterUpdate,
    /// Upgrade a contract to new code (target = contract address).
    ContractUpgrade,
    /// Spend from the treasury (param_i128 = amount, target = recipient).
    TreasurySpend,
    /// Change the governance quorum threshold (param_i128 = new quorum).
    QuorumChange,
    /// List / approve an asset for trading (param_u64 = asset id).
    AddAsset,
    /// Delist / remove an asset (param_u64 = asset id).
    RemoveAsset,
    /// Toggle an emergency pause (param_u64 = scope id, param_i128 = 1 on / 0 off).
    PauseToggle,
    /// Change the per-delegatee delegation limit (param_u64 = new limit).
    DelegationLimitChange,
    /// Signaling-only text proposal (no executable parameters).
    TextProposal,
}

/// Bounds applied to a template's parameters during validation.
struct TemplateBounds {
    /// Inclusive min / max for `param_u64` (when used).
    u64_min: u64,
    u64_max: u64,
    /// Inclusive min / max for `param_i128` (when used).
    i128_min: i128,
    i128_max: i128,
    /// Whether a non-`None` `target` address is required.
    requires_target: bool,
}

/// Metadata recorded for a proposal created from a template.
#[derive(Clone)]
#[contracttype]
pub struct TemplateProposal {
    pub template: ProposalTemplate,
    pub version: u32,
    pub param_u64: u64,
    pub param_i128: i128,
    pub target: Option<Address>,
}

// ---------------------------------------------------------------------------
// Proposal Status
// ---------------------------------------------------------------------------

#[derive(Clone, PartialEq, Debug)]
#[contracttype]
pub enum ProposalStatus {
    Active,
    Passed,
    Rejected,
}

// ---------------------------------------------------------------------------
// Proposal Struct
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub struct Proposal {
    pub proposal_id: u64,
    pub asset_id: u64,
    pub proposer: Address,
    pub description: String,
    pub votes_for: i128,
    pub votes_against: i128,
    pub quorum_threshold: i128,
    pub end_time: u64,
    pub status: ProposalStatus,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct Governance;

#[contractimpl]
impl Governance {
    // -----------------------------------------------------------------------
    // Initialization
    // -----------------------------------------------------------------------

    /// Initialize the governance contract.
    ///
    /// # Arguments
    /// * `admin` – Contract admin who can adjust parameters.
    /// * `token_contract` – Address of the governance token (AssetToken).
    /// * `quorum_threshold` – Minimum total weighted votes for a valid result.
    /// * `deposit_amount` – Tokens required as anti-spam deposit for proposals.
    pub fn initialize(
        env: Env,
        admin: Address,
        token_contract: Address,
        quorum_threshold: i128,
        deposit_amount: i128,
    ) {
        if env.storage().instance().has(&GovDataKey::Admin) {
            panic!("already initialized");
        }
        admin.require_auth();

        env.storage().instance().set(&GovDataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&GovDataKey::TokenContract, &token_contract);
        env.storage()
            .instance()
            .set(&GovDataKey::Quorum, &quorum_threshold);
        env.storage()
            .instance()
            .set(&GovDataKey::DepositAmount, &deposit_amount);
        env.storage()
            .instance()
            .set(&GovDataKey::NextProposalId, &1u64);
    }

    // -----------------------------------------------------------------------
    // Proposals
    // -----------------------------------------------------------------------

    /// Create a new governance proposal to list an asset.
    ///
    /// The proposer must hold at least `deposit_amount` governance tokens.
    /// Their current balance is snapshotted at the time of creation so that
    /// voting weight is locked to the creation ledger.
    ///
    /// Returns the new `proposal_id`.
    pub fn create_proposal(
        env: Env,
        proposer: Address,
        asset_id: u64,
        description: String,
        duration: u64,
    ) -> u64 {
        proposer.require_auth();

        let token_addr: Address = env
            .storage()
            .instance()
            .get(&GovDataKey::TokenContract)
            .expect("not initialized");

        let deposit_amount: i128 = env
            .storage()
            .instance()
            .get(&GovDataKey::DepositAmount)
            .unwrap_or(0);

        // Check proposer has enough tokens for the deposit
        let token_client = AssetTokenClient::new(&env, &token_addr);
        let balance = token_client.balance(&proposer);
        if balance < deposit_amount {
            panic!("insufficient tokens for deposit");
        }

        let quorum: i128 = env
            .storage()
            .instance()
            .get(&GovDataKey::Quorum)
            .unwrap_or(0);

        // Allocate proposal id
        let proposal_id: u64 = env
            .storage()
            .instance()
            .get(&GovDataKey::NextProposalId)
            .unwrap_or(1);
        env.storage()
            .instance()
            .set(&GovDataKey::NextProposalId, &(proposal_id.checked_add(1).unwrap()));

        // end_time = current ledger timestamp + duration
        let end_time = env.ledger().timestamp().checked_add(duration).expect("timestamp overflow");

        let proposal = Proposal {
            proposal_id,
            asset_id,
            proposer: proposer.clone(),
            description,
            votes_for: 0,
            votes_against: 0,
            quorum_threshold: quorum,
            end_time,
            status: ProposalStatus::Active,
        };

        env.storage()
            .persistent()
            .set(&GovDataKey::Proposal(proposal_id), &proposal);

        // Record depositor for refund on success
        env.storage()
            .persistent()
            .set(&GovDataKey::Depositor(proposal_id), &proposer);

        // Snapshot proposer balance
        env.storage()
            .persistent()
            .set(&GovDataKey::Snapshot(proposal_id, proposer.clone()), &balance);

        // Emit event – topic includes asset_id for multi-dimensional index filtering
        env.events().publish(
            (Symbol::new(&env, "proposal_created"), proposal_id, asset_id),
            ProposalCreatedEvent {
                version: EVENT_VERSION,
                proposer: proposer.clone(),
                end_time,
            },
        );

        proposal_id
    }

    // -----------------------------------------------------------------------
    // Voting
    // -----------------------------------------------------------------------

    /// Cast a vote on an active proposal.
    ///
    /// The voter's governance-token balance is snapshotted on first
    /// interaction with this proposal (the balance at the time of voting).
    /// Double-voting is prevented.
    pub fn vote(env: Env, voter: Address, proposal_id: u64, vote_yes: bool) {
        voter.require_auth();

        // Load proposal
        let mut proposal: Proposal = env
            .storage()
            .persistent()
            .get(&GovDataKey::Proposal(proposal_id))
            .expect("proposal not found");

        // Must still be active
        if proposal.status != ProposalStatus::Active {
            panic!("proposal not active");
        }

        // Must not be expired
        let now = env.ledger().timestamp();
        if now >= proposal.end_time {
            panic!("proposal expired");
        }

        // Prevent double-voting
        if env
            .storage()
            .persistent()
            .has(&GovDataKey::Voted(proposal_id, voter.clone()))
        {
            panic!("already voted");
        }

        // Snapshot voter balance (weight) — uses current on-chain balance
        let token_addr: Address = env
            .storage()
            .instance()
            .get(&GovDataKey::TokenContract)
            .expect("not initialized");
        let token_client = AssetTokenClient::new(&env, &token_addr);
        let own_balance = token_client.balance(&voter);

        // Prevent voting if the voter has delegated their power to someone else
        if env
            .storage()
            .persistent()
            .has(&GovDataKey::Delegate(voter.clone()))
        {
            panic!("voting power has been delegated; your delegate must vote");
        }

        if own_balance <= 0 {
            panic!("insufficient tokens to vote");
        }

        // Aggregate delegated power from all delegators who delegated to this voter
        let delegators: Vec<Address> = env
            .storage()
            .persistent()
            .get(&GovDataKey::DelegatorsOf(voter.clone()))
            .unwrap_or(Vec::new(&env));
        let mut weight: i128 = own_balance;
        for delegator in delegators.iter() {
            let d_bal = token_client.balance(&delegator);
            if d_bal > 0 {
                weight = weight.checked_add(d_bal).expect("vote overflow");
            }
        }

        // Record snapshot (includes delegated power)
        env.storage()
            .persistent()
            .set(&GovDataKey::Snapshot(proposal_id, voter.clone()), &weight);

        // Record vote
        if vote_yes {
            proposal.votes_for = proposal
                .votes_for
                .checked_add(weight)
                .expect("vote overflow");
        } else {
            proposal.votes_against = proposal
                .votes_against
                .checked_add(weight)
                .expect("vote overflow");
        }

        // Mark as voted
        env.storage()
            .persistent()
            .set(&GovDataKey::Voted(proposal_id, voter.clone()), &true);

        // Save updated proposal
        env.storage()
            .persistent()
            .set(&GovDataKey::Proposal(proposal_id), &proposal);

        // Emit event
        env.events().publish(
            (Symbol::new(&env, "vote_cast"), proposal_id),
            VoteCastEvent {
                version: EVENT_VERSION,
                voter,
                vote_yes,
                weight,
            },
        );
    }

    // -----------------------------------------------------------------------
    // Tally & Execute
    // -----------------------------------------------------------------------

    /// Tally votes after the proposal's `end_time` has passed.
    ///
    /// If quorum is met and votes_for > votes_against, the proposal is
    /// marked `Passed` and the asset_id is recorded as approved for listing.
    /// The proposer's deposit is refunded on success.
    pub fn tally_execute(env: Env, proposal_id: u64) {
        let mut proposal: Proposal = env
            .storage()
            .persistent()
            .get(&GovDataKey::Proposal(proposal_id))
            .expect("proposal not found");

        if proposal.status != ProposalStatus::Active {
            panic!("proposal already finalized");
        }

        let now = env.ledger().timestamp();
        if now < proposal.end_time {
            panic!("voting period not ended");
        }

        let total_votes = proposal
            .votes_for
            .checked_add(proposal.votes_against)
            .expect("overflow");

        if total_votes >= proposal.quorum_threshold && proposal.votes_for > proposal.votes_against {
            proposal.status = ProposalStatus::Passed;

            // Mark asset as approved for listing
            env.storage()
                .persistent()
                .set(&GovDataKey::Approved(proposal.asset_id), &true);

            // Emit execution event – asset_id in topics for direct asset-based queries
            env.events().publish(
                (Symbol::new(&env, "proposal_executed"), proposal_id, proposal.asset_id),
                ProposalFinalizedEvent {
                    version: EVENT_VERSION,
                    votes_for: proposal.votes_for,
                    votes_against: proposal.votes_against,
                },
            );
        } else {
            proposal.status = ProposalStatus::Rejected;

            // Emit rejection event
            env.events().publish(
                (Symbol::new(&env, "proposal_rejected"), proposal_id, proposal.asset_id),
                ProposalFinalizedEvent {
                    version: EVENT_VERSION,
                    votes_for: proposal.votes_for,
                    votes_against: proposal.votes_against,
                },
            );
        }

        // Save finalized proposal
        env.storage()
            .persistent()
            .set(&GovDataKey::Proposal(proposal_id), &proposal);
    }

    // -----------------------------------------------------------------------
    // Query helpers
    // -----------------------------------------------------------------------

    /// Check if an asset has been approved via governance.
    pub fn is_approved(env: Env, asset_id: u64) -> bool {
        env.storage()
            .persistent()
            .get(&GovDataKey::Approved(asset_id))
            .unwrap_or(false)
    }

    /// Enforce that an asset has been approved. Panics otherwise.
    pub fn require_approved(env: Env, asset_id: u64) {
        if !Self::is_approved(env, asset_id) {
            panic!("asset not approved by governance");
        }
    }

    /// Return proposal details.
    pub fn get_proposal(env: Env, proposal_id: u64) -> Option<Proposal> {
        env.storage()
            .persistent()
            .get(&GovDataKey::Proposal(proposal_id))
    }

    /// Check whether a voter has voted on a given proposal.
    pub fn has_voted(env: Env, proposal_id: u64, voter: Address) -> bool {
        env.storage()
            .persistent()
            .has(&GovDataKey::Voted(proposal_id, voter))
    }

    /// Return the snapshotted voting weight for a voter on a proposal.
    pub fn get_vote_weight(env: Env, proposal_id: u64, voter: Address) -> i128 {
        env.storage()
            .persistent()
            .get(&GovDataKey::Snapshot(proposal_id, voter))
            .unwrap_or(0)
    }

    // -----------------------------------------------------------------------
    // Delegation – Issue #70
    // -----------------------------------------------------------------------

    /// Set the maximum number of delegators a single delegatee may accept.
    /// Defaults to 20 if never configured.
    pub fn set_delegation_limit(env: Env, admin: Address, limit: u32) {
        Self::require_admin(&env, &admin);
        env.storage()
            .instance()
            .set(&GovDataKey::DelegationLimit, &limit);
    }

    /// Delegate the caller's voting power to `delegatee`.
    ///
    /// Rules enforced:
    /// - Cannot delegate to self.
    /// - Cannot delegate if already delegated (revoke first).
    /// - Cannot delegate to someone who has themselves delegated (prevents
    ///   multi-level / circular chains).
    /// - Delegation limit per delegatee must not be exceeded.
    pub fn delegate(env: Env, delegator: Address, delegatee: Address) {
        delegator.require_auth();

        if delegator == delegatee {
            panic!("cannot delegate to self");
        }

        // Multi-level prevention: delegatee must not itself have an active delegation
        if env
            .storage()
            .persistent()
            .has(&GovDataKey::Delegate(delegatee.clone()))
        {
            panic!("delegatee has already delegated; multi-level delegation not allowed");
        }

        // Prevent double-delegation without explicit revocation
        if env
            .storage()
            .persistent()
            .has(&GovDataKey::Delegate(delegator.clone()))
        {
            panic!("already delegated; revoke existing delegation first");
        }

        // Enforce per-delegatee limit
        let limit: u32 = env
            .storage()
            .instance()
            .get(&GovDataKey::DelegationLimit)
            .unwrap_or(20);
        let mut delegators: Vec<Address> = env
            .storage()
            .persistent()
            .get(&GovDataKey::DelegatorsOf(delegatee.clone()))
            .unwrap_or(Vec::new(&env));
        if delegators.len() >= limit {
            panic!("delegation limit reached for this delegatee");
        }

        // Store delegation
        env.storage()
            .persistent()
            .set(&GovDataKey::Delegate(delegator.clone()), &delegatee);
        delegators.push_back(delegator.clone());
        env.storage()
            .persistent()
            .set(&GovDataKey::DelegatorsOf(delegatee.clone()), &delegators);

        env.events().publish(
            (Symbol::new(&env, "delegate_set"), delegator.clone()),
            DelegationEvent {
                version: EVENT_VERSION,
                delegatee,
                timestamp: env.ledger().timestamp(),
            },
        );
    }

    /// Revoke the caller's active delegation, restoring their direct voting
    /// ability.
    pub fn revoke_delegation(env: Env, delegator: Address) {
        delegator.require_auth();
        Self::revoke_delegation_internal(&env, &delegator);
    }

    /// Emergency delegation removal by the contract admin.
    ///
    /// Use when a delegator is unresponsive and needs to be unblocked.
    pub fn revoke_delegation_admin(env: Env, admin: Address, delegator: Address) {
        Self::require_admin(&env, &admin);
        Self::revoke_delegation_internal(&env, &delegator);
        env.events().publish(
            (Symbol::new(&env, "delegation_admin_removed"), delegator),
            env.ledger().timestamp(),
        );
    }

    /// Return the current delegatee for `delegator`, if any.
    pub fn get_delegate(env: Env, delegator: Address) -> Option<Address> {
        env.storage()
            .persistent()
            .get(&GovDataKey::Delegate(delegator))
    }

    /// Return the list of addresses that have delegated to `delegatee`.
    pub fn get_delegators(env: Env, delegatee: Address) -> Vec<Address> {
        env.storage()
            .persistent()
            .get(&GovDataKey::DelegatorsOf(delegatee))
            .unwrap_or(Vec::new(&env))
    }

    /// Return the sum of token balances of every delegator currently
    /// delegating to `delegatee`.
    pub fn get_delegated_power(env: Env, delegatee: Address) -> i128 {
        let token_addr: Address = env
            .storage()
            .instance()
            .get(&GovDataKey::TokenContract)
            .expect("not initialized");
        let token_client = AssetTokenClient::new(&env, &token_addr);

        let delegators: Vec<Address> = env
            .storage()
            .persistent()
            .get(&GovDataKey::DelegatorsOf(delegatee))
            .unwrap_or(Vec::new(&env));

        let mut total: i128 = 0;
        for delegator in delegators.iter() {
            total = total
                .checked_add(token_client.balance(&delegator))
                .expect("overflow");
        }
        total
    }

    // -----------------------------------------------------------------------
    // Snapshot-based Voting (Issue #230)
    // -----------------------------------------------------------------------

    /// Create an on-demand balance snapshot for the caller.
    /// The snapshot records the caller's current token balance and is valid
    /// for `ttl_seconds`.  Returns the new `snapshot_id`.
    pub fn create_snapshot(env: Env, taker: Address, ttl_seconds: u64) -> u64 {
        taker.require_auth();

        let token_addr: Address = env
            .storage()
            .instance()
            .get(&GovDataKey::TokenContract)
            .expect("not initialized");
        let balance = AssetTokenClient::new(&env, &token_addr).balance(&taker);

        let snapshot_id: u64 = env
            .storage()
            .instance()
            .get(&GovDataKey::NextSnapshotId)
            .unwrap_or(1);
        env.storage()
            .instance()
            .set(&GovDataKey::NextSnapshotId, &(snapshot_id + 1));

        let expiry = env.ledger().timestamp() + ttl_seconds;
        env.storage()
            .persistent()
            .set(&GovDataKey::BalanceSnapshot(snapshot_id, taker.clone()), &balance);
        env.storage()
            .persistent()
            .set(&GovDataKey::SnapshotExpiry(snapshot_id), &expiry);

        env.events().publish(
            (Symbol::new(&env, "snapshot_created"), snapshot_id),
            (taker, balance, expiry),
        );

        snapshot_id
    }

    /// Return the snapshotted balance for a given snapshot_id and address.
    /// Panics if the snapshot has expired.
    pub fn get_snapshot_balance(env: Env, snapshot_id: u64, user: Address) -> i128 {
        let expiry: u64 = env
            .storage()
            .persistent()
            .get(&GovDataKey::SnapshotExpiry(snapshot_id))
            .expect("snapshot not found");
        if env.ledger().timestamp() > expiry {
            panic!("snapshot expired");
        }
        env.storage()
            .persistent()
            .get(&GovDataKey::BalanceSnapshot(snapshot_id, user))
            .unwrap_or(0)
    }

    /// Cast a vote using a pre-existing snapshot for off-chain / gasless
    /// voting verification.  The snapshot must not be expired.
    pub fn vote_with_snapshot(
        env: Env,
        voter: Address,
        proposal_id: u64,
        snapshot_id: u64,
        vote_yes: bool,
    ) {
        voter.require_auth();

        // Load and validate snapshot
        let expiry: u64 = env
            .storage()
            .persistent()
            .get(&GovDataKey::SnapshotExpiry(snapshot_id))
            .expect("snapshot not found");
        if env.ledger().timestamp() > expiry {
            panic!("snapshot expired");
        }
        let weight: i128 = env
            .storage()
            .persistent()
            .get(&GovDataKey::BalanceSnapshot(snapshot_id, voter.clone()))
            .unwrap_or(0);
        if weight <= 0 {
            panic!("no balance in snapshot");
        }

        // Load proposal
        let mut proposal: Proposal = env
            .storage()
            .persistent()
            .get(&GovDataKey::Proposal(proposal_id))
            .expect("proposal not found");
        if proposal.status != ProposalStatus::Active {
            panic!("proposal not active");
        }
        let now = env.ledger().timestamp();
        if now >= proposal.end_time {
            panic!("proposal expired");
        }
        if env
            .storage()
            .persistent()
            .has(&GovDataKey::Voted(proposal_id, voter.clone()))
        {
            panic!("already voted");
        }

        if vote_yes {
            proposal.votes_for = proposal.votes_for.checked_add(weight).expect("overflow");
        } else {
            proposal.votes_against = proposal.votes_against.checked_add(weight).expect("overflow");
        }

        env.storage()
            .persistent()
            .set(&GovDataKey::Voted(proposal_id, voter.clone()), &true);
        env.storage()
            .persistent()
            .set(&GovDataKey::Snapshot(proposal_id, voter.clone()), &weight);
        env.storage()
            .persistent()
            .set(&GovDataKey::Proposal(proposal_id), &proposal);

        env.events().publish(
            (Symbol::new(&env, "vote_cast_snapshot"), proposal_id),
            (voter, vote_yes, weight, snapshot_id),
        );
    }

    // -----------------------------------------------------------------------
    // Proposal Templates – Issue #211
    // -----------------------------------------------------------------------

    /// Register (or bump) the active version of a proposal template. Admin only.
    /// Template versioning lets the protocol evolve a template's accepted
    /// parameters while rejecting proposals built against a stale schema.
    pub fn register_template_version(
        env: Env,
        admin: Address,
        template: ProposalTemplate,
        version: u32,
    ) {
        Self::require_admin(&env, &admin);
        if version == 0 {
            panic!("template version must be >= 1");
        }
        env.storage()
            .instance()
            .set(&GovDataKey::TemplateVersion(template), &version);
        env.events().publish(
            (Symbol::new(&env, "template_version_set"),),
            (template, version),
        );
    }

    /// Return the currently registered version for a template (defaults to 1).
    pub fn get_template_version(env: Env, template: ProposalTemplate) -> u32 {
        env.storage()
            .instance()
            .get(&GovDataKey::TemplateVersion(template))
            .unwrap_or(1)
    }

    /// Read-only validation of a set of template parameters. Returns `true` if
    /// the version is current and every parameter is within bounds. Intended for
    /// off-chain pre-flight checks before submitting a proposal.
    pub fn validate_template_params(
        env: Env,
        template: ProposalTemplate,
        version: u32,
        param_u64: u64,
        param_i128: i128,
        target: Option<Address>,
    ) -> bool {
        if version != Self::get_template_version(env, template) {
            return false;
        }
        let bounds = Self::template_bounds(&template);
        Self::params_within_bounds(&bounds, param_u64, param_i128, &target)
    }

    /// Create a governance proposal from a pre-defined template (Issue #211).
    ///
    /// Validates the template version and enforces per-template parameter bounds
    /// before delegating to [`Governance::create_proposal`] for the usual
    /// deposit / snapshot / auth handling. The structured parameters are stored
    /// alongside the proposal for later execution and indexing.
    pub fn create_proposal_from_template(
        env: Env,
        proposer: Address,
        template: ProposalTemplate,
        version: u32,
        param_u64: u64,
        param_i128: i128,
        target: Option<Address>,
        description: String,
        duration: u64,
    ) -> u64 {
        // Reject proposals built against an unsupported template schema.
        if version != Self::get_template_version(env.clone(), template) {
            panic!("unsupported template version");
        }

        let bounds = Self::template_bounds(&template);
        if bounds.requires_target && target.is_none() {
            panic!("template requires a target address");
        }
        if !Self::params_within_bounds(&bounds, param_u64, param_i128, &target) {
            panic!("template parameters out of bounds");
        }

        // Asset-oriented templates feed their asset id through to the base
        // proposal so a passing vote approves the right asset; all other
        // templates use the neutral sentinel `0`.
        let asset_id = match template {
            ProposalTemplate::AddAsset | ProposalTemplate::RemoveAsset => param_u64,
            _ => 0,
        };

        let proposal_id =
            Self::create_proposal(env.clone(), proposer.clone(), asset_id, description, duration);

        let meta = TemplateProposal {
            template,
            version,
            param_u64,
            param_i128,
            target,
        };
        env.storage()
            .persistent()
            .set(&GovDataKey::TemplateMeta(proposal_id), &meta);

        env.events().publish(
            (Symbol::new(&env, "template_proposal_created"), proposal_id),
            (proposer, template, version),
        );

        proposal_id
    }

    /// Return the template metadata attached to a proposal, if any.
    pub fn get_template_proposal(env: Env, proposal_id: u64) -> Option<TemplateProposal> {
        env.storage()
            .persistent()
            .get(&GovDataKey::TemplateMeta(proposal_id))
    }

    /// List every supported proposal template.
    pub fn list_templates(env: Env) -> Vec<ProposalTemplate> {
        let mut v = Vec::new(&env);
        v.push_back(ProposalTemplate::FeeChange);
        v.push_back(ProposalTemplate::ParameterUpdate);
        v.push_back(ProposalTemplate::ContractUpgrade);
        v.push_back(ProposalTemplate::TreasurySpend);
        v.push_back(ProposalTemplate::QuorumChange);
        v.push_back(ProposalTemplate::AddAsset);
        v.push_back(ProposalTemplate::RemoveAsset);
        v.push_back(ProposalTemplate::PauseToggle);
        v.push_back(ProposalTemplate::DelegationLimitChange);
        v.push_back(ProposalTemplate::TextProposal);
        v
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    /// Per-template parameter bounds. Centralises the limits enforced for every
    /// template so validation stays consistent across entry points.
    fn template_bounds(template: &ProposalTemplate) -> TemplateBounds {
        match template {
            // Fee expressed in bps; capped at 10% to prevent confiscatory fees.
            ProposalTemplate::FeeChange => TemplateBounds {
                u64_min: 0,
                u64_max: 1_000,
                i128_min: 0,
                i128_max: 0,
                requires_target: false,
            },
            ProposalTemplate::ParameterUpdate => TemplateBounds {
                u64_min: 0,
                u64_max: u64::MAX,
                i128_min: 0,
                i128_max: i128::MAX,
                requires_target: false,
            },
            ProposalTemplate::ContractUpgrade => TemplateBounds {
                u64_min: 0,
                u64_max: u64::MAX,
                i128_min: 0,
                i128_max: 0,
                requires_target: true,
            },
            ProposalTemplate::TreasurySpend => TemplateBounds {
                u64_min: 0,
                u64_max: u64::MAX,
                i128_min: 1,
                i128_max: i128::MAX,
                requires_target: true,
            },
            ProposalTemplate::QuorumChange => TemplateBounds {
                u64_min: 0,
                u64_max: u64::MAX,
                i128_min: 1,
                i128_max: i128::MAX,
                requires_target: false,
            },
            ProposalTemplate::AddAsset | ProposalTemplate::RemoveAsset => TemplateBounds {
                u64_min: 1,
                u64_max: u64::MAX,
                i128_min: 0,
                i128_max: 0,
                requires_target: false,
            },
            // param_i128 acts as a boolean flag (0 = off, 1 = on).
            ProposalTemplate::PauseToggle => TemplateBounds {
                u64_min: 0,
                u64_max: u64::MAX,
                i128_min: 0,
                i128_max: 1,
                requires_target: false,
            },
            ProposalTemplate::DelegationLimitChange => TemplateBounds {
                u64_min: 1,
                u64_max: 10_000,
                i128_min: 0,
                i128_max: 0,
                requires_target: false,
            },
            ProposalTemplate::TextProposal => TemplateBounds {
                u64_min: 0,
                u64_max: 0,
                i128_min: 0,
                i128_max: 0,
                requires_target: false,
            },
        }
    }

    fn params_within_bounds(
        bounds: &TemplateBounds,
        param_u64: u64,
        param_i128: i128,
        target: &Option<Address>,
    ) -> bool {
        if param_u64 < bounds.u64_min || param_u64 > bounds.u64_max {
            return false;
        }
        if param_i128 < bounds.i128_min || param_i128 > bounds.i128_max {
            return false;
        }
        if bounds.requires_target && target.is_none() {
            return false;
        }
        true
    }

    fn require_admin(env: &Env, caller: &Address) {
        let admin: Address = env
            .storage()
            .instance()
            .get(&GovDataKey::Admin)
            .expect("not initialized");
        if *caller != admin {
            panic!("admin only");
        }
        caller.require_auth();
    }

    fn revoke_delegation_internal(env: &Env, delegator: &Address) {
        let delegatee: Address = env
            .storage()
            .persistent()
            .get(&GovDataKey::Delegate(delegator.clone()))
            .expect("no active delegation");

        // Remove delegator from the delegatee's list
        let existing: Vec<Address> = env
            .storage()
            .persistent()
            .get(&GovDataKey::DelegatorsOf(delegatee.clone()))
            .unwrap_or(Vec::new(env));
        let mut updated = Vec::new(env);
        for d in existing.iter() {
            if d != *delegator {
                updated.push_back(d);
            }
        }
        env.storage()
            .persistent()
            .set(&GovDataKey::DelegatorsOf(delegatee.clone()), &updated);

        // Remove the delegation record
        env.storage()
            .persistent()
            .remove(&GovDataKey::Delegate(delegator.clone()));

        env.events().publish(
            (Symbol::new(env, "delegation_revoked"), delegator.clone()),
            DelegationEvent {
                version: EVENT_VERSION,
                delegatee,
                timestamp: env.ledger().timestamp(),
            },
        );
    }
}

// ===========================================================================
// Unit Tests
// ===========================================================================

#[cfg(test)]
mod test {
    use super::*;
    use crate::asset_token::AssetToken;
    use crate::emergency_control::{EmergencyControl, EmergencyControlClient};
    use soroban_sdk::testutils::{Address as _, Ledger};
    use soroban_sdk::String;

    /// Helper: deploy governance ecosystem (emergency control + asset token + governance).
    fn setup() -> (
        Env,
        Address, // gov_id
        Address, // at_id
        Address, // ec_id
        Address, // admin
    ) {
        let env = Env::default();
        env.mock_all_auths();

        // Emergency control
        let ec_id = env.register_contract(None, EmergencyControl);
        let ec_client = EmergencyControlClient::new(&env, &ec_id);
        let admin = Address::generate(&env);
        ec_client.initialize(&admin);

        // Asset token (governance token)
        let at_id = env.register_contract(None, AssetToken);
        let at_client = AssetTokenClient::new(&env, &at_id);
        at_client.initialize(
            &admin,
            &String::from_str(&env, "GovToken"),
            &String::from_str(&env, "GOV"),
            &7,
            &0,
        );

        // Governance contract
        let gov_id = env.register_contract(None, Governance);
        let gov_client = GovernanceClient::new(&env, &gov_id);
        gov_client.initialize(&admin, &at_id, &100, &50); // quorum=100, deposit=50

        (env, gov_id, at_id, ec_id, admin)
    }

    /// Mint governance tokens to an address.
    fn mint_tokens(env: &Env, at_id: &Address, ec_id: &Address, to: &Address, amount: i128) {
        let at_client = AssetTokenClient::new(env, at_id);
        at_client.mint(to, &amount, &1, ec_id);
    }

    // -----------------------------------------------------------------------
    // 1. Initialization
    // -----------------------------------------------------------------------

    #[test]
    fn test_initialize() {
        let (env, gov_id, _at_id, _ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        // No proposals yet – get_proposal returns None
        assert!(client.get_proposal(&1).is_none());
    }

    #[test]
    #[should_panic(expected = "already initialized")]
    fn test_double_initialize_panics() {
        let (env, gov_id, at_id, _ec_id, admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        client.initialize(&admin, &at_id, &100, &50);
    }

    // -----------------------------------------------------------------------
    // 2. Proposal creation
    // -----------------------------------------------------------------------

    #[test]
    fn test_create_proposal() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        let proposer = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "List real estate asset"),
            &3600,
        );
        assert_eq!(pid, 1);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.asset_id, 1);
        assert_eq!(p.votes_for, 0);
        assert_eq!(p.votes_against, 0);
        assert_eq!(p.status, ProposalStatus::Active);
    }

    #[test]
    fn test_create_multiple_proposals_increments_id() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        let proposer = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);

        let pid1 = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "First"),
            &3600,
        );
        let pid2 = client.create_proposal(
            &proposer,
            &2,
            &String::from_str(&env, "Second"),
            &3600,
        );
        assert_eq!(pid1, 1);
        assert_eq!(pid2, 2);
    }

    #[test]
    #[should_panic(expected = "insufficient tokens for deposit")]
    fn test_create_proposal_insufficient_deposit() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        let proposer = Address::generate(&env);

        // Mint less than deposit_amount (50)
        mint_tokens(&env, &at_id, &ec_id, &proposer, 10);

        client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Under-funded"),
            &3600,
        );
    }

    // -----------------------------------------------------------------------
    // 3. Voting
    // -----------------------------------------------------------------------

    #[test]
    fn test_vote_yes() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 150);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Vote test"),
            &3600,
        );

        client.vote(&voter, &pid, &true);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.votes_for, 150);
        assert_eq!(p.votes_against, 0);
        assert!(client.has_voted(&pid, &voter));
        assert_eq!(client.get_vote_weight(&pid, &voter), 150);
    }

    #[test]
    fn test_vote_no() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 80);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Nay test"),
            &3600,
        );

        client.vote(&voter, &pid, &false);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.votes_for, 0);
        assert_eq!(p.votes_against, 80);
    }

    #[test]
    #[should_panic(expected = "already voted")]
    fn test_double_vote_panics() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 100);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Double vote"),
            &3600,
        );

        client.vote(&voter, &pid, &true);
        client.vote(&voter, &pid, &false); // should panic
    }

    #[test]
    #[should_panic(expected = "proposal expired")]
    fn test_vote_after_expiry_panics() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 100);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Expired"),
            &3600,
        );

        // Advance past end_time
        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.vote(&voter, &pid, &true);
    }

    #[test]
    #[should_panic(expected = "insufficient tokens to vote")]
    fn test_vote_with_zero_balance_panics() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env); // no tokens

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "No tokens"),
            &3600,
        );

        client.vote(&voter, &pid, &true);
    }

    // -----------------------------------------------------------------------
    // 4. Tally & Execute
    // -----------------------------------------------------------------------

    #[test]
    fn test_tally_proposal_passes() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter1 = Address::generate(&env);
        let voter2 = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter1, 80);
        mint_tokens(&env, &at_id, &ec_id, &voter2, 60);

        let pid = client.create_proposal(
            &proposer,
            &42,
            &String::from_str(&env, "List asset 42"),
            &3600,
        );

        client.vote(&voter1, &pid, &true);
        client.vote(&voter2, &pid, &true);

        // Advance past end
        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.status, ProposalStatus::Passed);
        assert!(client.is_approved(&42));
    }

    #[test]
    fn test_tally_proposal_rejected_more_against() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter1 = Address::generate(&env);
        let voter2 = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter1, 40);
        mint_tokens(&env, &at_id, &ec_id, &voter2, 80);

        let pid = client.create_proposal(
            &proposer,
            &5,
            &String::from_str(&env, "Reject me"),
            &3600,
        );

        client.vote(&voter1, &pid, &true);   // 40 for
        client.vote(&voter2, &pid, &false);   // 80 against

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.status, ProposalStatus::Rejected);
        assert!(!client.is_approved(&5));
    }

    #[test]
    fn test_tally_proposal_rejected_quorum_not_met() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 50); // quorum = 100, only 50 votes

        let pid = client.create_proposal(
            &proposer,
            &7,
            &String::from_str(&env, "Low quorum"),
            &3600,
        );

        client.vote(&voter, &pid, &true);

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.status, ProposalStatus::Rejected);
        assert!(!client.is_approved(&7));
    }

    #[test]
    #[should_panic(expected = "voting period not ended")]
    fn test_tally_before_end_panics() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Too early"),
            &3600,
        );

        client.tally_execute(&pid); // should panic
    }

    #[test]
    #[should_panic(expected = "proposal already finalized")]
    fn test_tally_twice_panics() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 150);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Finalized"),
            &3600,
        );

        client.vote(&voter, &pid, &true);

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);
        client.tally_execute(&pid); // should panic
    }

    // -----------------------------------------------------------------------
    // 5. Governance-gated listing (require_approved)
    // -----------------------------------------------------------------------

    #[test]
    #[should_panic(expected = "asset not approved by governance")]
    fn test_require_approved_panics_if_not_approved() {
        let (env, gov_id, _at_id, _ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        client.require_approved(&999);
    }

    #[test]
    fn test_require_approved_succeeds_after_passed_proposal() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let voter = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &voter, 120);

        let pid = client.create_proposal(
            &proposer,
            &10,
            &String::from_str(&env, "Approve asset 10"),
            &3600,
        );

        client.vote(&voter, &pid, &true);

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);

        // Should NOT panic
        client.require_approved(&10);
    }

    // -----------------------------------------------------------------------
    // 6. Delegation
    // -----------------------------------------------------------------------

    #[test]
    fn test_delegate_and_vote_with_delegated_power() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let delegator = Address::generate(&env);
        let delegatee = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &delegator, 60);
        mint_tokens(&env, &at_id, &ec_id, &delegatee, 40);

        // delegator delegates to delegatee
        client.delegate(&delegator, &delegatee);
        assert_eq!(client.get_delegate(&delegator), Some(delegatee.clone()));

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Delegation test"),
            &3600,
        );

        // delegatee votes; weight = own 40 + delegated 60 = 100
        client.vote(&delegatee, &pid, &true);
        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.votes_for, 100);
    }

    #[test]
    #[should_panic(expected = "voting power has been delegated")]
    fn test_delegator_cannot_vote() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let delegator = Address::generate(&env);
        let delegatee = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &delegator, 100);
        mint_tokens(&env, &at_id, &ec_id, &delegatee, 50);

        client.delegate(&delegator, &delegatee);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "Delegator vote blocked"),
            &3600,
        );

        client.vote(&delegator, &pid, &true); // must panic
    }

    #[test]
    #[should_panic(expected = "cannot delegate to self")]
    fn test_cannot_delegate_to_self() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);
        let user = Address::generate(&env);
        mint_tokens(&env, &at_id, &ec_id, &user, 100);
        client.delegate(&user, &user);
    }

    #[test]
    #[should_panic(expected = "multi-level delegation not allowed")]
    fn test_multi_level_delegation_prevented() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let a = Address::generate(&env);
        let b = Address::generate(&env);
        let c = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &a, 100);
        mint_tokens(&env, &at_id, &ec_id, &b, 100);
        mint_tokens(&env, &at_id, &ec_id, &c, 100);

        // b delegates to c
        client.delegate(&b, &c);
        // a tries to delegate to b (who already delegated) – must panic
        client.delegate(&a, &b);
    }

    #[test]
    fn test_revoke_delegation_restores_vote() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let delegator = Address::generate(&env);
        let delegatee = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &delegator, 80);
        mint_tokens(&env, &at_id, &ec_id, &delegatee, 20);

        client.delegate(&delegator, &delegatee);
        client.revoke_delegation(&delegator);

        assert_eq!(client.get_delegate(&delegator), None);
        // delegated power of delegatee is now 0
        assert_eq!(client.get_delegated_power(&delegatee), 0);

        let pid = client.create_proposal(
            &proposer,
            &1,
            &String::from_str(&env, "After revoke"),
            &3600,
        );

        // delegator can now vote directly
        client.vote(&delegator, &pid, &true);
        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.votes_for, 80);
    }

    #[test]
    fn test_get_delegators_list() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let delegatee = Address::generate(&env);
        let d1 = Address::generate(&env);
        let d2 = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &d1, 50);
        mint_tokens(&env, &at_id, &ec_id, &d2, 50);
        mint_tokens(&env, &at_id, &ec_id, &delegatee, 10);

        client.delegate(&d1, &delegatee);
        client.delegate(&d2, &delegatee);

        let list = client.get_delegators(&delegatee);
        assert_eq!(list.len(), 2);
    }

    #[test]
    fn test_admin_emergency_delegation_removal() {
        let (env, gov_id, at_id, ec_id, admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let delegator = Address::generate(&env);
        let delegatee = Address::generate(&env);
        mint_tokens(&env, &at_id, &ec_id, &delegator, 100);
        mint_tokens(&env, &at_id, &ec_id, &delegatee, 50);

        client.delegate(&delegator, &delegatee);
        client.revoke_delegation_admin(&admin, &delegator);

        assert_eq!(client.get_delegate(&delegator), None);
    }

    // -----------------------------------------------------------------------
    // 7. Multiple voters, weighted tally
    // -----------------------------------------------------------------------

    #[test]
    fn test_weighted_voting_with_multiple_voters() {
        let (env, gov_id, at_id, ec_id, _admin) = setup();
        let client = GovernanceClient::new(&env, &gov_id);

        let proposer = Address::generate(&env);
        let v1 = Address::generate(&env);
        let v2 = Address::generate(&env);
        let v3 = Address::generate(&env);

        mint_tokens(&env, &at_id, &ec_id, &proposer, 200);
        mint_tokens(&env, &at_id, &ec_id, &v1, 30);
        mint_tokens(&env, &at_id, &ec_id, &v2, 50);
        mint_tokens(&env, &at_id, &ec_id, &v3, 70);

        let pid = client.create_proposal(
            &proposer,
            &99,
            &String::from_str(&env, "Weighted"),
            &3600,
        );

        client.vote(&v1, &pid, &true);  // 30 for
        client.vote(&v2, &pid, &false); // 50 against
        client.vote(&v3, &pid, &true);  // 70 for  => total for=100, against=50

        env.ledger().with_mut(|li| {
            li.timestamp += 3601;
        });

        client.tally_execute(&pid);

        let p = client.get_proposal(&pid).unwrap();
        assert_eq!(p.votes_for, 100);
        assert_eq!(p.votes_against, 50);
        assert_eq!(p.status, ProposalStatus::Passed);
    }
}
