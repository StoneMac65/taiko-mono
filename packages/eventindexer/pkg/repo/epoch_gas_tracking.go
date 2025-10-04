package repo

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/taikoxyz/taiko-mono/packages/eventindexer"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer/pkg/db"
)

type EpochGasTrackingRepository struct {
	db db.DB
}

// NewEpochGasTrackingRepository creates a new epoch gas tracking repository
func NewEpochGasTrackingRepository(database db.DB) (eventindexer.EpochGasTrackingRepository, error) {
	if database == nil {
		return nil, db.ErrNoDB
	}

	return &EpochGasTrackingRepository{
		db: database,
	}, nil
}

// Save creates or updates epoch gas tracking data
func (r *EpochGasTrackingRepository) Save(ctx context.Context, opts eventindexer.SaveEpochGasTrackingOpts) (*eventindexer.EpochGasTracking, error) {
	epochGas := &eventindexer.EpochGasTracking{
		EpochID:        opts.EpochID,
		ChainID:        opts.ChainID,
		TotalGasUsed:   opts.TotalGasUsed,
		BlockCount:     opts.BlockCount,
		AvgGasPerBlock: opts.AvgGasPerBlock,
		MinGas:         opts.MinGas,
		MaxGas:         opts.MaxGas,
		FirstBlockID:   opts.FirstBlockID,
		LastBlockID:    opts.LastBlockID,
	}

	// Use ON DUPLICATE KEY UPDATE for MySQL
	result := r.db.GormDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "epoch_id"}, {Name: "chain_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"total_gas_used", "block_count", "avg_gas_per_block", "min_gas", "max_gas", "first_block_id", "last_block_id", "updated_at"}),
		}).
		Create(epochGas)

	if result.Error != nil {
		return nil, errors.Wrap(result.Error, "failed to save epoch gas tracking")
	}

	return epochGas, nil
}

// FindByEpochID finds epoch gas tracking data by epoch ID and chain ID
func (r *EpochGasTrackingRepository) FindByEpochID(ctx context.Context, epochID uint64, chainID uint64) (*eventindexer.EpochGasTracking, error) {
	var epochGas eventindexer.EpochGasTracking

	result := r.db.GormDB().WithContext(ctx).
		Where("epoch_id = ? AND chain_id = ?", epochID, chainID).
		First(&epochGas)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(result.Error, "failed to find epoch gas tracking by epoch ID")
	}

	return &epochGas, nil
}

// FindByEpochRange finds epoch gas tracking data within a range of epochs
func (r *EpochGasTrackingRepository) FindByEpochRange(ctx context.Context, opts eventindexer.FindEpochGasTrackingOpts) ([]*eventindexer.EpochGasTracking, error) {
	var epochGasData []*eventindexer.EpochGasTracking

	query := r.db.GormDB().WithContext(ctx).
		Where("chain_id = ?", opts.ChainID)

	// Add epoch filters
	if opts.EpochID != nil {
		query = query.Where("epoch_id = ?", *opts.EpochID)
	} else {
		if opts.StartEpoch != nil {
			query = query.Where("epoch_id >= ?", *opts.StartEpoch)
		}
		if opts.EndEpoch != nil {
			query = query.Where("epoch_id <= ?", *opts.EndEpoch)
		}
	}

	// Add pagination
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	// Order by epoch ID
	query = query.Order("epoch_id ASC")

	result := query.Find(&epochGasData)
	if result.Error != nil {
		return nil, errors.Wrap(result.Error, "failed to find epoch gas tracking by range")
	}

	return epochGasData, nil
}

// GetTotalCount returns the total count of epoch gas tracking records for a chain
func (r *EpochGasTrackingRepository) GetTotalCount(ctx context.Context, chainID uint64) (int64, error) {
	var count int64

	result := r.db.GormDB().WithContext(ctx).
		Model(&eventindexer.EpochGasTracking{}).
		Where("chain_id = ?", chainID).
		Count(&count)

	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "failed to get total count of epoch gas tracking")
	}

	return count, nil
}
