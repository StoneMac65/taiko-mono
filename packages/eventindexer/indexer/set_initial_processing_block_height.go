package indexer

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer"
)

func (i *Indexer) setInitialIndexingBlockByMode(
	ctx context.Context,
	mode SyncMode,
) error {
	var startingBlock uint64 = 0

	// Check if user provided a custom start block via --epochRevenueStartingBlock flag
	if i.epochRevenueStartingBlock != nil {
		startingBlock = *i.epochRevenueStartingBlock
		slog.Info("Using custom epoch revenue starting block from flag for main indexer",
			"chainID", i.srcChainID,
			"startBlock", startingBlock)
		i.latestIndexedBlockNumber = startingBlock
		return nil
	}

	// Use network-specific defaults (same as L2 block monitor)
	switch i.srcChainID {
	case 167000: // Mainnet
		startingBlock = 1320745 // Aug 11, 2025 at 13:48:31 (preconf implementation)
	case 167009: // Hekla
		startingBlock = 1472749 // Jun 10, 2025 at 06:13:22 (preconfirmation implementation)
	case 167012: // Hoodi
		startingBlock = 0 // From genesis
	default:
		// For unknown networks, check database or use L1 state
		// only check stateVars on L1, otherwise sync from 0
		if i.taikol1 != nil {
			slotA, _, err := i.taikol1.GetStateVariables(nil)
			if err != nil {
				// check v2
				slotA, _, err := i.taikol1V2.GetStateVariables(nil)
				if err != nil {
					// check v3
					stats1, err := i.taikoInbox.GetStats1(nil)
					if err != nil {
						return errors.Wrap(err, "i.taikoInbox.GetStats1")
					}

					startingBlock = stats1.GenesisHeight
				} else {
					startingBlock = slotA.GenesisHeight
				}
			} else {
				startingBlock = slotA.GenesisHeight
			}
		}

		switch mode {
		case Sync:
			// get most recently processed block height from the DB
			latest, err := i.eventRepo.FindLatestBlockID(ctx,
				i.srcChainID,
			)
			if err != nil {
				return errors.Wrap(err, "svc.eventRepo.FindLatestBlockID")
			}

			if latest != 0 {
				startingBlock = latest - 1
			}

		case Resync:
		default:
			return eventindexer.ErrInvalidMode
		}
	}

	slog.Info("startingBlock", "startingBlock", startingBlock)

	i.latestIndexedBlockNumber = startingBlock

	return nil
}
