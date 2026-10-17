package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	rpcclient "github.com/stellar/go/clients/rpcclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	protocol "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
)

const (
	// defaultTestnetSorobanRPC is the public Soroban RPC endpoint for testnet.
	defaultTestnetSorobanRPC = "https://soroban-testnet.stellar.org"
	// defaultMainnetSorobanRPC is the public Soroban RPC endpoint for mainnet.
	defaultMainnetSorobanRPC = "https://mainnet.sorobanrpc.com"

	// sorobanSubmitTimeout bounds how long we wait for a submitted tx to settle.
	sorobanSubmitTimeout = 60 * time.Second
	// sorobanPollInterval is the delay between transaction status polls.
	sorobanPollInterval = 1 * time.Second
	// sorobanMaxPolls caps the number of status polls before giving up.
	sorobanMaxPolls = 60
)

// SorobanRPC abstracts the subset of the Soroban RPC client used by
// InvokeContract. It exists so the invocation pipeline can be unit tested with
// a fake implementation.
type SorobanRPC interface {
	LoadAccount(ctx context.Context, address string) (txnbuild.Account, error)
	SimulateTransaction(ctx context.Context, req protocol.SimulateTransactionRequest) (protocol.SimulateTransactionResponse, error)
	SendTransaction(ctx context.Context, req protocol.SendTransactionRequest) (protocol.SendTransactionResponse, error)
	GetTransaction(ctx context.Context, req protocol.GetTransactionRequest) (protocol.GetTransactionResponse, error)
}

type StellarClient struct {
	HorizonClient *horizonclient.Client
	RPCClient     SorobanRPC
	SignerKey     *keypair.Full
	NetworkPass   string
	NetworkType   string
}

// NewStellarClient creates a new Stellar client.
//
// The Soroban RPC endpoint is taken from STELLAR_SOROBAN_RPC_URL when set,
// otherwise a sensible public default is chosen based on the network. A signer
// key is loaded from STELLAR_SIGNER_SECRET when present; contract invocation
// requires it.
func NewStellarClient(horizonURL string, networkType string) (*StellarClient, error) {
	client := horizonclient.DefaultTestNetClient
	networkPass := network.TestNetworkPassphrase

	if horizonURL != "" {
		client = &horizonclient.Client{
			HorizonURL: horizonURL,
		}
	}

	if networkType == "public" {
		networkPass = network.PublicNetworkPassphrase
	}

	sc := &StellarClient{
		HorizonClient: client,
		NetworkPass:   networkPass,
		NetworkType:   networkType,
	}

	sorobanURL := os.Getenv("STELLAR_SOROBAN_RPC_URL")
	if sorobanURL == "" {
		if networkType == "public" {
			sorobanURL = defaultMainnetSorobanRPC
		} else {
			sorobanURL = defaultTestnetSorobanRPC
		}
	}
	sc.RPCClient = rpcclient.NewClient(sorobanURL, http.DefaultClient)

	if secret := os.Getenv("STELLAR_SIGNER_SECRET"); secret != "" {
		kp, err := keypair.ParseFull(secret)
		if err != nil {
			return nil, fmt.Errorf("invalid STELLAR_SIGNER_SECRET: %w", err)
		}
		sc.SignerKey = kp
	}

	return sc, nil
}

// ValidateAddress checks if a Stellar address is valid
func (sc *StellarClient) ValidateAddress(address string) error {
	_, err := keypair.ParseAddress(address)
	return err
}

// GetAccountBalance retrieves account balance
func (sc *StellarClient) GetAccountBalance(address string) (string, error) {
	account, err := sc.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: address,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get account: %w", err)
	}

	// Return native balance (XLM)
	for _, balance := range account.Balances {
		if balance.Asset.Type == "native" {
			return balance.Balance, nil
		}
	}

	return "0", nil
}

// InvokeContract invokes a Soroban smart contract, returning the transaction
// hash once the transaction has settled successfully on-chain.
func (sc *StellarClient) InvokeContract(contractID string, method string, params []interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sorobanSubmitTimeout)
	defer cancel()
	return sc.InvokeContractContext(ctx, contractID, method, params)
}

