# Deployment Guide

This guide covers deploying the Coinbase Base API using Docker and GitHub Actions.

## GitHub Actions Setup

### 1. Repository Setup

1. **Fork or create** this repository on GitHub
2. **Enable GitHub Packages** in repository settings:
   - Go to Settings → General → Features
   - Enable "Packages" feature

### 2. Configure Secrets

1. Go to your repository → Settings → Secrets and variables → Actions
2. Add the following secrets:

```
COINBASE_API_KEY=your_coinbase_api_key_id
COINBASE_API_SECRET=your_coinbase_private_key_pem
```

**Important**: 
- Use the full API key ID from Coinbase
- For the private key, use the PEM format with escaped newlines:
  ```
  -----BEGIN EC PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg...\n-----END EC PRIVATE KEY-----
  ```

### 3. Trigger Build

The GitHub Actions workflow will automatically:
- Build on push to `main` or `master` branch
- Build on tag push (e.g., `v1.0.0`)
- Test on pull requests

## Docker Deployment

### Local Development

```bash
# Clone and setup
git clone <your-repo-url>
cd coinbase-base
cp env.example .env
# Edit .env with your credentials

# Run with Docker Compose
docker-compose up -d

# Check health
curl http://localhost:8080/health
```

### Production Deployment

#### Option 1: Using Published Image

```bash
# Pull the latest image
docker pull ghcr.io/YOUR_USERNAME/coinbase-base:latest

# Run with environment variables
docker run -d \
  --name coinbase-api \
  --restart unless-stopped \
  -p 8080:8080 \
  -e COINBASE_API_KEY=your_api_key \
  -e COINBASE_API_SECRET=your_private_key \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=WARN \
  ghcr.io/YOUR_USERNAME/coinbase-base:latest
```

#### Option 2: Using Docker Compose

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  coinbase-api:
    image: ghcr.io/YOUR_USERNAME/coinbase-base:latest
    ports:
      - "8080:8080"
    environment:
      - COINBASE_API_KEY=${COINBASE_API_KEY}
      - COINBASE_API_SECRET=${COINBASE_API_SECRET}
      - ENVIRONMENT=production
      - LOG_LEVEL=WARN
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/ping"]
      interval: 30s
      timeout: 10s
      retries: 3
```

```bash
# Run production
docker-compose -f docker-compose.prod.yml up -d
```

### Kubernetes Deployment

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coinbase-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: coinbase-api
  template:
    metadata:
      labels:
        app: coinbase-api
    spec:
      containers:
      - name: coinbase-api
        image: ghcr.io/YOUR_USERNAME/coinbase-base:latest
        ports:
        - containerPort: 8080
        env:
        - name: COINBASE_API_KEY
          valueFrom:
            secretKeyRef:
              name: coinbase-secrets
              key: api-key
        - name: COINBASE_API_SECRET
          valueFrom:
            secretKeyRef:
              name: coinbase-secrets
              key: api-secret
        - name: ENVIRONMENT
          value: "production"
        - name: LOG_LEVEL
          value: "WARN"
        livenessProbe:
          httpGet:
            path: /ping
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: coinbase-api-service
spec:
  selector:
    app: coinbase-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
---
apiVersion: v1
kind: Secret
metadata:
  name: coinbase-secrets
type: Opaque
data:
  api-key: <base64-encoded-api-key>
  api-secret: <base64-encoded-private-key>
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `COINBASE_API_KEY` | Yes | - | Coinbase API key ID |
| `COINBASE_API_SECRET` | Yes | - | Coinbase private key (PEM format) |
| `PORT` | No | 8080 | Server port |
| `ENVIRONMENT` | No | development | Environment (development/production) |
| `LOG_LEVEL` | No | auto | Log level (DEBUG/INFO/WARN/ERROR) |

## Health Checks

### Basic Health
```bash
curl http://your-server:8080/ping
```

### Comprehensive Health
```bash
curl http://your-server:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "coinbase": {
    "authentication": "valid",
    "communication": "successful",
    "accounts_found": 2,
    "btc_account": true,
    "usdc_account": true
  },
  "timestamp": "2025-07-27T01:10:00Z"
}
```

## Monitoring

### Docker Logs
```bash
# View logs
docker logs coinbase-api

# Follow logs
docker logs -f coinbase-api

# View last 100 lines
docker logs --tail 100 coinbase-api
```

### Health Monitoring
```bash
# Check container health
docker ps

# Manual health check
curl -f http://localhost:8080/health || echo "Health check failed"
```

## Troubleshooting

### Common Issues

1. **Container won't start**
   ```bash
   # Check logs
   docker logs coinbase-api
   
   # Verify environment variables
   docker exec coinbase-api env | grep COINBASE
   ```

2. **Health check failures**
   ```bash
   # Check if service is responding
   curl http://localhost:8080/ping
   
   # Check detailed health
   curl http://localhost:8080/health
   ```

3. **Authentication errors**
   - Verify API credentials are correct
   - Check that the private key is in PEM format
   - Ensure API key has proper permissions

4. **Port conflicts**
   ```bash
   # Check if port is in use
   lsof -i :8080
   
   # Use different port
   docker run -p 8081:8080 ...
   ```

### Debug Mode

For debugging, run with debug logging:
```bash
docker run -e LOG_LEVEL=DEBUG -e ENVIRONMENT=development ...
```

## Security Considerations

1. **Never commit secrets** to version control
2. **Use environment variables** for sensitive data
3. **Restrict network access** in production
4. **Use HTTPS** for external access
5. **Monitor logs** for suspicious activity
6. **Regular updates** of base images
7. **Non-root execution** (already configured in Dockerfile)

## Backup and Recovery

### Backup Configuration
```bash
# Export environment variables
docker exec coinbase-api env > backup.env

# Backup logs
docker logs coinbase-api > logs-backup.txt
```

### Recovery
```bash
# Restore from backup
docker run -d --env-file backup.env ...
```

## Performance Tuning

### Resource Limits
```bash
docker run -d \
  --memory=512m \
  --cpus=1.0 \
  --name coinbase-api \
  ...
```

### Scaling
```bash
# Scale with Docker Compose
docker-compose up -d --scale coinbase-api=3

# Or use Kubernetes
kubectl scale deployment coinbase-api --replicas=3
``` 