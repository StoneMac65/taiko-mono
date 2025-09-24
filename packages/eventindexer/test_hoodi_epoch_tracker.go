package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// NetworkConfig holds network-specific configuration
type NetworkConfig struct {
	ChainID              uint64
	GenesisTimestamp     uint64
	PreconfStartBlock    uint64
	EpochDurationSeconds uint64
	Name                 string
}

// GetNetworkConfig returns configuration for supported networks
func GetNetworkConfig(chainID uint64) *NetworkConfig {
	configs := map[uint64]*NetworkConfig{
		167000: { // Taiko Mainnet
			ChainID:              167000,
			GenesisTimestamp:     1606824023, // Ethereum mainnet beacon genesis
			PreconfStartBlock:    1320745,    // Aug 11, 2025 at 13:48:31 (preconf implementation)
			EpochDurationSeconds: 384,        // 32 slots * 12 seconds
			Name:                 "mainnet",
		},
		167009: { // Taiko Hekla
			ChainID:              167009,
			GenesisTimestamp:     1695902400, // Ethereum Holesky beacon genesis
			PreconfStartBlock:    1000000,    // Approximate - need exact block when preconf was enabled
			EpochDurationSeconds: 384,
			Name:                 "hekla",
		},
		167012: { // Taiko Hoodi
			ChainID:              167012,
			GenesisTimestamp:     1742213400, // Ethereum Hoodi beacon genesis (L1: 560048)
			PreconfStartBlock:    0,          // From L2 genesis - preconf enabled from start
			EpochDurationSeconds: 384,
			Name:                 "hoodi",
		},
		167010: { // Taiko Preconf Testnet (for testing)
			ChainID:              167010,
			GenesisTimestamp:     1742213400, // Same as Hoodi for testing
			PreconfStartBlock:    0,          // From genesis
			EpochDurationSeconds: 384,
			Name:                 "preconf-test",
		},
	}

	return configs[chainID]
}

func main() {
	fmt.Println("Testing Hoodi Network Epoch Revenue Tracking")
	fmt.Println("============================================")

	// Test all supported networks with focus on Hoodi
	testAllNetworkConfigs()

	// Test Hoodi-specific functionality
	testHoodiNetwork()

	// Test epoch calculations for all networks
	testEpochCalculationsAllNetworks()
}

func testAllNetworkConfigs() {
	fmt.Println("\n1. Testing All Network Configurations:")

	networks := []struct {
		chainID uint64
		name    string
	}{
		{167000, "Taiko Mainnet"},
		{167009, "Taiko Hekla"},
		{167012, "Taiko Hoodi"},
		{167010, "Taiko Preconf Test"},
		{999999, "Unsupported"},
	}

	for _, network := range networks {
		config := GetNetworkConfig(network.chainID)
		if config != nil {
			fmt.Printf("  ✅ %s (Chain ID: %d):\n", network.name, network.chainID)
			fmt.Printf("    Network Name: %s\n", config.Name)
			fmt.Printf("    Genesis Timestamp: %d (%s)\n",
				config.GenesisTimestamp,
				time.Unix(int64(config.GenesisTimestamp), 0).Format("2006-01-02 15:04:05 UTC"))
			fmt.Printf("    Preconf Start Block: %d\n", config.PreconfStartBlock)
			fmt.Printf("    Epoch Duration: %d seconds (%.1f minutes)\n",
				config.EpochDurationSeconds,
				float64(config.EpochDurationSeconds)/60.0)

			// Calculate current epoch for this network
			now := time.Now()
			currentTimestamp := uint64(now.Unix())
			if currentTimestamp >= config.GenesisTimestamp {
				epochNumber := (currentTimestamp - config.GenesisTimestamp) / config.EpochDurationSeconds
				fmt.Printf("    Current Epoch: %d\n", epochNumber)
			} else {
				fmt.Printf("    Current Epoch: Not started yet\n")
			}
			fmt.Println()
		} else {
			fmt.Printf("  ❌ %s (Chain ID: %d): Not supported\n", network.name, network.chainID)
		}
	}
}

