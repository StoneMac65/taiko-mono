package indexer

import (
	"context"
	"math/big"

	"github.com/taikoxyz/taiko-mono/packages/eventindexer"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer/pkg/repo"
	"golang.org/x/exp/slog"
)

// EpochData represents gas usage data for a specific epoch (simplified)
type EpochData struct {
	EpochID        uint64
	TotalGasUsed   uint64
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

	// Save or update epoch gas data
	return i.saveEpochGasData(ctx, epochID, blockNumber, gasUsed)
}

// saveEpochGasData saves epoch gas tracking data to database
func (i *Indexer) saveEpochGasData(ctx context.Context, epochID uint64, blockNumber uint64, gasUsed uint64) error {
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

	slog.Debug("Saved epoch gas data",
		"epochID", epochID,
		"blockNumber", blockNumber,
		"gasUsed", gasUsed,
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
