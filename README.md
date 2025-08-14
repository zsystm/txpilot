# TxPilot

A simple transaction launcher for testing blockchain scenarios like nonce gaps and transaction batching.

## Features

- Interactive TUI for sending pre-defined transactions
- Supports both Cosmos and Ethereum networks
- Easy testing of nonce gap scenarios
- Quick transaction batching

## Installation

```bash
go build ./cmd/txpilot
```

## Usage

```bash
./txpilot
```

Navigate through the interface to configure and send transactions for testing purposes.

## Purpose

Created to simplify testing of transaction ordering, nonce gaps, and other blockchain edge cases without manual transaction crafting.