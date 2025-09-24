package indexer

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"golang.org/x/exp/slog"
)

// EpochTracker manages epoch-based revenue tracking for preconfers
type EpochTracker struct {
	currentEpoch *EpochData
	indexer      *Indexer
}

// EpochData represents a single preconfer epoch with revenue data
type EpochData struct {
	EpochNumber      uint64    `json:"epochNumber"`
	PreconferAddress string    `json:"preconferAddress"`
	StartTime        time.Time `json:"startTime"`
	EndTime          time.Time `json:"endTime"`
	TotalL2Revenue   *big.Int  `json:"totalL2Revenue"`
	BlockCount       int64     `json:"blockCount"`
	BatchCount       int64     `json:"batchCount"`
	StartBlockNumber uint64    `json:"startBlockNumber"`
	EndBlockNumber   uint64    `json:"endBlockNumber"`
	GasUsed          uint64    `json:"gasUsed"`
	TransactionCount int64     `json:"transactionCount"`
}

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

func NewEpochTracker(indexer *Indexer) *EpochTracker {
	return &EpochTracker{
		indexer: indexer,
	}
}

// trackL2BlockForEpoch processes a single L2 block for epoch revenue tracking
func (i *Indexer) trackL2BlockForEpoch(ctx context.Context, blockNumber uint64) error {
	config := GetNetworkConfig(i.srcChainID)
	if config == nil {
		return fmt.Errorf("unsupported chain ID: %d", i.srcChainID)
	}

	if blockNumber < config.PreconfStartBlock {
		return nil // Skip blocks before preconfirmation started
	}

	block, err := i.ethClient.BlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		return errors.Wrap(err, "failed to fetch L2 block")
	}

	proposer := block.Coinbase()
	blockTime := time.Unix(int64(block.Time()), 0)

	// Check if we need to start a new epoch
	if i.isNewEpochForBlock(proposer, blockTime, config) {
		if err := i.endCurrentEpoch(ctx); err != nil {
			slog.Warn("Failed to end previous epoch", "error", err)
		}

		if err := i.startNewEpochForBlock(ctx, proposer, blockTime, blockNumber, config); err != nil {
			return errors.Wrap(err, "failed to start new epoch")
		}
	}

	if err := i.addL2BlockToEpoch(ctx, block); err != nil {
		return errors.Wrap(err, "failed to add block to epoch")
	}

	return nil
}

// isNewEpochForBlock determines if a new epoch should be started
func (i *Indexer) isNewEpochForBlock(proposer common.Address, blockTime time.Time, config *NetworkConfig) bool {
	if i.currentEpoch == nil {
		return true
	}

	// Check if we're in a different epoch based on timestamp
	currentEpochNumber := i.calculateEpochNumberForTime(blockTime, config)
	if currentEpochNumber != i.currentEpoch.EpochNumber {
		return true
	}

	// Check if proposer changed (additional epoch boundary condition)
	if i.currentEpoch.PreconferAddress != proposer.Hex() {
		slog.Info("Proposer changed, starting new epoch",
			"oldProposer", i.currentEpoch.PreconferAddress,
			"newProposer", proposer.Hex(),
			"blockNumber", i.currentEpoch.EndBlockNumber)
		return true
	}

	return false
}

// startNewEpochForBlock initializes a new epoch
func (i *Indexer) startNewEpochForBlock(ctx context.Context, proposer common.Address, blockTime time.Time, blockNumber uint64, config *NetworkConfig) error {
	epochNumber := i.calculateEpochNumberForTime(blockTime, config)

	i.currentEpoch = &EpochData{
		EpochNumber:      epochNumber,
		PreconferAddress: proposer.Hex(),
		StartTime:        blockTime,
		TotalL2Revenue:   big.NewInt(0),
		BlockCount:       0,
		BatchCount:       0,
		StartBlockNumber: blockNumber,
		EndBlockNumber:   blockNumber,
		GasUsed:          0,
		TransactionCount: 0,
	}

	slog.Info("Started new epoch",
		"epochNumber", epochNumber,
		"proposer", proposer.Hex(),
		"startBlock", blockNumber,
		"startTime", blockTime)

	return nil
}

