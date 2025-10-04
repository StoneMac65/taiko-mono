package indexer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/shopspring/decimal"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer/pkg/repo"
	"golang.org/x/exp/slog"
)

// EpochData represents gas usage and revenue data for a specific epoch
type EpochData struct {
	EpochID        uint64
	TotalGasUsed   uint64
	TotalRevenue   *big.Int // Revenue in wei using 75% base fee + 100% tips
	BlockCount     int
	MinGas         uint64
	MaxGas         uint64
	FirstBlockID   uint64
	LastBlockID    uint64
	AvgGasPerBlock float64
}

// trackL2BlockForEpoch processes a single L2 block for epoch gas tracking
func (i *Indexer) trackL2BlockForEpoch(ctx context.Context, blockNumber uint64) error {
	// Get block data
	block, err := i.ethClient.BlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		return err
	}

	// Calculate epoch ID from L1 origin timestamp (12-second epochs)
	epochID := uint64(block.Time()) / 12

	// Get gas used in this block
	gasUsed := block.GasUsed()

	// Calculate block revenue using 75% base fee + 100% tips formula
	blockRevenue := i.calculateBlockRevenue(block)

	// Save or update epoch gas data
	return i.saveEpochGasData(ctx, epochID, blockNumber, gasUsed, blockRevenue)
}

// calculateBlockRevenue calculates block revenue using 75% base fee + 100% tips formula
func (i *Indexer) calculateBlockRevenue(block *types.Block) *big.Int {
	totalRevenue := big.NewInt(0)

	baseFee := block.BaseFee()
	if baseFee == nil {
		slog.Warn("Block has no base fee, skipping revenue calculation", "blockNumber", block.NumberU64())
		return totalRevenue
	}

	blockGasUsed := block.GasUsed()
	if blockGasUsed == 0 {
		return totalRevenue
	}

	// Calculate average tip per gas from all transactions
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
	totalRevenue = new(big.Int).Mul(proposerFeePerGas, big.NewInt(int64(blockGasUsed)))

	return totalRevenue
}

// saveEpochGasData saves epoch gas tracking data to database
func (i *Indexer) saveEpochGasData(ctx context.Context, epochID uint64, blockNumber uint64, gasUsed uint64, blockRevenue *big.Int) error {
	// Check if we have an epoch gas tracking repository
	epochRepo, err := i.getEpochGasTrackingRepo()
	if err != nil {
		return err
	}

	// Find existing epoch data
	existing, err := epochRepo.FindByEpochID(ctx, epochID, i.srcChainID)
	if err != nil {
		return err
	}

	var opts eventindexer.SaveEpochGasTrackingOpts
	if existing != nil {
		// Update existing epoch data
		opts = eventindexer.SaveEpochGasTrackingOpts{
			EpochID:      epochID,
			ChainID:      i.srcChainID,
			TotalGasUsed: existing.TotalGasUsed + gasUsed,
			BlockCount:   existing.BlockCount + 1,
			MinGas:       min(existing.MinGas, gasUsed),
			MaxGas:       max(existing.MaxGas, gasUsed),
			FirstBlockID: existing.FirstBlockID, // Keep original first block
			LastBlockID:  blockNumber,           // Update to latest block
		}
		// Calculate new average
		opts.AvgGasPerBlock = float64(opts.TotalGasUsed) / float64(opts.BlockCount)
	} else {
		// Create new epoch data
		opts = eventindexer.SaveEpochGasTrackingOpts{
			EpochID:        epochID,
			ChainID:        i.srcChainID,
			TotalGasUsed:   gasUsed,
			BlockCount:     1,
			MinGas:         gasUsed,
			MaxGas:         gasUsed,
			FirstBlockID:   blockNumber,
			LastBlockID:    blockNumber,
			AvgGasPerBlock: float64(gasUsed),
		}
	}

	_, err = epochRepo.Save(ctx, opts)
	if err != nil {
		slog.Error("Failed to save epoch gas data", "epochID", epochID, "error", err)
		return err
	}

	// Convert revenue to ETH for logging
	weiPerEth := decimal.NewFromBigInt(big.NewInt(1000000000000000000), 0) // 10^18
	revenueWei := decimal.NewFromBigInt(blockRevenue, 0)
	revenueETH := revenueWei.Div(weiPerEth)

	slog.Debug("Saved epoch gas data",
		"epochID", epochID,
		"blockNumber", blockNumber,
		"gasUsed", gasUsed,
		"blockRevenueETH", revenueETH.String(),
		"totalGas", opts.TotalGasUsed,
		"blockCount", opts.BlockCount)

	return nil
}

// getEpochGasTrackingRepo returns the epoch gas tracking repository
func (i *Indexer) getEpochGasTrackingRepo() (eventindexer.EpochGasTrackingRepository, error) {
	return repo.NewEpochGasTrackingRepository(i.db)
}

// Helper functions
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