// InvokeContractContext is the context-aware variant of InvokeContract. It
// builds, simulates, signs and submits a Soroban invoke-host-function
// transaction, then polls Stellar-RPC until the transaction is included in a
// ledger.
func (sc *StellarClient) InvokeContractContext(ctx context.Context, contractID string, method string, params []interface{}) (string, error) {
	if sc.RPCClient == nil {
		return "", fmt.Errorf("soroban RPC client is not configured")
	}
	if sc.SignerKey == nil {
		return "", fmt.Errorf("stellar signer key is not configured (set STELLAR_SIGNER_SECRET)")
	}
	if method == "" {
		return "", fmt.Errorf("contract method name must not be empty")
	}

	contractAddress, err := contractIDToScAddress(contractID)
	if err != nil {
		return "", err
	}

	args, err := toScVals(params)
	if err != nil {
		return "", err
	}

	op := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractAddress,
				FunctionName:    xdr.ScSymbol(method),
				Args:            args,
			},
		},
		SourceAccount: sc.SignerKey.Address(),
	}

	account, err := sc.RPCClient.LoadAccount(ctx, sc.SignerKey.Address())
	if err != nil {
		return "", fmt.Errorf("failed to load source account: %w", err)
	}

	// First build to obtain the transaction envelope used for simulation.
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        account,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(sorobanSubmitTimeout.Seconds()))},
	})
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	txEnvelope, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("failed to encode transaction: %w", err)
	}

	sim, err := sc.RPCClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{
		Transaction: txEnvelope,
		ResourceConfig: &protocol.ResourceConfig{
			InstructionLeeway: protocol.DefaultInstructionLeeway,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to simulate transaction: %w", err)
	}
	if sim.Error != "" {
		return "", fmt.Errorf("contract simulation failed: %s", sim.Error)
	}
	if sim.RestorePreamble != nil {
		return "", fmt.Errorf("contract requires ledger entry restoration before invocation")
	}

	var sorobanData xdr.SorobanTransactionData
	if err := xdr.SafeUnmarshalBase64(sim.TransactionDataXDR, &sorobanData); err != nil {
		return "", fmt.Errorf("failed to decode simulation transaction data: %w", err)
	}

	auth, err := authEntriesFromSimulation(sim.Results)
	if err != nil {
		return "", err
	}

	// Apply the simulated footprint, resource fee and auth entries.
	op.Auth = auth
	op.Ext = xdr.TransactionExt{V: 1, SorobanData: &sorobanData}

	finalTx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        account,
		IncrementSequenceNum: false,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee + sim.MinResourceFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(sorobanSubmitTimeout.Seconds()))},
	})
	if err != nil {
		return "", fmt.Errorf("failed to build final transaction: %w", err)
	}

	signedTx, err := finalTx.Sign(sc.NetworkPass, sc.SignerKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	signedEnvelope, err := signedTx.Base64()
	if err != nil {
		return "", fmt.Errorf("failed to encode signed transaction: %w", err)
	}

	sendResp, err := sc.RPCClient.SendTransaction(ctx, protocol.SendTransactionRequest{
		Transaction: signedEnvelope,
	})
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}
	if sendResp.Status == "ERROR" {
		return "", fmt.Errorf("transaction rejected: %s", sendResp.ErrorResultXDR)
	}

	return sc.waitForTransaction(ctx, sendResp.Hash)
}

// waitForTransaction polls Stellar-RPC until the transaction settles or the
// polling budget is exhausted.
func (sc *StellarClient) waitForTransaction(ctx context.Context, hash string) (string, error) {
	for i := 0; i < sorobanMaxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for transaction %s: %w", hash, ctx.Err())
		case <-time.After(sorobanPollInterval):
		}

		resp, err := sc.RPCClient.GetTransaction(ctx, protocol.GetTransactionRequest{Hash: hash})
		if err != nil {
			continue
		}

		switch resp.Status {
		case protocol.TransactionStatusSuccess:
			return hash, nil
		case protocol.TransactionStatusFailed:
			return "", fmt.Errorf("transaction %s failed on-chain", hash)
		case protocol.TransactionStatusNotFound:
			continue
		}
	}

	return "", fmt.Errorf("timed out waiting for transaction %s", hash)
}

// contractIDToScAddress decodes a strkey contract ID (C...) into an ScAddress.
func contractIDToScAddress(contractID string) (xdr.ScAddress, error) {
	decoded, err := strkey.Decode(strkey.VersionByteContract, contractID)
	if err != nil {
		return xdr.ScAddress{}, fmt.Errorf("invalid contract ID %q: %w", contractID, err)
	}

	var id xdr.ContractId
	copy(id[:], decoded)

	return xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &id,
	}, nil
}

// toScVals converts a slice of Go values into Soroban contract arguments.
func toScVals(params []interface{}) ([]xdr.ScVal, error) {
	args := make([]xdr.ScVal, 0, len(params))
	for _, p := range params {
		v, err := toScVal(p)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	return args, nil
}

// toScVal converts a single Go value into an xdr.ScVal. Supported types are
// string, bool, int, int64, uint32 and uint64.
func toScVal(v interface{}) (xdr.ScVal, error) {
	switch val := v.(type) {
	case string:
		s := xdr.ScString(val)
		return xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &s}, nil
	case bool:
		b := val
		return xdr.ScVal{Type: xdr.ScValTypeScvBool, B: &b}, nil
	case int:
		i := xdr.Int64(val)
		return xdr.ScVal{Type: xdr.ScValTypeScvI64, I64: &i}, nil
	case int64:
		i := xdr.Int64(val)
		return xdr.ScVal{Type: xdr.ScValTypeScvI64, I64: &i}, nil
	case uint32:
		u := xdr.Uint32(val)
		return xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &u}, nil
	case uint64:
		u := xdr.Uint64(val)
		return xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &u}, nil
	default:
		return xdr.ScVal{}, fmt.Errorf("unsupported contract parameter type %T", v)
	}
}

// authEntriesFromSimulation extracts any recorded authorization entries from a
// simulation result.
func authEntriesFromSimulation(results []protocol.SimulateHostFunctionResult) ([]xdr.SorobanAuthorizationEntry, error) {
	auth := make([]xdr.SorobanAuthorizationEntry, 0)
	for _, r := range results {
		if r.AuthXDR == nil {
			continue
		}
		for _, a := range *r.AuthXDR {
			var entry xdr.SorobanAuthorizationEntry
			if err := xdr.SafeUnmarshalBase64(a, &entry); err != nil {
				return nil, fmt.Errorf("failed to decode simulation auth entry: %w", err)
			}
			auth = append(auth, entry)
		}
	}
	return auth, nil
}
