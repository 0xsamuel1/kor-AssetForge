package utils

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	protocol "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSorobanRPC struct {
	account           txnbuild.Account
	simResponse       protocol.SimulateTransactionResponse
	sendResponse      protocol.SendTransactionResponse
	txResponse        protocol.GetTransactionResponse
	txResponseErr     error
	getTransactionHit bool
	sendErr           error
	simErr            error
}

func (f *fakeSorobanRPC) LoadAccount(_ context.Context, address string) (txnbuild.Account, error) {
	if f.account != nil {
		return f.account, nil
	}
	return &txnbuild.SimpleAccount{AccountID: address, Sequence: 1}, nil
}

func (f *fakeSorobanRPC) SimulateTransaction(_ context.Context, _ protocol.SimulateTransactionRequest) (protocol.SimulateTransactionResponse, error) {
	return f.simResponse, f.simErr
}

func (f *fakeSorobanRPC) SendTransaction(_ context.Context, _ protocol.SendTransactionRequest) (protocol.SendTransactionResponse, error) {
	return f.sendResponse, f.sendErr
}

func (f *fakeSorobanRPC) GetTransaction(_ context.Context, _ protocol.GetTransactionRequest) (protocol.GetTransactionResponse, error) {
	f.getTransactionHit = true
	return f.txResponse, f.txResponseErr
}

func validContractID(t *testing.T) string {
	t.Helper()
	id, err := strkey.Encode(strkey.VersionByteContract, make([]byte, 32))
	require.NoError(t, err)
	return id
}

func TestContractIDToScAddress(t *testing.T) {
	contractID := validContractID(t)

	addr, err := contractIDToScAddress(contractID)
	require.NoError(t, err)
	assert.Equal(t, xdr.ScAddressTypeScAddressTypeContract, addr.Type)
	require.NotNil(t, addr.ContractId)
}

func TestContractIDToScAddressInvalid(t *testing.T) {
	_, err := contractIDToScAddress("not-a-contract")
	assert.Error(t, err)
}

func TestToScValConversions(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected xdr.ScValType
	}{
		{"string", "hello", xdr.ScValTypeScvString},
		{"bool", true, xdr.ScValTypeScvBool},
		{"int", 42, xdr.ScValTypeScvI64},
		{"int64", int64(42), xdr.ScValTypeScvI64},
		{"uint32", uint32(7), xdr.ScValTypeScvU32},
		{"uint64", uint64(7), xdr.ScValTypeScvU64},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toScVal(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got.Type)
		})
	}
}

func TestToScValUnsupported(t *testing.T) {
	_, err := toScVal(struct{}{})
	assert.Error(t, err)
}

func TestInvokeContractRequiresSigner(t *testing.T) {
	sc := &StellarClient{RPCClient: &fakeSorobanRPC{}}
	_, err := sc.InvokeContract(validContractID(t), "mint", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signer key")
}

func TestInvokeContractRequiresRPC(t *testing.T) {
	sc := &StellarClient{}
	_, err := sc.InvokeContract(validContractID(t), "mint", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RPC client")
}

func TestInvokeContractRejectsEmptyMethod(t *testing.T) {
	sc := &StellarClient{RPCClient: &fakeSorobanRPC{}, SignerKey: mustRandomKey(t)}
	_, err := sc.InvokeContract(validContractID(t), "", nil)
	assert.Error(t, err)
}

func TestInvokeContractReportsSimulationError(t *testing.T) {
	rpc := &fakeSorobanRPC{
		simResponse: protocol.SimulateTransactionResponse{Error: "host function trap"},
	}
	sc := &StellarClient{RPCClient: rpc, SignerKey: mustRandomKey(t)}
	_, err := sc.InvokeContract(validContractID(t), "mint", []interface{}{"AXN", 100})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulation failed")
}

func TestInvokeContractSuccess(t *testing.T) {
	contractID := validContractID(t)

	sorobanData, err := xdr.MarshalBase64(xdr.SorobanTransactionData{})
	require.NoError(t, err)

	rpc := &fakeSorobanRPC{
		simResponse: protocol.SimulateTransactionResponse{
			TransactionDataXDR: sorobanData,
			MinResourceFee:     100,
		},
		sendResponse: protocol.SendTransactionResponse{
			Status: "PENDING",
			Hash:   "abc123hash",
		},
		txResponse: protocol.GetTransactionResponse{
			TransactionDetails: protocol.TransactionDetails{Status: protocol.TransactionStatusSuccess},
		},
	}

	sc := &StellarClient{
		RPCClient:   rpc,
		SignerKey:   mustRandomKey(t),
		NetworkPass: "Test SDF Network ; September 2015",
	}

	hash, err := sc.InvokeContract(contractID, "mint", []interface{}{"Asset", "AXN", int64(1000)})
	require.NoError(t, err)
	assert.Equal(t, "abc123hash", hash)
	assert.True(t, rpc.getTransactionHit)
}

func TestInvokeContractOnChainFailure(t *testing.T) {
	sorobanData, err := xdr.MarshalBase64(xdr.SorobanTransactionData{})
	require.NoError(t, err)

	rpc := &fakeSorobanRPC{
		simResponse: protocol.SimulateTransactionResponse{TransactionDataXDR: sorobanData},
		sendResponse: protocol.SendTransactionResponse{
			Status: "PENDING",
			Hash:   "deadbeef",
		},
		txResponse: protocol.GetTransactionResponse{
			TransactionDetails: protocol.TransactionDetails{Status: protocol.TransactionStatusFailed},
		},
	}

	sc := &StellarClient{
		RPCClient:   rpc,
		SignerKey:   mustRandomKey(t),
		NetworkPass: "Test SDF Network ; September 2015",
	}

	_, err = sc.InvokeContract(validContractID(t), "mint", nil)
	assert.Error(t, err)
}

func mustRandomKey(t *testing.T) *keypair.Full {
	t.Helper()
	kp, err := keypair.Random()
	require.NoError(t, err)
	return kp
}
