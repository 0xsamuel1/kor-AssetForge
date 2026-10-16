#![cfg(test)]

use soroban_sdk::testutils::Address as _;
use soroban_sdk::{Address, Env};

use kor_assetforge_contracts::p2p_market::{
    FeeConfig, OrderSide, OrderStatus, P2PMarket, P2PMarketClient,
};

fn setup(env: &Env) -> (P2PMarketClient, Address) {
    let id = env.register_contract(None, P2PMarket);
    let client = P2PMarketClient::new(env, &id);
    let admin = Address::generate(env);
    client.initialize(&admin);
    (client, admin)
}

#[test]
fn test_place_and_match_orders_price_time_priority() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);
    let asset_id: u64 = 1;

    // Sell order placed first — becomes the maker
    let (sell_id, sell_trades) =
        client.place_order(&seller, &asset_id, &OrderSide::Sell, &1000, &100);
    assert_eq!(sell_id, 1);
    assert_eq!(sell_trades.len(), 0);

    // Buy order matches the sell — becomes the taker
    let (buy_id, buy_trades) =
        client.place_order(&buyer, &asset_id, &OrderSide::Buy, &1000, &100);
    assert_eq!(buy_id, 2);
    assert_eq!(buy_trades.len(), 1);

    let sell_order = client.get_order(&sell_id).unwrap();
    assert_eq!(sell_order.status, OrderStatus::Filled);

    let buy_order = client.get_order(&buy_id).unwrap();
    assert_eq!(buy_order.status, OrderStatus::Filled);

    // Verify trade details
    let trade = client.get_trade(&1).unwrap();
    assert_eq!(trade.buyer, buyer);
    assert_eq!(trade.seller, seller);
    assert_eq!(trade.price, 1000);
    assert_eq!(trade.quantity, 100);
    assert_eq!(trade.total_value, 100_000);
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
    assert_eq!(buy_order.filled, 60);
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
fn test_maker_taker_fees() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, admin) = setup(&env);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);
    let asset_id: u64 = 10;

    // Set 30 bps maker fee, 50 bps taker fee
    client.set_fee_config(&admin, &30, &50);

    let fee_config: FeeConfig = client.get_fee_config().unwrap();
    assert_eq!(fee_config.maker_fee_bps, 30);
    assert_eq!(fee_config.taker_fee_bps, 50);

    // Sell 100 at price 1000 → total_value = 100,000
    client.place_order(&seller, &asset_id, &OrderSide::Sell, &1000, &100);
    let (_, buy_trades) =
        client.place_order(&buyer, &asset_id, &OrderSide::Buy, &1000, &100);

    assert_eq!(buy_trades.len(), 1);
    let trade = client.get_trade(&buy_trades.get(0).unwrap()).unwrap();

    // maker_fee = 100_000 / 10_000 * 30 = 300
    // taker_fee = 100_000 / 10_000 * 50 = 500
    assert_eq!(trade.maker_fee, 300);
    assert_eq!(trade.taker_fee, 500);
    assert_eq!(trade.total_value, 100_000);
}

#[test]
fn test_no_fee_when_not_configured() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    client.place_order(&seller, &1, &OrderSide::Sell, &500, &10);
    let (_, trades) = client.place_order(&buyer, &1, &OrderSide::Buy, &500, &10);

    let trade = client.get_trade(&trades.get(0).unwrap()).unwrap();
    assert_eq!(trade.maker_fee, 0);
    assert_eq!(trade.taker_fee, 0);
}

#[test]
fn test_price_incompatible_no_match() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    // Seller wants 1000, buyer only offers 900 — no match
    client.place_order(&seller, &1, &OrderSide::Sell, &1000, &50);
    let (_, trades) = client.place_order(&buyer, &1, &OrderSide::Buy, &900, &50);

    assert_eq!(trades.len(), 0);

    let sell_order = client.get_order(&1).unwrap();
    assert_eq!(sell_order.status, OrderStatus::Open);

    let buy_order = client.get_order(&2).unwrap();
    assert_eq!(buy_order.status, OrderStatus::Open);
}

#[test]
fn test_get_asset_orders_and_trades() {
    let env = Env::default();
    env.mock_all_auths();

    let (client, _) = setup(&env);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);
    let asset_id: u64 = 5;

    client.place_order(&seller, &asset_id, &OrderSide::Sell, &100, &50);
    client.place_order(&buyer, &asset_id, &OrderSide::Buy, &100, &50);

    let orders = client.get_asset_orders(&asset_id);
    assert_eq!(orders.len(), 2);

    let trades = client.get_asset_trades(&asset_id);
    assert_eq!(trades.len(), 1);
}
