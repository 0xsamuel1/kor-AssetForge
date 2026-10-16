use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, Vec};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum OracleDataKey {
    /// Administrator address
    Admin,
    /// Set of authorized oracle addresses
    AuthorizedOracles,
    /// Latest price record for an asset: OracleDataKey::Price(asset_id)
    Price(u64),
    /// Aggregated (median) price for an asset: OracleDataKey::AggregatedPrice(asset_id)
    AggregatedPrice(u64),
    /// Per-oracle reputation score: OracleDataKey::Reputation(oracle_address)
    Reputation(Address),
    /// All submitted prices for the current round: OracleDataKey::Round(asset_id)
    Round(u64),
    /// Maximum allowed price deviation in basis points before an alert is emitted
    DeviationThresholdBps,
    /// Fallback price used when no valid feed is available: OracleDataKey::Fallback(asset_id)
    Fallback(u64),
    /// Timestamp of the last price update: OracleDataKey::LastUpdate(asset_id)
    LastUpdate(u64),
    /// Chainlink-like aggregator reference: OracleDataKey::Aggregator(asset_id)
    Aggregator(u64),
    /// Band Protocol validator set
    BandValidators,
    /// Band Protocol latest round data: OracleDataKey::BandRound(asset_id)
    BandRound(u64),
    /// Heartbeat interval in seconds for price update automation
    HeartbeatInterval,
    /// Last automation trigger timestamp
    LastAutomationTrigger,
    /// Staleness threshold in seconds: OracleDataKey::StalenessThreshold(asset_id)
    StalenessThreshold(u64),
    /// Maximum accepted age of a submitted price: OracleDataKey::MaxPriceAge
    MaxPriceAge,
    /// Address of a secondary oracle consulted when the primary feed is stale:
    /// OracleDataKey::FallbackSource(asset_id)
    FallbackSource(u64),
    /// Latest price reported by the fallback oracle: OracleDataKey::FallbackPrice(asset_id)
    FallbackPrice(u64),
    /// Default staleness threshold applied when an asset has no override.
    DefaultStalenessThreshold,
    /// Oldest recorded price timestamp (for stale window calculation)
    OldestPriceTimestamp(u64),
    /// Price source type flag: OracleDataKey::SourceType(asset_id) -> u32 (0=manual, 1=chainlink, 2=band)
    SourceType(u64),
    /// Marketplace contract address for price push
    Marketplace,
}

// ---------------------------------------------------------------------------
// Data Structures
// ---------------------------------------------------------------------------

/// A single price submission from one oracle.
#[derive(Clone)]
#[contracttype]
pub struct PriceSubmission {
    pub oracle: Address,
    pub price: i128,
    pub timestamp: u64,
}

/// The aggregated, canonical price for an asset.
#[derive(Clone)]
#[contracttype]
pub struct AggregatedPrice {
    pub asset_id: u64,
    pub price: i128,
    pub timestamp: u64,
    pub sources: u32,
}

/// Custom error codes for the oracle module.
#[derive(Clone, Copy, Debug, PartialEq)]
#[contracttype]
#[repr(u32)]
pub enum OracleError {
    NotAdmin = 1,
    OracleNotAuthorized = 2,
    NoSubmissionsForAsset = 3,
    PriceDeviationExceedsThreshold = 4,
    InsufficientSources = 5,
    StalePrice = 6,
    AggregatorNotSet = 7,
    BandValidatorNotAuthorized = 8,
    HeartbeatNotConfigured = 9,
    PriceTooOld = 10,
    InvalidSourceType = 11,
    FallbackSourceNotSet = 12,
}

/// Maximum age (in seconds) accepted for a price before it is considered stale.
/// Five minutes: long enough to tolerate a missed heartbeat, short enough that a
/// dead feed cannot be arbitraged against.
pub const DEFAULT_STALENESS_THRESHOLD: u64 = 300;

/// Chainlink-style price round data.
#[derive(Clone)]
#[contracttype]
pub struct RoundData {
    pub round_id: u64,
    pub price: i128,
    pub timestamp: u64,
    pub started_at: u64,
    pub answered_in_round: u64,
}

/// Band Protocol price data with multi-validator signature verification.
#[derive(Clone)]
#[contracttype]
pub struct BandPriceData {
    pub asset_id: u64,
    pub price: i128,
    pub timestamp: u64,
    pub validator_signatures: u32,
    pub required_signatures: u32,
}

/// Weighted price source from a specific provider.
#[derive(Clone)]
#[contracttype]
pub struct WeightedSource {
    pub source: Address,
    pub weight: u32,
    pub price: i128,
    pub timestamp: u64,
}

