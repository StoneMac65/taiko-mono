package blocksinserter

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/encoding"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/metadata"
	pacayaBindings "github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/pacaya"
	anchorTxConstructor "github.com/taikoxyz/taiko-mono/packages/taiko-client/driver/anchor_tx_constructor"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/driver/chain_syncer/beaconsync"
	preconfblocks "github.com/taikoxyz/taiko-mono/packages/taiko-client/driver/preconf_blocks"
	txListDecompressor "github.com/taikoxyz/taiko-mono/packages/taiko-client/driver/txlist_decompressor"
	txlistFetcher "github.com/taikoxyz/taiko-mono/packages/taiko-client/driver/txlist_fetcher"
	eventIterator "github.com/taikoxyz/taiko-mono/packages/taiko-client/pkg/chain_iterator/event_iterator"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/pkg/rpc"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/pkg/utils"
)

// BlocksInserterOntake is responsible for inserting Ontake blocks to the L2 execution engine.
type BlocksInserterPacaya struct {
	rpc                *rpc.Client
	progressTracker    *beaconsync.SyncProgressTracker
	blobDatasource     *rpc.BlobDataSource
	txListDecompressor *txListDecompressor.TxListDecompressor   // Transactions list decompressor
	anchorConstructor  *anchorTxConstructor.AnchorTxConstructor // TaikoL2.anchor transactions constructor
	calldataFetcher    txlistFetcher.TxListFetcher
	blobFetcher        txlistFetcher.TxListFetcher
	mutex              sync.Mutex
}

// NewBlocksInserterOntake creates a new BlocksInserterOntake instance.
func NewBlocksInserterPacaya(
	rpc *rpc.Client,
	progressTracker *beaconsync.SyncProgressTracker,
	blobDatasource *rpc.BlobDataSource,
	txListDecompressor *txListDecompressor.TxListDecompressor,
	anchorConstructor *anchorTxConstructor.AnchorTxConstructor,
	calldataFetcher txlistFetcher.TxListFetcher,
	blobFetcher txlistFetcher.TxListFetcher,
) *BlocksInserterPacaya {
	return &BlocksInserterPacaya{
		rpc:                rpc,
		progressTracker:    progressTracker,
		blobDatasource:     blobDatasource,
		txListDecompressor: txListDecompressor,
		anchorConstructor:  anchorConstructor,
		calldataFetcher:    calldataFetcher,
		blobFetcher:        blobFetcher,
	}
}

