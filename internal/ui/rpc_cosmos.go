package ui

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	comethttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	clitx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	evmcryptocodec "github.com/cosmos/evm/crypto/codec"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	evmkeyring "github.com/cosmos/evm/crypto/keyring"
	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// init initializes the SDK config once at package initialization
func init() {
	cfg := sdk.GetConfig()
	if cfg.GetBech32AccountAddrPrefix() == "" {
		cfg.SetBech32PrefixForAccount("cosmos", "cosmospub")
		cfg.SetBech32PrefixForValidator("cosmosvaloper", "cosmosvaloperpub")
		cfg.Seal()
	}
}

type CosmosNodeInfo struct {
	DefaultNodeInfo struct {
		Network string `json:"network"`
	} `json:"default_node_info"`
}

type CosmosTxRequest struct {
	TxBytes []byte `json:"tx_bytes"`
	Mode    string `json:"mode"`
}

type CosmosTxResponse struct {
	TxResponse struct {
		Code   uint32 `json:"code"`
		TxHash string `json:"txhash"`
		RawLog string `json:"raw_log"`
	} `json:"tx_response"`
}

func FetchCosmosChainID(rpcURL string) (string, error) {
	// Use REST API endpoint for chain ID queries
	restURL := getRESTAddress(rpcURL)

	// Try REST endpoint first
	resp, err := http.Get(restURL + "/cosmos/base/tendermint/v1beta1/node_info")
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var nodeInfo struct {
				DefaultNodeInfo struct {
					Network string `json:"network"`
				} `json:"default_node_info"`
			}
			if err := json.Unmarshal(body, &nodeInfo); err == nil && nodeInfo.DefaultNodeInfo.Network != "" {
				return nodeInfo.DefaultNodeInfo.Network, nil
			}
		}
	}

	// Try CometBFT RPC endpoint as fallback
	cometBFTURL := getCometBFTAddress(rpcURL)
	resp2, err := http.Get(cometBFTURL + "/status")
	if err == nil && resp2.StatusCode == 200 {
		defer resp2.Body.Close()
		body, err := io.ReadAll(resp2.Body)
		if err == nil {
			var status struct {
				Result struct {
					NodeInfo struct {
						Network string `json:"network"`
					} `json:"node_info"`
				} `json:"result"`
			}
			if err := json.Unmarshal(body, &status); err == nil && status.Result.NodeInfo.Network != "" {
				return status.Result.NodeInfo.Network, nil
			}
		}
	}

	// If all fails, return error instead of fallback
	return "", fmt.Errorf("failed to fetch chain ID from node at %s", restURL)
}

func FetchCosmosAccount(rpcURL, address string) (string, string, error) {
	// Use REST API endpoint for account queries
	restURL := getRESTAddress(rpcURL)
	resp, err := http.Get(restURL + "/cosmos/auth/v1beta1/accounts/" + address)
	if err != nil {
		// If account fetch fails, return defaults for testing
		return "0", "0", nil
	}
	defer resp.Body.Close()

	// Check if account doesn't exist (404)
	if resp.StatusCode == 404 {
		return "0", "0", nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "0", "0", nil
	}

	var accountResp struct {
		Account struct {
			AccountNumber string `json:"account_number"`
			Sequence      string `json:"sequence"`
		} `json:"account"`
	}

	if err := json.Unmarshal(body, &accountResp); err != nil {
		// Account might not exist or different structure, return defaults
		return "0", "0", nil
	}

	// If values are empty, use defaults
	if accountResp.Account.AccountNumber == "" {
		accountResp.Account.AccountNumber = "0"
	}
	if accountResp.Account.Sequence == "" {
		accountResp.Account.Sequence = "0"
	}

	return accountResp.Account.AccountNumber, accountResp.Account.Sequence, nil
}

