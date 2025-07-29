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
	"strconv"
	"strings"
	"sync"
	"time"
)

// Context key for health check tracking
type contextKey string

const healthCheckKey contextKey = "health_check"

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
	// Trend state tracking
	lastTrendState      string // "bullish", "bearish", or "neutral"
	lastSignalTime      time.Time
	trendChangeCooldown time.Duration // Minimum time between trend change signals
	// Asset value tracking
	assetValueHistory []AccountValue
	assetValueMutex   sync.RWMutex
}

// NewCoinbaseClient creates a new Coinbase client using ECDSA private key
func NewCoinbaseClient(tradingPair string, webhookURL string, webhookMaxRetries int, webhookTimeout int) (*CoinbaseClient, error) {
	apiKey := os.Getenv("COINBASE_API_KEY")
	apiSecret := os.Getenv("COINBASE_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("missing required environment variables: COINBASE_API_KEY, COINBASE_API_SECRET")
	}

	// Initialize logger
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
		logger:              logger,
		apiKey:              apiKey,
		privateKey:          privateKey,
		tradingPair:         tradingPair,
		webhookURL:          webhookURL,
		webhookMaxRetries:   webhookMaxRetries,
		webhookTimeout:      webhookTimeout,
		httpClient:          httpClient,
		startTime:           time.Now(),
		trendChangeCooldown: 8 * time.Minute, // Increased from 2 to 8 minutes to reduce signal frequency
	}, nil
}

// TrackAssetValue adds the current asset value to the historical tracking
func (c *CoinbaseClient) TrackAssetValue() error {
	// Get current account balances
	accounts, err := c.GetAccounts()
	if err != nil {
		return fmt.Errorf("failed to get current accounts: %w", err)
	}

	// Find BTC and USDC accounts
	var btcAccount, usdcAccount *Account
	for i := range accounts {
		if accounts[i].Currency == "BTC" {
			btcAccount = &accounts[i]
		} else if accounts[i].Currency == "USDC" {
			usdcAccount = &accounts[i]
		}
	}

	if btcAccount == nil || usdcAccount == nil {
		return fmt.Errorf("missing BTC or USDC accounts")
	}

	// Get current BTC price for USD calculation
	orderBook, err := c.GetOrderBook(1)
	if err != nil {
		return fmt.Errorf("failed to get current price: %w", err)
	}

	var currentPrice float64
	if len(orderBook.Bids) > 0 {
		currentPrice, _ = strconv.ParseFloat(orderBook.Bids[0].Price, 64)
	} else {
		return fmt.Errorf("no current price available")
	}

	// Calculate total USD value
	btcBalance, _ := strconv.ParseFloat(btcAccount.AvailableBalance, 64)
	usdcBalance, _ := strconv.ParseFloat(usdcAccount.AvailableBalance, 64)
	totalUSD := usdcBalance + (btcBalance * currentPrice)

	// Create account value entry
	accountValue := AccountValue{
		Timestamp: time.Now().Unix(),
		BTC:       btcBalance,
		USDC:      usdcBalance,
		TotalUSD:  totalUSD,
	}

	// Add to history with thread safety
	c.assetValueMutex.Lock()
	defer c.assetValueMutex.Unlock()

	// Keep only last 1000 entries to prevent memory bloat
	if len(c.assetValueHistory) >= 1000 {
		c.assetValueHistory = c.assetValueHistory[1:]
	}

	c.assetValueHistory = append(c.assetValueHistory, accountValue)

	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Asset value tracked: $%.2f (BTC: %.8f, USDC: %.2f)",
			totalUSD, btcBalance, usdcBalance)
	}

	return nil
}

// GetAssetValueHistory returns the historical asset values
func (c *CoinbaseClient) GetAssetValueHistory() []AccountValue {
	c.assetValueMutex.RLock()
	defer c.assetValueMutex.RUnlock()

	// Return a copy to prevent race conditions
	result := make([]AccountValue, len(c.assetValueHistory))
	copy(result, c.assetValueHistory)
	return result
}