// InsertBlocks inserts new Pacaya blocks to the L2 execution engine.
func (i *BlocksInserterPacaya) InsertBlocks(
	ctx context.Context,
	metadata metadata.TaikoProposalMetaData,
	proposingTx *types.Transaction,
	endIter eventIterator.EndBlockProposedEventIterFunc,
) (err error) {
	if !metadata.IsPacaya() {
		return fmt.Errorf("metadata is not for Pacaya fork")
	}
	i.mutex.Lock()
	defer i.mutex.Unlock()

	var (
		meta        = metadata.Pacaya()
		txListBytes []byte
	)

	// Fetch transactions list.
	if len(meta.GetBlobHashes()) != 0 {
		if txListBytes, err = i.blobFetcher.FetchPacaya(ctx, proposingTx, meta); err != nil {
			return fmt.Errorf("failed to fetch tx list from blob: %w", err)
		}
	} else {
		if txListBytes, err = i.calldataFetcher.FetchPacaya(ctx, proposingTx, meta); err != nil {
			return fmt.Errorf("failed to fetch tx list from calldata: %w", err)
		}
	}

	var (
		allTxs = i.txListDecompressor.TryDecompress(
			i.rpc.L2.ChainID,
			txListBytes,
			len(meta.GetBlobHashes()) != 0,
			true,
		)
		parent          *types.Header
		lastPayloadData *engine.ExecutableData
		txListCursor    = 0
	)

	for j, blockInfo := range meta.GetBlocks() {
		// Fetch the L2 parent block, if the node is just finished a P2P sync, we simply use the tracker's
		// last synced verified block as the parent, otherwise, we fetch the parent block from L2 EE.
		if i.progressTracker.Triggered() {
			// Already synced through beacon sync, just skip this event.
			if new(big.Int).SetUint64(meta.GetLastBlockID()).Cmp(i.progressTracker.LastSyncedBlockID()) <= 0 {
				return nil
			}

			parent, err = i.rpc.L2.HeaderByHash(ctx, i.progressTracker.LastSyncedBlockHash())
		} else {
			var parentNumber *big.Int
			if lastPayloadData == nil {
				if meta.GetBatchID().Uint64() == i.rpc.PacayaClients.ForkHeight {
					parentNumber = new(big.Int).SetUint64(meta.GetBatchID().Uint64() - 1)
				} else {
					lastBatch, err := i.rpc.GetBatchByID(ctx, new(big.Int).SetUint64(meta.GetBatchID().Uint64()-1))
					if err != nil {
						return fmt.Errorf("failed to fetch last batch (%d): %w", meta.GetBatchID().Uint64()-1, err)
					}
					parentNumber = new(big.Int).SetUint64(lastBatch.LastBlockId)
				}
			} else {
				parentNumber = new(big.Int).SetUint64(lastPayloadData.Number)
			}

			parent, err = i.rpc.L2ParentByCurrentBlockID(ctx, new(big.Int).Add(parentNumber, common.Big1))
		}
		if err != nil {
			return fmt.Errorf("failed to fetch L2 parent block: %w", err)
		}

		log.Debug(
			"Parent block",
			"blockID", parent.Number,
			"hash", parent.Hash(),
			"beaconSyncTriggered", i.progressTracker.Triggered(),
		)

		blockID := new(big.Int).SetUint64(parent.Number.Uint64() + 1)
		difficulty, err := encoding.CalculatePacayaDifficulty(blockID)
		if err != nil {
			return fmt.Errorf("failed to calculate difficulty: %w", err)
		}
		timestamp := meta.GetLastBlockTimestamp()
		for i := len(meta.GetBlocks()) - 1; i >= 0; i-- {
			timestamp = timestamp - uint64(meta.GetBlocks()[i].TimeShift)
		}

		baseFee, err := i.rpc.CalculateBaseFee(
			ctx,
			parent,
			true,
			meta.GetBaseFeeConfig(),
			timestamp,
		)
		if err != nil {
			return err
		}

		log.Info(
			"L2 baseFee",
			"blockID", blockID,
			"baseFee", utils.WeiToGWei(baseFee),
			"parentGasUsed", parent.GasUsed,
			"batchID", meta.GetBatchID(),
			"indexInBatch", j,
		)

		// Assemble a TaikoAnchor.anchorV3 transaction
		anchorBlockHeader, err := i.rpc.L1.HeaderByHash(ctx, meta.GetAnchorBlockHash())
		if err != nil {
			return fmt.Errorf("failed to fetch anchor block: %w", err)
		}
		anchorTx, err := i.anchorConstructor.AssembleAnchorV3Tx(
			ctx,
			new(big.Int).SetUint64(meta.GetAnchorBlockID()),
			anchorBlockHeader.Root,
			meta.GetAnchorInput(),
			parent.GasUsed,
			meta.GetBaseFeeConfig(),
			meta.GetSignalSlots(),
			new(big.Int).Add(parent.Number, common.Big1),
			baseFee,
		)
		if err != nil {
			return fmt.Errorf("failed to create TaikoAnchor.anchorV3 transaction: %w", err)
		}

		// Get transactions in the block.
		txs := types.Transactions{}
		if txListCursor+int(blockInfo.NumTransactions) <= len(allTxs) {
			txs = allTxs[txListCursor : txListCursor+int(blockInfo.NumTransactions)]
		}

		// Decompress the transactions list and try to insert a new head block to L2 EE.
		if lastPayloadData, err = createPayloadAndSetHead(
			ctx,
			i.rpc,
			&createPayloadAndSetHeadMetaData{
				createExecutionPayloadsMetaData: &createExecutionPayloadsMetaData{
					BlockID:               blockID,
					ExtraData:             meta.GetExtraData(),
					SuggestedFeeRecipient: meta.GetCoinbase(),
					GasLimit:              uint64(meta.GetGasLimit()),
					Difficulty:            common.BytesToHash(difficulty),
					Timestamp:             timestamp,
					ParentHash:            parent.Hash(),
					L1Origin: &rawdb.L1Origin{
						BlockID:       blockID,
						L2BlockHash:   common.Hash{}, // Will be set by taiko-geth.
						L1BlockHeight: meta.GetRawBlockHeight(),
						L1BlockHash:   meta.GetRawBlockHash(),
					},
					Txs:         txs,
					Withdrawals: make([]*types.Withdrawal, 0),
					BaseFee:     baseFee,
				},
				AnchorBlockID:   new(big.Int).SetUint64(meta.GetAnchorBlockID()),
				AnchorBlockHash: meta.GetAnchorBlockHash(),
				BaseFeeConfig:   meta.GetBaseFeeConfig(),
				Parent:          parent,
			},
			anchorTx,
		); err != nil {
			return fmt.Errorf("failed to insert new head to L2 execution engine: %w", err)
		}

		log.Debug("Payload data", "hash", lastPayloadData.BlockHash, "txs", len(lastPayloadData.Transactions))

		log.Info(
			"🔗 New L2 block inserted",
			"blockID", blockID,
			"hash", lastPayloadData.BlockHash,
			"transactions", len(lastPayloadData.Transactions),
			"timestamp", lastPayloadData.Timestamp,
			"baseFee", utils.WeiToGWei(lastPayloadData.BaseFeePerGas),
			"withdrawals", len(lastPayloadData.Withdrawals),
			"batchID", meta.GetBatchID(),
			"indexInBatch", j,
		)

		txListCursor += int(blockInfo.NumTransactions)
	}

	return nil
}

