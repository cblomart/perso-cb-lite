package client

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// CoinbaseClient represents a custom Coinbase Advanced Trade API client
type CoinbaseClient struct {
	logger      *log.Logger
	apiKey      string
	privateKey  *ecdsa.PrivateKey
	tradingPair string
	httpClient  *http.Client
	// Performance tracking
	requestCount int64
	startTime    time.Time
}

// NewCoinbaseClient creates a new Coinbase client using ECDSA private key
func NewCoinbaseClient(tradingPair string) (*CoinbaseClient, error) {
	apiKey := os.Getenv("COINBASE_API_KEY")
	apiSecret := os.Getenv("COINBASE_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("missing required environment variables: COINBASE_API_KEY, COINBASE_API_SECRET")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		environment := os.Getenv("ENVIRONMENT")
		if environment == "production" {
			logLevel = "WARN"
		} else {
			logLevel = "INFO"
		}
	}
	logger := log.New(os.Stdout, fmt.Sprintf("[COINBASE-%s] ", logLevel), log.LstdFlags|log.Lshortfile)

	// Clean up the PEM key - remove extra whitespace and ensure proper formatting
	apiSecret = strings.TrimSpace(apiSecret)

	// If the key doesn't start with the PEM header, try to format it
	if !strings.HasPrefix(apiSecret, "-----BEGIN EC PRIVATE KEY-----") {
		// Try to add PEM headers if they're missing
		if !strings.Contains(apiSecret, "-----BEGIN") {
			apiSecret = "-----BEGIN EC PRIVATE KEY-----\n" + apiSecret + "\n-----END EC PRIVATE KEY-----"
		}
	}

	// Parse ECDSA private key from PEM format
	block, _ := pem.Decode([]byte(apiSecret))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block - check your private key format")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA private key: %w", err)
	}

	logger.Printf("Successfully loaded ECDSA private key")
	logger.Printf("Trading pair: %s", tradingPair)

	// Create optimized HTTP client with connection pooling
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,              // Maximum number of idle connections
			MaxIdleConnsPerHost: 10,               // Maximum idle connections per host
			IdleConnTimeout:     90 * time.Second, // How long to keep idle connections
			DisableCompression:  false,            // Keep compression for smaller payloads
			ForceAttemptHTTP2:   true,             // Use HTTP/2 for better performance
			DisableKeepAlives:   false,            // Enable keep-alive for connection reuse
			MaxConnsPerHost:     0,                // No limit on connections per host
		},
	}

	return &CoinbaseClient{
		logger:       logger,
		apiKey:       apiKey,
		privateKey:   privateKey,
		tradingPair:  tradingPair,
		httpClient:   httpClient,
		requestCount: 0,
		startTime:    time.Now(),
	}, nil
}

// GetTradingPair returns the configured trading pair
func (c *CoinbaseClient) GetTradingPair() string {
	return c.tradingPair
}

// Close closes the HTTP client and cleans up resources
func (c *CoinbaseClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// GetPerformanceStats returns performance statistics
func (c *CoinbaseClient) GetPerformanceStats() map[string]interface{} {
	uptime := time.Since(c.startTime)
	return map[string]interface{}{
		"uptime_seconds":      uptime.Seconds(),
		"total_requests":      c.requestCount,
		"requests_per_second": float64(c.requestCount) / uptime.Seconds(),
		"trading_pair":        c.tradingPair,
	}
}
