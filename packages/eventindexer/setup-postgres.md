# PostgreSQL Setup for Epoch Tracking Testing

## Prerequisites

The eventindexer currently uses MySQL by default. To use PostgreSQL, we need to:
1. Add PostgreSQL support to the codebase
2. Convert MySQL migrations to PostgreSQL
3. Set up a fresh PostgreSQL database

## Step 1: Install PostgreSQL

### Windows
```powershell
# Using Chocolatey
choco install postgresql

# Or download installer from:
# https://www.postgresql.org/download/windows/

# Start PostgreSQL service
net start postgresql-x64-15
```

### macOS
```bash
# Using Homebrew
brew install postgresql@15
brew services start postgresql@15

# Add to PATH
echo 'export PATH="/opt/homebrew/opt/postgresql@15/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### Linux (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

## Step 2: Create Fresh Test Database

```bash
# Connect as postgres user
sudo -u postgres psql

# Or on Windows (after installation):
psql -U postgres
```

```sql
-- Create fresh test database
CREATE DATABASE eventindexer_test;

-- Create test user with password
CREATE USER taiko_test WITH PASSWORD 'testpass123';

-- Grant all privileges on test database
GRANT ALL PRIVILEGES ON DATABASE eventindexer_test TO taiko_test;

-- Connect to the new database
\c eventindexer_test

-- Grant schema privileges (PostgreSQL 15+)
GRANT ALL ON SCHEMA public TO taiko_test;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO taiko_test;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO taiko_test;

-- Exit psql
\q
```

## Step 3: Convert Migrations to PostgreSQL

The existing migrations are MySQL-specific. We need to convert them to PostgreSQL syntax.

### Key Differences:
- `AUTO_INCREMENT` → `SERIAL` or `BIGSERIAL`
- `DATETIME` → `TIMESTAMP`
- `JSON` → `JSONB` (better performance in PostgreSQL)
- Index syntax differences
- No `charset=utf8mb4` in connection string

### Create PostgreSQL Migrations Directory
```bash
cd packages/eventindexer
mkdir -p migrations-postgres
```

## Step 4: Install Goose Migration Tool

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

## Step 5: PostgreSQL Connection String Format

```
postgresql://username:password@host:port/database?sslmode=disable
```

Example:
```
postgresql://taiko_test:testpass123@localhost:5432/eventindexer_test?sslmode=disable
```

## Step 6: Run Migrations (After Conversion)

```bash
cd packages/eventindexer/migrations-postgres

# Check migration status
goose postgres "postgresql://taiko_test:testpass123@localhost:5432/eventindexer_test?sslmode=disable" status

# Run all migrations
goose postgres "postgresql://taiko_test:testpass123@localhost:5432/eventindexer_test?sslmode=disable" up
```

## Step 7: Update Code to Support PostgreSQL

We need to modify the following files:
1. `packages/eventindexer/api/config.go` - Add PostgreSQL driver support
2. `packages/eventindexer/indexer/config.go` - Add PostgreSQL driver support
3. `packages/eventindexer/pkg/db/db.go` - Update DSN format for PostgreSQL
4. Add a database type flag to choose between MySQL and PostgreSQL

## Step 8: Start API Server with PostgreSQL

```bash
cd packages/eventindexer

# Run with PostgreSQL database
go run cmd/main.go api \
  --db.host="localhost:5432" \
  --db.name="eventindexer_test" \
  --db.username="taiko_test" \
  --db.password="testpass123" \
  --db.type="postgres" \
  --http.port=8080 \
  --rpc.url="https://rpc.mainnet.taiko.xyz"
```

## Step 9: Test Epoch Tracking Endpoints

```bash
# Test epoch gas endpoint
curl "http://localhost:8080/epochGas?chainId=167000&startEpoch=1&endEpoch=10"

# Test chart endpoint
curl "http://localhost:8080/chart?chainId=167000&task=epochGasUsage&start=2024-01-01&end=2024-12-31"
```

## Step 10: Verify Database

```bash
# Connect to test database
psql -U taiko_test -d eventindexer_test

# Check tables
\dt

# Describe events table
\d events

# Check data
SELECT COUNT(*) FROM events;
SELECT COUNT(*) FROM time_series_data;

# Exit
\q
```

## Clean Up After Testing

```bash
# Connect as postgres user
sudo -u postgres psql

# Drop database and user
DROP DATABASE eventindexer_test;
DROP USER taiko_test;

# Exit
\q
```

## Troubleshooting

### Connection Issues
```bash
# Check if PostgreSQL is running
sudo systemctl status postgresql  # Linux
brew services list  # macOS
net start  # Windows (look for postgresql service)

# Check port availability
netstat -tlnp | grep 5432  # Linux/macOS
netstat -an | findstr 5432  # Windows

# Test connection
psql -U taiko_test -h localhost -d eventindexer_test
```

### Authentication Issues
```bash
# Edit pg_hba.conf to allow password authentication
# Location varies by OS:
# - Linux: /etc/postgresql/15/main/pg_hba.conf
# - macOS: /opt/homebrew/var/postgresql@15/pg_hba.conf
# - Windows: C:\Program Files\PostgreSQL\15\data\pg_hba.conf

# Change method from 'peer' to 'md5' for local connections
# Then restart PostgreSQL
sudo systemctl restart postgresql  # Linux
brew services restart postgresql@15  # macOS
net stop postgresql-x64-15 && net start postgresql-x64-15  # Windows
```

### Migration Issues
```bash
# Check goose version
goose --version

# Verify connection string
psql "postgresql://taiko_test:testpass123@localhost:5432/eventindexer_test?sslmode=disable"

# Check migration files
ls -la packages/eventindexer/migrations-postgres/
```

## Next Steps

Would you like me to:
1. **Convert the MySQL migrations to PostgreSQL** (create all migration files)
2. **Add PostgreSQL support to the codebase** (modify config files)
3. **Both** - Full PostgreSQL support implementation

Let me know and I'll implement the necessary changes!
