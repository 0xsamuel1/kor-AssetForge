// Upgradeable contract modules.
//
// `proxy` implements a transparent upgradeable proxy so a deployed contract
// address keeps pointing at the same state while its WASM can be replaced
// (and rolled back) without migrating balances off the contract.

pub mod proxy;

pub use proxy::Proxy;