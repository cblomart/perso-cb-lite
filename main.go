package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"coinbase-base/client"
	"coinbase-base/config"
	"coinbase-base/middleware"
)

// Logger interface for consistent logging
type Logger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// SimpleLogger implements Logger interface
type SimpleLogger struct {
	*log.Logger
	level string
}

func (l *SimpleLogger) Info(format string, args ...interface{}) {
	if l.level == "INFO" || l.level == "DEBUG" || l.level == "WARN" || l.level == "ERROR" {
		l.Printf("[INFO] "+format, args...)
	}
}

func (l *SimpleLogger) Warn(format string, args ...interface{}) {
	if l.level == "WARN" || l.level == "DEBUG" || l.level == "ERROR" {
		l.Printf("[WARN] "+format, args...)
	}
}

func (l *SimpleLogger) Error(format string, args ...interface{}) {
	if l.level == "DEBUG" || l.level == "ERROR" {
		l.Printf("[ERROR] "+format, args...)
	}
}

func (l *SimpleLogger) Debug(format string, args ...interface{}) {
	if l.level == "DEBUG" {
		l.Printf("[DEBUG] "+format, args...)
	}
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		// No .env file found, using system environment variables
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

	logger := &SimpleLogger{
		Logger: log.New(os.Stdout, "", log.LstdFlags),
		level:  logLevel,
	}

	// Set Gin mode based on environment
	environment := os.Getenv("ENVIRONMENT")
	if environment == "production" {
		gin.SetMode(gin.ReleaseMode)
		logger.Info("Running in production mode with log level: %s", logLevel)
	} else {
		logger.Info("Running in development mode with log level: %s", logLevel)
	}

	// Load configurations
	tradingConfig := config.LoadTradingConfig()
	securityConfig := middleware.LoadSecurityConfig()

	// Log startup information
	logger.Info("📈 Trading pair: %s (%s/%s)", tradingConfig.GetTradingPair(), tradingConfig.GetBaseCurrency(), tradingConfig.GetQuoteCurrency())

	logger.Info("🔐 Security features:")
	logger.Info("   - Rate limiting: %v (%d req/min)", securityConfig.EnableRateLimiting, securityConfig.RateLimitPerMinute)
	logger.Info("   - IP whitelist: %v", securityConfig.EnableIPWhitelist)
	logger.Info("   - Access key auth: %v", securityConfig.EnableAccessKeyAuth)

	if securityConfig.AccessKey != "" {
		logger.Info("   - API Access Key: %s", securityConfig.GetAccessKey())
		logger.Info("   - Usage: X-API-Key header or ?api_key query param")
	} else {
		logger.Warn("   - API Access Key: [SET VIA ENV]")
		logger.Info("   - Usage: X-API-Key header or ?api_key query param")
	}

	// Create Coinbase client
	coinbaseClient, err := client.NewCoinbaseClient(
		tradingConfig.GetTradingPair(),
		tradingConfig.WebhookURL,
		tradingConfig.WebhookMaxRetries,
		tradingConfig.WebhookTimeout,
	)
	if err != nil {
		logger.Error("Failed to create Coinbase client: %v", err)
		os.Exit(1)
	}
	defer coinbaseClient.Close()

	// Initialize handlers
	handlers := NewHandlers(coinbaseClient)

	// Start background signal polling if webhook URL is configured
	if tradingConfig.WebhookURL != "" {
		logger.Info("🔔 Starting background signal polling (every 10 minutes)")
		logger.Debug("   - Webhook URL: %s", tradingConfig.WebhookURL)
		go startSignalPolling(coinbaseClient, tradingConfig.WebhookURL)
	} else {
		logger.Info("🔕 No webhook URL configured - signal polling disabled")
		logger.Debug("   - Set WEBHOOK_URL to enable automatic signal notifications")
	}

	// Create Gin router
	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityMiddleware(securityConfig))

	// Health check endpoint (no logging for frequent health checks)
	router.GET("/health", func(c *gin.Context) {
		// Test Coinbase communication and authentication
		accounts, err := coinbaseClient.GetAccountsWithLogging(false) // Suppress debug logs for health checks
		if err != nil {
			c.JSON(503, gin.H{
				"status":    "unhealthy",
				"error":     "Coinbase API communication failed",
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		// Check if we have both BTC and USDC accounts
		var hasBTC, hasUSDC bool
		for _, account := range accounts {
			if account.Currency == "BTC" && account.TradingEnabled {
				hasBTC = true
			}
			if account.Currency == "USDC" && account.TradingEnabled {
				hasUSDC = true
			}
		}

		if !hasBTC || !hasUSDC {
			c.JSON(503, gin.H{
				"status":    "unhealthy",
				"error":     "Missing required trading accounts",
				"message":   "Both BTC and USDC accounts must be available and enabled for trading",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		c.JSON(200, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"accounts": gin.H{
				"btc_available":  hasBTC,
				"usdc_available": hasUSDC,
			},
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		api.GET("/performance", handlers.GetPerformance)
		api.GET("/signal", handlers.GetSignal)
		api.GET("/signal/check", handlers.CheckSignal) // Manual signal check
		api.GET("/accounts", handlers.GetAccounts)
		api.GET("/orders", handlers.GetOrders)
		api.POST("/buy", handlers.BuyBTC)
		api.POST("/sell", handlers.SellBTC)
		api.DELETE("/orders", handlers.CancelAllOrders)
		api.GET("/candles", handlers.GetCandles)
		api.GET("/market", handlers.GetMarketState)
		api.GET("/graph", handlers.GetGraph)
	}

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create HTTP server
	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("🚀 Starting server on port %s", port)
		logger.Debug("📖 API Documentation:")
		logger.Debug("   - Health check: GET http://localhost:%s/health", port)
		logger.Debug("   - Performance: GET http://localhost:%s/api/v1/performance", port)
		logger.Debug("   - Signal: GET http://localhost:%s/api/v1/signal", port)
		logger.Debug("   - Accounts: GET http://localhost:%s/api/v1/accounts", port)
		logger.Debug("   - Orders: GET http://localhost:%s/api/v1/orders", port)
		logger.Debug("   - Buy: POST http://localhost:%s/api/v1/buy", port)
		logger.Debug("   - Sell: POST http://localhost:%s/api/v1/sell", port)
		logger.Debug("   - Cancel all: DELETE http://localhost:%s/api/v1/orders", port)
		logger.Debug("   - Candles: GET http://localhost:%s/api/v1/candles", port)
		logger.Debug("   - Market: GET http://localhost:%s/api/v1/market", port)
		logger.Debug("   - Graph: GET http://localhost:%s/api/v1/graph?period=week", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	// Close HTTP client connections
	if err := coinbaseClient.Close(); err != nil {
		logger.Error("Error closing HTTP client: %v", err)
	}

	logger.Info("Server stopped.")
}

// startSignalPolling runs background signal polling every 10 minutes
func startSignalPolling(client *client.CoinbaseClient, webhookURL string) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	log.Printf("[COINBASE-INFO] 🚀 Background signal polling started - checking every 10 minutes")

	// Send startup webhook to establish baseline position
	log.Printf("[COINBASE-INFO] 🔍 Sending startup webhook with current market position...")
	sendStartupWebhook(client, webhookURL)

	// Run initial check immediately
	log.Printf("[COINBASE-INFO] 🔍 Running initial signal check...")
	checkSignal(client)

	// Continue polling every 10 minutes
	for range ticker.C {
		checkSignal(client)
	}
}

// sendStartupWebhook sends a webhook at startup to establish current market position
func sendStartupWebhook(client *client.CoinbaseClient, webhookURL string) {
	// Track current asset value
	if err := client.TrackAssetValue(); err != nil {
		log.Printf("[COINBASE-INFO] ⚠️ Failed to track asset value for startup webhook: %v", err)
	}

	// Get current signal to establish baseline
	signal, err := client.GetSignalLightweight()
	if err != nil {
		log.Printf("[COINBASE-INFO] ❌ Failed to get signal for startup webhook: %v", err)
		return
	}

	// Create startup webhook request
	req, err := http.NewRequest("GET", webhookURL, nil)
	if err != nil {
		log.Printf("[COINBASE-INFO] ❌ Failed to create startup webhook request: %v", err)
		return
	}

	// Add query parameters for startup webhook
	q := req.URL.Query()
	q.Add("startup", "true")
	q.Add("baseline", "true")
	q.Add("current_trend", getCurrentTrendState(signal))
	q.Add("timestamp", fmt.Sprintf("%d", signal.Timestamp))

	// Add signal information if any triggers are present
	if len(signal.Triggers) > 0 {
		q.Add("triggers", strings.Join(signal.Triggers, ","))
		q.Add("bearish", "true")
	} else {
		q.Add("bearish", "false")
	}

	req.URL.RawQuery = q.Encode()

	// Set timeout for startup webhook
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	// Create HTTP client for startup webhook
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Send startup webhook
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[COINBASE-INFO] ❌ Startup webhook failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[COINBASE-INFO] ✅ Startup webhook sent successfully to %s", webhookURL)
	} else {
		log.Printf("[COINBASE-INFO] ⚠️ Startup webhook returned status %d", resp.StatusCode)
	}
}

// getCurrentTrendState determines the current trend from signal indicators
func getCurrentTrendState(signal *client.SignalResponse) string {
	if signal.BearishSignal {
		return "bearish"
	} else if len(signal.Triggers) > 0 {
		// If there are triggers but not bearish, it's likely bullish
		return "bullish"
	}
	return "neutral"
}

var (
	lastTrendState = "neutral" // Track the previous trend state
)

// checkSignal performs a signal check and sends webhook if needed
func checkSignal(client *client.CoinbaseClient) {
	// Only log in debug mode to reduce noise
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		log.Printf("[COINBASE-INFO] 🔍 Checking for trading signals (lightweight mode)...")
	}

	// Track asset value before checking signals
	if err := client.TrackAssetValue(); err != nil {
		log.Printf("[COINBASE-INFO] ⚠️ Failed to track asset value: %v", err)
	}

	signal, err := client.GetSignalLightweight() // Uses lightweight signal
	if err != nil {
		log.Printf("[COINBASE-INFO] ❌ Signal check failed: %v", err)
		return
	}

	// Determine current trend state
	currentTrend := "neutral"
	if signal.BearishSignal {
		currentTrend = "bearish"
	} else if len(signal.Triggers) > 0 {
		// Check if there are bullish signals (opposite of bearish)
		currentTrend = "bullish"
	}

	// Log signal check result focusing on trend changes
	if len(signal.Triggers) > 0 {
		log.Printf("[COINBASE-INFO] 🔄 Signal check: TREND CHANGE detected - %s → %s with triggers: %v", lastTrendState, currentTrend, signal.Triggers)
	} else {
		log.Printf("[COINBASE-INFO] ✅ Signal check: No trend change - current trend: %s", currentTrend)
	}

	// Update the last trend state for next comparison
	lastTrendState = currentTrend

	if len(signal.Triggers) > 0 { // Check if any triggers are present
		log.Printf("[COINBASE-INFO] 🔄 TREND CHANGE DETECTED: %v", signal.Triggers)
	} else {
		// Only log in debug mode to reduce noise
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			log.Printf("[COINBASE-INFO] ✅ No trend changes detected")
		}
	}
}
