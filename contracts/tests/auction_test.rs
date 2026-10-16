// Tests for Dutch and sealed-bid auction mechanisms – Issue #206
//
// The AuctionHouse contract already implements all three auction types.
// This file provides a comprehensive test suite covering:
//   Dutch:     price decay, reserve clamp, buy_dutch, expired auction
//   SealedBid: single/multiple bidders, winner selection, reserve enforcement,
//              automatic loser refunds, improved-bid flow
//   General:   reserve price enforcement, automatic settlement, cancel

extern crate kor_assetforge_contracts;

use kor_assetforge_contracts::auction::{AuctionHouse, AuctionHouseClient, AuctionStatus, AuctionType};
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::{Address, Env};

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

fn setup() -> (Env, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();
    let admin = Address::generate(&env);
    let id = env.register_contract(None, AuctionHouse);
    let client = AuctionHouseClient::new(&env, &id);
    client.initialize(&admin);
    (env, id, admin)
}

// ===========================================================================
// Dutch auction tests
// ===========================================================================

#[test]
fn test_dutch_price_decays_over_time() {
    let (env, id, _admin) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);

    // start=1000, reserve=100, decrement=100 every 60s
    let aid = client.create_dutch_auction(&seller, &1, &100, &1000, &100, &100, &60, &3600);

    // At t=0 price is 1000
    assert_eq!(client.get_dutch_current_price(&aid), 1000);

    // After 60s: 1 step → 1000 - 100 = 900
    env.ledger().with_mut(|li| li.timestamp += 60);
    assert_eq!(client.get_dutch_current_price(&aid), 900);

    // After 300s: 5 steps → 1000 - 500 = 500
    env.ledger().with_mut(|li| li.timestamp = 300);
    assert_eq!(client.get_dutch_current_price(&aid), 500);
}

#[test]
fn test_dutch_price_clamped_at_reserve() {
    let (env, id, _admin) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);

    // Would decay past reserve after 10 steps (10 * 100 = 1000 drop on start=1000)
    let aid = client.create_dutch_auction(&seller, &1, &100, &1000, &200, &100, &60, &7200);

    env.ledger().with_mut(|li| li.timestamp += 3600); // 60 steps → fully decayed
    // Price should be clamped at reserve=200
    assert_eq!(client.get_dutch_current_price(&aid), 200);
}

#[test]
fn test_buy_dutch_settles_at_current_price() {
    let (env, id, _admin) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    // start=1000, decrement=100 every 60s
    let aid = client.create_dutch_auction(&seller, &1, &50, &1000, &100, &100, &60, &3600);

    // Advance 120s → price = 1000 - 200 = 800
    env.ledger().with_mut(|li| li.timestamp += 120);
    client.buy_dutch(&buyer, &aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Settled);
    assert_eq!(auction.highest_bid, 800);
    assert_eq!(auction.highest_bidder, Some(buyer));
}

#[test]
fn test_dutch_buy_at_reserve_price() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    // start=500, reserve=100, decrement=100 every 60s, duration=7200s
    // Steps to reach reserve: (500-100)/100 = 4 steps = 240s
    let aid = client.create_dutch_auction(&seller, &2, &10, &500, &100, &100, &60, &7200);

    // Advance 600s → 10 steps → 500 - 1000 = clamped to 100 (reserve)
    // Still within duration (600 < 7200)
    env.ledger().with_mut(|li| li.timestamp += 600);
    client.buy_dutch(&buyer, &aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.highest_bid, 100); // clamped at reserve
    assert_eq!(auction.status, AuctionStatus::Settled);
}

#[test]
#[should_panic(expected = "auction has ended")]
fn test_buy_dutch_after_expiry_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    let aid = client.create_dutch_auction(&seller, &1, &10, &1000, &100, &100, &60, &600);

    // Advance past end_time (600s)
    env.ledger().with_mut(|li| li.timestamp += 601);
    client.buy_dutch(&buyer, &aid);
}

#[test]
#[should_panic(expected = "not a Dutch auction")]
fn test_buy_dutch_on_english_auction_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let buyer = Address::generate(&env);

    let aid = client.create_english_auction(&seller, &1, &10, &100, &50, &10, &3600, &300);
    client.buy_dutch(&buyer, &aid);
}

#[test]
#[should_panic(expected = "price_decrement must be positive")]
fn test_dutch_zero_decrement_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    client.create_dutch_auction(&seller, &1, &10, &1000, &100, &0, &60, &3600);
}

#[test]
#[should_panic(expected = "decrement_interval must be non-zero")]
fn test_dutch_zero_interval_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    client.create_dutch_auction(&seller, &1, &10, &1000, &100, &100, &0, &3600);
}

// ===========================================================================
// Sealed-bid auction tests
// ===========================================================================

