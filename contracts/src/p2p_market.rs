use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, Vec};

// ============================================================================
// Data Types
// ============================================================================

#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum OrderSide {
    Buy,
    Sell,
}

#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum OrderStatus {
    Open,
    Partial,
    Filled,
    Cancelled,
}

#[derive(Clone)]
#[contracttype]
pub struct Order {
    pub id: u64,
    pub asset_id: u64,
    pub owner: Address,
    pub side: OrderSide,
    pub price: i128,
    pub quantity: i128,
    pub filled: i128,
    pub status: OrderStatus,
    pub created_at: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct Trade {
    pub id: u64,
    pub asset_id: u64,
    pub buy_order_id: u64,
    pub sell_order_id: u64,
    pub buyer: Address,
    pub seller: Address,
    pub price: i128,
    pub quantity: i128,
    pub total_value: i128,
    pub maker_fee: i128,
    pub taker_fee: i128,
    pub executed_at: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct FeeConfig {
    pub maker_fee_bps: u32,
    pub taker_fee_bps: u32,
}

#[derive(Clone)]
#[contracttype]
pub enum P2PDataKey {
    Admin,
    OrderNonce,
    TradeNonce,
    Order(u64),
    AssetOrders(u64),    // Vec<u64> of order IDs for an asset
    TradeHistory(u64),   // Vec<u64> of trade IDs for an asset
    Trade(u64),
    FeeConfig,
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct P2PMarket;

#[contractimpl]
impl P2PMarket {
    /// Initialize the P2P market contract.
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&P2PDataKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&P2PDataKey::Admin, &admin);
    }

    /// Admin: configure maker and taker fees (in basis points, e.g. 30 = 0.3%).
    pub fn set_fee_config(env: Env, admin: Address, maker_fee_bps: u32, taker_fee_bps: u32) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&P2PDataKey::Admin)
            .expect("not initialized");
        if admin != stored_admin {
            panic!("caller is not admin");
        }
        if maker_fee_bps > 1_000 || taker_fee_bps > 1_000 {
            panic!("fee_bps must not exceed 1000 (10%)");
        }
        let config = FeeConfig { maker_fee_bps, taker_fee_bps };
        env.storage().instance().set(&P2PDataKey::FeeConfig, &config);