func testHoodiNetwork() {
	fmt.Println("\n2. Testing Hoodi Network Connectivity:")

	// Try to connect to Hoodi RPC (you'll need to provide the actual RPC URL)
	hoodiRPCs := []string{
		"https://rpc.hoodi.taiko.xyz", // Hypothetical - replace with actual
		"http://localhost:8547",       // Local development
	}

	var client *ethclient.Client
	var err error
	var workingRPC string

	for _, rpc := range hoodiRPCs {
		fmt.Printf("  Trying RPC: %s\n", rpc)
		client, err = ethclient.Dial(rpc)
		if err == nil {
			// Test the connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			chainID, err := client.ChainID(ctx)
			if err == nil {
				fmt.Printf("  ✅ Connected! Chain ID: %s\n", chainID.String())
				workingRPC = rpc
				break
			}
		}
		fmt.Printf("  ❌ Failed to connect: %v\n", err)
	}

	if client == nil {
		fmt.Println("  ⚠️  No working RPC found. Testing with mock data.")
		testHoodiWithMockData()
		return
	}

	fmt.Printf("  Using RPC: %s\n", workingRPC)
	testHoodiWithRealData(client)
}

func testHoodiWithMockData() {
	fmt.Println("\n3. Testing Hoodi with Mock Data:")

	config := GetNetworkConfig(167012)
	if config == nil {
		fmt.Println("  ❌ Failed to get Hoodi config")
		return
	}

	// Test epoch calculation with current time
	now := time.Now()
	currentTimestamp := uint64(now.Unix())

	if currentTimestamp < config.GenesisTimestamp {
		fmt.Printf("  ⚠️  Current time (%d) is before genesis (%d)\n",
			currentTimestamp, config.GenesisTimestamp)
		// Use a time after genesis for testing
		currentTimestamp = config.GenesisTimestamp + 3600 // 1 hour after genesis
		now = time.Unix(int64(currentTimestamp), 0)
		fmt.Printf("  Using test time: %s\n", now.Format("2006-01-02 15:04:05 UTC"))
	}

	epochNumber := (currentTimestamp - config.GenesisTimestamp) / config.EpochDurationSeconds

	// Calculate epoch boundaries
	epochStart := config.GenesisTimestamp + (epochNumber * config.EpochDurationSeconds)
	epochEnd := epochStart + config.EpochDurationSeconds

	fmt.Printf("  Current Epoch: %d\n", epochNumber)
	fmt.Printf("  Epoch Start: %s\n", time.Unix(int64(epochStart), 0).Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Epoch End: %s\n", time.Unix(int64(epochEnd), 0).Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Time in Epoch: %v\n", now.Sub(time.Unix(int64(epochStart), 0)))
	fmt.Printf("  Time Remaining: %v\n", time.Unix(int64(epochEnd), 0).Sub(now))

	// Test preconf start block
	fmt.Printf("  Preconf Start Block: %d (from genesis)\n", config.PreconfStartBlock)
	fmt.Printf("  ✅ Hoodi preconf is active from L2 genesis block\n")
}

