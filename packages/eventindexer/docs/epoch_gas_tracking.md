# Epoch Revenue Tracking

Tracks L2 revenue per preconfer epoch in ETH.

## How It Works

- Monitors L2 blocks directly (every 2 seconds)
- New epoch: proposer changes OR >19 minutes (3 epochs)
- Revenue: 75% base fee + 100% priority fee per transaction
- Values stored and returned in ETH

## Configuration

### Network Defaults
- **Mainnet (167000)**: Block 1,320,745 (Aug 11, 2025)
- **Hekla (167009)**: Block 1,472,749 (Jun 10, 2025)
- **Hoodi (167012)**: Block 0 (genesis)

### Custom Starting Block
```bash
--epochRevenueStartingBlock=2590000
```
Overrides network defaults. Use for testing specific block ranges.

## Usage

### Start Indexer
```bash
# Use network defaults
./eventindexer indexer --rpcUrl="https://rpc.mainnet.taiko.xyz" --layer="L2"

# Custom starting block
./eventindexer indexer --epochRevenueStartingBlock=2590000 --rpcUrl="https://rpc.mainnet.taiko.xyz" --layer="L2"
```

### API Endpoints
```bash
# Get epoch revenue
GET /epoch-revenue?chain_id=167000

# Filter by proposer
GET /epoch-revenue?chain_id=167000&proposer=0x5F62d006...861C99990

# Date range
GET /epoch-revenue?chain_id=167000&start_date=2025-08-11&end_date=2025-08-22
```

**Response:**
```json
{
  "chart": [
    {
      "date": "2025-08-11",
      "value": "0.08064",
      "fee_token_address": "0x5F62d006...861C99990"
    }
  ]
}
```

Values are in ETH.
