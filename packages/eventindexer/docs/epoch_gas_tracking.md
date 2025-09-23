# Epoch Revenue Tracking

Tracks L2 revenue per preconfer epoch in ETH.

## How It Works

- Monitors L2 blocks directly (every 2 seconds)
- New epoch: different proposer OR >6.4 minutes
- Revenue: 75% base fee + 100% priority fee per transaction
- Values stored and returned in ETH

## Networks

- **Mainnet (167000)**: Starts block 1,320,745
- **Hekla (167009)**: Starts block 1,000,000 (approximate)
- **Hoodi (167012)**: Starts from genesis (block 0)

## API Usage

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
