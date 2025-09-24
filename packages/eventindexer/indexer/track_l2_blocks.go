package indexer

import (
	"context"
	"time"

	"golang.org/x/exp/slog"
)

// startL2BlockMonitor starts monitoring L2 blocks for epoch tracking
func (i *Indexer) startL2BlockMonitor(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second) // Check every 2 seconds
	defer ticker.Stop()

	var lastProcessedBlock uint64 = 0

	// Get starting block number
	// Check if user provided a custom start block via --epochRevenueStartingBlock flag
	if i.epochRevenueStartingBlock != nil {
		lastProcessedBlock = *i.epochRevenueStartingBlock
		slog.Info("Using custom epoch revenue starting block from flag",
			"chainID", i.srcChainID,
			"startBlock", lastProcessedBlock)
	} else {
		// Use network-specific defaults
		switch i.srcChainID {
		case 167000: // Mainnet
			lastProcessedBlock = 1320745 // Aug 11, 2025 at 13:48:31 (preconf implementation)
		case 167009: // Hekla
			lastProcessedBlock = 1472749 // Jun 10, 2025 at 06:13:22 (preconfirmation implementation)
		case 167012: // Hoodi
			lastProcessedBlock = 0 // From genesis
		default:
			// For unknown networks, start from recent blocks
			latestBlock, err := i.ethClient.BlockNumber(ctx)
			if err != nil {
				slog.Error("Failed to get latest block number", "error", err)
				return
			}
			lastProcessedBlock = latestBlock - 100 // Start 100 blocks back for unknown networks
		}
	}

	slog.Info("Starting L2 block monitor for epoch tracking",
		"chainID", i.srcChainID,
		"startBlock", lastProcessedBlock)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := i.processNewL2Blocks(ctx, &lastProcessedBlock); err != nil {
				slog.Warn("Failed to process new L2 blocks", "error", err)
			}
		}
	}
}

// processNewL2Blocks processes any new L2 blocks since last check
func (i *Indexer) processNewL2Blocks(ctx context.Context, lastProcessedBlock *uint64) error {
	latestBlock, err := i.ethClient.BlockNumber(ctx)
	if err != nil {
		return err
	}

	// Process each new block
	for blockNum := *lastProcessedBlock + 1; blockNum <= latestBlock; blockNum++ {
		if err := i.trackL2BlockForEpoch(ctx, blockNum); err != nil {
			slog.Warn("Failed to track L2 block for epoch",
				"blockNumber", blockNum,
				"error", err)
			continue
		}

		*lastProcessedBlock = blockNum
	}

	return nil
}