// GetAssetValueHistoryForPeriod returns asset values within a specific time period
func (c *CoinbaseClient) GetAssetValueHistoryForPeriod(startTime, endTime time.Time) []AccountValue {
	c.assetValueMutex.RLock()
	defer c.assetValueMutex.RUnlock()

	var result []AccountValue
	for _, av := range c.assetValueHistory {
		avTime := time.Unix(av.Timestamp, 0)
		if avTime.After(startTime) && avTime.Before(endTime) {
			result = append(result, av)
		}
	}
	return result
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
		return fmt.Errorf("no webhook URL configured")
	}

	maxRetries := c.webhookMaxRetries
	baseDelay := 1 * time.Second
	startTime := time.Now()

	// Debug: Log webhook start
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("🚀 Starting webhook delivery (max retries: %d, timeout: %ds)", maxRetries, c.webhookTimeout)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if os.Getenv("LOG_LEVEL") == "DEBUG" && attempt > 0 {
			c.logger.Printf("🔄 Webhook attempt %d/%d", attempt+1, maxRetries+1)
		}

		attemptStartTime := time.Now()
		err := c.sendWebhookAttempt(signal)
		duration := time.Since(attemptStartTime)

		if err == nil {
			// Success - log based on retry count
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

		// Error logging - always log errors
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("❌ Webhook failed (attempt %d/%d, duration: %v): %v", attempt+1, maxRetries+1, duration, err)
		} else {
			c.logger.Printf("Webhook failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
		}

		// If this was the last attempt, give up
		if attempt == maxRetries {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("💀 Webhook failed after %d attempts, giving up (total time: %v)", maxRetries+1, time.Since(startTime))
			} else {
				c.logger.Printf("Webhook failed after %d attempts, giving up", maxRetries+1)
			}
			return fmt.Errorf("webhook failed after %d attempts", maxRetries+1)
		}

		// Calculate delay with exponential backoff
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

// detectTrendChange determines if there's been a significant trend change that warrants a webhook
func (c *CoinbaseClient) detectTrendChange(indicators TechnicalIndicators) (bool, string, []string) {
	// Determine current trend state based on indicators
	currentTrend := c.determineTrendState(indicators)

	// Calculate scores for debug logging
	bearishScore := c.calculateBearishScore(indicators)
	bullishScore := c.calculateBullishScore(indicators)

	// Debug logging for weighted scores
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("📊 Weighted Scores - Bearish: %.2f, Bullish: %.2f, Trend: %s",
			bearishScore, bullishScore, currentTrend)
	}

	// Check for immediate dip detection (more sensitive)
	dipDetected, dipTriggers := c.detectImmediateDip(indicators)
	if dipDetected {
		// Check longer cooldown for dips (5 minutes instead of 2)
		if time.Since(c.lastSignalTime) < 5*time.Minute {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("🕐 Dip detected but cooldown active (last signal: %v ago)",
					time.Since(c.lastSignalTime))
			}
		} else {
			// Valid dip detected
			c.lastSignalTime = time.Now()
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("📉 Immediate dip detected: %v", dipTriggers)
			}
			return true, "bearish", dipTriggers
		}
	}

	// Check if this is a significant change from the last known state
	if c.lastTrendState == "neutral" {
		// First signal - only send if we have a clear trend
		if currentTrend != "neutral" {
			c.lastTrendState = currentTrend
			c.lastSignalTime = time.Now()
			triggers := c.calculateTriggers(indicators, currentTrend)
			return true, currentTrend, triggers
		}
		return false, currentTrend, nil
	}

	// Check if trend has changed
	if currentTrend != c.lastTrendState && currentTrend != "neutral" {
		// Check cooldown period to avoid spam (increased to 8 minutes)
		if time.Since(c.lastSignalTime) < 8*time.Minute {
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("🕐 Trend change detected but cooldown active (last signal: %v ago)",
					time.Since(c.lastSignalTime))
			}
			return false, currentTrend, nil
		}

		// Valid trend change detected
		oldTrend := c.lastTrendState
		c.lastTrendState = currentTrend
		c.lastSignalTime = time.Now()

		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("🔄 Trend change detected: %s → %s", oldTrend, currentTrend)
		}

		triggers := c.calculateTriggers(indicators, currentTrend)
		return true, currentTrend, triggers
	}

	return false, currentTrend, nil
}

