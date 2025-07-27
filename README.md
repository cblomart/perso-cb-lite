# Perso CB Lite

A simple Go API for trading cryptocurrencies on Coinbase.

## What it does

- Buy and sell crypto with automatic stop-loss protection
- Check your account balance and positions
- View and cancel orders
- Secure API with rate limiting

## Quick Start

### 1. Get Coinbase API Keys
- Go to [Coinbase Advanced Trade](https://advanced.coinbase.com/)
- Settings → API → Create New API Key
- Choose **ECDSA** (not Ed25519)
- Save your API Key ID and Private Key

### 2. Run with Docker (Easiest)

```bash
# Clone and setup
git clone <repository-url>
cd perso-cb-lite
cp env.example .env

# Edit .env with your Coinbase credentials
# COINBASE_API_KEY=your_api_key_id
# COINBASE_API_SECRET=your_private_key

# Run
docker build -t perso-cb-lite .
docker run -d --name perso-cb-lite -p 8080:8080 --env-file .env perso-cb-lite
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

# Buy 0.001 BTC at $45,000
curl -X POST http://localhost:8080/api/v1/buy \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{"size": "0.001", "price": "45000.00"}'

# Buy with stop-loss at $43,000
curl -X POST http://localhost:8080/api/v1/buy \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_ACCESS_KEY" \
  -d '{
    "size": "0.001", 
    "price": "45000.00",
    "stop_price": "43000.00",
    "limit_price": "42900.00"
  }'
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
| `LOG_LEVEL` | No | auto | Log level (auto: WARN in prod, INFO in dev) |

## Local Development

```bash
go mod tidy
go run .
```

## License

MIT License 