/// Automatically updated price record with heartbeat tracking.
#[derive(Clone)]
#[contracttype]
pub struct AutomatedPrice {
    pub asset_id: u64,
    pub price: i128,
    pub timestamp: u64,
    pub next_update_at: u64,
    pub source_type: u32,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn get_admin(env: &Env) -> Address {
    env.storage()
        .instance()
        .get::<OracleDataKey, Address>(&OracleDataKey::Admin)
        .expect("Oracle: admin not set")
}

fn require_admin(env: &Env, caller: &Address) {
    caller.require_auth();
    let admin = get_admin(env);
    if *caller != admin {
        panic!("Oracle: caller is not admin");
    }
}

fn get_authorized_oracles(env: &Env) -> Vec<Address> {
    env.storage()
        .instance()
        .get::<OracleDataKey, Vec<Address>>(&OracleDataKey::AuthorizedOracles)
        .unwrap_or_else(|| Vec::new(env))
}

fn is_authorized(env: &Env, oracle: &Address) -> bool {
    let oracles = get_authorized_oracles(env);
    for i in 0..oracles.len() {
        if oracles.get(i).unwrap() == *oracle {
            return true;
        }
    }
    false
}

/// Compute the median of a sorted slice represented as a Vec<i128>.
/// Assumes values are already sorted ascending.
fn median_sorted(values: &Vec<i128>) -> i128 {
    let len = values.len();
    if len == 0 {
        return 0;
    }
    let mid = len / 2;
    if len % 2 == 0 {
        let a = values.get((mid - 1) as u32).unwrap_or(0);
        let b = values.get(mid as u32).unwrap_or(0);
        (a + b) / 2
    } else {
        values.get(mid as u32).unwrap_or(0)
    }
}

/// Detect outliers: reject submissions whose price deviates more than
/// `threshold_bps` basis points from the current median.
fn filter_outliers(env: &Env, submissions: &Vec<PriceSubmission>, threshold_bps: u32) -> Vec<i128> {
    // Collect all prices into a sortable list.
    let mut prices: Vec<i128> = Vec::new(env);
    for i in 0..submissions.len() {
        prices.push_back(submissions.get(i).unwrap().price);
    }

    // Insertion sort (small datasets expected).
    let n = prices.len();
    for i in 1..n {
        let key = prices.get(i).unwrap();
        let mut j = i;
        while j > 0 {
            let prev = prices.get(j - 1).unwrap();
            if prev > key {
                prices.set(j, prev);
                j -= 1;
            } else {
                break;
            }
        }
        prices.set(j, key);
    }

    let med = median_sorted(&prices);
    if med == 0 {
        return prices;
    }

    let mut filtered: Vec<i128> = Vec::new(env);
    for i in 0..prices.len() {
        let p = prices.get(i).unwrap();
        let deviation_bps = if p >= med {
            ((p - med) * 10_000 / med) as u32
        } else {
            ((med - p) * 10_000 / med) as u32
        };
        if deviation_bps <= threshold_bps {
            filtered.push_back(p);
        }
    }
    filtered
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct Oracle;

#[contractimpl]
impl Oracle {
    /// Initialize the oracle contract with an admin address and default
    /// deviation threshold.
    pub fn initialize(env: Env, admin: Address, deviation_threshold_bps: u32) {
        admin.require_auth();
        env.storage()
            .instance()
            .set(&OracleDataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&OracleDataKey::DeviationThresholdBps, &deviation_threshold_bps);
        env.storage()
            .instance()
            .set(&OracleDataKey::AuthorizedOracles, &Vec::<Address>::new(&env));
        env.storage()
            .instance()
            .set(&OracleDataKey::DefaultStalenessThreshold, &DEFAULT_STALENESS_THRESHOLD);
    }

    /// Authorize a new oracle address to submit prices.
    pub fn authorize_oracle(env: Env, caller: Address, oracle: Address) {
        require_admin(&env, &caller);
        let mut oracles = get_authorized_oracles(&env);
        // Avoid duplicates.
        if !is_authorized(&env, &oracle) {
            oracles.push_back(oracle);
            env.storage()
                .instance()
                .set(&OracleDataKey::AuthorizedOracles, &oracles);
        }
    }

    /// Remove an oracle from the authorized set (e.g., after slashing).
    pub fn revoke_oracle(env: Env, caller: Address, oracle: Address) {
        require_admin(&env, &caller);
        let oracles = get_authorized_oracles(&env);
        let mut updated: Vec<Address> = Vec::new(&env);
        for i in 0..oracles.len() {
            let o = oracles.get(i).unwrap();
            if o != oracle {
                updated.push_back(o);
            }
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::AuthorizedOracles, &updated);
    }

    /// Submit a price for an asset. Only authorized oracles may call this.
    pub fn submit_price(env: Env, oracle: Address, asset_id: u64, price: i128) {
        oracle.require_auth();
        if !is_authorized(&env, &oracle) {
            panic!("Oracle: not authorized");
        }
        if price <= 0 {
            panic!("Oracle: price must be positive");
        }

        let timestamp = env.ledger().timestamp();
        let submission = PriceSubmission {
            oracle: oracle.clone(),
            price,
            timestamp,
        };

        // Append to the round buffer for this asset.
        let mut round: Vec<PriceSubmission> = env
            .storage()
            .temporary()
            .get::<OracleDataKey, Vec<PriceSubmission>>(&OracleDataKey::Round(asset_id))
            .unwrap_or_else(|| Vec::new(&env));
        round.push_back(submission);
        env.storage()
            .temporary()
            .set(&OracleDataKey::Round(asset_id), &round);

        // Update the per-oracle reputation (increment submission count).
        let reputation: u32 = env
            .storage()
            .instance()
            .get::<OracleDataKey, u32>(&OracleDataKey::Reputation(oracle.clone()))
            .unwrap_or(0);
        env.storage()
            .instance()
            .set(&OracleDataKey::Reputation(oracle), &(reputation + 1));
    }

    /// Aggregate all round submissions for an asset into a median price,
    /// filtering outliers. Requires at least `min_sources` valid submissions.
    pub fn aggregate(env: Env, asset_id: u64, min_sources: u32) -> AggregatedPrice {
        let threshold_bps: u32 = env
            .storage()
            .instance()
            .get::<OracleDataKey, u32>(&OracleDataKey::DeviationThresholdBps)
            .unwrap_or(500); // default 5%

        let round: Vec<PriceSubmission> = env
            .storage()
            .temporary()
            .get::<OracleDataKey, Vec<PriceSubmission>>(&OracleDataKey::Round(asset_id))
            .unwrap_or_else(|| Vec::new(&env));

        if round.is_empty() {
            // Try fallback.
            if let Some(fallback) = env
                .storage()
                .instance()
                .get::<OracleDataKey, i128>(&OracleDataKey::Fallback(asset_id))
            {
                return AggregatedPrice {
                    asset_id,
                    price: fallback,
                    timestamp: env.ledger().timestamp(),
                    sources: 0,
                };
            }
            panic!("Oracle: no submissions and no fallback for asset");
        }

        // Drop submissions whose timestamp is already older than the accepted
        // maximum age: a late relayer must not be able to resurrect a dead feed.
        let now = env.ledger().timestamp();
        let max_age = Self::get_max_price_age(env.clone());
        let mut fresh_round: Vec<PriceSubmission> = Vec::new(&env);
        for i in 0..round.len() {
            let submission = round.get(i).unwrap();
            if now.saturating_sub(submission.timestamp) <= max_age {
                fresh_round.push_back(submission);
            }
        }
        if fresh_round.is_empty() {
            panic!("Oracle: all price submissions are stale");
        }

        let filtered = filter_outliers(&env, &fresh_round, threshold_bps);

        if filtered.len() < min_sources {
            panic!("Oracle: insufficient valid price sources after outlier removal");
        }

        let price = median_sorted(&filtered);
        let timestamp = env.ledger().timestamp();

        let result = AggregatedPrice {
            asset_id,
            price,
            timestamp,
            sources: filtered.len(),
        };

        // Persist and clear round buffer.
        env.storage()
            .instance()
            .set(&OracleDataKey::AggregatedPrice(asset_id), &result);
        env.storage()
            .instance()
            .set(&OracleDataKey::LastUpdate(asset_id), &timestamp);
        env.storage()
            .temporary()
            .remove(&OracleDataKey::Round(asset_id));

        // Emit price deviation alert if new price deviates from previous.
        if let Some(prev) = env
            .storage()
            .instance()
            .get::<OracleDataKey, AggregatedPrice>(&OracleDataKey::Price(asset_id))
        {
            let prev_price = prev.price;
            if prev_price > 0 {
                let deviation_bps = if price >= prev_price {
                    ((price - prev_price) * 10_000 / prev_price) as u32
                } else {
                    ((prev_price - price) * 10_000 / prev_price) as u32
                };
                if deviation_bps > threshold_bps {
                    env.events().publish(
                        (Symbol::new(&env, "price_deviation"),),
                        (asset_id, prev_price, price, deviation_bps),
                    );
                }
            }
        }

        env.storage()
            .instance()
            .set(&OracleDataKey::Price(asset_id), &result);

        result
    }

    /// Return the latest aggregated price for an asset.
    pub fn get_price(env: Env, asset_id: u64) -> AggregatedPrice {
        env.storage()
            .instance()
            .get::<OracleDataKey, AggregatedPrice>(&OracleDataKey::AggregatedPrice(asset_id))
            .expect("Oracle: no aggregated price for asset")
    }

    /// Set a fallback price used when no live oracle data is available.
    pub fn set_fallback_price(env: Env, caller: Address, asset_id: u64, price: i128) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::Fallback(asset_id), &price);
    }