// detectImmediateDip detects immediate price dips using weighted scoring
func (c *CoinbaseClient) detectImmediateDip(indicators TechnicalIndicators) (bool, []string) {
	var triggers []string
	dipScore := 0.0

	// Price drop detection (weight: 2.0 - direct price action)
	if indicators.PriceDropPct12h < -3 {
		dropStrength := math.Abs(indicators.PriceDropPct12h)
		if dropStrength > 7 {
			dipScore += 3.0 // Strong drop
			triggers = append(triggers, "STRONG_PRICE_DROP")
		} else if dropStrength > 5 {
			dipScore += 2.0 // Moderate drop
			triggers = append(triggers, "IMMEDIATE_PRICE_DROP")
		} else {
			dipScore += 1.0 // Slight drop
		}
	}

	// RSI oversold condition (weight: 1.5 - momentum)
	if indicators.RSI < 35 {
		if indicators.RSI < 25 {
			dipScore += 2.5 // Extreme oversold
			triggers = append(triggers, "EXTREME_RSI_OVERSOLD")
		} else {
			dipScore += 1.5 // Moderate oversold
			triggers = append(triggers, "RSI_OVERSOLD")
		}
	}

	// MACD bearish crossover (weight: 2.0 - trend indicator)
	if indicators.MACD < indicators.SignalLine {
		macdStrength := math.Abs(indicators.MACD - indicators.SignalLine)
		if indicators.MACD < -0.15 {
			dipScore += 2.5 + (macdStrength * 10) // Strong bearish MACD
			triggers = append(triggers, "STRONG_MACD_BEARISH")
		} else if indicators.MACD < -0.05 {
			dipScore += 2.0 // Moderate bearish MACD
			triggers = append(triggers, "MACD_BEARISH_CROSSOVER")
		} else {
			dipScore += 1.0 // Slight bearish MACD
		}
	}

	// EMA bearish crossover (weight: 2.0 - trend indicator)
	if indicators.EMA12 < indicators.EMA26 {
		emaStrength := (indicators.EMA26 - indicators.EMA12) / indicators.EMA26 * 100
		dipScore += 2.0 + (emaStrength * 0.1) // Bonus for stronger crossover
		triggers = append(triggers, "EMA_BEARISH_CROSSOVER")
	}

	// Volume spike with price drop (weight: 1.0 - confirmation)
	if indicators.VolumeSpike && indicators.PriceDropPct12h < -2 {
		dipScore += 1.0
		triggers = append(triggers, "VOLUME_SPIKE_WITH_DROP")
	}

	// Strong bearish momentum (weight: 1.5 - trend strength)
	if indicators.ADX > 25 && indicators.MACD < indicators.SignalLine {
		dipScore += 1.5
		triggers = append(triggers, "STRONG_BEARISH_MOMENTUM")
	}

	// Price below EMA200 with momentum (weight: 1.0 - long-term trend)
	if indicators.CurrentPrice < indicators.EMA200 && indicators.RSI < 40 {
		ema200Strength := (indicators.EMA200 - indicators.CurrentPrice) / indicators.EMA200 * 100
		dipScore += 1.0 + (ema200Strength * 0.05)
		triggers = append(triggers, "BELOW_EMA200_WITH_MOMENTUM")
	}

	// Require a minimum weighted score for dip detection
	if dipScore >= 6.0 { // High confidence dip
		return true, triggers
	}

	return false, nil
}

