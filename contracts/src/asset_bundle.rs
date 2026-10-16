use soroban_sdk::{
    contract, contractimpl, contracttype, Address, Env, String, Symbol, Vec,
};

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone)]
#[contracttype]
pub enum BundleDataKey {
    Admin,
    NextBundleId,
    Bundle(u64),
    /// Index of bundles owned by an address
    OwnerBundles(Address),
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

#[derive(Clone, PartialEq, Debug)]
#[contracttype]
pub enum BundleStatus {
    Active,
    Listed,
    Sold,
    Unwrapped,
}

#[derive(Clone)]
#[contracttype]
pub struct AssetBundle {
    pub bundle_id: u64,
    pub name: String,
    pub description: String,
    pub creator: Address,
    pub owner: Address,
    /// List of asset IDs that form this bundle
    pub asset_ids: Vec<u64>,
    /// Listing price in stroops (0 if not listed)
    pub list_price: i128,
    /// Discount in basis points applied when purchased through marketplace (0-10000)
    pub discount_bps: u32,
    pub status: BundleStatus,
    pub created_at: u64,
    pub sold_at: Option<u64>,
    pub buyer: Option<Address>,
}

// ---------------------------------------------------------------------------
// Contract
// ---------------------------------------------------------------------------

#[contract]
pub struct AssetBundleContract;

#[contractimpl]
impl AssetBundleContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        env.storage().instance().set(&BundleDataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&BundleDataKey::NextBundleId, &1u64);
    }

    /// Creator assembles a bundle of asset IDs. Validates that asset_ids are non-empty
    /// and that the creator is declared as the owner at creation time.
    pub fn create_bundle(
        env: Env,
        creator: Address,
        name: String,
        description: String,
        asset_ids: Vec<u64>,
    ) -> u64 {
        creator.require_auth();
        assert!(!asset_ids.is_empty(), "bundle must contain at least one asset");

        // Validate no duplicate asset IDs in the bundle
        Self::assert_no_duplicate_assets(&env, &asset_ids);

        let id: u64 = env
            .storage()
            .instance()
            .get(&BundleDataKey::NextBundleId)
            .unwrap_or(1);
        let bundle = AssetBundle {
            bundle_id: id,
            name: name.clone(),
            description,
            creator: creator.clone(),
            owner: creator.clone(),
            asset_ids: asset_ids.clone(),
            list_price: 0,
            discount_bps: 0,
            status: BundleStatus::Active,
            created_at: env.ledger().timestamp(),
            sold_at: None,
            buyer: None,
        };
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(id), &bundle);

        let mut owned: Vec<u64> = env
            .storage()
            .instance()
            .get(&BundleDataKey::OwnerBundles(creator.clone()))
            .unwrap_or(Vec::new(&env));
        owned.push_back(id);
        env.storage()
            .instance()
            .set(&BundleDataKey::OwnerBundles(creator.clone()), &owned);

        env.storage()
            .instance()
            .set(&BundleDataKey::NextBundleId, &(id + 1));

        env.events().publish(
            (Symbol::new(&env, "bundle_created"), creator),
            (id, name, asset_ids.len()),
        );

        id
    }

    /// Creator lists the bundle for sale at a given price with an optional discount.
    pub fn list_bundle(env: Env, creator: Address, bundle_id: u64, price: i128, discount_bps: u32) {
        creator.require_auth();
        assert!(price > 0, "price must be positive");
        assert!(discount_bps <= 10000, "discount must be <= 10000");

        let mut bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        assert_eq!(bundle.owner, creator, "only owner can list");
        assert_eq!(bundle.status, BundleStatus::Active, "bundle not active");

        bundle.list_price = price;
        bundle.discount_bps = discount_bps;
        bundle.status = BundleStatus::Listed;
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(bundle_id), &bundle);

        env.events().publish(
            (Symbol::new(&env, "bundle_listed"), creator),
            (bundle_id, price, discount_bps),
        );
    }

    /// Compute the effective price after applying the stored discount.
    pub fn get_effective_price(env: Env, bundle_id: u64) -> i128 {
        let bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        Self::apply_discount(bundle.list_price, bundle.discount_bps)
    }

    /// Buyer purchases a listed bundle. Payment goes from buyer to owner at the
    /// discounted effective price.
    pub fn buy_bundle(
        env: Env,
        buyer: Address,
        bundle_id: u64,
        token: Address,
    ) {
        buyer.require_auth();
        let mut bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        assert_eq!(bundle.status, BundleStatus::Listed, "bundle not listed for sale");
        assert_ne!(bundle.owner, buyer, "owner cannot buy own bundle");

        let effective_price = Self::apply_discount(bundle.list_price, bundle.discount_bps);
        let token_client = soroban_sdk::token::Client::new(&env, &token);
        token_client.transfer(&buyer, &bundle.owner, &effective_price);

        let prev_owner = bundle.owner.clone();
        bundle.status = BundleStatus::Sold;
        bundle.buyer = Some(buyer.clone());
        bundle.owner = buyer.clone();
        bundle.sold_at = Some(env.ledger().timestamp());
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(bundle_id), &bundle);

        // Update ownership index
        Self::remove_from_owner_index(&env, &prev_owner, bundle_id);
        let mut new_owned: Vec<u64> = env
            .storage()
            .instance()
            .get(&BundleDataKey::OwnerBundles(buyer.clone()))
            .unwrap_or(Vec::new(&env));
        new_owned.push_back(bundle_id);
        env.storage()
            .instance()
            .set(&BundleDataKey::OwnerBundles(buyer.clone()), &new_owned);

        env.events().publish(
            (Symbol::new(&env, "bundle_purchased"), buyer),
            (bundle_id, effective_price),
        );
    }

    /// Transfer bundle ownership directly (without marketplace) from current owner to recipient.
    pub fn transfer_bundle(
        env: Env,
        from: Address,
        to: Address,
        bundle_id: u64,
    ) {
        from.require_auth();
        assert_ne!(from, to, "cannot transfer to self");

        let mut bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        assert_eq!(bundle.owner, from, "only owner can transfer");
        assert!(
            bundle.status == BundleStatus::Active || bundle.status == BundleStatus::Sold,
            "bundle cannot be transferred in current status"
        );

        bundle.owner = to.clone();
        bundle.status = BundleStatus::Active;
        bundle.list_price = 0;
        bundle.discount_bps = 0;
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(bundle_id), &bundle);

        // Update ownership indexes
        Self::remove_from_owner_index(&env, &from, bundle_id);
        let mut to_owned: Vec<u64> = env
            .storage()
            .instance()
            .get(&BundleDataKey::OwnerBundles(to.clone()))
            .unwrap_or(Vec::new(&env));
        to_owned.push_back(bundle_id);
        env.storage()
            .instance()
            .set(&BundleDataKey::OwnerBundles(to.clone()), &to_owned);

        env.events().publish(
            (Symbol::new(&env, "bundle_transferred"), from),
            (bundle_id, to),
        );
    }

    /// Unwrap a bundle back into its individual assets. The bundle is marked Unwrapped
    /// and the asset IDs are returned to the caller so downstream logic can re-issue them.
    /// Only the current owner may unwrap, and only when Active or Sold.
    pub fn unwrap_bundle(env: Env, owner: Address, bundle_id: u64) -> Vec<u64> {
        owner.require_auth();

        let mut bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        assert_eq!(bundle.owner, owner, "only owner can unwrap");
        assert!(
            bundle.status == BundleStatus::Active || bundle.status == BundleStatus::Sold,
            "bundle cannot be unwrapped in current status"
        );

        let asset_ids = bundle.asset_ids.clone();
        bundle.status = BundleStatus::Unwrapped;
        bundle.list_price = 0;
        bundle.discount_bps = 0;
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(bundle_id), &bundle);

        env.events().publish(
            (Symbol::new(&env, "bundle_unwrapped"), owner),
            (bundle_id, asset_ids.len()),
        );

        asset_ids
    }

    /// Validate that a given address is recorded as the owner of the bundle.
    pub fn validate_ownership(env: Env, bundle_id: u64, claimant: Address) -> bool {
        match env
            .storage()
            .instance()
            .get::<_, AssetBundle>(&BundleDataKey::Bundle(bundle_id))
        {
            Some(bundle) => bundle.owner == claimant,
            None => false,
        }
    }

    /// Delist a bundle (owner only, only if status is Listed).
    pub fn delist_bundle(env: Env, owner: Address, bundle_id: u64) {
        owner.require_auth();
        let mut bundle: AssetBundle = env
            .storage()
            .instance()
            .get(&BundleDataKey::Bundle(bundle_id))
            .expect("bundle not found");
        assert_eq!(bundle.owner, owner, "only owner can delist");
        assert_eq!(bundle.status, BundleStatus::Listed, "bundle not listed");

        bundle.status = BundleStatus::Active;
        bundle.list_price = 0;
        bundle.discount_bps = 0;
        env.storage()
            .instance()
            .set(&BundleDataKey::Bundle(bundle_id), &bundle);

        env.events().publish(
            (Symbol::new(&env, "bundle_delisted"), owner),
            bundle_id,
        );
    }

    pub fn get_bundle(env: Env, bundle_id: u64) -> Option<AssetBundle> {
        env.storage().instance().get(&BundleDataKey::Bundle(bundle_id))
    }

    pub fn get_owner_bundles(env: Env, owner: Address) -> Vec<u64> {
        env.storage()
            .instance()
            .get(&BundleDataKey::OwnerBundles(owner))
            .unwrap_or(Vec::new(&env))
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    fn apply_discount(price: i128, discount_bps: u32) -> i128 {
        if discount_bps == 0 {
            return price;
        }
        let discount = price
            .checked_mul(discount_bps as i128)
            .expect("discount overflow")
            / 10_000;
        price - discount
    }

    fn remove_from_owner_index(env: &Env, owner: &Address, bundle_id: u64) {
        let owned: Vec<u64> = env
            .storage()
            .instance()
            .get(&BundleDataKey::OwnerBundles(owner.clone()))
            .unwrap_or(Vec::new(env));
        let mut updated = Vec::new(env);
        for id in owned.iter() {
            if id != bundle_id {
                updated.push_back(id);
            }
        }
        env.storage()
            .instance()
            .set(&BundleDataKey::OwnerBundles(owner.clone()), &updated);
    }

    fn assert_no_duplicate_assets(env: &Env, asset_ids: &Vec<u64>) {
        let mut seen = Vec::new(env);
        for id in asset_ids.iter() {
            for s in seen.iter() {
                assert_ne!(s, id, "duplicate asset id in bundle");
            }
            seen.push_back(id);
        }
    }
}
