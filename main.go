package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coinbase-base/client"
	"coinbase-base/config"
	"coinbase-base/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Set Gin mode based on environment
	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	// Automatically correlate log level with environment
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		if environment == "production" {
			logLevel = "WARN" // Less verbose in production
		} else {
			logLevel = "INFO" // More verbose in development
		}
	}

	if environment == "production" {
		gin.SetMode(gin.ReleaseMode)
		log.Printf("Running in production mode with log level: %s", logLevel)
	} else {
		log.Printf("Running in development mode with log level: %s", logLevel)
	}

	// Load trading configuration
	tradingConfig := config.LoadTradingConfig()
	if err := tradingConfig.Validate(); err != nil {
		log.Fatalf("Invalid trading configuration: %v", err)
	}
	log.Printf("📈 Trading pair: %s (%s/%s)", tradingConfig.GetTradingPair(), tradingConfig.GetBaseCurrency(), tradingConfig.GetQuoteCurrency())

	// Load security configuration
	securityConfig := middleware.LoadSecurityConfig()
	log.Printf("🔐 Security features:")
	log.Printf("   - Rate limiting: %v (%d req/min)", securityConfig.EnableRateLimiting, securityConfig.RateLimitPerMinute)
	log.Printf("   - IP whitelist: %v", securityConfig.EnableIPWhitelist)
	log.Printf("   - Access key auth: %v", securityConfig.EnableAccessKeyAuth)
	if securityConfig.EnableAccessKeyAuth {
		// Only show the access key if it was auto-generated (not set in env)
		if os.Getenv("API_ACCESS_KEY") == "" {
			log.Printf("   - API Access Key: %s", securityConfig.GetAccessKey())
			log.Printf("   - Usage: X-API-Key header or ?api_key query param")
		} else {
			log.Printf("   - API Access Key: [SET VIA ENV]")
			log.Printf("   - Usage: X-API-Key header or ?api_key query param")
		}
	}

	// Initialize Coinbase client
	coinbaseClient, err := client.NewCoinbaseClient(tradingConfig.GetTradingPair(), tradingConfig.WebhookURL)
	if err != nil {
		log.Fatalf("Failed to initialize Coinbase client: %v", err)
	}

	// Initialize handlers
	handlers := NewHandlers(coinbaseClient)

	// Setup router
	router := gin.Default()

	// Configure trusted proxies for proper client IP detection
	// This is important when running behind a reverse proxy or load balancer
	if environment == "production" {
		// In production, trust common proxy IP ranges
		// This allows X-Forwarded-For headers from Traefik/Caddy/nginx to be read
		router.SetTrustedProxies([]string{
			"10.0.0.0/8",     // Private network
			"172.16.0.0/12",  // Docker network
			"192.168.0.0/16", // Private network
			"127.0.0.1",      // Localhost
			"::1",            // IPv6 localhost
		})
	} else {
		// In development, trust all proxies for easier testing
		router.SetTrustedProxies([]string{"0.0.0.0/0"})
	}

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Add security middleware
	router.Use(middleware.SecurityMiddleware(securityConfig))

	// API routes
	api := router.Group("/api/v1")
	{
		// Account operations
		api.GET("/accounts", handlers.GetAccounts)

		// Market data
		api.GET("/candles", handlers.GetCandles)
		api.GET("/market", handlers.GetMarketState)

		// Trading operations (with integrated stop limit)
		api.POST("/buy", handlers.BuyBTC)
		api.POST("/sell", handlers.SellBTC)

		// Order management
		api.GET("/orders", handlers.GetOrders)
		api.DELETE("/orders/:order_id", handlers.CancelOrder)
		api.DELETE("/orders", handlers.CancelAllOrders)

		// Performance monitoring
		api.GET("/performance", handlers.GetPerformance)

		// Trading signals
		api.GET("/signal", handlers.GetSignal)
	}

	// Simple ping for basic server status
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Debug endpoint to check client IP detection (useful for Docker/proxy debugging)
	router.GET("/debug/ip", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"client_ip":         c.ClientIP(),
			"remote_addr":       c.Request.RemoteAddr,
			"x_forwarded_for":   c.GetHeader("X-Forwarded-For"),
			"x_real_ip":         c.GetHeader("X-Real-IP"),
			"x_forwarded_proto": c.GetHeader("X-Forwarded-Proto"),
			"user_agent":        c.GetHeader("User-Agent"),
			"timestamp":         time.Now().Format(time.RFC3339),
		})
	})

	// Health check with Coinbase validation
	router.GET("/health", func(c *gin.Context) {
		// Test Coinbase communication and authentication
		accounts, err := coinbaseClient.GetAccounts()
		if err != nil {
			c.JSON(503, gin.H{
				"status":    "unhealthy",
				"error":     "Coinbase API communication failed",
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		// Check if we have the expected accounts for the trading pair
		baseCurrency := tradingConfig.GetBaseCurrency()
		quoteCurrency := tradingConfig.GetQuoteCurrency()
		hasBase := false
		hasQuote := false
		for _, account := range accounts {
			if account.Currency == baseCurrency {
				hasBase = true
			}
			if account.Currency == quoteCurrency {
				hasQuote = true
			}
		}

		response := gin.H{
			"status": "healthy",
			"trading": gin.H{
				"pair":           tradingConfig.GetTradingPair(),
				"base_currency":  baseCurrency,
				"quote_currency": quoteCurrency,
			},
			"coinbase": gin.H{
				"authentication": "valid",
				"communication":  "successful",
				"accounts_found": len(accounts),
				"base_account":   hasBase,
				"quote_account":  hasQuote,
			},
			"security": gin.H{
				"rate_limiting":   securityConfig.EnableRateLimiting,
				"ip_whitelist":    securityConfig.EnableIPWhitelist,
				"access_key_auth": securityConfig.EnableAccessKeyAuth,
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}

		// If missing expected accounts, mark as degraded
		if !hasBase || !hasQuote {
			response["status"] = "degraded"
			response["warning"] = fmt.Sprintf("Missing expected %s or %s accounts", baseCurrency, quoteCurrency)
		}

		c.JSON(200, response)
	})

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Starting server on port %s", port)
	log.Printf("📖 API Documentation:")
	log.Printf("   - Health check: GET http://localhost:%s/health", port)
	log.Printf("   - Performance: GET http://localhost:%s/api/v1/performance", port)
	log.Printf("   - Signal: GET http://localhost:%s/api/v1/signal", port)
	log.Printf("   - Accounts: GET http://localhost:%s/api/v1/accounts", port)
	log.Printf("   - Orders: GET http://localhost:%s/api/v1/orders", port)
	log.Printf("   - Buy: POST http://localhost:%s/api/v1/buy", port)
	log.Printf("   - Sell: POST http://localhost:%s/api/v1/sell", port)
	log.Printf("   - Cancel all: DELETE http://localhost:%s/api/v1/orders", port)

	// Create HTTP server for graceful shutdown
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Start the server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for an interrupt signal (e.g., Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Create a deadline for server shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close HTTP client connections
	if err := coinbaseClient.Close(); err != nil {
		log.Printf("Error closing HTTP client: %v", err)
	}

	log.Println("Server stopped.")
}