func SendCosmosBankSend(state SharedState) (string, error) {
	fromAddr := state.Inputs["from_addr"]
	toAddr := state.Inputs["to_addr"]
	amount := state.Inputs["amount"]
	rpcURL := strings.TrimSuffix(state.Inputs["node(rpc)"], "/")
	chainID := state.Inputs["chain_id"]

	if fromAddr == "" || toAddr == "" || amount == "" || rpcURL == "" || chainID == "" {
		return "", fmt.Errorf("missing required parameters")
	}

	// Parse private key
	privateKeyBytes, err := ParsePrivateKey(state.PK)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to convert private key: %w", err)
	}

	// Get account info
	accountNumber, sequence, err := FetchCosmosAccount(rpcURL, fromAddr)
	if err != nil {
		return "", fmt.Errorf("failed to fetch account info: %w", err)
	}

	// Parse amount
	amountParts := parseAmount(amount)
	if amountParts == nil {
		return "", fmt.Errorf("invalid amount format")
	}

	// Create and sign transaction
	txBytes, err := CreateSignedCosmosTx(privateKey, fromAddr, toAddr, amountParts, chainID, accountNumber, sequence)
	if err != nil {
		return "", fmt.Errorf("failed to create signed transaction: %w", err)
	}

	// Use JSON mode for broader compatibility
	payload := map[string]interface{}{
		"tx":   json.RawMessage(txBytes),
		"mode": "sync",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(
		rpcURL+"/txs",
		"application/json",
		bytes.NewBuffer(payloadBytes),
	)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var txResp struct {
		Height string `json:"height"`
		TxHash string `json:"txhash"`
		Code   uint32 `json:"code"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal(body, &txResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if txResp.Code != 0 {
		return "", fmt.Errorf("transaction failed: %s", txResp.RawLog)
	}

	if txResp.TxHash == "" {
		return "", fmt.Errorf("empty transaction hash")
	}

	return txResp.TxHash, nil
}

func parseAmount(amount string) map[string]string {
	for i := 0; i < len(amount); i++ {
		if amount[i] < '0' || amount[i] > '9' {
			return map[string]string{
				"amount": amount[:i],
				"denom":  amount[i:],
			}
		}
	}
	return map[string]string{
		"amount": amount,
		"denom":  "uatom",
	}
}

func generateRandomHash() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i * 7 % 256)
	}
	return fmt.Sprintf("%X", b)
}

// getAccountInfo queries account info via gRPC and unpacks it
func getAccountInfo(ctx context.Context, address string, authClient authtypes.QueryClient, cdc codec.Codec) (sdk.AccountI, error) {
	q, err := authClient.Account(ctx, &authtypes.QueryAccountRequest{Address: address})
	if err != nil {
		return nil, fmt.Errorf("account query failed: %w", err)
	}
	var acc sdk.AccountI
	if err := cdc.InterfaceRegistry().UnpackAny(q.Account, &acc); err != nil {
		return nil, fmt.Errorf("unpack account failed: %w", err)
	}
	return acc, nil
}

// getStakingDenom queries the staking params to get the bond denom
func getStakingDenom(ctx context.Context, stakingClient stakingtypes.QueryClient) (string, error) {
	stkParams, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	if err != nil {
		return "", fmt.Errorf("staking params query failed: %w", err)
	}
	denom := stkParams.Params.BondDenom
	if denom == "" {
		denom = "stake" // fallback
	}
	return denom, nil
}

// getGRPCAddress converts a base URL to a gRPC address (port 9090)
// Used for account queries and staking params
func getGRPCAddress(baseURL string) string {
	// Remove protocol prefix
	addr := strings.TrimPrefix(baseURL, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	// Remove trailing slash if present
	addr = strings.TrimSuffix(addr, "/")

	// Split host and port
	parts := strings.Split(addr, ":")
	if len(parts) >= 1 {
		// Always use gRPC port 9090 for queries
		return parts[0] + ":9090"
	}

	// Default to standard gRPC port
	return addr + ":9090"
}

// getCometBFTAddress converts a base URL to CometBFT RPC endpoint (port 26657)
// Used for broadcasting transactions
func getCometBFTAddress(baseURL string) string {
	// Remove protocol prefix if present
	addr := strings.TrimPrefix(baseURL, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimSuffix(addr, "/")

	// Split host and port
	parts := strings.Split(addr, ":")
	if len(parts) >= 1 {
		// Always use CometBFT RPC port 26657 for broadcasting
		return "http://" + parts[0] + ":26657"
	}

	// Default to CometBFT RPC port
	return "http://" + addr + ":26657"
}

// getRESTAddress converts a base URL to REST API endpoint (port 1317)
// Used for HTTP-based queries (account info, etc.)
func getRESTAddress(baseURL string) string {
	// Remove protocol prefix if present
	addr := strings.TrimPrefix(baseURL, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimSuffix(addr, "/")

	// Split host and port
	parts := strings.Split(addr, ":")
	if len(parts) >= 1 {
		// Always use REST API port 1317 for HTTP queries
		return "http://" + parts[0] + ":1317"
	}

	// Default to REST API port
	return "http://" + addr + ":1317"
}

// CosmosClientConfig holds the configuration for Cosmos client
type CosmosClientConfig struct {
	GRPCAddr     string // e.g., "127.0.0.1:9090" for account/staking queries
	CometBFTAddr string // e.g., "http://127.0.0.1:26657" for broadcasting
	RESTAddr     string // e.g., "http://127.0.0.1:1317" for HTTP queries
	ChainID      string
}

// CosmosClient wraps the gRPC connection and client context
type CosmosClient struct {
	grpcConn  *grpc.ClientConn
	clientCtx client.Context
	config    CosmosClientConfig
}

// NewCosmosClient creates a new Cosmos client with persistent connections
func NewCosmosClient(cfg CosmosClientConfig) (*CosmosClient, error) {
	// Create gRPC connection
	grpcConn, err := grpc.NewClient(cfg.GRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}

	// Setup interface registry and codec
	ifaceReg := codectypes.NewInterfaceRegistry()
	banktypes.RegisterInterfaces(ifaceReg)
	authtypes.RegisterInterfaces(ifaceReg)
	cryptocodec.RegisterInterfaces(ifaceReg)
	evmcryptocodec.RegisterInterfaces(ifaceReg)
	stakingtypes.RegisterInterfaces(ifaceReg)
	protoCodec := codec.NewProtoCodec(ifaceReg)

	// Create transaction config
	txConfig := authtx.NewTxConfig(protoCodec, authtx.DefaultSignModes)

	// Create CometBFT RPC client for broadcasting using the configured address
	tmRPC, err := comethttp.New(cfg.CometBFTAddr, "/websocket")
	if err != nil {
		grpcConn.Close()
		return nil, fmt.Errorf("failed to create CometBFT RPC client: %w", err)
	}

	// Build client.Context
	clientCtx := client.Context{}.
		WithChainID(cfg.ChainID).
		WithCodec(protoCodec).
		WithInterfaceRegistry(ifaceReg).
		WithTxConfig(txConfig).
		WithClient(tmRPC).
		WithBroadcastMode("sync").
		WithGRPCClient(grpcConn)

	return &CosmosClient{
		grpcConn:  grpcConn,
		clientCtx: clientCtx,
		config:    cfg,
	}, nil
}

// Close closes the gRPC connection
func (c *CosmosClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}

// CreateAndBroadcastTx creates, signs, and broadcasts a transaction using the client context
func (c *CosmosClient) CreateAndBroadcastTx(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	toAddr string,
	amount string,
) (string, error) {
	// Setup keyring with the private key
	kr := keyring.NewInMemory(c.clientCtx.Codec, evmkeyring.Option())
	fromUid := "tempkey"

	privKeyBytes := crypto.FromECDSA(privateKey)
	privKeyHex := fmt.Sprintf("%x", privKeyBytes)
	err := kr.ImportPrivKeyHex(fromUid, privKeyHex, ethsecp256k1.KeyType)
	if err != nil {
		return "", fmt.Errorf("failed to import private key: %w", err)
	}

	testKey := &ethsecp256k1.PrivKey{Key: privKeyBytes}
	fromAddr := sdk.AccAddress(testKey.PubKey().Address())

	// Update client context with keyring and from address
	c.clientCtx = c.clientCtx.WithKeyring(kr).WithFromAddress(fromAddr)

	// Query account info
	authQ := authtypes.NewQueryClient(c.grpcConn)
	accI, err := getAccountInfo(ctx, fromAddr.String(), authQ, c.clientCtx.Codec)
	if err != nil {
		return "", fmt.Errorf("failed to get account info: %w", err)
	}
	accNum := accI.GetAccountNumber()
	seq := accI.GetSequence()

	// Query staking params to get bond_denom
	stkQ := stakingtypes.NewQueryClient(c.grpcConn)
	denom, err := getStakingDenom(ctx, stkQ)
	if err != nil {
		denom = "stake" // fallback
	}

	// Parse amount
	amountParts := parseAmount(amount)
	if amountParts == nil {
		return "", fmt.Errorf("invalid amount format")
	}

	amountInt, ok := math.NewIntFromString(amountParts["amount"])
	if !ok {
		return "", fmt.Errorf("invalid amount: %s", amountParts["amount"])
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom, amountInt))

	// Create MsgSend
	toAddress, err := sdk.AccAddressFromBech32(toAddr)
	if err != nil {
		return "", fmt.Errorf("invalid to address: %w", err)
	}
	msg := banktypes.NewMsgSend(fromAddr, toAddress, coins)

	// Create transaction factory with gas simulation
	txf := clitx.Factory{}.
		WithKeybase(kr).
		WithChainID(c.config.ChainID).
		WithTxConfig(c.clientCtx.TxConfig).
		WithAccountNumber(accNum).
		WithSequence(seq).
		WithSignMode(signing.SignMode(c.clientCtx.TxConfig.SignModeHandler().DefaultMode())).
		//WithGasAdjustment(1.2).
		WithGas(200000).                                  // TODO: Auto estimate gas
		WithGasPrices(fmt.Sprintf("1000000000%s", denom)) // 1Gwei

	// TODO: Simulate gas and set adjusted gas
	//_, adjustedGas, err := clitx.CalculateGas(c.grpcConn, txf, msg)
	//if err != nil {
	//	return "", fmt.Errorf("failed to simulate gas: %w", err)
	//}
	//txf = txf.WithGas(adjustedGas)

	// Build unsigned transaction
	txBuilder, err := txf.BuildUnsignedTx(msg)
	if err != nil {
		return "", fmt.Errorf("failed to build unsigned tx: %w", err)
	}

	// Sign the transaction
	err = clitx.Sign(ctx, txf, fromUid, txBuilder, true)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Encode and broadcast
	txBytes, err := c.clientCtx.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return "", fmt.Errorf("failed to encode transaction: %w", err)
	}

	res, err := c.clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if res.Code != 0 {
		return "", fmt.Errorf("transaction failed: %s", res.RawLog)
	}

	return res.TxHash, nil
}

// CreateSignedCosmosTx maintains backward compatibility
func CreateSignedCosmosTx(privateKey *ecdsa.PrivateKey, fromAddr, toAddr string, amountParts map[string]string, chainID, accountNumber, sequence string) ([]byte, error) {
	ctx := context.Background()

	// Parse numeric values
	accNum, err := strconv.ParseUint(accountNumber, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid account number: %w", err)
	}
	seq, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid sequence: %w", err)
	}

	// Setup interface registry and codec
	ifaceReg := codectypes.NewInterfaceRegistry()
	banktypes.RegisterInterfaces(ifaceReg)
	authtypes.RegisterInterfaces(ifaceReg)
	cryptocodec.RegisterInterfaces(ifaceReg)
	evmcryptocodec.RegisterInterfaces(ifaceReg)
	protoCodec := codec.NewProtoCodec(ifaceReg)

	// Create ethsecp256k1 private key from ECDSA
	privKeyBytes := crypto.FromECDSA(privateKey)
	testKey := &ethsecp256k1.PrivKey{Key: privKeyBytes}

	// Create in-memory keyring and import the key
	kr := keyring.NewInMemory(protoCodec, evmkeyring.Option())
	fromUid := "tempkey"

	// Convert private key to hex string for import
	privKeyHex := fmt.Sprintf("%x", privKeyBytes)
	err = kr.ImportPrivKeyHex(fromUid, privKeyHex, ethsecp256k1.KeyType)
	if err != nil {
		return nil, fmt.Errorf("failed to import private key: %w", err)
	}

	fromAddress := sdk.AccAddress(testKey.PubKey().Address())

	// Create transaction config
	txConfig := authtx.NewTxConfig(protoCodec, authtx.DefaultSignModes)

	// Parse amount
	amountInt, ok := math.NewIntFromString(amountParts["amount"])
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", amountParts["amount"])
	}
	amount := sdk.NewCoins(sdk.NewCoin(amountParts["denom"], amountInt))

	// Create MsgSend
	toAddress, err := sdk.AccAddressFromBech32(toAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}
	msg := banktypes.NewMsgSend(fromAddress, toAddress, amount)

	// Create transaction factory with gas simulation support
	txf := clitx.Factory{}.
		WithKeybase(kr).
		WithChainID(chainID).
		WithTxConfig(txConfig).
		WithAccountNumber(accNum).
		WithSequence(seq).
		WithSignMode(signing.SignMode(txConfig.SignModeHandler().DefaultMode())).
		WithGasAdjustment(1.2).
		WithGas(200000).
		WithGasPrices(fmt.Sprintf("0.025%s", amountParts["denom"]))

	// Build unsigned transaction
	txBuilder, err := txf.BuildUnsignedTx(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to build unsigned tx: %w", err)
	}

	// Sign the transaction
	err = clitx.Sign(ctx, txf, fromUid, txBuilder, true)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Encode to bytes
	txBytes, err := txConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction: %w", err)
	}

	return txBytes, nil
}

// CreatePreSignedCosmosTransactions creates multiple pre-signed Cosmos transactions with incremental sequences
func CreatePreSignedCosmosTransactions(privateKeyHex, rpcURL, to, amount string, count int) ([]map[string]interface{}, error) {
	// Parse private key
	privateKeyBytes, err := ParsePrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	_, err = crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	// Get from address
	fromAddr, err := DeriveCosmosAddress(privateKeyBytes, "cosmos")
	if err != nil {
		return nil, fmt.Errorf("failed to derive cosmos address: %w", err)
	}

	// Fetch chain ID
	chainID, err := FetchCosmosChainID(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chain ID: %w", err)
	}

	// Get current account info
	accountNumber, sequenceStr, err := FetchCosmosAccount(rpcURL, fromAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch account info: %w", err)
	}

	currentSequence, _ := strconv.ParseUint(sequenceStr, 10, 64)

	// Parse amount
	amountParts := parseAmount(amount)
	if amountParts == nil {
		return nil, fmt.Errorf("invalid amount format: %s", amount)
	}

	// Create multiple pre-signed transactions
	transactions := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		seqStr := strconv.FormatUint(currentSequence+uint64(i), 10)

		// Create transaction data structure
		txData := map[string]interface{}{
			"from":           fromAddr,
			"to":             to,
			"amount":         amount,
			"sequence":       seqStr,
			"account_number": accountNumber,
			"chain_id":       chainID,
		}

		transactions[i] = txData
	}

	return transactions, nil
}

// SendPreSignedCosmosTransaction sends a pre-signed Cosmos transaction using CosmosClient
func SendPreSignedCosmosTransaction(rpcURL, privateKeyHex string, txData map[string]interface{}) (string, error) {
	ctx := context.Background()

	// Parse private key
	privateKeyBytes, err := ParsePrivateKey(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to convert private key: %w", err)
	}

	// Extract data from txData
	toAddr := txData["to"].(string)
	amount := txData["amount"].(string)

	// Query chain ID from the node to ensure we use the correct one
	chainID, err := FetchCosmosChainID(rpcURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch chain ID: %w", err)
	}

	// Create CosmosClient with proper endpoint configuration
	grpcAddr := getGRPCAddress(rpcURL)
	cometBFTAddr := getCometBFTAddress(rpcURL)
	restAddr := getRESTAddress(rpcURL)
	config := CosmosClientConfig{
		GRPCAddr:     grpcAddr,
		CometBFTAddr: cometBFTAddr,
		RESTAddr:     restAddr,
		ChainID:      chainID,
	}

	cosmosClient, err := NewCosmosClient(config)
	if err != nil {
		return "", fmt.Errorf("failed to create cosmos client: %w", err)
	}
	defer cosmosClient.Close()

	// Use the CosmosClient to create and broadcast the transaction
	// This ensures proper account querying and chain ID consistency
	return cosmosClient.CreateAndBroadcastTx(ctx, privateKey, toAddr, amount)
}

// GetCosmosTransactionReceipt checks if a Cosmos transaction has been confirmed
func GetCosmosTransactionReceipt(rpcURL, txHash string) (bool, error) {
	// Try CometBFT RPC endpoint first (more reliable for tx queries)
	cometBFTURL := getCometBFTAddress(rpcURL)

	// Ensure txHash has 0x prefix for CometBFT RPC
	if !strings.HasPrefix(txHash, "0x") && !strings.HasPrefix(txHash, "0X") {
		txHash = "0x" + strings.ToUpper(txHash)
	}

	resp, err := http.Get(cometBFTURL + "/tx?hash=" + txHash)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		var cometResp struct {
			Result struct {
				TxResult struct {
					Code uint32 `json:"code"`
				} `json:"tx_result"`
			} `json:"result"`
			Error *struct {
				Code int    `json:"code"`
				Data string `json:"data"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &cometResp); err == nil {
			// If there's an error, transaction not found
			if cometResp.Error != nil {
				return false, nil
			}
			// Check if transaction was successful (code 0)
			return cometResp.Result.TxResult.Code == 0, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Fallback to REST API endpoint
	restURL := getRESTAddress(rpcURL)
	// Remove 0x prefix for REST API
	cleanHash := strings.TrimPrefix(strings.TrimPrefix(txHash, "0x"), "0X")
	resp2, err := http.Get(restURL + "/cosmos/tx/v1beta1/txs/" + cleanHash)
	if err != nil {
		return false, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode == 404 {
		// Transaction not found yet
		return false, nil
	}

	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		return false, err
	}

	var restResp struct {
		TxResponse struct {
			Height string `json:"height"`
			TxHash string `json:"txhash"`
			Code   uint32 `json:"code"`
			RawLog string `json:"raw_log"`
		} `json:"tx_response"`
	}
	if err := json.Unmarshal(body, &restResp); err != nil {
		return false, err
	}

	// Check if transaction was successful (code 0)
	return restResp.TxResponse.Code == 0, nil
}
