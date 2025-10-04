# Fresh Database Setup for Epoch Tracking Testing

## Option 1: Local MySQL Installation (No Docker)

### 1. Install MySQL
```bash
# Windows (using Chocolatey)
choco install mysql

# Or download installer from: https://dev.mysql.com/downloads/mysql/
# Choose "MySQL Installer for Windows"

# macOS (using Homebrew)
brew install mysql
brew services start mysql

# Ubuntu/Debian
sudo apt update && sudo apt install mysql-server
sudo systemctl start mysql
sudo systemctl enable mysql

# Set root password (if not set during installation)
sudo mysql_secure_installation
```

### 2. Create Test Database and User
```bash
# Connect to MySQL as root
mysql -u root -p

# In MySQL console, run:
```

```sql
-- Create fresh test database
CREATE DATABASE eventindexer_test;

-- Create test user with password
CREATE USER 'taiko'@'localhost' IDENTIFIED BY 'taikopass';

-- Grant all privileges on test database
GRANT ALL PRIVILEGES ON eventindexer_test.* TO 'taiko'@'localhost';

-- Apply changes
FLUSH PRIVILEGES;

-- Exit MySQL
EXIT;
```

### 3. Run Database Migrations
```bash
# Install goose if not already installed
go install github.com/pressly/goose/v3/cmd/goose@latest

# Navigate to migrations directory
cd packages/eventindexer/migrations

# Run all migrations to create fresh schema
goose mysql "taiko:taikopass@tcp(localhost:3306)/eventindexer_test?parseTime=true" up
```

### 4. Environment Variables for Testing
```bash
export DATABASE_HOST="localhost:3306"
export DATABASE_NAME="eventindexer_test"
export DATABASE_USER="taiko"
export DATABASE_PASSWORD="taikopass"
```

## Option 2: Use Existing MySQL with New Database

### 1. Create New Database (if you have MySQL already)
```bash
# Connect to your existing MySQL
mysql -u root -p

# Create fresh test database
CREATE DATABASE eventindexer_test_new;
CREATE USER 'taiko_test'@'localhost' IDENTIFIED BY 'testpass123';
GRANT ALL PRIVILEGES ON eventindexer_test_new.* TO 'taiko_test'@'localhost';
FLUSH PRIVILEGES;
EXIT;
```

### 2. Run Migrations on New Database
```bash
cd packages/eventindexer/migrations
goose mysql "taiko_test:testpass123@tcp(localhost:3306)/eventindexer_test_new?parseTime=true" up
```

## Testing the Epoch Tracking System

### 1. Start the API Server
```bash
cd packages/eventindexer

# Run with fresh test database
go run cmd/main.go api \
  --db.host="localhost:3306" \
  --db.name="eventindexer_test" \
  --db.username="taiko" \
  --db.password="taikopass" \
  --http.port=8080 \
  --rpc.url="https://rpc.mainnet.taiko.xyz"
```

### 2. Test Epoch Gas Tracking Endpoints
```bash
# Test the new epoch gas endpoint
curl "http://localhost:8080/epochGas?chainId=167000&startEpoch=1&endEpoch=10"

# Test the chart endpoint with epoch data
curl "http://localhost:8080/chart?chainId=167000&task=epochGasUsage&start=2024-01-01&end=2024-12-31"
```

### 3. Verify Database Schema
```sql
-- Connect to test database
mysql -u taiko -p eventindexer_test

-- Check if all tables exist
SHOW TABLES;

-- Verify events table structure (should include our epoch tracking fields)
DESCRIBE events;

-- Check for any existing data
SELECT COUNT(*) FROM events;
SELECT COUNT(*) FROM time_series_data;
```

## Clean Up After Testing
```bash
# Stop and remove test container
docker stop taiko-eventindexer-test
docker rm taiko-eventindexer-test

# Or if using local MySQL
mysql -u root -p -e "DROP DATABASE eventindexer_test; DROP USER 'taiko'@'localhost';"
```

## Troubleshooting

### Connection Issues
- Ensure MySQL is running: `docker ps` or `systemctl status mysql`
- Check port availability: `netstat -tlnp | grep 3307`
- Verify credentials with: `mysql -u taiko -p -h localhost -P 3307 eventindexer_test`

### Migration Issues
- Check goose version: `goose --version`
- Verify migration files: `ls -la packages/eventindexer/migrations/`
- Check migration status: `goose mysql "connection_string" status`

### API Issues
- Check if port 8080 is available: `netstat -tlnp | grep 8080`
- Verify RPC URL is accessible: `curl -X POST https://rpc.mainnet.taiko.xyz -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'`
