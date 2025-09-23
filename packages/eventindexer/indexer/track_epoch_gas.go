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

type EpochTracker struct {
	currentEpoch *EpochData
	indexer      *Indexer
}

type EpochData struct {
	EpochNumber      uint64
	PreconferAddress string
	StartTime        time.Time
	EndTime          time.Time
	TotalL2Revenue   *big.Int
	BlockCount       int64
	BatchCount       int64
	StartBlockNumber uint64
	EndBlockNumber   uint64
}

func NewEpochTracker(indexer *Indexer) *EpochTracker {
	return &EpochTracker{
		indexer: indexer,
	}
}

func (i *Indexer) trackL2BlockForEpoch(ctx context.Context, blockNumber uint64) error {
	if !i.isPreconfirmationActiveForBlock(blockNumber) {
		return nil
	}

	block, err := i.ethClient.BlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		return errors.Wrap(err, "failed to fetch L2 block")
	}

	proposer := block.Coinbase()
	blockTime := time.Unix(int64(block.Time()), 0)

	if i.isNewEpochForBlock(proposer, blockTime) {
		if err := i.endCurrentEpoch(ctx); err != nil {
			slog.Warn("Failed to end previous epoch", "error", err)
		}

		if err := i.startNewEpochForBlock(ctx, proposer, blockTime, blockNumber); err != nil {
			return errors.Wrap(err, "failed to start new epoch")
		}
	}

	if err := i.addL2BlockToEpoch(ctx, block); err != nil {
		return errors.Wrap(err, "failed to add block to epoch")
	}

	return nil
}

func (i *Indexer) isNewEpochForBlock(proposer common.Address, blockTime time.Time) bool {
	if i.currentEpoch == nil {
		return true
	}

	// Check if we're in a different epoch based on timestamp
	currentEpochNumber := i.calculateEpochNumberForTime(blockTime)
	if currentEpochNumber != i.currentEpoch.EpochNumber {
		return true
	}

	return false
}

func (i *Indexer) startNewEpochForBlock(ctx context.Context, proposer common.Address, blockTime time.Time, blockNumber uint64) error {
	epochNumber := i.calculateEpochNumberForTime(blockTime)

	i.currentEpoch = &EpochData{
		EpochNumber:      epochNumber,
		PreconferAddress: proposer.Hex(),
		StartTime:        blockTime,
		TotalL2Revenue:   big.NewInt(0),
		BlockCount:       0,
		BatchCount:       0,
		StartBlockNumber: blockNumber,
		EndBlockNumber:   blockNumber,
	}

	return nil
}

func (i *Indexer) calculateEpochNumberForTime(blockTime time.Time) uint64 {
	genesisTimestamp := i.getGenesisTimestamp()
	if genesisTimestamp == 0 {
		return 0
	}

	currentTimestamp := uint64(blockTime.Unix())
	if currentTimestamp < genesisTimestamp {
		return 0
	}

	return (currentTimestamp - genesisTimestamp) / 384
}

func (i *Indexer) getGenesisTimestamp() uint64 {
	switch i.srcChainID {
	case 1:
		return 1606824023
	case 17000:
		return 1695902400
	case 167012: // Hoodi
		return 0 // Genesis block timestamp is 0
	default:
		return 0
	}
}

func (i *Indexer) isPreconfirmationActiveForBlock(blockNumber uint64) bool {
	var preconfStartBlock uint64

	switch i.srcChainID {
	case 167000: // Mainnet
		preconfStartBlock = 1320745 // Exact start: Aug 11, 2025 at 13:48:31
	case 167009: // Hekla
		preconfStartBlock = 1000000 // Approximate (exact date unknown)
	case 167012: // Hoodi
		preconfStartBlock = 0 // From genesis
	default:
		preconfStartBlock = 0
	}

	return blockNumber >= preconfStartBlock
}

