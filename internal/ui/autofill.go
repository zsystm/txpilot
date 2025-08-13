package ui

import (
	"fmt"
	"strings"
)

type AutoFillInput struct {
	TxType string
	PKHex  string
	RPCURL string
}

type AutoFillOutput map[string]string

var rpcCache = make(map[string]string)

func AutoFillParams(in AutoFillInput) (AutoFillOutput, error) {
	switch in.TxType {
	case "bank/send":
		return autoFillCosmosBankSend(in)
	case "eth/transfer":
		return autoFillEVMTransfer(in)
	default:
		return AutoFillOutput{}, fmt.Errorf("unsupported tx type: %s", in.TxType)
	}
}

func autoFillCosmosBankSend(in AutoFillInput) (AutoFillOutput, error) {
	out := make(AutoFillOutput)

	privateKey, err := ParsePrivateKey(in.PKHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	fromAddr, err := DeriveCosmosAddress(privateKey, "cosmos")
	if err != nil {
		fromAddr = "cosmos1placeholder"
	}
	out["from_addr"] = fromAddr

	hrp, err := ExtractHRP(fromAddr)
	if err != nil {
		hrp = "cosmos"
	}

	toAddr, err := GenerateRandomCosmosAddress(hrp)
	if err != nil {
		toAddr = hrp + "1randomaddress"
	}
	out["to_addr"] = toAddr

	out["amount"] = "1000uatom"

	if in.RPCURL != "" {
		out["node(rpc)"] = in.RPCURL

		cacheKey := "cosmos_chain_" + in.RPCURL
		if chainID, ok := rpcCache[cacheKey]; ok {
			out["chain_id"] = chainID
		} else {
			chainID, err := FetchCosmosChainID(in.RPCURL)
			if err == nil && chainID != "" {
				out["chain_id"] = chainID
				rpcCache[cacheKey] = chainID
			} else {
				out["chain_id"] = "cosmoshub-4"
			}
		}
	} else {
		if lastRPC, ok := rpcCache["last_cosmos_rpc"]; ok {
			out["node(rpc)"] = lastRPC
		} else {
			out["node(rpc)"] = ""
		}
		out["chain_id"] = "cosmoshub-4"
	}

	return out, nil
}

func autoFillEVMTransfer(in AutoFillInput) (AutoFillOutput, error) {
	out := make(AutoFillOutput)

	privateKey, err := ParsePrivateKey(in.PKHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	fromAddr, err := DeriveEVMAddress(privateKey)
	if err != nil {
		fromAddr = "0x0000000000000000000000000000000000000000"
	}
	out["from"] = fromAddr

	out["to"] = GenerateRandomEVMAddress()

	out["value(wei)"] = "100000000000000"

	if in.RPCURL != "" {
		out["rpc_url"] = in.RPCURL

		cacheKey := "evm_gas_price_" + in.RPCURL
		if gasPrice, ok := rpcCache[cacheKey]; ok {
			out["gas_price(wei)"] = gasPrice
		} else {
			gasPrice, err := FetchEVMGasPrice(in.RPCURL)
			if err == nil && gasPrice != "" {
				out["gas_price(wei)"] = gasPrice
				rpcCache[cacheKey] = gasPrice
			} else {
				out["gas_price(wei)"] = "20000000000"
			}
		}

		nonce, err := FetchEVMNonce(in.RPCURL, fromAddr)
		if err == nil && nonce != "" {
			out["nonce(optional)"] = nonce
		} else {
			out["nonce(optional)"] = "0"
		}

		gas, err := EstimateEVMGas(in.RPCURL, fromAddr, out["to"], out["value(wei)"])
		if err == nil && gas != "" {
			out["gas"] = gas
		} else {
			out["gas"] = "21000"
		}
	} else {
		if lastRPC, ok := rpcCache["last_evm_rpc"]; ok {
			out["rpc_url"] = lastRPC
		} else {
			out["rpc_url"] = ""
		}
		out["gas"] = "21000"
		out["gas_price(wei)"] = "20000000000"
		out["nonce(optional)"] = "0"
	}

	return out, nil
}

func CacheRPC(chain, rpcURL string) {
	if chain == "cosmos" {
		rpcCache["last_cosmos_rpc"] = rpcURL
	} else if chain == "evm" {
		rpcCache["last_evm_rpc"] = rpcURL
	}
}

func GetCachedRPC(chain string) string {
	if chain == "cosmos" {
		if rpc, ok := rpcCache["last_cosmos_rpc"]; ok {
			return rpc
		}
	} else if chain == "evm" {
		if rpc, ok := rpcCache["last_evm_rpc"]; ok {
			return rpc
		}
	}
	return ""
}

func shouldMakeFieldReadOnly(label string) bool {
	readOnlyFields := []string{
		"from_addr",
		"from",
		"chain_id",
	}

	fieldName := strings.Split(label, "(")[0]
	for _, ro := range readOnlyFields {
		if fieldName == ro {
			return true
		}
	}
	return false
}