// calculateTriggers calculates the relevant triggers for the current trend
func (c *CoinbaseClient) calculateTriggers(indicators TechnicalIndicators, trend string) []string {
	var triggers []string

	if trend == "bearish" {
		// Bearish triggers
		if indicators.MACD < indicators.SignalLine && indicators.MACD < 0 {
			triggers = append(triggers, "MACD_BEARISH_CROSSOVER")
		}
		if indicators.EMA12 < indicators.EMA26 {
			triggers = append(triggers, "EMA_BEARISH_CROSSOVER")
		}
		if indicators.RSI < 40 {
			triggers = append(triggers, "RSI_MOMENTUM_BREAKDOWN")
		}
		if indicators.PriceDropPct12h < -5 {
			triggers = append(triggers, "PRICE_TREND_REVERSAL")
		}
		if indicators.CurrentPrice < indicators.EMA200 && indicators.RSI < 45 {
			triggers = append(triggers, "MAJOR_TREND_BREAKDOWN")
		}
	} else if trend == "bullish" {
		// Bullish triggers
		if indicators.MACD > indicators.SignalLine && indicators.MACD > 0 {
			triggers = append(triggers, "MACD_BULLISH_CROSSOVER")
		}
		if indicators.EMA12 > indicators.EMA26 {
			triggers = append(triggers, "EMA_BULLISH_CROSSOVER")
		}
		if indicators.RSI > 60 {
			triggers = append(triggers, "RSI_MOMENTUM_BUILDUP")
		}
		if indicators.PriceDropPct12h > 5 {
			triggers = append(triggers, "PRICE_TREND_REVERSAL")
		}
		if indicators.CurrentPrice > indicators.EMA200 && indicators.RSI > 55 {
			triggers = append(triggers, "MAJOR_TREND_BREAKOUT")
		}
	}

	return triggers
}

// determineTrendState determines the current trend state based on weighted technical indicators
func (c *CoinbaseClient) determineTrendState(indicators TechnicalIndicators) string {
	// Calculate weighted scores for bearish and bullish signals
	bearishScore := c.calculateBearishScore(indicators)
	bullishScore := c.calculateBullishScore(indicators)

	// Determine trend based on weighted scores
	// Higher threshold for trend change to avoid false signals
	if bearishScore >= 7.0 { // High confidence bearish
		return "bearish"
	} else if bullishScore >= 7.0 { // High confidence bullish
		return "bullish"
	} else {
		return "neutral"
	}
}

// calculateBearishScore calculates a weighted score for bearish signals
func (c *CoinbaseClient) calculateBearishScore(indicators TechnicalIndicators) float64 {
	score := 0.0

	// MACD bearish crossover (weight: 2.0 - very reliable)
	if indicators.MACD < indicators.SignalLine {
		macdStrength := math.Abs(indicators.MACD - indicators.SignalLine)
		if indicators.MACD < -0.1 {
			score += 2.0 + (macdStrength * 10) // Bonus for strong bearish MACD
		} else {
			score += 1.5
		}
	}

	// EMA bearish crossover (weight: 2.0 - very reliable)
	if indicators.EMA12 < indicators.EMA26 {
		emaStrength := (indicators.EMA26 - indicators.EMA12) / indicators.EMA26 * 100
		score += 2.0 + (emaStrength * 0.1) // Bonus for stronger crossover
	}

	// RSI oversold conditions (weight: 1.5 - momentum indicator)
	if indicators.RSI < 40 {
		if indicators.RSI < 30 {
			score += 2.0 // Strong oversold
		} else {
			score += 1.5 // Moderate oversold
		}
	} else if indicators.RSI < 45 {
		score += 0.5 // Slight bearish momentum
	}

	// Price drop percentage (weight: 1.5 - direct price action)
	if indicators.PriceDropPct12h < 0 {
		dropStrength := math.Abs(indicators.PriceDropPct12h)
		if dropStrength > 5 {
			score += 2.0 // Strong drop
		} else if dropStrength > 3 {
			score += 1.5 // Moderate drop
		} else if dropStrength > 1 {
			score += 0.5 // Slight drop
		}
	}

	// Price vs EMA200 (weight: 1.0 - long-term trend)
	if indicators.CurrentPrice < indicators.EMA200 {
		ema200Strength := (indicators.EMA200 - indicators.CurrentPrice) / indicators.EMA200 * 100
		if indicators.RSI < 40 {
			score += 1.5 + (ema200Strength * 0.1) // Bonus for RSI confirmation
		} else {
			score += 1.0 + (ema200Strength * 0.05)
		}
	}

	// ADX trend strength (weight: 1.0 - trend confirmation)
	if indicators.ADX > 25 {
		if indicators.MACD < indicators.SignalLine {
			score += 1.5 // Strong trend with bearish momentum
		} else {
			score += 0.5 // Strong trend but no bearish momentum
		}
	}

	// Volume spike confirmation (weight: 0.5 - volume confirmation)
	if indicators.VolumeSpike && indicators.PriceDropPct12h < -2 {
		score += 0.5
	}

	return score
}

