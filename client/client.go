package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

// CoinbaseClient represents a custom Coinbase Advanced Trade API client
type CoinbaseClient struct {
	logger            *log.Logger
	apiKey            string
	privateKey        *ecdsa.PrivateKey
	tradingPair       string
	webhookURL        string
	webhookMaxRetries int
	webhookTimeout    int
	httpClient        *http.Client
	// Performance tracking
	requestCount int64
	startTime    time.Time
}

// NewCoinbaseClient creates a new Coinbase client using ECDSA private key
func NewCoinbaseClient(tradingPair string, webhookURL string, webhookMaxRetries int, webhookTimeout int) (*CoinbaseClient, error) {
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
		logger:            logger,
		apiKey:            apiKey,
		privateKey:        privateKey,
		tradingPair:       tradingPair,
		webhookURL:        webhookURL,
		webhookMaxRetries: webhookMaxRetries,
		webhookTimeout:    webhookTimeout,
		httpClient:        httpClient,
		requestCount:      0,
		startTime:         time.Now(),
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

// SendWebhook sends a webhook notification to n8n with retry logic
func (c *CoinbaseClient) SendWebhook(signal *SignalResponse) error {
	if c.webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	// Retry configuration
	maxRetries := c.webhookMaxRetries
	baseDelay := 1 * time.Second

	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("🚀 Starting webhook delivery (max retries: %d, timeout: %ds)", maxRetries, c.webhookTimeout)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if os.Getenv("LOG_LEVEL") == "DEBUG" && attempt > 0 {
			c.logger.Printf("🔄 Webhook attempt %d/%d", attempt+1, maxRetries+1)
		}

		startTime := time.Now()
		err := c.sendWebhookAttempt(signal)
		duration := time.Since(startTime)

		if err == nil {
			// Success on first attempt
			if attempt == 0 {
				if os.Getenv("LOG_LEVEL") == "DEBUG" {
					c.logger.Printf("✅ Webhook sent successfully to %s (duration: %v)", c.webhookURL, duration)
				} else {
					c.logger.Printf("Webhook sent successfully to %s", c.webhookURL)
				}
			} else {
				if os.Getenv("LOG_LEVEL") == "DEBUG" {
					c.logger.Printf("✅ Webhook sent successfully to %s after %d retries (total duration: %v)", c.webhookURL, attempt, duration)
				} else {
					c.logger.Printf("Webhook sent successfully to %s after %d retries", c.webhookURL, attempt)
				}
			}
			return nil
		}

		// Log the error
		if attempt == 0 {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("❌ Webhook failed (attempt %d/%d, duration: %v): %v", attempt+1, maxRetries+1, duration, err)
			} else {
				c.logger.Printf("Webhook failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			}
		} else {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("❌ Webhook retry failed (attempt %d/%d, duration: %v): %v", attempt+1, maxRetries+1, duration, err)
			} else {
				c.logger.Printf("Webhook retry failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			}
		}

		// Don't retry on the last attempt
		if attempt == maxRetries {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("💀 Webhook failed after %d attempts, giving up (total time: %v)", maxRetries+1, time.Since(startTime))
			} else {
				c.logger.Printf("Webhook failed after %d attempts, giving up", maxRetries+1)
			}
			return fmt.Errorf("webhook failed after %d attempts: %w", maxRetries+1, err)
		}

		// Calculate exponential backoff delay
		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("⏳ Retrying webhook in %v (exponential backoff: attempt %d)", delay, attempt+1)
		} else {
			c.logger.Printf("Retrying webhook in %v...", delay)
		}
		time.Sleep(delay)
	}

	return fmt.Errorf("webhook failed after %d attempts", maxRetries+1)
}

// sendWebhookAttempt performs a single webhook attempt
func (c *CoinbaseClient) sendWebhookAttempt(signal *SignalResponse) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", c.webhookURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Add query parameters for GET request
	q := req.URL.Query()
	q.Add("signal", "true")
	q.Add("bearish", "true")
	q.Add("triggers", strings.Join(signal.Triggers, ","))
	q.Add("timestamp", fmt.Sprintf("%d", signal.Timestamp))
	req.URL.RawQuery = q.Encode()

	// Debug logging for webhook request
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("🔗 Webhook Request:")
		c.logger.Printf("   URL: %s", req.URL.String())
		c.logger.Printf("   Method: %s", req.Method)
		c.logger.Printf("   Headers: %v", req.Header)
		c.logger.Printf("   Query Params: signal=true, bearish=true, triggers=%s, timestamp=%d",
			strings.Join(signal.Triggers, ","), signal.Timestamp)
	}

	// Set timeout for this attempt
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.webhookTimeout)*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("❌ Webhook Request Failed: %v", err)
		}
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Debug logging for webhook response
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("📡 Webhook Response:")
		c.logger.Printf("   Status: %s", resp.Status)
		c.logger.Printf("   Status Code: %d", resp.StatusCode)
		c.logger.Printf("   Headers: %v", resp.Header)

		// Read and log response body if present
		if resp.Body != nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil && len(bodyBytes) > 0 {
				c.logger.Printf("   Body: %s", string(bodyBytes))
			}
			// Recreate the response body for potential future use
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	// Check response status
	if resp.StatusCode >= 400 {
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("❌ Webhook Response Error: HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("webhook failed with status %d", resp.StatusCode)
	}

	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("✅ Webhook Response Success: HTTP %d", resp.StatusCode)
	}

	return nil
}
