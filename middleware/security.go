package middleware

import (
	"fmt"
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

// SecurityConfig holds security configuration
type SecurityConfig struct {
	AccessKey           string
	RateLimitPerMinute  int
	AllowedIPs          []string
	EnableRateLimiting  bool
	EnableIPWhitelist   bool
	EnableAccessKeyAuth bool
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

	// Load access key
	config.AccessKey = os.Getenv("API_ACCESS_KEY")
	if config.AccessKey == "" {
		// Auto-generate if not provided
		config.AccessKey = uuid.New().String()
		fmt.Printf("🔐 Auto-generated API Access Key: %s\n", config.AccessKey)
		fmt.Printf("🔐 Use this key in your API calls: X-API-Key: %s\n", config.AccessKey)
		fmt.Printf("⚠️  WARNING: This key will change on container restart! Add to .env: API_ACCESS_KEY=%s\n", config.AccessKey)
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
	return strings.ToLower(value) == "true"
}

// SecurityMiddleware creates a middleware with all security features
func SecurityMiddleware(config *SecurityConfig) gin.HandlerFunc {
	rateLimiter := NewRateLimiter()

	return func(c *gin.Context) {
		// Get client IP
		clientIP := c.ClientIP()

		// IP Whitelist check
		if config.EnableIPWhitelist && len(config.AllowedIPs) > 0 {
			if !isIPAllowed(clientIP, config.AllowedIPs) {
				log.Printf("🚫 IP WHITELIST REJECTED: %s (User-Agent: %s, Path: %s)",
					clientIP, c.GetHeader("User-Agent"), c.Request.URL.Path)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Access denied",
				})
				c.Abort()
				return
			}
			// Log successful IP whitelist check for monitoring
			log.Printf("✅ IP WHITELIST ALLOWED: %s (Path: %s)", clientIP, c.Request.URL.Path)
		}

		// Rate limiting
		if config.EnableRateLimiting {
			limiter := rateLimiter.GetLimiter(clientIP, config.RateLimitPerMinute)
			if !limiter.Allow() {
				log.Printf("⏱️ RATE LIMIT EXCEEDED: %s (User-Agent: %s, Path: %s)",
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
				log.Printf("🔑 INVALID ACCESS KEY: %s (User-Agent: %s, Path: %s)",
					clientIP, c.GetHeader("User-Agent"), c.Request.URL.Path)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Invalid or missing API access key",
				})
				c.Abort()
				return
			}
			// Log successful access key authentication for monitoring
			log.Printf("🔑 ACCESS KEY VALID: %s (Path: %s)", clientIP, c.Request.URL.Path)
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