// calculateBullishScore calculates a weighted score for bullish signals
func (c *CoinbaseClient) calculateBullishScore(indicators TechnicalIndicators) float64 {
	score := 0.0

	// MACD bullish crossover (weight: 2.0 - very reliable)
	if indicators.MACD > indicators.SignalLine {
		macdStrength := math.Abs(indicators.MACD - indicators.SignalLine)
		if indicators.MACD > 0.1 {
			score += 2.0 + (macdStrength * 10) // Bonus for strong bullish MACD
		} else {
			score += 1.5
		}
	}

	// EMA bullish crossover (weight: 2.0 - very reliable)
	if indicators.EMA12 > indicators.EMA26 {
		emaStrength := (indicators.EMA12 - indicators.EMA26) / indicators.EMA26 * 100
		score += 2.0 + (emaStrength * 0.1) // Bonus for stronger crossover
	}

	// RSI overbought conditions (weight: 1.5 - momentum indicator)
	if indicators.RSI > 60 {
		if indicators.RSI > 70 {
			score += 2.0 // Strong overbought
		} else {
			score += 1.5 // Moderate overbought
		}
	} else if indicators.RSI > 55 {
		score += 0.5 // Slight bullish momentum
	}

	// Price increase percentage (weight: 1.5 - direct price action)
	if indicators.PriceDropPct12h > 0 {
		gainStrength := indicators.PriceDropPct12h
		if gainStrength > 5 {
			score += 2.0 // Strong gain
		} else if gainStrength > 3 {
			score += 1.5 // Moderate gain
		} else if gainStrength > 1 {
			score += 0.5 // Slight gain
		}
	}

	// Price vs EMA200 (weight: 1.0 - long-term trend)
	if indicators.CurrentPrice > indicators.EMA200 {
		ema200Strength := (indicators.CurrentPrice - indicators.EMA200) / indicators.EMA200 * 100
		if indicators.RSI > 60 {
			score += 1.5 + (ema200Strength * 0.1) // Bonus for RSI confirmation
		} else {
			score += 1.0 + (ema200Strength * 0.05)
		}
	}

	// ADX trend strength (weight: 1.0 - trend confirmation)
	if indicators.ADX > 25 {
		if indicators.MACD > indicators.SignalLine {
			score += 1.5 // Strong trend with bullish momentum
		} else {
			score += 0.5 // Strong trend but no bullish momentum
		}
	}

	// Volume spike confirmation (weight: 0.5 - volume confirmation)
	if indicators.VolumeSpike && indicators.PriceDropPct12h > 2 {
		score += 0.5
	}

	return score
}