func (i *Indexer) addL2BlockToEpoch(ctx context.Context, block *types.Block) error {
	if i.currentEpoch == nil {
		return errors.New("no current epoch to add block to")
	}

	blockRevenue := i.calculateBlockRevenue(block)

	// Convert to ETH for logging
	weiPerEth := decimal.NewFromBigInt(big.NewInt(1000000000000000000), 0)
	revenueWei := decimal.NewFromBigInt(blockRevenue, 0)
	revenueETH := revenueWei.Div(weiPerEth)

	slog.Debug("Adding L2 block to epoch",
		"blockNumber", block.NumberU64(),
		"proposer", block.Coinbase().Hex(),
		"txCount", len(block.Transactions()),
		"blockRevenueETH", revenueETH.String(),
		"epochNumber", i.currentEpoch.EpochNumber)

	i.currentEpoch.TotalL2Revenue.Add(i.currentEpoch.TotalL2Revenue, blockRevenue)
	i.currentEpoch.BlockCount++
	i.currentEpoch.EndBlockNumber = block.NumberU64()

	return nil
}

func (i *Indexer) calculateBlockRevenue(block *types.Block) *big.Int {
	totalRevenue := big.NewInt(0)
	baseFee := block.BaseFee()
	if baseFee == nil {
		slog.Warn("Block has no base fee, skipping revenue calculation", "blockNumber", block.NumberU64())
		return totalRevenue
	}

	for _, tx := range block.Transactions() {
		receipt, err := i.ethClient.TransactionReceipt(context.Background(), tx.Hash())
		if err != nil {
			slog.Warn("Failed to get transaction receipt, skipping", "txHash", tx.Hash().Hex(), "error", err)
			continue
		}

		gasUsed := big.NewInt(int64(receipt.GasUsed))
		var proposerRevenue *big.Int

		if tx.Type() == 2 && tx.GasFeeCap() != nil && tx.GasTipCap() != nil {
			// EIP-1559: 75% base fee + 100% tip
			maxFeeMinusBase := new(big.Int).Sub(tx.GasFeeCap(), baseFee)
			priorityFee := tx.GasTipCap()
			if maxFeeMinusBase.Cmp(priorityFee) < 0 {
				priorityFee = maxFeeMinusBase
			}

			proposerBaseFee := new(big.Int).Mul(baseFee, big.NewInt(75))
			proposerBaseFee.Div(proposerBaseFee, big.NewInt(100))

			proposerFeePerGas := new(big.Int).Add(proposerBaseFee, priorityFee)
			proposerRevenue = new(big.Int).Mul(gasUsed, proposerFeePerGas)
		} else {
			// Legacy: 75% of total fee
			gasPrice := tx.GasPrice()
			if gasPrice == nil {
				slog.Warn("Transaction has no gas price, skipping", "txHash", tx.Hash().Hex())
				continue
			}
			totalFee := new(big.Int).Mul(gasUsed, gasPrice)
			proposerRevenue = new(big.Int).Mul(totalFee, big.NewInt(75))
			proposerRevenue.Div(proposerRevenue, big.NewInt(100))
		}

		totalRevenue.Add(totalRevenue, proposerRevenue)
	}

	return totalRevenue
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

func (i *Indexer) saveEpochGasData(ctx context.Context, epoch *EpochData) error {
	// Convert wei to ETH (divide by 10^18)
	weiPerEth := decimal.NewFromBigInt(big.NewInt(1000000000000000000), 0) // 10^18
	revenueWei := decimal.NewFromBigInt(epoch.TotalL2Revenue, 0)
	revenueETH := revenueWei.Div(weiPerEth)

	date := epoch.EndTime.Format("2006-01-02")
	taskName := fmt.Sprintf("epoch_l2_revenue_%d", i.srcChainID)

	slog.Info("Saving epoch revenue data",
		"epochNumber", epoch.EpochNumber,
		"proposer", epoch.PreconferAddress,
		"revenueETH", revenueETH.String(),
		"blockCount", epoch.BlockCount,
		"startBlock", epoch.StartBlockNumber,
		"endBlock", epoch.EndBlockNumber,
		"duration", epoch.EndTime.Sub(epoch.StartTime).String())

	query := `
		INSERT INTO time_series_data (task, value, date, fee_token_address, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		value = value + VALUES(value),
		updated_at = VALUES(updated_at)
	`

	err := i.db.GormDB().WithContext(ctx).Exec(query,
		taskName,
		revenueETH.String(), // Now in ETH, not wei
		date,
		epoch.PreconferAddress,
		epoch.EndTime,
		epoch.EndTime,
	).Error

	if err != nil {
		return errors.Wrap(err, "failed to save epoch revenue data")
	}

	return nil
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