    /// Override the deviation threshold (in basis points).
    pub fn set_deviation_threshold(env: Env, caller: Address, threshold_bps: u32) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::DeviationThresholdBps, &threshold_bps);
    }

    /// Return the reputation (submission count) of an oracle.
    pub fn get_reputation(env: Env, oracle: Address) -> u32 {
        env.storage()
            .instance()
            .get::<OracleDataKey, u32>(&OracleDataKey::Reputation(oracle))
            .unwrap_or(0)
    }

    /// Emergency override: admin sets a canonical price directly, bypassing
    /// the oracle round (used during incidents).
    pub fn emergency_price_override(env: Env, caller: Address, asset_id: u64, price: i128) {
        require_admin(&env, &caller);
        let timestamp = env.ledger().timestamp();
        let result = AggregatedPrice {
            asset_id,
            price,
            timestamp,
            sources: 0,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::AggregatedPrice(asset_id), &result);
        env.storage()
            .instance()
            .set(&OracleDataKey::Price(asset_id), &result);
        env.storage()
            .instance()
            .set(&OracleDataKey::LastUpdate(asset_id), &timestamp);
        env.events().publish(
            (Symbol::new(&env, "emergency_price"),),
            (asset_id, price, caller),
        );
    }

    // -----------------------------------------------------------------------
    // Price Feed Aggregator — Chainlink-style round management
    // -----------------------------------------------------------------------

    /// Set an aggregator contract address for a given asset (Chainlink-style).
    pub fn set_aggregator(env: Env, caller: Address, asset_id: u64, aggregator: Address) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::Aggregator(asset_id), &aggregator);
        env.events().publish(
            (Symbol::new(&env, "aggregator_set"),),
            (asset_id, aggregator),
        );
    }

    /// Submit a round of price data from an aggregator (simulates Chainlink
    /// OCR / off-chain reporting).
    pub fn submit_aggregator_round(
        env: Env,
        caller: Address,
        asset_id: u64,
        round_id: u64,
        price: i128,
        started_at: u64,
    ) {
        require_admin(&env, &caller);
        let timestamp = env.ledger().timestamp();
        let round = RoundData {
            round_id,
            price,
            timestamp,
            started_at,
            answered_in_round: round_id,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::Price(asset_id), &round);

        let aggregated = AggregatedPrice {
            asset_id,
            price,
            timestamp,
            sources: 1,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::AggregatedPrice(asset_id), &aggregated);
        env.storage()
            .instance()
            .set(&OracleDataKey::LastUpdate(asset_id), &timestamp);

        env.events().publish(
            (Symbol::new(&env, "aggregator_round"),),
            (asset_id, round_id, price),
        );
    }

    /// Get the latest round data for an asset (Chainlink compatibility).
    pub fn latest_round_data(env: Env, asset_id: u64) -> RoundData {
        env.storage()
            .instance()
            .get::<OracleDataKey, RoundData>(&OracleDataKey::Price(asset_id))
            .expect("Oracle: no round data for asset")
    }

    // -----------------------------------------------------------------------
    // Band Protocol Integration — multi-validator price feed
    // -----------------------------------------------------------------------

    /// Register a Band Protocol validator address.
    pub fn register_band_validator(env: Env, caller: Address, validator: Address) {
        require_admin(&env, &caller);
        let mut validators: Vec<Address> = env
            .storage()
            .instance()
            .get(&OracleDataKey::BandValidators)
            .unwrap_or_else(|| Vec::new(&env));
        let mut exists = false;
        for i in 0..validators.len() {
            if validators.get(i).unwrap() == validator {
                exists = true;
                break;
            }
        }
        if !exists {
            validators.push_back(validator);
            env.storage()
                .instance()
                .set(&OracleDataKey::BandValidators, &validators);
        }
    }

    /// Submit a Band Protocol price with validator signatures.
    pub fn submit_band_price(
        env: Env,
        asset_id: u64,
        price: i128,
        timestamp: u64,
        signatures: Vec<Address>,
        required_signatures: u32,
    ) {
        let validators: Vec<Address> = env
            .storage()
            .instance()
            .get(&OracleDataKey::BandValidators)
            .unwrap_or_else(|| Vec::new(&env));
        if validators.is_empty() {
            panic!("Oracle: no Band validators registered");
        }

        // A validator supplied timestamp must be recent: a stale round must never
        // be able to overwrite the aggregated price.
        Self::validate_submission_timestamp(&env, timestamp);

        if price <= 0 {
            panic!("Oracle: price must be positive");
        }

        let mut valid_sigs: u32 = 0;
        for i in 0..signatures.len() {
            let signer = signatures.get(i).unwrap();
            for j in 0..validators.len() {
                if validators.get(j).unwrap() == signer {
                    valid_sigs += 1;
                    break;
                }
            }
        }

        if valid_sigs < required_signatures {
            panic!("Oracle: insufficient Band validator signatures");
        }

        let band = BandPriceData {
            asset_id,
            price,
            timestamp,
            validator_signatures: valid_sigs,
            required_signatures,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::BandRound(asset_id), &band);

        let aggregated = AggregatedPrice {
            asset_id,
            price,
            timestamp,
            sources: valid_sigs,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::AggregatedPrice(asset_id), &aggregated);
        env.storage()
            .instance()
            .set(&OracleDataKey::LastUpdate(asset_id), &timestamp);

        env.events().publish(
            (Symbol::new(&env, "band_price"),),
            (asset_id, price, valid_sigs),
        );
    }

    /// Get the latest Band Protocol price data.
    pub fn get_band_price(env: Env, asset_id: u64) -> BandPriceData {
        env.storage()
            .instance()
            .get::<OracleDataKey, BandPriceData>(&OracleDataKey::BandRound(asset_id))
            .expect("Oracle: no Band price for asset")
    }

    // -----------------------------------------------------------------------
    // Price Update Automation — heartbeat-based triggers
    // -----------------------------------------------------------------------

    /// Set the global heartbeat interval for automated price updates.
    pub fn set_heartbeat_interval(env: Env, caller: Address, interval_seconds: u64) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::HeartbeatInterval, &interval_seconds);
        env.events().publish(
            (Symbol::new(&env, "heartbeat_set"),),
            interval_seconds,
        );
    }

    /// Get the heartbeat interval.
    pub fn get_heartbeat_interval(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&OracleDataKey::HeartbeatInterval)
            .unwrap_or(3600)
    }

    /// Check if a price update is due for the given asset based on heartbeat.
    pub fn is_update_due(env: Env, asset_id: u64) -> bool {
        let last_update: u64 = env
            .storage()
            .instance()
            .get(&OracleDataKey::LastUpdate(asset_id))
            .unwrap_or(0);
        let heartbeat = Self::get_heartbeat_interval(env.clone());
        let now = env.ledger().timestamp();
        now >= last_update + heartbeat
    }

    /// Automatically trigger a price update using the latest aggregated price
    /// from round data. This can be called by any external automation (keeper).
    pub fn automated_price_update(env: Env, asset_id: u64) -> AutomatedPrice {
        let heartbeat = Self::get_heartbeat_interval(env.clone());
        let last_update: u64 = env
            .storage()
            .instance()
            .get(&OracleDataKey::LastUpdate(asset_id))
            .unwrap_or(0);
        let now = env.ledger().timestamp();

        if now < last_update + heartbeat {
            panic!("Oracle: heartbeat interval not elapsed");
        }

        let aggregated: AggregatedPrice = env
            .storage()
            .instance()
            .get(&OracleDataKey::AggregatedPrice(asset_id))
            .expect("Oracle: no aggregated price to automate");

        let auto = AutomatedPrice {
            asset_id,
            price: aggregated.price,
            timestamp: now,
            next_update_at: now + heartbeat,
            source_type: 0,
        };

        env.storage()
            .instance()
            .set(&OracleDataKey::LastUpdate(asset_id), &now);
        env.storage()
            .instance()
            .set(&OracleDataKey::LastAutomationTrigger, &now);

        env.events().publish(
            (Symbol::new(&env, "price_auto_updated"),),
            (asset_id, aggregated.price, now),
        );

        auto
    }

    /// Set the source type for an asset: 0=manual, 1=chainlink, 2=band.
    pub fn set_source_type(env: Env, caller: Address, asset_id: u64, source_type: u32) {
        require_admin(&env, &caller);
        if source_type > 2 {
            panic!("Oracle: invalid source type");
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::SourceType(asset_id), &source_type);
    }

    /// Get the source type for an asset.
    pub fn get_source_type(env: Env, asset_id: u64) -> u32 {
        env.storage()
            .instance()
            .get(&OracleDataKey::SourceType(asset_id))
            .unwrap_or(0)
    }

    // -----------------------------------------------------------------------
    // Marketplace Integration
    // -----------------------------------------------------------------------

    /// Set the marketplace contract address for price push notifications.
    pub fn set_marketplace(env: Env, caller: Address, marketplace: Address) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::Marketplace, &marketplace);
        env.events().publish(
            (Symbol::new(&env, "marketplace_set"),),
            marketplace,
        );
    }

    /// Get the marketplace contract address.
    pub fn get_marketplace(env: Env) -> Option<Address> {
        env.storage().instance().get(&OracleDataKey::Marketplace)
    }

    // -----------------------------------------------------------------------
    // Stale Price Detection
    // -----------------------------------------------------------------------

    /// Set the staleness threshold for an asset (in seconds).
    ///
    /// A threshold of `0` is rejected: it would mark every price, including one
    /// submitted in the current ledger, as stale.
    pub fn set_staleness_threshold(env: Env, caller: Address, asset_id: u64, threshold: u64) {
        require_admin(&env, &caller);
        if threshold == 0 {
            panic!("Oracle: staleness threshold must be greater than zero");
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::StalenessThreshold(asset_id), &threshold);
    }

    /// Set the default staleness threshold used for assets without an override.
    pub fn set_default_staleness_threshold(env: Env, caller: Address, threshold: u64) {
        require_admin(&env, &caller);
        if threshold == 0 {
            panic!("Oracle: staleness threshold must be greater than zero");
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::DefaultStalenessThreshold, &threshold);
    }

    /// Resolve the effective staleness threshold for an asset: the per-asset
    /// override if present, otherwise the configured default, otherwise the
    /// compile-time default of five minutes.
    pub fn get_staleness_threshold(env: Env, asset_id: u64) -> u64 {
        if let Some(threshold) = env
            .storage()
            .instance()
            .get::<OracleDataKey, u64>(&OracleDataKey::StalenessThreshold(asset_id))
        {
            return threshold;
        }
        env.storage()
            .instance()
            .get::<OracleDataKey, u64>(&OracleDataKey::DefaultStalenessThreshold)
            .unwrap_or(DEFAULT_STALENESS_THRESHOLD)
    }

    /// Check whether the price for an asset is stale.
    pub fn is_price_stale(env: Env, asset_id: u64) -> bool {
        let threshold = Self::get_staleness_threshold(env.clone(), asset_id);
        let last_update: u64 = env
            .storage()
            .instance()
            .get(&OracleDataKey::LastUpdate(asset_id))
            .unwrap_or(0);
        let now = env.ledger().timestamp();
        // An asset that has never reported is always stale.
        if last_update == 0 {
            return true;
        }
        now.saturating_sub(last_update) > threshold
    }

    /// Return the age of the current price for an asset in seconds.
    ///
    /// Returns `u64::MAX` when the asset has never reported, i.e. the data is
    /// infinitely old.
    pub fn get_price_age(env: Env, asset_id: u64) -> u64 {
        let last_update: u64 = env
            .storage()
            .instance()
            .get(&OracleDataKey::LastUpdate(asset_id))
            .unwrap_or(0);
        let now = env.ledger().timestamp();
        if last_update == 0 {
            return u64::MAX;
        }
        now.saturating_sub(last_update)
    }

    /// Return the current price, failing if it is older than the staleness
    /// threshold. This is the entry point every pricing transaction must use so
    /// stale data can never be traded against.
    pub fn get_fresh_price(env: Env, asset_id: u64) -> AggregatedPrice {
        let price: AggregatedPrice = env
            .storage()
            .instance()
            .get(&OracleDataKey::AggregatedPrice(asset_id))
            .expect("Oracle: no aggregated price for asset");

        if Self::is_price_stale(env.clone(), asset_id) {
            let age = Self::get_price_age(env.clone(), asset_id);
            env.events().publish((Symbol::new(&env, "stale_price"),), (asset_id, age));
            panic!("Oracle: stale price");
        }
        price
    }

    /// Return the current price, falling back to the secondary oracle when the
    /// primary feed is stale. Panics when the price is stale and no fresh
    /// fallback is available, so a transaction can never settle against stale
    /// data.
    pub fn get_price_with_fallback(env: Env, asset_id: u64) -> AggregatedPrice {
        if !Self::is_price_stale(env.clone(), asset_id) {
            return Self::get_fresh_price(env, asset_id);
        }

        let age = Self::get_price_age(env.clone(), asset_id);
        env.events().publish((Symbol::new(&env, "stale_price"),), (asset_id, age));

        let fallback_source: Address = env
            .storage()
            .instance()
            .get(&OracleDataKey::FallbackSource(asset_id))
            .expect("Oracle: stale price and no fallback source configured");

        let fallback: AggregatedPrice = env
            .storage()
            .instance()
            .get(&OracleDataKey::FallbackPrice(asset_id))
            .expect("Oracle: stale price and fallback source has not reported");

        let now = env.ledger().timestamp();
        let threshold = Self::get_staleness_threshold(env.clone(), asset_id);
        if now.saturating_sub(fallback.timestamp) > threshold {
            panic!("Oracle: stale price and fallback price is also stale");
        }

        env.events().publish(
            (Symbol::new(&env, "fallback_price_used"),),
            (asset_id, fallback.price, fallback_source),
        );

        fallback
    }

    /// Submit a price as the designated fallback oracle for an asset. Only the
    /// configured fallback source may call this.
    pub fn submit_fallback_price(env: Env, oracle: Address, asset_id: u64, price: i128) {
        oracle.require_auth();
        if !is_authorized(&env, &oracle) {
            panic!("Oracle: not authorized");
        }
        let fallback_source: Address = env
            .storage()
            .instance()
            .get(&OracleDataKey::FallbackSource(asset_id))
            .expect("Oracle: no fallback source configured for asset");
        if oracle != fallback_source {
            panic!("Oracle: caller is not the fallback source for this asset");
        }

        let timestamp = env.ledger().timestamp();
        let fallback = AggregatedPrice {
            asset_id,
            price,
            timestamp,
            sources: 1,
        };
        env.storage()
            .instance()
            .set(&OracleDataKey::FallbackPrice(asset_id), &fallback);
        env.events().publish(
            (Symbol::new(&env, "fallback_price_submitted"),),
            (asset_id, price, timestamp),
        );
    }

    /// Get the latest price submitted by the fallback oracle, if any.
    pub fn get_fallback_price(env: Env, asset_id: u64) -> Option<AggregatedPrice> {
        env.storage()
            .instance()
            .get(&OracleDataKey::FallbackPrice(asset_id))
    }

    /// Register a secondary oracle consulted when the primary feed is stale.
    pub fn set_fallback_source(env: Env, caller: Address, asset_id: u64, source: Address) {
        require_admin(&env, &caller);
        if !is_authorized(&env, &source) {
            panic!("Oracle: fallback source is not an authorized oracle");
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::FallbackSource(asset_id), &source);
        env.events().publish(
            (Symbol::new(&env, "fallback_source_set"),),
            (asset_id, source),
        );
    }

    /// Get the configured fallback source for an asset.
    pub fn get_fallback_source(env: Env, asset_id: u64) -> Option<Address> {
        env.storage()
            .instance()
            .get(&OracleDataKey::FallbackSource(asset_id))
    }

    /// Reject a submission whose reported timestamp is older than the accepted
    /// maximum age. Prevents a relayer from pinning a permanently stale price.
    pub fn set_max_price_age(env: Env, caller: Address, max_age: u64) {
        require_admin(&env, &caller);
        if max_age == 0 {
            panic!("Oracle: max price age must be greater than zero");
        }
        env.storage()
            .instance()
            .set(&OracleDataKey::MaxPriceAge, &max_age);
    }

    /// Maximum accepted age of a submitted price timestamp.
    pub fn get_max_price_age(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&OracleDataKey::MaxPriceAge)
            .unwrap_or(DEFAULT_STALENESS_THRESHOLD)
    }

    /// Validate that `timestamp` is not too old and not in the future.
    fn validate_submission_timestamp(env: &Env, timestamp: u64) {
        let now = env.ledger().timestamp();
        let max_age = Self::get_max_price_age(env.clone());
        if timestamp > now {
            panic!("Oracle: submission timestamp is in the future");
        }
        if now.saturating_sub(timestamp) > max_age {
            panic!("Oracle: submitted price is too old");
        }
    }

    /// Record the oldest price timestamp for tracking the full staleness window.
    pub fn record_oldest_timestamp(env: Env, caller: Address, asset_id: u64, timestamp: u64) {
        require_admin(&env, &caller);
        env.storage()
            .instance()
            .set(&OracleDataKey::OldestPriceTimestamp(asset_id), &timestamp);
    }

    /// Get the oldest recorded price timestamp.
    pub fn get_oldest_timestamp(env: Env, asset_id: u64) -> u64 {
        env.storage()
            .instance()
            .get(&OracleDataKey::OldestPriceTimestamp(asset_id))
            .unwrap_or(0)
    }

    /// Age of the oldest price still inside the validity window, used by
    /// monitoring to detect feeds that are about to go stale.
    pub fn get_staleness_window(env: Env, asset_id: u64) -> u64 {
        let oldest = Self::get_oldest_timestamp(env.clone(), asset_id);
        if oldest == 0 {
            return 0;
        }
        let now = env.ledger().timestamp();
        now.saturating_sub(oldest)
    }

    // -----------------------------------------------------------------------
    // Weighted Multi-Source Aggregation
    // -----------------------------------------------------------------------

    /// Aggregate prices from multiple weighted sources into a single price.
    pub fn aggregate_weighted(_env: Env, sources: Vec<WeightedSource>, min_total_weight: u32) -> i128 {
        let mut total_weight: u32 = 0;
        let mut weighted_sum: i128 = 0;

        for i in 0..sources.len() {
            let source = sources.get(i).unwrap();
            total_weight += source.weight;
            weighted_sum += source.price * source.weight as i128;
        }

        if total_weight < min_total_weight {
            panic!("Oracle: insufficient total weight for aggregation");
        }

        if total_weight == 0 {
            return 0;
        }

        weighted_sum / total_weight as i128
    }
}

