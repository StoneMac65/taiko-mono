package eventindexer

import (
	"time"

	"github.com/shopspring/decimal"
)

type TimeSeriesData struct {
	ID              int
	Task            string
	Value           decimal.NullDecimal
	Date            string
	FeeTokenAddress string `gorm:"column:fee_token_address"`
	Tier            *int   `gorm:"column:tier"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