func testHoodiWithRealData(client *ethclient.Client) {
	fmt.Println("\n3. Testing Hoodi with Real Data:")

	ctx := context.Background()

	// Get chain ID
	chainID, err := client.ChainID(ctx)
	if err != nil {
		fmt.Printf("  ❌ Failed to get chain ID: %v\n", err)
		return
	}

	fmt.Printf("  Chain ID: %s\n", chainID.String())

	// Verify it's Hoodi
	if chainID.Uint64() != 167012 {
		fmt.Printf("  ⚠️  Expected chain ID 167012, got %d\n", chainID.Uint64())
	}

	// Get latest block
	block, err := client.BlockByNumber(ctx, nil)
	if err != nil {
		fmt.Printf("  ❌ Failed to get latest block: %v\n", err)
		return
	}

	fmt.Printf("  Latest Block: %d\n", block.NumberU64())
	fmt.Printf("  Block Time: %s\n", time.Unix(int64(block.Time()), 0).Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Proposer: %s\n", block.Coinbase().Hex())
	fmt.Printf("  Base Fee: %s wei\n", block.BaseFee().String())
	fmt.Printf("  Gas Used: %d\n", block.GasUsed())
	fmt.Printf("  Transaction Count: %d\n", len(block.Transactions()))

	// Test epoch calculation with real block time
	config := GetNetworkConfig(167012)
	blockTime := time.Unix(int64(block.Time()), 0)

	if uint64(blockTime.Unix()) >= config.GenesisTimestamp {
		epochNumber := (uint64(blockTime.Unix()) - config.GenesisTimestamp) / config.EpochDurationSeconds
		fmt.Printf("  Block Epoch: %d\n", epochNumber)

		// Test revenue calculation
		testHoodiRevenueCalculation(block)
	} else {
		fmt.Printf("  ⚠️  Block time is before genesis timestamp\n")
	}
}

func testHoodiRevenueCalculation(block *types.Block) {
	fmt.Println("\n4. Testing Hoodi Revenue Calculation:")

	start := time.Now()

	baseFee := block.BaseFee()
	if baseFee == nil {
		fmt.Println("  ❌ Block has no base fee")
		return
	}

	totalGasUsed := block.GasUsed()
	avgTip := big.NewInt(0)
	validTxCount := 0

	// Calculate average tip efficiently
	for _, tx := range block.Transactions() {
		var tipPerGas *big.Int

		if tx.Type() == 2 && tx.GasFeeCap() != nil && tx.GasTipCap() != nil {
			// EIP-1559 transaction
			maxFeeMinusBase := new(big.Int).Sub(tx.GasFeeCap(), baseFee)
			tipPerGas = tx.GasTipCap()
			if maxFeeMinusBase.Cmp(tipPerGas) < 0 {
				tipPerGas = maxFeeMinusBase
			}
		} else if tx.GasPrice() != nil {
			// Legacy transaction
			if tx.GasPrice().Cmp(baseFee) > 0 {
				tipPerGas = new(big.Int).Sub(tx.GasPrice(), baseFee)
			} else {
				tipPerGas = big.NewInt(0)
			}
		}

		if tipPerGas != nil {
			avgTip.Add(avgTip, tipPerGas)
			validTxCount++
		}
	}

	if validTxCount > 0 {
		avgTip.Div(avgTip, big.NewInt(int64(validTxCount)))
	}

	// Calculate proposer revenue: 75% base fee + 100% tips
	proposerBaseFee := new(big.Int).Mul(baseFee, big.NewInt(75))
	proposerBaseFee.Div(proposerBaseFee, big.NewInt(100))

	proposerFeePerGas := new(big.Int).Add(proposerBaseFee, avgTip)
	totalRevenue := new(big.Int).Mul(proposerFeePerGas, big.NewInt(int64(totalGasUsed)))

	elapsed := time.Since(start)

	// Convert to ETH
	weiPerEth := big.NewInt(1000000000000000000)
	revenueETH := new(big.Float).Quo(new(big.Float).SetInt(totalRevenue), new(big.Float).SetInt(weiPerEth))

	fmt.Printf("  ✅ Revenue calculation completed in: %v\n", elapsed)
	fmt.Printf("  Base Fee: %s wei\n", baseFee.String())
	fmt.Printf("  Average Tip: %s wei\n", avgTip.String())
	fmt.Printf("  Proposer Fee Per Gas: %s wei\n", proposerFeePerGas.String())
	fmt.Printf("  Total Gas Used: %d\n", totalGasUsed)
	fmt.Printf("  Total Revenue: %s ETH\n", revenueETH.Text('f', 8))
	fmt.Printf("  Valid Transactions: %d/%d\n", validTxCount, len(block.Transactions()))
}

func testEpochCalculationsAllNetworks() {
	fmt.Println("\n5. Testing Epoch Calculations for All Networks:")

	networks := []uint64{167000, 167009, 167012, 167010}
	now := time.Now()

	for _, chainID := range networks {
		config := GetNetworkConfig(chainID)
		if config == nil {
			continue
		}

		fmt.Printf("  %s (Chain %d):\n", config.Name, chainID)

		currentTimestamp := uint64(now.Unix())

		if currentTimestamp < config.GenesisTimestamp {
			fmt.Printf("    ⚠️  Network not started yet\n")
			continue
		}

		epochNumber := (currentTimestamp - config.GenesisTimestamp) / config.EpochDurationSeconds
		epochStart := config.GenesisTimestamp + (epochNumber * config.EpochDurationSeconds)
		epochEnd := epochStart + config.EpochDurationSeconds

		fmt.Printf("    Current Epoch: %d\n", epochNumber)
		fmt.Printf("    Progress: %.1f%% through epoch\n",
			float64(currentTimestamp-epochStart)/float64(config.EpochDurationSeconds)*100)
		fmt.Printf("    Next Epoch in: %v\n",
			time.Unix(int64(epochEnd), 0).Sub(now).Round(time.Second))

		// Test epoch boundary
		boundaryTime := time.Unix(int64(epochEnd), 0)
		nextEpochNumber := (uint64(boundaryTime.Unix()) - config.GenesisTimestamp) / config.EpochDurationSeconds

		if nextEpochNumber == epochNumber+1 {
			fmt.Printf("    ✅ Epoch boundary calculation correct\n")
		} else {
			fmt.Printf("    ❌ Epoch boundary calculation incorrect\n")
		}
		fmt.Println()
	}
}