        env.events().publish(
            (Symbol::new(&env, "fee_config_set"), admin),
            (maker_fee_bps, taker_fee_bps),
        );
    }

    /// Get the current fee configuration.
    pub fn get_fee_config(env: Env) -> Option<FeeConfig> {
        env.storage().instance().get(&P2PDataKey::FeeConfig)
    }

    /// Place a buy or sell order. Immediately attempts matching.
    /// Returns the new order ID and a Vec of executed trade IDs.
    pub fn place_order(
        env: Env,
        owner: Address,
        asset_id: u64,
        side: OrderSide,
        price: i128,
        quantity: i128,
    ) -> (u64, Vec<u64>) {
        owner.require_auth();

        if price <= 0 {
            panic!("price must be positive");
        }
        if quantity <= 0 {
            panic!("quantity must be positive");
        }

        let order_id: u64 = env
            .storage()
            .instance()
            .get(&P2PDataKey::OrderNonce)
            .unwrap_or(0)
            + 1;
        env.storage()
            .instance()
            .set(&P2PDataKey::OrderNonce, &order_id);

        let mut order = Order {
            id: order_id,
            asset_id,
            owner: owner.clone(),
            side: side.clone(),
            price,
            quantity,
            filled: 0,
            status: OrderStatus::Open,
            created_at: env.ledger().timestamp(),
        };

        // Append to asset order index
        let mut asset_orders: Vec<u64> = env
            .storage()
            .instance()
            .get(&P2PDataKey::AssetOrders(asset_id))
            .unwrap_or(Vec::new(&env));
        asset_orders.push_back(order_id);
        env.storage()
            .instance()
            .set(&P2PDataKey::AssetOrders(asset_id), &asset_orders);

        // Persist before matching so counter-orders can see it
        env.storage()
            .persistent()
            .set(&P2PDataKey::Order(order_id), &order);

        // Match against existing orders
        let trade_ids = Self::match_order(&env, &mut order);

        // Persist final state
        env.storage()
            .persistent()
            .set(&P2PDataKey::Order(order_id), &order);

        env.events().publish(
            (Symbol::new(&env, "order_placed"), order_id),
            (owner, asset_id, price, quantity),
        );

        (order_id, trade_ids)
    }

    /// Cancel an open or partially filled order.
    pub fn cancel_order(env: Env, owner: Address, order_id: u64) {
        owner.require_auth();

        let mut order: Order = env
            .storage()
            .persistent()
            .get(&P2PDataKey::Order(order_id))
            .expect("order not found");

        if order.owner != owner {
            panic!("not order owner");
        }
        if order.status == OrderStatus::Filled || order.status == OrderStatus::Cancelled {
            panic!("order cannot be cancelled");
        }

        order.status = OrderStatus::Cancelled;
        env.storage()
            .persistent()
            .set(&P2PDataKey::Order(order_id), &order);

        env.events()
            .publish((Symbol::new(&env, "order_cancelled"), order_id), owner);
    }

    /// Retrieve an order by ID.
    pub fn get_order(env: Env, order_id: u64) -> Option<Order> {
        env.storage()
            .persistent()
            .get(&P2PDataKey::Order(order_id))
    }

    /// Retrieve a trade by ID.
    pub fn get_trade(env: Env, trade_id: u64) -> Option<Trade> {
        env.storage()
            .persistent()
            .get(&P2PDataKey::Trade(trade_id))
    }

    /// Get all order IDs for an asset.
    pub fn get_asset_orders(env: Env, asset_id: u64) -> Vec<u64> {
        env.storage()
            .instance()
            .get(&P2PDataKey::AssetOrders(asset_id))
            .unwrap_or(Vec::new(&env))
    }

    /// Get all trade IDs for an asset.
    pub fn get_asset_trades(env: Env, asset_id: u64) -> Vec<u64> {
        env.storage()
            .instance()
            .get(&P2PDataKey::TradeHistory(asset_id))
            .unwrap_or(Vec::new(&env))
    }

    /// Total number of orders placed.
    pub fn get_order_count(env: Env) -> u64 {
        env.storage()
            .instance()
            .get(&P2PDataKey::OrderNonce)
            .unwrap_or(0)
    }

    // -----------------------------------------------------------------------
    // Internal: price-time priority matching
    // -----------------------------------------------------------------------

    fn compute_fee(notional: i128, fee_bps: u32) -> i128 {
        // Divide before multiply to avoid i128 overflow at large notional values.
        notional / 10_000 * fee_bps as i128
    }

    fn match_order(env: &Env, taker: &mut Order) -> Vec<u64> {
        let mut trade_ids: Vec<u64> = Vec::new(env);
        let asset_ids: Vec<u64> = env
            .storage()
            .instance()
            .get(&P2PDataKey::AssetOrders(taker.asset_id))
            .unwrap_or(Vec::new(env));

        let remaining_before = taker.quantity - taker.filled;
        let mut remaining = remaining_before;

        for counter_id in asset_ids.iter() {
            if remaining <= 0 {
                break;
            }
            if counter_id == taker.id {
                continue;
            }

            let counter_opt: Option<Order> = env
                .storage()
                .persistent()
                .get(&P2PDataKey::Order(counter_id));
            let mut counter = match counter_opt {
                Some(o) => o,
                None => continue,
            };

            // Only match active orders on the opposite side
            if counter.status == OrderStatus::Filled || counter.status == OrderStatus::Cancelled {
                continue;
            }
            let sides_match = matches!(
                (&taker.side, &counter.side),
                (OrderSide::Buy, OrderSide::Sell) | (OrderSide::Sell, OrderSide::Buy)
            );
            if !sides_match {
                continue;
            }

            // Price compatibility check
            let price_matches = match taker.side {
                OrderSide::Buy => taker.price >= counter.price,
                OrderSide::Sell => taker.price <= counter.price,
            };
            if !price_matches {
                continue;
            }

            let counter_remaining = counter.quantity - counter.filled;
            let fill_qty = if remaining < counter_remaining {
                remaining
            } else {
                counter_remaining
            };
            let trade_price = counter.price;

            let (buyer, seller, buy_id, sell_id) = match taker.side {
                OrderSide::Buy => (
                    taker.owner.clone(),
                    counter.owner.clone(),
                    taker.id,
                    counter.id,
                ),
                OrderSide::Sell => (
                    counter.owner.clone(),
                    taker.owner.clone(),
                    counter.id,
                    taker.id,
                ),
            };

            let trade_id: u64 = env
                .storage()
                .instance()
                .get(&P2PDataKey::TradeNonce)
                .unwrap_or(0)
                + 1;
            env.storage()
                .instance()
                .set(&P2PDataKey::TradeNonce, &trade_id);

            let total_value = trade_price.checked_mul(fill_qty).unwrap_or(0);
            let (maker_fee, taker_fee) = if let Some(fee_cfg) = env
                .storage()
                .instance()
                .get::<P2PDataKey, FeeConfig>(&P2PDataKey::FeeConfig)
            {
                (
                    Self::compute_fee(total_value, fee_cfg.maker_fee_bps),
                    Self::compute_fee(total_value, fee_cfg.taker_fee_bps),
                )
            } else {
                (0, 0)
            };

            let trade = Trade {
                id: trade_id,
                asset_id: taker.asset_id,
                buy_order_id: buy_id,
                sell_order_id: sell_id,
                buyer: buyer.clone(),
                seller: seller.clone(),
                price: trade_price,
                quantity: fill_qty,
                total_value,
                maker_fee,
                taker_fee,
                executed_at: env.ledger().timestamp(),
            };

            env.storage()
                .persistent()
                .set(&P2PDataKey::Trade(trade_id), &trade);

            // Update counters
            counter.filled += fill_qty;
            if counter.filled >= counter.quantity {
                counter.status = OrderStatus::Filled;
            } else {
                counter.status = OrderStatus::Partial;
            }
            env.storage()
                .persistent()
                .set(&P2PDataKey::Order(counter_id), &counter);

            taker.filled += fill_qty;
            remaining -= fill_qty;

            // Append trade to asset history
            let mut history: Vec<u64> = env
                .storage()
                .instance()
                .get(&P2PDataKey::TradeHistory(taker.asset_id))
                .unwrap_or(Vec::new(env));
            history.push_back(trade_id);
            env.storage()
                .instance()
                .set(&P2PDataKey::TradeHistory(taker.asset_id), &history);

            trade_ids.push_back(trade_id);

            env.events().publish(
                (Symbol::new(env, "trade_executed"), trade_id),
                (buyer, seller, trade_price, fill_qty),
            );
        }

        if taker.filled >= taker.quantity {
            taker.status = OrderStatus::Filled;
        } else if taker.filled > 0 {
            taker.status = OrderStatus::Partial;
        }

        trade_ids
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::Address as _;
    use soroban_sdk::{Address, Env};

    fn setup(env: &Env) -> (P2PMarketClient, Address) {
        let id = env.register_contract(None, P2PMarket);
        let client = P2PMarketClient::new(env, &id);
        let admin = Address::generate(env);
        client.initialize(&admin);
        (client, admin)
    }

    #[test]
    fn test_place_and_match_orders() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);

        let seller = Address::generate(&env);
        let buyer = Address::generate(&env);
        let asset_id: u64 = 1;

        // Seller places a sell order at price 1000
        let (sell_id, sell_trades) = client.place_order(&seller, &asset_id, &OrderSide::Sell, &1000, &100);
        assert_eq!(sell_id, 1);
        assert_eq!(sell_trades.len(), 0); // no buyers yet

        // Buyer places a matching buy order at price 1000
        let (buy_id, buy_trades) = client.place_order(&buyer, &asset_id, &OrderSide::Buy, &1000, &100);
        assert_eq!(buy_id, 2);
        assert_eq!(buy_trades.len(), 1); // matched!

        let sell_order = client.get_order(&sell_id).unwrap();
        assert_eq!(sell_order.status, OrderStatus::Filled);

        let buy_order = client.get_order(&buy_id).unwrap();
        assert_eq!(buy_order.status, OrderStatus::Filled);
    }

    #[test]
    fn test_cancel_order() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let seller = Address::generate(&env);

        let (order_id, _) = client.place_order(&seller, &1, &OrderSide::Sell, &500, &50);
        client.cancel_order(&seller, &order_id);

        let order = client.get_order(&order_id).unwrap();
        assert_eq!(order.status, OrderStatus::Cancelled);
    }

    #[test]
    fn test_partial_fill() {
        let env = Env::default();
        env.mock_all_auths();

        let (client, _) = setup(&env);
        let seller = Address::generate(&env);
        let buyer = Address::generate(&env);

        // Sell 100 units
        let (sell_id, _) = client.place_order(&seller, &1, &OrderSide::Sell, &200, &100);
        // Buy only 60 units
        let (buy_id, trades) = client.place_order(&buyer, &1, &OrderSide::Buy, &200, &60);

        assert_eq!(trades.len(), 1);

        let sell_order = client.get_order(&sell_id).unwrap();
        assert_eq!(sell_order.status, OrderStatus::Partial);
        assert_eq!(sell_order.filled, 60);

        let buy_order = client.get_order(&buy_id).unwrap();
        assert_eq!(buy_order.status, OrderStatus::Filled);
    }
}
