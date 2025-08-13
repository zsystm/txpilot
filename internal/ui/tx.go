package ui

import (
	"fmt"
)

func txSend(state SharedState) (string, error) {
	switch {
	case state.Chain == "cosmos" && state.TxType == "bank/send":
		return SendCosmosBankSend(state)
	case state.Chain == "evm" && state.TxType == "eth/transfer":
		return SendEVMTransfer(state)
	default:
		return "", fmt.Errorf("unsupported tx type: %s/%s", state.Chain, state.TxType)
	}
}
