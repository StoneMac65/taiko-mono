package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer/indexer"
)

func main() {
	fmt.Println("Testing Improved Epoch Revenue Tracking System")
	fmt.Println("==============================================")

	// Test network configurations
	testNetworkConfigs()

	// Test RPC connection and block processing
	testRPCAndBlocks()

	// Test epoch calculations
	testEpochCalculations()
}

func testNetworkConfigs() {
	fmt.Println("\n1. Testing Network Configurations:")

	networks := []uint64{167000, 167009, 167012, 999999}

	for _, chainID := range networks {
		config := indexer.GetNetworkConfig(chainID)
		if config != nil {
			fmt.Printf("  Chain ID %d (%s):\n", chainID, config.Name)
			fmt.Printf("    Genesis: %d (%s)\n",
				config.GenesisTimestamp,
				time.Unix(int64(config.GenesisTimestamp), 0).Format("2006-01-02 15:04:05"))
			fmt.Printf("    Preconf Start Block: %d\n", config.PreconfStartBlock)
			fmt.Printf("    Epoch Duration: %d seconds\n", config.EpochDurationSeconds)
		} else {
			fmt.Printf("  Chain ID %d: Not supported\n", chainID)
		}
	}
}

func testRPCAndBlocks() {
	fmt.Println("\n2. Testing RPC Connection and Block Processing:")

	client, err := ethclient.Dial("https://rpc.mainnet.taiko.xyz")
	if err != nil {
		fmt.Printf("  ❌ Failed to connect to RPC: %v\n", err)
		return
	}

	fmt.Printf("  ✅ Connected to Taiko RPC\n")

	// Get chain ID
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		fmt.Printf("  ❌ Failed to get chain ID: %v\n", err)
		return
	}

	fmt.Printf("  Chain ID: %s\n", chainID.String())

	// Get latest block
	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		fmt.Printf("  ❌ Failed to get latest block: %v\n", err)
		return
	}

	fmt.Printf("  Latest Block: %d\n", block.NumberU64())
	fmt.Printf("  Block Time: %s\n", time.Unix(int64(block.Time()), 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("  Proposer: %s\n", block.Coinbase().Hex())
	fmt.Printf("  Base Fee: %s wei\n", block.BaseFee().String())
	fmt.Printf("  Gas Used: %d\n", block.GasUsed())
	fmt.Printf("  Transaction Count: %d\n", len(block.Transactions()))

	// Test revenue calculation efficiency
	testRevenueCalculation(block)
}

func testRevenueCalculation(block *types.Block) {
	fmt.Println("\n3. Testing Revenue Calculation:")

	start := time.Now()

	// Simulate the efficient calculation
	totalRevenue := big.NewInt(0)
	totalGasUsed := block.GasUsed()
	baseFee := block.BaseFee()

	if baseFee == nil {
		fmt.Printf("  ❌ Block has no base fee\n")
		return
	}

	// Calculate average tip
	avgTip := big.NewInt(0)
	validTxCount := 0

	for _, tx := range block.Transactions() {
		var tipPerGas *big.Int

		if tx.Type() == 2 && tx.GasFeeCap() != nil && tx.GasTipCap() != nil {
			maxFeeMinusBase := new(big.Int).Sub(tx.GasFeeCap(), baseFee)
			tipPerGas = tx.GasTipCap()
			if maxFeeMinusBase.Cmp(tipPerGas) < 0 {
				tipPerGas = maxFeeMinusBase
			}
		} else if tx.GasPrice() != nil {
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
	totalRevenue = new(big.Int).Mul(proposerFeePerGas, big.NewInt(int64(totalGasUsed)))

	elapsed := time.Since(start)

	// Convert to ETH
	weiPerEth := big.NewInt(1000000000000000000)
	revenueETH := new(big.Float).Quo(new(big.Float).SetInt(totalRevenue), new(big.Float).SetInt(weiPerEth))

	fmt.Printf("  ✅ Revenue calculation completed in %v\n", elapsed)
	fmt.Printf("  Total Gas Used: %d\n", totalGasUsed)
	fmt.Printf("  Average Tip: %s wei\n", avgTip.String())
	fmt.Printf("  Proposer Fee Per Gas: %s wei\n", proposerFeePerGas.String())
	fmt.Printf("  Total Revenue: %s ETH\n", revenueETH.Text('f', 6))
}

func testEpochCalculations() {
	fmt.Println("\n4. Testing Epoch Calculations:")

	config := indexer.GetNetworkConfig(167000) // Taiko mainnet
	if config == nil {
		fmt.Printf("  ❌ Failed to get network config\n")
		return
	}

	now := time.Now()
	currentTimestamp := uint64(now.Unix())

	// Calculate current epoch
	epochNumber := (currentTimestamp - config.GenesisTimestamp) / config.EpochDurationSeconds

	// Calculate epoch boundaries
	epochStartTimestamp := config.GenesisTimestamp + (epochNumber * config.EpochDurationSeconds)
	epochEndTimestamp := epochStartTimestamp + config.EpochDurationSeconds

	epochStart := time.Unix(int64(epochStartTimestamp), 0)
	epochEnd := time.Unix(int64(epochEndTimestamp), 0)

	fmt.Printf("  Current Time: %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Current Epoch: %d\n", epochNumber)
	fmt.Printf("  Epoch Start: %s\n", epochStart.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Epoch End: %s\n", epochEnd.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Time in Epoch: %v\n", now.Sub(epochStart))
	fmt.Printf("  Time Remaining: %v\n", epochEnd.Sub(now))

	// Test edge cases
	fmt.Println("\n  Testing Edge Cases:")

	// Test epoch boundary
	boundaryTime := time.Unix(int64(epochEndTimestamp), 0)
	nextEpochNumber := (uint64(boundaryTime.Unix()) - config.GenesisTimestamp) / config.EpochDurationSeconds
	fmt.Printf("  At epoch boundary (%s): Epoch %d\n",
		boundaryTime.Format("15:04:05"), nextEpochNumber)

	// Test one second after boundary
	afterBoundary := boundaryTime.Add(1 * time.Second)
	afterEpochNumber := (uint64(afterBoundary.Unix()) - config.GenesisTimestamp) / config.EpochDurationSeconds
	fmt.Printf("  One second after boundary: Epoch %d\n", afterEpochNumber)

	if afterEpochNumber == nextEpochNumber+1 {
		fmt.Printf("  ✅ Epoch boundary calculation is correct\n")
	} else {
		fmt.Printf("  ❌ Epoch boundary calculation is incorrect\n")
	}
}
