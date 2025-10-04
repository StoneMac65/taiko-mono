package eventindexer

import (
	"context"
	"time"
)

// EpochGasTracking represents epoch-based gas usage data
type EpochGasTracking struct {
	ID              int       `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	EpochID         uint64    `json:"epochId" gorm:"column:epoch_id;not null;index"`
	ChainID         uint64    `json:"chainId" gorm:"column:chain_id;not null;index"`
	TotalGasUsed    uint64    `json:"totalGasUsed" gorm:"column:total_gas_used;not null;default:0"`
	BlockCount      int       `json:"blockCount" gorm:"column:block_count;not null;default:0"`
	AvgGasPerBlock  float64   `json:"avgGasPerBlock" gorm:"column:avg_gas_per_block;type:decimal(20,2);not null;default:0.00"`
	MinGas          uint64    `json:"minGas" gorm:"column:min_gas;not null;default:0"`
	MaxGas          uint64    `json:"maxGas" gorm:"column:max_gas;not null;default:0"`
	FirstBlockID    uint64    `json:"firstBlockId" gorm:"column:first_block_id;not null"`
	LastBlockID     uint64    `json:"lastBlockId" gorm:"column:last_block_id;not null"`
	CreatedAt       time.Time `json:"createdAt" gorm:"column:created_at;default:CURRENT_TIMESTAMP"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"column:updated_at;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
}

// TableName returns the table name for GORM
func (EpochGasTracking) TableName() string {
	return "epoch_gas_tracking"
}

// EpochGasTrackingRepository defines the interface for epoch gas tracking operations
type EpochGasTrackingRepository interface {
	// Save creates or updates epoch gas tracking data
	Save(ctx context.Context, opts SaveEpochGasTrackingOpts) (*EpochGasTracking, error)
	
	// FindByEpochID finds epoch gas tracking data by epoch ID and chain ID
	FindByEpochID(ctx context.Context, epochID uint64, chainID uint64) (*EpochGasTracking, error)
	
	// FindByEpochRange finds epoch gas tracking data within a range of epochs
	FindByEpochRange(ctx context.Context, opts FindEpochGasTrackingOpts) ([]*EpochGasTracking, error)
	
	// GetTotalCount returns the total count of epoch gas tracking records for a chain
	GetTotalCount(ctx context.Context, chainID uint64) (int64, error)
}

// SaveEpochGasTrackingOpts represents options for saving epoch gas tracking data
type SaveEpochGasTrackingOpts struct {
	EpochID        uint64
	ChainID        uint64
	TotalGasUsed   uint64
	BlockCount     int
	AvgGasPerBlock float64
	MinGas         uint64
	MaxGas         uint64
	FirstBlockID   uint64
	LastBlockID    uint64
}

// FindEpochGasTrackingOpts represents options for finding epoch gas tracking data
type FindEpochGasTrackingOpts struct {
	ChainID     uint64
	StartEpoch  *uint64
	EndEpoch    *uint64
	EpochID     *uint64
	Limit       int
	Offset      int
}

// EpochGasResponse represents the API response for epoch gas data
type EpochGasResponse struct {
	Data       []*EpochGasTracking `json:"data"`
	TotalCount int64               `json:"totalCount"`
	Page       int                 `json:"page"`
	Size       int                 `json:"size"`
}
