# Perso CB Lite

A simple Go API for trading cryptocurrencies on Coinbase.

## What it does

- Buy and sell crypto with market orders
- Buy/sell percentage of available balance with `percentage` setting (includes actual Coinbase fees)
- Check your account balance
- View and cancel orders (individual or all)
- Get market data (candles) for technical analysis
- View current market state (bid/ask, spread, order book)
- Automatic balance validation before placing orders
- Secure API with rate limiting

### Fee Structure

The API automatically accounts for Coinbase's actual fee structure:
- **0.50% spread** per transaction
- **Tiered flat fees**:
  - $0.99 for trades up to $10
  - $1.49 for trades $10–$25
  - $1.99 for trades $25–$50
  - $2.99 for trades $50–$200
  - 1.49% for trades over $200

### Trading Signals

The `/api/v1/signal` endpoint provides comprehensive technical analysis:

**Technical Indicators Calculated:**
- **MACD** (Moving Average Convergence Divergence) and signal line
- **EMA 12, 26, 200** (Exponential Moving Averages)
- **RSI** (Relative Strength Index)
- **ADX** (Average Directional Index)
- **Price percentage change** over last 4 hours
- **Volume spike detection** (last candle > 2× average)

**Trend Change Detection:**
- **Bullish to Bearish**: When 3+ bearish signals align (trend reversal)
- **Bearish to Bullish**: When 3+ bullish signals align (trend reversal)
- **Neutral to Trend**: First clear trend establishment
- **Cooldown Period**: 30-minute minimum between trend change signals

**Response Codes:**
- **200 OK**: Trend change detected (includes full indicator data)
- **204 No Content**: No trend changes detected

**Webhook Integration:**
When `WEBHOOK_URL` is configured, the API automatically:
- **Polls Coinbase every 10 minutes** for signal data
- **Sends GET requests to n8n** when bearish signals are detected
- **Retries failed webhooks** with exponential backoff (configurable)
- **Logs all signal checks** for monitoring

```bash
# Example webhook URL format
WEBHOOK_URL=http://n8n:5678/webhook/signal

# Query parameters sent to n8n:
# ?signal=true&bearish=true&triggers=MACD_BEARISH_CROSSOVER,EMA_BEARISH_CROSSOVER&timestamp=1234567890
```

**Webhook Reliability:**
- **Retry attempts**: Configurable (default: 3 attempts)
- **Exponential backoff**: 1s, 2s, 4s delays between retries
- **Timeout per attempt**: Configurable (default: 5 seconds)
- **Failure logging**: Detailed error messages for debugging

**n8n Integration:**
1. Create a webhook trigger in n8n
2. Set the webhook URL (e.g., `http://n8n:5678/webhook/signal`)
3. Configure the API with `WEBHOOK_URL=http://n8n:5678/webhook/signal`
4. The API will automatically poll and notify n8n when bearish signals are detected

**Polling Behavior:**
- **With WEBHOOK_URL**: Automatic lightweight polling every 10 minutes + webhook notifications
- **Without WEBHOOK_URL**: Manual polling only via `/api/v1/signal` endpoint

**Polling Optimization:**
- **Background polling**: Uses 144 5-minute candles (12 hours) for efficiency and responsiveness
- **Manual endpoint**: Uses 300 5-minute candles (25 hours) for comprehensive analysis
- **Network traffic**: ~52% reduction in data transfer for background polling
- **CPU usage**: ~80% reduction in calculation overhead (parallel processing + early termination)
- **Parallel indicators**: All technical indicators calculated concurrently using goroutines
- **Early termination**: Stops calculations immediately when a signal is detected

**Data Requirements:**
- **Timeframe**: 25 hours of 5-minute candles (~1 day)
- **Data Points**: 300 candles for comprehensive technical analysis
- **Update Frequency**: Designed for 10-minute intervals
- **Granularity**: 5-minute intervals (FIVE_MINUTE)

## Quick Start