// InsertPreconfBlockFromTransactionsBatch inserts a preconf block from transactions batch.
func (i *BlocksInserterPacaya) InsertPreconfBlockFromTransactionsBatch(
	ctx context.Context,
	executableData *preconfblocks.ExecutableData,
	anchorBlockID uint64,
	anchorStateRoot common.Hash,
	anchorInput [32]byte,
	signalSlots [][32]byte,
	baseFeeConfig *pacayaBindings.LibSharedDataBaseFeeConfig,
) (*types.Header, error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	parentHeader, err := i.rpc.L2.HeaderByHash(ctx, executableData.ParentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent block: %w", err)
	}

	if parentHeader.Number.Uint64()+1 != executableData.Number {
		return nil, fmt.Errorf("invalid parent hash %s", executableData.ParentHash)
	}

	baseFee, err := i.rpc.CalculateBaseFee(
		ctx,
		parentHeader,
		true,
		baseFeeConfig,
		executableData.Timestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate base fee: %w", err)
	}

	log.Info(
		"L2 baseFee for the next preconfirmation block",
		"blockID", executableData.Number,
		"baseFee", utils.WeiToGWei(baseFee),
		"parentGasUsed", parentHeader.GasUsed,
	)
	anchorBlockHeader, err := i.rpc.L1.HeaderByNumber(ctx, new(big.Int).SetUint64(anchorBlockID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch anchor block: %w", err)
	}
	if anchorBlockHeader.Root != anchorStateRoot {
		return nil, fmt.Errorf("invalid anchor state root %s", anchorStateRoot)
	}

	// Assemble a TaikoAnchor.anchorV3 transaction
	anchorTx, err := i.anchorConstructor.AssembleAnchorV3Tx(
		ctx,
		new(big.Int).SetUint64(anchorBlockID),
		anchorStateRoot,
		anchorInput,
		parentHeader.GasUsed,
		baseFeeConfig,
		signalSlots,
		new(big.Int).Add(parentHeader.Number, common.Big1),
		baseFee,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create TaikoAnchor.anchorV3 transaction: %w", err)
	}
	difficulty, err := encoding.CalculatePacayaDifficulty(new(big.Int).SetUint64(executableData.Number))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate difficulty: %w", err)
	}
	var (
		extraData = encoding.EncodeBaseFeeConfig(baseFeeConfig)
		txs       = i.txListDecompressor.TryDecompress(i.rpc.L2.ChainID, executableData.Transactions, true, true)
	)

	payloadData, err := createPayloadAndSetHead(
		ctx,
		i.rpc,
		&createPayloadAndSetHeadMetaData{
			createExecutionPayloadsMetaData: &createExecutionPayloadsMetaData{
				BlockID:               new(big.Int).SetUint64(executableData.Number),
				ExtraData:             extraData[:],
				SuggestedFeeRecipient: executableData.FeeRecipient,
				GasLimit:              executableData.GasLimit,
				Difficulty:            common.BytesToHash(difficulty),
				Timestamp:             executableData.Timestamp,
				ParentHash:            executableData.ParentHash,
				L1Origin: &rawdb.L1Origin{
					BlockID:       new(big.Int).SetUint64(executableData.Number),
					L2BlockHash:   common.Hash{}, // Will be set by taiko-geth.
					L1BlockHeight: nil,
					L1BlockHash:   common.Hash{},
				},
				Txs:         txs,
				Withdrawals: make([]*types.Withdrawal, 0),
				BaseFee:     baseFee,
			},
			AnchorBlockID:   new(big.Int).SetUint64(anchorBlockID),
			AnchorBlockHash: anchorBlockHeader.Hash(),
			BaseFeeConfig:   baseFeeConfig,
			Parent:          parentHeader,
		},
		anchorTx,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert new preconfirmation head to L2 execution engine: %w", err)
	}

	log.Info(
		"⏰ New preconfirmation L2 block inserted",
		"blockID", executableData.Number,
		"hash", payloadData.BlockHash,
		"transactions", txs.Len(),
		"baseFee", utils.WeiToGWei(payloadData.BaseFeePerGas),
		"withdrawals", len(payloadData.Withdrawals),
	)

	return i.rpc.L2.HeaderByHash(ctx, payloadData.BlockHash)
}

// RemovePreconfBlocks removes preconf blocks from the L2 execution engine.
func (i *BlocksInserterPacaya) RemovePreconfBlocks(ctx context.Context, newLastBlockID uint64) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	newHead, err := i.rpc.L2.HeaderByNumber(ctx, new(big.Int).SetUint64(newLastBlockID))
	if err != nil {
		return err
	}

	fc := &engine.ForkchoiceStateV1{HeadBlockHash: newHead.Hash()}
	fcRes, err := i.rpc.L2Engine.ForkchoiceUpdate(ctx, fc, nil)
	if err != nil {
		return err
	}
	if fcRes.PayloadStatus.Status != engine.VALID {
		return fmt.Errorf("unexpected ForkchoiceUpdate response status: %s", fcRes.PayloadStatus.Status)
	}

	return nil
}
