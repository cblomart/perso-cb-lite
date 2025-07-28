package middleware

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
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

// SecurityConfig holds security configuration
type SecurityConfig struct {
	AccessKey           string
	RateLimitPerMinute  int
	AllowedIPs          []string
	EnableRateLimiting  bool
	EnableIPWhitelist   bool
	EnableAccessKeyAuth bool
	logger              Logger
}

// RateLimiter holds rate limiting data per IP
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

// GetLimiter returns or creates a rate limiter for an IP
func (rl *RateLimiter) GetLimiter(ip string, requestsPerMinute int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), requestsPerMinute)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// LoadSecurityConfig loads security configuration from environment variables
func LoadSecurityConfig() *SecurityConfig {
	config := &SecurityConfig{}

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

	config.logger = &SimpleLogger{
		Logger: log.New(os.Stdout, "", log.LstdFlags),
		level:  logLevel,
	}

	// Load access key
	config.AccessKey = os.Getenv("API_ACCESS_KEY")
	if config.AccessKey == "" {
		// Auto-generate if not provided
		config.AccessKey = uuid.New().String()
		config.logger.Warn("🔐 Auto-generated API Access Key: %s", config.AccessKey)
		config.logger.Warn("⚠️  WARNING: This key will change on container restart! Add to .env: API_ACCESS_KEY=%s", config.AccessKey)
	}

	// Load rate limiting config
	rateLimitStr := os.Getenv("RATE_LIMIT_REQUESTS_PER_MINUTE")
	if rateLimitStr != "" {
		if rateLimit, err := strconv.Atoi(rateLimitStr); err == nil {
			config.RateLimitPerMinute = rateLimit
		} else {
			config.RateLimitPerMinute = 60 // default
		}
	} else {
		config.RateLimitPerMinute = 60 // default
	}

	// Load IP whitelist
	allowedIPsStr := os.Getenv("ALLOWED_IPS")
	if allowedIPsStr != "" {
		config.AllowedIPs = strings.Split(allowedIPsStr, ",")
		for i, ip := range config.AllowedIPs {
			config.AllowedIPs[i] = strings.TrimSpace(ip)
		}
	}

	// Load feature flags
	config.EnableRateLimiting = getEnvBool("ENABLE_RATE_LIMITING", true)
	config.EnableIPWhitelist = getEnvBool("ENABLE_IP_WHITELIST", false)
	config.EnableAccessKeyAuth = getEnvBool("ENABLE_ACCESS_KEY_AUTH", true)

	return config
}

// getEnvBool helper function to parse boolean environment variables
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return strings.ToLower(value) == "true" || value == "1"
}

// SecurityMiddleware creates a Gin middleware for security features
func SecurityMiddleware(config *SecurityConfig) gin.HandlerFunc {
	rateLimiter := NewRateLimiter()

	return func(c *gin.Context) {
		// Get client IP
		clientIP := c.ClientIP()

		// IP Whitelist check
		if config.EnableIPWhitelist && len(config.AllowedIPs) > 0 {
			if !isIPAllowed(clientIP, config.AllowedIPs) {
				config.logger.Warn("🚫 IP WHITELIST REJECTED: %s (User-Agent: %s, Path: %s)",
					clientIP, c.GetHeader("User-Agent"), c.Request.URL.Path)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Access denied",
				})
				c.Abort()
				return
			}
			// Log successful IP whitelist check for monitoring (debug only)
			config.logger.Debug("✅ IP WHITELIST ALLOWED: %s (Path: %s)", clientIP, c.Request.URL.Path)
		}

		// Rate limiting
		if config.EnableRateLimiting {
			limiter := rateLimiter.GetLimiter(clientIP, config.RateLimitPerMinute)
			if !limiter.Allow() {
				config.logger.Warn("⏱️ RATE LIMIT EXCEEDED: %s (User-Agent: %s, Path: %s)",
					clientIP, c.GetHeader("User-Agent"), c.Request.URL.Path)
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Too Many Requests",
					"message": "Rate limit exceeded",
				})
				c.Abort()
				return
			}
		}

		// Access key authentication (skip for health checks)
		if config.EnableAccessKeyAuth && !isHealthCheck(c.Request.URL.Path) {
			accessKey := c.GetHeader("X-API-Key")
			if accessKey == "" {
				accessKey = c.Query("api_key")
			}

			if accessKey != config.AccessKey {
				config.logger.Warn("🔑 INVALID ACCESS KEY: %s (User-Agent: %s, Path: %s)",
					clientIP, c.GetHeader("User-Agent"), c.Request.URL.Path)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Invalid or missing API access key",
				})
				c.Abort()
				return
			}
			// Log successful access key authentication for monitoring (debug only)
			config.logger.Debug("🔑 ACCESS KEY VALID: %s (Path: %s)", clientIP, c.Request.URL.Path)
		}

		c.Next()
	}
}

// isIPAllowed checks if an IP is in the allowed list (supports CIDR notation)
func isIPAllowed(clientIP string, allowedIPs []string) bool {
	// Parse the client IP
	clientIPAddr := net.ParseIP(clientIP)
	if clientIPAddr == nil {
		return false // Invalid IP address
	}

	for _, allowedIP := range allowedIPs {
		// Check for exact IP match
		if allowedIP == clientIP {
			return true
		}

		// Check for CIDR notation (e.g., 192.168.0.0/24)
		if strings.Contains(allowedIP, "/") {
			_, ipNet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				continue // Skip invalid CIDR
			}
			if ipNet.Contains(clientIPAddr) {
				return true
			}
		}
	}
	return false
}

// isHealthCheck checks if the request is for a health check endpoint
func isHealthCheck(path string) bool {
	return path == "/ping" || path == "/health"
}

// GetAccessKey returns the current access key (for display purposes)
func (config *SecurityConfig) GetAccessKey() string {
	return config.AccessKey
}