### 1. Get Coinbase API Keys
- Go to [Coinbase Advanced Trade](https://advanced.coinbase.com/)
- Settings → API → Create New API Key
- Choose **ECDSA** (not Ed25519)
- Save your API Key ID and Private Key

### 2. Run with Docker (Easiest)

#### Option A: Build from source
```bash
# Clone and setup
git clone <repository-url>
cd perso-cb-lite
cp env.example .env

# Edit .env with your Coinbase credentials
# COINBASE_API_KEY=your_api_key_id
# COINBASE_API_SECRET=your_private_key

# Build and run
docker build -t perso-cb-lite .
docker run -d --name perso-cb-lite -p 8080:8080 --env-file .env perso-cb-lite
```

#### Option B: Use pre-built image from GitHub Container Registry
```bash
# Pull and run the latest version
docker pull ghcr.io/cblomart/perso-cb-lite:latest
docker run -d --name perso-cb-lite -p 8080:8080 \
  -e COINBASE_API_KEY=your_api_key_id \
  -e COINBASE_API_SECRET=your_private_key \
  ghcr.io/cblomart/perso-cb-lite:latest
```

### 3. Test it works

```bash
# Health check
curl http://localhost:8080/health

# Get your access key from the logs
docker logs perso-cb-lite | grep "Access Key"
```

## Usage Examples

```bash
# Check accounts (use your access key from logs)
curl -H "X-API-Key: YOUR_ACCESS_KEY" http://localhost:8080/api/v1/accounts

# Get current market state (bid/ask, spread, order book)
curl -H "X-API-Key: YOUR_ACCESS_KEY" http://localhost:8080/api/v1/market

# Get market state with custom limit (50 bid/ask entries)
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/market?limit=50"

# Get trading signals (technical analysis)
curl -H "X-API-Key: YOUR_ACCESS_KEY" http://localhost:8080/api/v1/signal

# Custom time range with specific granularity
curl -H "X-API-Key: YOUR_ACCESS_KEY" \
  "http://localhost:8080/api/v1/candles?start=1639508050&end=1639594450&granularity=ONE_HOUR"

# Preset periods (convenient shortcuts)
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/candles?period=last_year"   # Daily candles (350 days)
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/candles?period=last_month"   # 6-hour candles  
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/candles?period=last_week"    # 6-hour candles
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/candles?period=last_day"     # 15-minute candles
curl -H "X-API-Key: YOUR_ACCESS_KEY" "http://localhost:8080/api/v1/candles?period=last_hour"    # 1-minute candles

# Buy 0.001 BTC at $45,000 (regular limit order)
curl -X POST http://localhost:8080/api/v1/buy \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{"size": "0.001", "price": 45000.00}'

# Buy 50% of available USDC at $45,000 (includes actual Coinbase fees)
curl -X POST http://localhost:8080/api/v1/buy \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{"percentage": 50.0, "price": 45000.00}'

# Sell 0.001 BTC at $50,000 (market order)
curl -X POST http://localhost:8080/api/v1/sell \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{"size": "0.001", "price": 50000.00}'

# Sell 25% of available BTC at $50,000 (includes actual Coinbase fees)
curl -X POST http://localhost:8080/api/v1/sell \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{"percentage": 25.0, "price": 50000.00}'

# Cancel all open orders
curl -X DELETE http://localhost:8080/api/v1/orders \
  -H "X-API-Key: YOUR_ACCESS_KEY"
```

## Configuration

Edit `.env` to change trading pairs:

```env
# Trade BTC/USDC (default)
TRADING_BASE_CURRENCY=BTC
TRADING_QUOTE_CURRENCY=USDC

# Or trade ETH/USD
TRADING_BASE_CURRENCY=ETH
TRADING_QUOTE_CURRENCY=USD
```

## Security

- Access key required for all API calls (except health checks)
- Rate limiting: 60 requests per minute per IP
- Optional IP whitelisting
- Never commit your `.env` file

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `COINBASE_API_KEY` | Yes | - | Coinbase API key ID |
| `COINBASE_API_SECRET` | Yes | - | Coinbase private key (PEM format) |
| `TRADING_BASE_CURRENCY` | No | BTC | Base currency (e.g., BTC, ETH, SOL) |
| `TRADING_QUOTE_CURRENCY` | No | USDC | Quote currency (e.g., USDC, USD, EUR) |
| `API_ACCESS_KEY` | No | auto-gen | Custom access key (auto-generated if empty, shown in logs) |
| `RATE_LIMIT_REQUESTS_PER_MINUTE` | No | 60 | Rate limit per IP per minute |
| `ENABLE_RATE_LIMITING` | No | true | Enable/disable rate limiting |
| `ENABLE_IP_WHITELIST` | No | false | Enable/disable IP whitelisting |
| `ENABLE_ACCESS_KEY_AUTH` | No | true | Enable/disable access key authentication |
| `ALLOWED_IPS` | No | - | Comma-separated list of allowed IPs/subnets |
| `PORT` | No | 8080 | Server port |
| `ENVIRONMENT` | No | development | Environment (development/production) |
| `LOG_LEVEL` | No | auto | Log level (DEBUG/INFO/WARN/ERROR, auto: WARN in prod, INFO in dev) |
| `WEBHOOK_URL` | No | - | n8n webhook URL for signal notifications (optional) |
| `WEBHOOK_MAX_RETRIES` | No | 3 | Maximum webhook retry attempts (0-10) |
| `WEBHOOK_TIMEOUT_SECONDS` | No | 5 | Webhook timeout per attempt in seconds (1-30) |

## Docker Deployment

### Available Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `latest` | Latest stable build from main branch | `ghcr.io/cblomart/perso-cb-lite:latest` |
| `main` | Latest build from main branch (same as latest) | `ghcr.io/cblomart/perso-cb-lite:main` |
| `v1.0.0` | Specific version (semantic versioning) | `ghcr.io/cblomart/perso-cb-lite:v1.0.0` |
| `v1.0` | Latest patch version of 1.0.x | `ghcr.io/cblomart/perso-cb-lite:v1.0` |
| `abc1234` | Specific commit SHA | `ghcr.io/cblomart/perso-cb-lite:abc1234` |

### Tag Differences

- **`latest`**: Always points to the most recent main branch build (recommended for development)
- **`main`**: Same as latest - most recent main branch build
- **`v1.0.0`**: Fixed version, won't change (recommended for production)
- **`v1.0`**: Latest patch updates within 1.0.x series
- **Commit SHA**: Exact build from specific commit

### Production Deployment

```bash
# Use specific version for production (recommended)
docker run -d --name perso-cb-lite \
  --restart unless-stopped \
  -p 8080:8080 \
  -e COINBASE_API_KEY=your_api_key_id \
  -e COINBASE_API_SECRET=your_private_key \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=WARN \
  ghcr.io/cblomart/perso-cb-lite:v1.0.0
```

### Development Deployment

```bash
# Use latest for development/testing
docker run -d --name perso-cb-lite-dev \
  -p 8081:8080 \
  -e COINBASE_API_KEY=your_api_key_id \
  -e COINBASE_API_SECRET=your_private_key \
  -e ENVIRONMENT=development \
  -e LOG_LEVEL=INFO \
  ghcr.io/cblomart/perso-cb-lite:latest
```

### Using Environment File

```bash
# Create .env file with your credentials
echo "COINBASE_API_KEY=your_api_key_id" > .env
echo "COINBASE_API_SECRET=your_private_key" >> .env
echo "ENVIRONMENT=production" >> .env

# Run with environment file
docker run -d --name perso-cb-lite \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file .env \
  ghcr.io/cblomart/perso-cb-lite:latest
```

### Docker Compose

```yaml
# docker-compose.yml
version: '3.8'
services:
  perso-cb-lite:
    image: ghcr.io/cblomart/perso-cb-lite:latest
    ports:
      - "8080:8080"
    environment:
      - COINBASE_API_KEY=${COINBASE_API_KEY}
      - COINBASE_API_SECRET=${COINBASE_API_SECRET}
      - ENVIRONMENT=production
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 120s
      timeout: 30s
      retries: 3
```

```bash
# Run with Docker Compose
docker-compose up -d
```

## Local Development

```bash
go mod tidy
go run .
```

## License

MIT License 