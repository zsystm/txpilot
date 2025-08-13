package ui

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type JSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *JSONRPCError   `json:"error"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func callEVMRPC(rpcURL, method string, params []interface{}) (json.RawMessage, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(rpcURL, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, err
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func FetchEVMChainID(rpcURL string) (string, error) {
	result, err := callEVMRPC(rpcURL, "eth_chainId", []interface{}{})
	if err != nil {
		return "", err
	}

	var chainIDHex string
	if err := json.Unmarshal(result, &chainIDHex); err != nil {
		return "", err
	}

	chainID, ok := new(big.Int).SetString(strings.TrimPrefix(chainIDHex, "0x"), 16)
	if !ok {
		return "", fmt.Errorf("invalid chain ID format")
	}

	return chainID.String(), nil
}

func FetchEVMGasPrice(rpcURL string) (string, error) {
	result, err := callEVMRPC(rpcURL, "eth_gasPrice", []interface{}{})
	if err != nil {
		return "", err
	}

	var gasPriceHex string
	if err := json.Unmarshal(result, &gasPriceHex); err != nil {
		return "", err
	}

	gasPrice, ok := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16)
	if !ok {
		return "", fmt.Errorf("invalid gas price format")
	}

	return gasPrice.String(), nil
}

func FetchEVMNonce(rpcURL, address string) (string, error) {
	result, err := callEVMRPC(rpcURL, "eth_getTransactionCount", []interface{}{address, "latest"})
	if err != nil {
		return "", err
	}

	var nonceHex string
	if err := json.Unmarshal(result, &nonceHex); err != nil {
		return "", err
	}

	nonce, ok := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)
	if !ok {
		return "", fmt.Errorf("invalid nonce format")
	}

	return nonce.String(), nil
}

func EstimateEVMGas(rpcURL string, from, to, value string) (string, error) {
	params := map[string]string{
		"from":  from,
		"to":    to,
		"value": "0x" + new(big.Int).SetUint64(parseUint64(value)).Text(16),
	}

	result, err := callEVMRPC(rpcURL, "eth_estimateGas", []interface{}{params})
	if err != nil {
		return "21000", nil
	}

	var gasHex string
	if err := json.Unmarshal(result, &gasHex); err != nil {
		return "21000", nil
	}

	gas, ok := new(big.Int).SetString(strings.TrimPrefix(gasHex, "0x"), 16)
	if !ok {
		return "21000", nil
	}

	return gas.String(), nil
}

func SendEVMTransfer(state SharedState) (string, error) {
	from := state.Inputs["from"]
	to := state.Inputs["to"]
	value := state.Inputs["value(wei)"]
	gas := state.Inputs["gas"]
	gasPrice := state.Inputs["gas_price(wei)"]
	nonceStr := state.Inputs["nonce(optional)"]
	rpcURL := state.Inputs["rpc_url"]

	if from == "" || to == "" || value == "" || gas == "" || gasPrice == "" || rpcURL == "" {
		return "", fmt.Errorf("missing required parameters")
	}

	// Parse private key to ECDSA private key
	privateKeyBytes, err := ParsePrivateKey(state.PK)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to convert private key: %w", err)
	}

	// Fetch chain ID
	chainID, err := FetchEVMChainID(rpcURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch chain ID: %w", err)
	}

	chainIDBig, ok := new(big.Int).SetString(chainID, 10)
	if !ok {
		return "", fmt.Errorf("invalid chain ID: %s", chainID)
	}

	// Get nonce from the blockchain if not provided
	var nonce uint64
	if nonceStr == "" || nonceStr == "0" {
		nonceStr, err := FetchEVMNonce(rpcURL, from)
		if err != nil {
			return "", fmt.Errorf("failed to fetch nonce: %w", err)
		}
		nonceBig, ok := new(big.Int).SetString(nonceStr, 10)
		if !ok {
			return "", fmt.Errorf("invalid nonce: %s", nonceStr)
		}
		nonce = nonceBig.Uint64()
	} else {
		nonceBig, ok := new(big.Int).SetString(nonceStr, 10)
		if !ok {
			return "", fmt.Errorf("invalid nonce: %s", nonceStr)
		}
		nonce = nonceBig.Uint64()
	}

	// Parse transaction parameters
	valueBig, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return "", fmt.Errorf("invalid value: %s", value)
	}

	gasBig, ok := new(big.Int).SetString(gas, 10)
	if !ok {
		return "", fmt.Errorf("invalid gas: %s", gas)
	}

	gasPriceBig, ok := new(big.Int).SetString(gasPrice, 10)
	if !ok {
		return "", fmt.Errorf("invalid gas price: %s", gasPrice)
	}

	// Create transaction
	toAddress := common.HexToAddress(to)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Value:    valueBig,
		Gas:      gasBig.Uint64(),
		GasPrice: gasPriceBig,
		Data:     nil,
	})

	// Sign transaction
	signer := types.NewEIP155Signer(chainIDBig)
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Encode transaction
	txData, err := signedTx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction: %w", err)
	}

	// Send transaction
	result, err := callEVMRPC(rpcURL, "eth_sendRawTransaction", []interface{}{"0x" + hex.EncodeToString(txData)})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return txHash, nil
}

// CreatePreSignedEVMTransactions creates multiple pre-signed EVM transactions with incremental nonces
func CreatePreSignedEVMTransactions(privateKeyHex, rpcURL, to, value string, count int) ([]*types.Transaction, error) {
	// Parse private key to ECDSA private key
	privateKeyBytes, err := hex.DecodeString(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	// Get from address
	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Fetch chain ID
	chainIDStr, err := FetchEVMChainID(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chain ID: %w", err)
	}

	chainID, ok := new(big.Int).SetString(chainIDStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid chain ID: %s", chainIDStr)
	}

	// Get current nonce from the blockchain
	nonceStr, err := FetchEVMNonce(rpcURL, fromAddress.Hex())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nonce: %w", err)
	}

	nonceBig, ok := new(big.Int).SetString(nonceStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid nonce: %s", nonceStr)
	}
	currentNonce := nonceBig.Uint64()

	// Get gas price
	gasPriceStr, err := FetchEVMGasPrice(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gas price: %w", err)
	}

	gasPrice, ok := new(big.Int).SetString(gasPriceStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid gas price: %s", gasPriceStr)
	}

	// Parse value
	valueBig, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("invalid value: %s", value)
	}

	// Create signer
	signer := types.NewEIP155Signer(chainID)
	toAddress := common.HexToAddress(to)

	// Create multiple pre-signed transactions
	transactions := make([]*types.Transaction, count)
	for i := 0; i < count; i++ {
		tx := types.NewTx(&types.LegacyTx{
			Nonce:    currentNonce + uint64(i),
			To:       &toAddress,
			Value:    valueBig,
			Gas:      21000, // Standard gas limit for ETH transfer
			GasPrice: gasPrice,
			Data:     nil,
		})

		signedTx, err := types.SignTx(tx, signer, privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction %d: %w", i, err)
		}

		transactions[i] = signedTx
	}

	return transactions, nil
}

// SendPreSignedEVMTransaction sends a pre-signed EVM transaction
func SendPreSignedEVMTransaction(rpcURL string, tx *types.Transaction) (string, error) {
	// Encode transaction
	txData, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction: %w", err)
	}

	// Send transaction
	result, err := callEVMRPC(rpcURL, "eth_sendRawTransaction", []interface{}{"0x" + hex.EncodeToString(txData)})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return txHash, nil
}

// GetTransactionReceipt checks if a transaction has been confirmed
func GetTransactionReceipt(rpcURL, txHash string) (bool, error) {
	result, err := callEVMRPC(rpcURL, "eth_getTransactionReceipt", []interface{}{txHash})
	if err != nil {
		return false, err
	}

	// If result is null, transaction is not yet mined
	if string(result) == "null" {
		return false, nil
	}

	var receipt map[string]interface{}
	if err := json.Unmarshal(result, &receipt); err != nil {
		return false, err
	}

	// Check if transaction was successful (status "0x1")
	status, exists := receipt["status"].(string)
	if !exists {
		return false, fmt.Errorf("no status in receipt")
	}

	return status == "0x1", nil
}

func buildEVMTransaction(to, value, gas, gasPrice, nonce, chainID string) []byte {
	nonceInt := parseUint64(nonce)
	gasPriceInt := parseUint64(gasPrice)
	gasInt := parseUint64(gas)
	valueInt := parseUint64(value)

	tx := []interface{}{
		nonceInt,
		gasPriceInt,
		gasInt,
		hexToBytes(to),
		valueInt,
		[]byte{},
	}

	return rlpEncode(tx)
}

func signEVMTransaction(txData, privateKey []byte, chainID string) []byte {
	hash := Keccak256(txData)

	r, s := signMessage(hash, privateKey)
	v := byte(27)

	if chainID != "" && chainID != "0" {
		chainIDInt := parseUint64(chainID)
		v = byte(chainIDInt*2 + 35)
	}

	signedTx := append(txData, rlpEncode([]interface{}{v, r, s})...)
	return signedTx
}

func signMessage(hash, privateKey []byte) ([]byte, []byte) {
	r := make([]byte, 32)
	s := make([]byte, 32)
	for i := range r {
		r[i] = byte((i * 3) % 256)
		s[i] = byte((i * 5) % 256)
	}
	return r, s
}

func rlpEncode(val interface{}) []byte {
	switch v := val.(type) {
	case []byte:
		if len(v) == 1 && v[0] < 0x80 {
			return v
		}
		if len(v) < 56 {
			return append([]byte{byte(0x80 + len(v))}, v...)
		}
		lenBytes := encodeInt(uint64(len(v)))
		return append(append([]byte{byte(0xb7 + len(lenBytes))}, lenBytes...), v...)

	case []interface{}:
		var output []byte
		for _, item := range v {
			output = append(output, rlpEncode(item)...)
		}
		if len(output) < 56 {
			return append([]byte{byte(0xc0 + len(output))}, output...)
		}
		lenBytes := encodeInt(uint64(len(output)))
		return append(append([]byte{byte(0xf7 + len(lenBytes))}, lenBytes...), output...)

	case uint64:
		if v == 0 {
			return []byte{0x80}
		}
		return rlpEncode(encodeInt(v))

	case byte:
		if v == 0 {
			return []byte{0x80}
		}
		if v < 0x80 {
			return []byte{v}
		}
		return []byte{0x81, v}

	default:
		return []byte{}
	}
}

func encodeInt(val uint64) []byte {
	if val == 0 {
		return []byte{}
	}

	var bytes []byte
	for val > 0 {
		bytes = append([]byte{byte(val & 0xff)}, bytes...)
		val >>= 8
	}
	return bytes
}

func parseUint64(s string) uint64 {
	val := new(big.Int)
	val.SetString(s, 10)
	return val.Uint64()
}

func hexToBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, _ := hex.DecodeString(s)
	return b
}