#[test]
fn test_sealed_bid_winner_is_highest_bidder() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder_a = Address::generate(&env);
    let bidder_b = Address::generate(&env);
    let bidder_c = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);

    client.place_bid(&bidder_a, &aid, &200);
    client.place_bid(&bidder_b, &aid, &350); // highest
    client.place_bid(&bidder_c, &aid, &150);

    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Settled);
    assert_eq!(auction.highest_bidder, Some(bidder_b.clone()));
    assert_eq!(auction.highest_bid, 350);
}

#[test]
fn test_sealed_bid_losers_receive_refunds() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let winner = Address::generate(&env);
    let loser = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.place_bid(&winner, &aid, &300);
    client.place_bid(&loser, &aid, &100);

    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    // loser should have a pending refund
    assert_eq!(client.get_pending_refund(&aid, &loser), 100);
    // winner has no refund
    assert_eq!(client.get_pending_refund(&aid, &winner), 0);
}

#[test]
fn test_sealed_bid_reserve_not_met_refunds_all() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    // reserve=500, all bids below it
    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &500, &3600);
    client.place_bid(&bidder, &aid, &100);

    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Ended); // no winner
    assert_eq!(auction.highest_bidder, None);
    assert_eq!(client.get_pending_refund(&aid, &bidder), 100);
}

#[test]
fn test_sealed_bid_single_bidder_at_reserve_wins() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &100, &3600);
    client.place_bid(&bidder, &aid, &100); // exactly at reserve

    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Settled);
    assert_eq!(auction.highest_bidder, Some(bidder));
}

#[test]
fn test_sealed_bid_improved_bid_replaces_previous() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.place_bid(&bidder, &aid, &100);

    // Improve the bid
    client.place_bid(&bidder, &aid, &250);

    let stored = client.get_bid(&aid, &bidder).unwrap();
    assert_eq!(stored.amount, 250);
}

#[test]
#[should_panic(expected = "new bid must exceed previous bid")]
fn test_sealed_bid_lower_rebid_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.place_bid(&bidder, &aid, &200);
    client.place_bid(&bidder, &aid, &100); // lower – should panic
}

#[test]
fn test_sealed_bid_no_bids_ends_without_winner() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Ended);
    assert_eq!(auction.highest_bidder, None);
    assert_eq!(auction.highest_bid, 0);
}

#[test]
#[should_panic(expected = "use buy_dutch for Dutch auctions")]
fn test_place_bid_on_dutch_auction_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    let aid = client.create_dutch_auction(&seller, &1, &10, &1000, &100, &100, &60, &3600);
    client.place_bid(&bidder, &aid, &500);
}

// ===========================================================================
// Reserve price enforcement (general)
// ===========================================================================

#[test]
fn test_english_reserve_not_met_refunds_highest_bidder() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    // reserve=1000 but bid=100
    let aid = client.create_english_auction(&seller, &1, &50, &50, &1000, &10, &3600, &0);
    client.place_bid(&bidder, &aid, &100);

    env.ledger().with_mut(|li| li.timestamp += 3601);
    client.settle_auction(&aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Ended);
    // Highest bidder refunded
    assert_eq!(client.get_pending_refund(&aid, &bidder), 100);
}

// ===========================================================================
// Automatic settlement / cancel
// ===========================================================================

#[test]
fn test_cancel_dutch_auction_by_seller() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);

    let aid = client.create_dutch_auction(&seller, &1, &10, &1000, &100, &100, &60, &3600);
    client.cancel_auction(&seller, &aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Cancelled);
}

#[test]
fn test_emergency_cancel_sealed_bid_by_admin() {
    let (env, id, admin) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let bidder = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.place_bid(&bidder, &aid, &200);
    client.emergency_cancel(&admin, &aid);

    let auction = client.get_auction(&aid).unwrap();
    assert_eq!(auction.status, AuctionStatus::Cancelled);
}

#[test]
#[should_panic(expected = "only seller or admin can cancel")]
fn test_cancel_by_unauthorized_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let rando = Address::generate(&env);

    let aid = client.create_dutch_auction(&seller, &1, &10, &1000, &100, &100, &60, &3600);
    client.cancel_auction(&rando, &aid);
}

#[test]
#[should_panic(expected = "auction has not ended yet")]
fn test_settle_before_end_time_panics() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.settle_auction(&aid); // no time advance
}

#[test]
fn test_get_all_bidders_for_sealed_bid() {
    let (env, id, _) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    let seller = Address::generate(&env);
    let b1 = Address::generate(&env);
    let b2 = Address::generate(&env);

    let aid = client.create_sealed_bid_auction(&seller, &1, &100, &50, &3600);
    client.place_bid(&b1, &aid, &100);
    client.place_bid(&b2, &aid, &200);

    let bidders = client.get_bidders(&aid);
    assert_eq!(bidders.len(), 2);
}

#[test]
#[should_panic(expected = "already initialized")]
fn test_double_initialize_panics() {
    let (env, id, admin) = setup();
    let client = AuctionHouseClient::new(&env, &id);
    client.initialize(&admin);
}
