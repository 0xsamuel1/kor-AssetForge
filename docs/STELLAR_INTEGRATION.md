# Stellar / Soroban Integration

This document describes how the AssetForge backend integrates with the Stellar
network and Soroban smart contracts, with a focus on contract invocation.

## Overview

The backend talks to two Stellar services:

| Service        | Client type                            | Used for                                     |
| -------------- | -------------------------------------- | -------------------------------------------- |
| Horizon        | `github.com/stellar/go/clients/horizonclient` | Account balances, ledger data                |
| Soroban RPC    | `github.com/stellar/go/clients/rpcclient`     | Contract simulation, submission, status      |

Both are wrapped by `utils.StellarClient` (`backend/utils/stellar_utils.go`).

## Configuration

Set the following environment variables (see `backend/.env.example`):

| Variable                   | Required | Description                                                        |
| -------------------------- | -------- | ------------------------------------------------------------------ |
| `STELLAR_NETWORK`          | no       | `testnet` (default) or `public`.                                   |
| `STELLAR_HORIZON_URL`      | no       | Horizon endpoint. Defaults to the public testnet client.           |
| `STELLAR_SOROBAN_RPC_URL`  | no       | Soroban RPC endpoint. Defaults to a public endpoint per network.   |
| `STELLAR_SIGNER_SECRET`    | yes\*    | `S...` secret key used to sign invocation transactions.            |

\* Required only to invoke contracts. Read-only operations (balances,
validation) work without it.

> **Security:** `STELLAR_SIGNER_SECRET` must be provided through a secret
> manager. Never commit it to source control.

## Invoking a contract

```go
client, err := config.InitStellarClient()
if err != nil {
    return err
}

params := []interface{}{
    "AssetForge",   // string  -> ScvString
    "AXN",          // string  -> ScvString
    int64(1000000), // int64   -> ScvI64
}

txHash, err := client.InvokeContract(contractID, "mint", params)
if err != nil {
    log.Fatalf("contract invocation failed: %v", err)
}
log.Printf("contract invoked in transaction %s", txHash)
```

`InvokeContract` executes the following pipeline:

1. Build an `InvokeHostFunction` operation with the `invokeContract` host
   function, encoding the arguments as `xdr.ScVal`s.
2. Load the signer account (sequence number) from Soroban RPC.
3. Simulate the transaction via `simulateTransaction` to obtain the ledger
   footprint, resource fee and any recorded authorization entries.
4. Rebuild the transaction with the simulated `SorobanTransactionData`,
   authorization entries and resource fee.
5. Sign the transaction with the configured signer key.
6. Submit with `sendTransaction` and poll `getTransaction` until the
   transaction reaches `SUCCESS` (or `FAILED`).

If the caller already has a `context.Context`, use
`InvokeContractContext(ctx, ...)` to control cancellation and deadlines.

### Supported parameter types

| Go type    | Soroban type |
| ---------- | ------------ |
| `string`   | `ScvString`  |
| `bool`     | `ScvBool`    |
| `int`      | `ScvI64`     |
| `int64`    | `ScvI64`     |
| `uint32`   | `ScvU32`     |
| `uint64`   | `ScvU64`     |

Support for additional types (addresses, vectors, maps, `i128`/`u128`) can be
added in `toScVal`.

## Error handling

`InvokeContract` returns a descriptive error at each stage:

- `soroban RPC client is not configured`
- `stellar signer key is not configured (set STELLAR_SIGNER_SECRET)`
- `invalid contract ID "..."`
- `unsupported contract parameter type`
- `failed to simulate transaction` / `contract simulation failed: ...`
- `contract requires ledger entry restoration before invocation`
- `transaction rejected: ...`
- `transaction <hash> failed on-chain`
- `timed out waiting for transaction <hash>`

Callers should treat a timeout as an **unknown** transaction state and reconcile
using the returned transaction hash rather than blindly retrying (contract
invocations may not be idempotent).

## Testing

`backend/utils/stellar_utils_test.go` unit tests the pipeline with a fake
`SorobanRPC` implementation, covering:

- contract ID decoding (valid and invalid)
- parameter type encoding
- missing signer / RPC configuration
- simulation errors
- successful submission and on-chain failure

Because the fake never leaves the process, no network access or funded account
is required in CI. Integration tests against testnet should be run separately
with a funded `STELLAR_SIGNER_SECRET`, and must not run on every PR.

## References

- [Soroban RPC API](https://developers.stellar.org/docs/data/rpc/api-reference)
- [Stellar Go SDK](https://github.com/stellar/go)