// calculateEpochNumberForTime calculates the epoch number for a given block time
func (i *Indexer) calculateEpochNumberForTime(blockTime time.Time, config *NetworkConfig) uint64 {
	if config == nil || config.GenesisTimestamp == 0 {
		return 0
	}

	currentTimestamp := uint64(blockTime.Unix())
	if currentTimestamp < config.GenesisTimestamp {
		return 0
	}

	return (currentTimestamp - config.GenesisTimestamp) / config.EpochDurationSeconds
}

// getEpochBoundaries returns the start and end times for a given epoch
func (i *Indexer) getEpochBoundaries(epochNumber uint64, config *NetworkConfig) (time.Time, time.Time) {
	if config == nil {
		return time.Time{}, time.Time{}
	}

	epochStartTimestamp := config.GenesisTimestamp + (epochNumber * config.EpochDurationSeconds)
	epochEndTimestamp := epochStartTimestamp + config.EpochDurationSeconds

	return time.Unix(int64(epochStartTimestamp), 0), time.Unix(int64(epochEndTimestamp), 0)
}

// isPreconfirmationActiveForBlock checks if preconfirmation is active for a block
// This is now handled in trackL2BlockForEpoch using NetworkConfig
func (i *Indexer) isPreconfirmationActiveForBlock(blockNumber uint64) bool {
	config := GetNetworkConfig(i.srcChainID)
	if config == nil {
		return false
	}
	return blockNumber >= config.PreconfStartBlock
}

// addL2BlockToEpoch adds a block's revenue to the current epoch
func (i *Indexer) addL2BlockToEpoch(ctx context.Context, block *types.Block) error {
	if i.currentEpoch == nil {
		return errors.New("no current epoch to add block to")
	}

	blockRevenue, gasUsed := i.calculateBlockRevenueEfficient(block)
	txCount := int64(len(block.Transactions()))

	// Convert to ETH for logging
	weiPerEth := decimal.NewFromBigInt(big.NewInt(1000000000000000000), 0)
	revenueWei := decimal.NewFromBigInt(blockRevenue, 0)
	revenueETH := revenueWei.Div(weiPerEth)

	slog.Debug("Adding L2 block to epoch",
		"blockNumber", block.NumberU64(),
		"proposer", block.Coinbase().Hex(),
		"txCount", txCount,
		"gasUsed", gasUsed,
		"blockRevenueETH", revenueETH.String(),
		"epochNumber", i.currentEpoch.EpochNumber)

	// Update epoch data
	i.currentEpoch.TotalL2Revenue.Add(i.currentEpoch.TotalL2Revenue, blockRevenue)
	i.currentEpoch.BlockCount++
	i.currentEpoch.EndBlockNumber = block.NumberU64()
	i.currentEpoch.GasUsed += gasUsed
	i.currentEpoch.TransactionCount += txCount

	return nil
}

// calculateBlockRevenueEfficient calculates block revenue without individual receipt calls
// This is much more efficient as it uses block-level data and estimates gas usage
func (i *Indexer) calculateBlockRevenueEfficient(block *types.Block) (*big.Int, uint64) {
	totalRevenue := big.NewInt(0)
	totalGasUsed := uint64(0)

	baseFee := block.BaseFee()
	if baseFee == nil {
		slog.Warn("Block has no base fee, skipping revenue calculation", "blockNumber", block.NumberU64())
		return totalRevenue, totalGasUsed
	}

	// Use block gas limit as approximation if gas used is not available
	blockGasUsed := block.GasUsed()
	if blockGasUsed == 0 {
		// Estimate based on transaction count and average gas per tx
		avgGasPerTx := uint64(21000) // Base gas for simple transfer
		if len(block.Transactions()) > 0 {
			avgGasPerTx = 50000 // Higher estimate for contract interactions
		}
		blockGasUsed = uint64(len(block.Transactions())) * avgGasPerTx
	}

	totalGasUsed = blockGasUsed

	// Calculate revenue based on block-level data
	// For efficiency, we'll estimate the average fee and apply it to total gas used
	avgTipPerGas := big.NewInt(0)
	validTxCount := 0

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
			avgTipPerGas.Add(avgTipPerGas, tipPerGas)
			validTxCount++
		}
	}

	// Calculate average tip per gas
	if validTxCount > 0 {
		avgTipPerGas.Div(avgTipPerGas, big.NewInt(int64(validTxCount)))
	}

	// Calculate proposer revenue: 75% of base fee + 100% of tips
	proposerBaseFee := new(big.Int).Mul(baseFee, big.NewInt(75))
	proposerBaseFee.Div(proposerBaseFee, big.NewInt(100))

	proposerFeePerGas := new(big.Int).Add(proposerBaseFee, avgTipPerGas)
	totalRevenue = new(big.Int).Mul(proposerFeePerGas, big.NewInt(int64(totalGasUsed)))

	return totalRevenue, totalGasUsed
}