// ===========================================================================
// Staleness Tests (#344)
// ===========================================================================

#[cfg(test)]
mod staleness_tests {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    const ASSET: u64 = 42;

    fn setup() -> (Env, Address, Address, OracleClient<'static>) {
        let env = Env::default();
        env.mock_all_auths();

        // Start the mocked ledger away from zero so "never reported" (stored
        // timestamp 0) stays distinguishable from a real report.
        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);
        let fallback = Address::generate(&env);
        let contract_id = env.register_contract(None, Oracle);
        let client = OracleClient::new(&env, &contract_id);

        client.initialize(&admin, &500);
        client.authorize_oracle(&admin, &oracle);
        client.authorize_oracle(&admin, &fallback);

        (env, admin, oracle, client)
    }

    /// Publish an aggregated price for `ASSET` at the current ledger timestamp.
    fn publish_price(client: &OracleClient, admin: &Address, price: i128) {
        client.emergency_price_override(admin, &ASSET, &price);
    }

    fn signers(env: &Env, addresses: &[Address]) -> Vec<Address> {
        let mut signers = Vec::new(env);
        for addr in addresses {
            signers.push_back(addr.clone());
        }
        signers
    }

    fn advance(env: &Env, seconds: u64) {
        env.ledger().with_mut(|li| li.timestamp += seconds);
    }

    #[test]
    fn default_staleness_threshold_is_five_minutes() {
        let (_env, admin, _oracle, client) = setup();
        assert_eq!(client.get_staleness_threshold(&ASSET), 300);
        assert_eq!(DEFAULT_STALENESS_THRESHOLD, 300);
        let _ = admin;
    }

    #[test]
    fn asset_never_reported_is_stale() {
        let (_env, _admin, _oracle, client) = setup();
        assert!(client.is_price_stale(&ASSET));
        assert_eq!(client.get_price_age(&ASSET), u64::MAX);
    }

    #[test]
    fn price_within_threshold_is_fresh() {
        let (env, admin, _oracle, client) = setup();
        publish_price(&client, &admin, 1_000);

        advance(&env, DEFAULT_STALENESS_THRESHOLD);
        assert!(!client.is_price_stale(&ASSET));
        assert_eq!(client.get_price_age(&ASSET), 300);

        let price = client.get_fresh_price(&ASSET);
        assert_eq!(price.price, 1_000);
    }

    #[test]
    #[should_panic(expected = "Oracle: stale price")]
    fn fresh_price_panics_once_threshold_exceeded() {
        let (env, admin, _oracle, client) = setup();
        publish_price(&client, &admin, 1_000);

        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);
        client.get_fresh_price(&ASSET);
    }

    #[test]
    fn per_asset_threshold_overrides_default() {
        let (env, admin, _oracle, client) = setup();
        publish_price(&client, &admin, 1_000);

        client.set_staleness_threshold(&admin, &ASSET, &30);
        assert_eq!(client.get_staleness_threshold(&ASSET), 30);

        advance(&env, 31);
        assert!(client.is_price_stale(&ASSET));
    }

    #[test]
    #[should_panic(expected = "Oracle: staleness threshold must be greater than zero")]
    fn zero_staleness_threshold_rejected() {
        let (_env, admin, _oracle, client) = setup();
        client.set_staleness_threshold(&admin, &ASSET, &0);
    }

    #[test]
    #[should_panic(expected = "Oracle: staleness threshold must be greater than zero")]
    fn zero_default_staleness_threshold_rejected() {
        let (_env, admin, _oracle, client) = setup();
        client.set_default_staleness_threshold(&admin, &0);
    }

    #[test]
    fn default_staleness_threshold_can_be_configured() {
        let (env, admin, _oracle, client) = setup();
        client.set_default_staleness_threshold(&admin, &60);
        assert_eq!(client.get_staleness_threshold(&ASSET), 60);

        publish_price(&client, &admin, 500);
        advance(&env, 61);
        assert!(client.is_price_stale(&ASSET));
    }

    #[test]
    #[should_panic(expected = "Oracle: stale price and no fallback source configured")]
    fn fallback_lookup_panics_without_configured_source() {
        let (env, admin, _oracle, client) = setup();
        publish_price(&client, &admin, 1_000);

        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);
        client.get_price_with_fallback(&ASSET);
    }

    #[test]
    fn fallback_source_serves_price_when_primary_is_stale() {
        let (env, admin, _oracle, client) = setup();
        let fallback = Address::generate(&env);
        client.authorize_oracle(&admin, &fallback);
        client.set_fallback_source(&admin, &ASSET, &fallback);
        assert_eq!(client.get_fallback_source(&ASSET), Some(fallback.clone()));

        publish_price(&client, &admin, 1_000);
        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);
        assert!(client.is_price_stale(&ASSET));

        client.submit_fallback_price(&fallback, &ASSET, &1_100i128);
        let price = client.get_price_with_fallback(&ASSET);
        assert_eq!(price.price, 1_100);
        assert_eq!(client.get_fallback_price(&ASSET).unwrap().price, 1_100);
    }

    #[test]
    fn primary_source_used_while_fresh_even_with_fallback() {
        let (env, admin, _oracle, client) = setup();
        let fallback = Address::generate(&env);
        client.authorize_oracle(&admin, &fallback);
        client.set_fallback_source(&admin, &ASSET, &fallback);
        client.submit_fallback_price(&fallback, &ASSET, &1_100i128);

        publish_price(&client, &admin, 1_000);
        assert_eq!(client.get_price_with_fallback(&ASSET).price, 1_000);
    }

    #[test]
    #[should_panic(expected = "Oracle: stale price and fallback price is also stale")]
    fn stale_fallback_is_also_rejected() {
        let (env, admin, _oracle, client) = setup();
        let fallback = Address::generate(&env);
        client.authorize_oracle(&admin, &fallback);
        client.set_fallback_source(&admin, &ASSET, &fallback);

        publish_price(&client, &admin, 1_000);
        client.submit_fallback_price(&fallback, &ASSET, &1_100i128);

        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);
        client.get_price_with_fallback(&ASSET);
    }

    #[test]
    #[should_panic(expected = "Oracle: fallback source is not an authorized oracle")]
    fn fallback_source_must_be_authorized() {
        let (_env, admin, _oracle, client) = setup();
        let stranger = Address::generate(&_env);
        client.set_fallback_source(&admin, &ASSET, &stranger);
    }

    #[test]
    #[should_panic(expected = "Oracle: caller is not the fallback source for this asset")]
    fn only_fallback_source_may_submit_fallback_price() {
        let (env, admin, oracle, client) = setup();
        let fallback = Address::generate(&env);
        client.authorize_oracle(&admin, &fallback);
        client.set_fallback_source(&admin, &ASSET, &fallback);

        client.submit_fallback_price(&oracle, &ASSET, &42i128);
    }

    #[test]
    #[should_panic(expected = "Oracle: submitted price is too old")]
    fn band_submission_with_stale_timestamp_is_rejected() {
        let (env, admin, _oracle, client) = setup();
        let validator = Address::generate(&env);
        client.register_band_validator(&admin, &validator);

        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);
        client.submit_band_price(&ASSET, &1_000i128, &0, &signers(&env, &[validator.clone()]), &1);
    }

    #[test]
    #[should_panic(expected = "Oracle: submission timestamp is in the future")]
    fn band_submission_with_future_timestamp_is_rejected() {
        let (env, admin, _oracle, client) = setup();
        let validator = Address::generate(&env);
        client.register_band_validator(&admin, &validator);

        let future = env.ledger().timestamp() + 1_000;
        client.submit_band_price(&ASSET, &1_000i128, &future, &signers(&env, &[validator]), &1);
    }

    #[test]
    fn band_submission_within_max_age_is_accepted() {
        let (env, admin, _oracle, client) = setup();
        let validator = Address::generate(&env);
        client.register_band_validator(&admin, &validator);

        let now = env.ledger().timestamp();
        advance(&env, 60);
        client.submit_band_price(
            &ASSET,
            &1_000i128,
            &(now - 60),
            &signers(&env, &[validator]),
            &1,
        );
        assert_eq!(client.get_band_price(&ASSET).price, 1_000);
    }

    #[test]
    #[should_panic(expected = "Oracle: price must be positive")]
    fn non_positive_submission_is_rejected() {
        let (_env, _admin, oracle, client) = setup();
        client.submit_price(&oracle, &ASSET, &0i128);
    }

    #[test]
    fn max_price_age_is_configurable() {
        let (_env, admin, _oracle, client) = setup();
        assert_eq!(client.get_max_price_age(), 300);
        client.set_max_price_age(&admin, &60);
        assert_eq!(client.get_max_price_age(), 60);
    }

    #[test]
    #[should_panic(expected = "Oracle: max price age must be greater than zero")]
    fn zero_max_price_age_is_rejected() {
        let (_env, admin, _oracle, client) = setup();
        client.set_max_price_age(&admin, &0);
    }

    #[test]
    fn aggregation_rejects_all_stale_submissions() {
        let (env, _admin, oracle, client) = setup();

        client.submit_price(&oracle, &ASSET, &1_000);
        advance(&env, DEFAULT_STALENESS_THRESHOLD + 1);

        assert!(client.try_aggregate(&ASSET, &1).is_err());
    }

    #[test]
    fn staleness_window_tracks_oldest_price() {
        let (env, admin, _oracle, client) = setup();
        // Nothing recorded yet: no window.
        assert_eq!(client.get_staleness_window(&ASSET), 0);

        let first_report = env.ledger().timestamp();
        client.record_oldest_timestamp(&admin, &ASSET, &first_report);
        advance(&env, 120);

        assert_eq!(client.get_staleness_window(&ASSET), 120);
        assert_eq!(client.get_oldest_timestamp(&ASSET), first_report);
    }
}