func (i *Indexer) endCurrentEpoch(ctx context.Context) error {
	if i.currentEpoch == nil {
		return nil
	}

	i.currentEpoch.EndTime = time.Now().UTC()

	if err := i.saveEpochGasData(ctx, i.currentEpoch); err != nil {
		return errors.Wrap(err, "failed to save epoch gas data")
	}

	i.currentEpoch = nil
	return nil
}

// saveEpochGasData saves epoch revenue data to the database
func (i *Indexer) saveEpochGasData(ctx context.Context, epoch *EpochData) error {
	// Convert wei to ETH (divide by 10^18)
	weiPerEth := decimal.NewFromBigInt(big.NewInt(1000000000000000000), 0) // 10^18
	revenueWei := decimal.NewFromBigInt(epoch.TotalL2Revenue, 0)
	revenueETH := revenueWei.Div(weiPerEth)

	date := epoch.EndTime.Format("2006-01-02")
	config := GetNetworkConfig(i.srcChainID)
	taskName := fmt.Sprintf("epoch_l2_revenue_%d", i.srcChainID)

	if config != nil {
		taskName = fmt.Sprintf("epoch_l2_revenue_%s", config.Name)
	}

	duration := epoch.EndTime.Sub(epoch.StartTime)
	avgGasPerBlock := float64(0)
	if epoch.BlockCount > 0 {
		avgGasPerBlock = float64(epoch.GasUsed) / float64(epoch.BlockCount)
	}

	networkName := "unknown"
	if config != nil {
		networkName = config.Name
	}

	slog.Info("Saving epoch revenue data",
		"epochNumber", epoch.EpochNumber,
		"proposer", epoch.PreconferAddress,
		"revenueETH", revenueETH.String(),
		"blockCount", epoch.BlockCount,
		"txCount", epoch.TransactionCount,
		"gasUsed", epoch.GasUsed,
		"avgGasPerBlock", avgGasPerBlock,
		"startBlock", epoch.StartBlockNumber,
		"endBlock", epoch.EndBlockNumber,
		"duration", duration.String(),
		"network", networkName)

	// Use a transaction to ensure data consistency
	tx := i.db.GormDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "failed to begin transaction")
	}
	defer tx.Rollback()

	// Insert or update the main revenue record
	query := `
		INSERT INTO time_series_data (task, value, date, fee_token_address, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		value = VALUES(value),
		updated_at = VALUES(updated_at)
	`

	err := tx.Exec(query,
		taskName,
		revenueETH.String(),
		date,
		epoch.PreconferAddress,
		epoch.EndTime,
		epoch.EndTime,
	).Error

	if err != nil {
		return errors.Wrap(err, "failed to save epoch revenue data")
	}

	// Also save additional metrics if needed
	metricsTaskName := fmt.Sprintf("epoch_metrics_%d", i.srcChainID)
	metricsQuery := `
		INSERT INTO time_series_data (task, value, date, fee_token_address, tier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		value = VALUES(value),
		updated_at = VALUES(updated_at)
	`

	// Save block count as a separate metric
	err = tx.Exec(metricsQuery,
		metricsTaskName,
		epoch.BlockCount,
		date,
		epoch.PreconferAddress,
		1, // tier 1 for block count
		epoch.EndTime,
		epoch.EndTime,
	).Error

	if err != nil {
		slog.Warn("Failed to save epoch metrics", "error", err)
		// Don't fail the whole operation for metrics
	}

	return tx.Commit().Error
}

func (i *Indexer) getCurrentEpochBlockCount() int64 {
	if i.currentEpoch == nil {
		return 0
	}
	return i.currentEpoch.BlockCount
}

func (i *Indexer) getCurrentEpochRevenue() *big.Int {
	if i.currentEpoch == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(i.currentEpoch.TotalL2Revenue)
}

func (i *Indexer) forceEndCurrentEpoch(ctx context.Context) error {
	return i.endCurrentEpoch(ctx)
}
