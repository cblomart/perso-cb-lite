package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TradingConfig holds trading configuration
type TradingConfig struct {
	BaseCurrency      string
	QuoteCurrency     string
	TradingPair       string
	WebhookURL        string
	WebhookMaxRetries int
	WebhookTimeout    int
}

// LoadTradingConfig loads trading configuration from environment variables
func LoadTradingConfig() *TradingConfig {
	config := &TradingConfig{}

	// Load base currency
	config.BaseCurrency = os.Getenv("TRADING_BASE_CURRENCY")
	if config.BaseCurrency == "" {
		config.BaseCurrency = "BTC" // default
	}
	config.BaseCurrency = strings.ToUpper(config.BaseCurrency)

	// Load quote currency
	config.QuoteCurrency = os.Getenv("TRADING_QUOTE_CURRENCY")
	if config.QuoteCurrency == "" {
		config.QuoteCurrency = "USDC" // default
	}
	config.QuoteCurrency = strings.ToUpper(config.QuoteCurrency)

	// Load or generate trading pair
	config.TradingPair = os.Getenv("TRADING_PAIR")
	if config.TradingPair == "" {
		config.TradingPair = fmt.Sprintf("%s-%s", config.BaseCurrency, config.QuoteCurrency)
	}
	config.TradingPair = strings.ToUpper(config.TradingPair)

	// Load webhook URL for n8n notifications
	config.WebhookURL = os.Getenv("WEBHOOK_URL")

	// Load webhook retry configuration
	webhookMaxRetries := os.Getenv("WEBHOOK_MAX_RETRIES")
	if webhookMaxRetries == "" {
		config.WebhookMaxRetries = 3 // Default: 3 retries
	} else {
		if retries, err := strconv.Atoi(webhookMaxRetries); err == nil && retries >= 0 {
			config.WebhookMaxRetries = retries
		} else {
			config.WebhookMaxRetries = 3 // Default on invalid value
		}
	}

	// Load webhook timeout configuration
	webhookTimeout := os.Getenv("WEBHOOK_TIMEOUT_SECONDS")
	if webhookTimeout == "" {
		config.WebhookTimeout = 5 // Default: 5 seconds
	} else {
		if timeout, err := strconv.Atoi(webhookTimeout); err == nil && timeout > 0 {
			config.WebhookTimeout = timeout
		} else {
			config.WebhookTimeout = 5 // Default on invalid value
		}
	}

	return config
}

// GetTradingPair returns the configured trading pair
func (config *TradingConfig) GetTradingPair() string {
	return config.TradingPair
}

// GetBaseCurrency returns the base currency
func (config *TradingConfig) GetBaseCurrency() string {
	return config.BaseCurrency
}

// GetQuoteCurrency returns the quote currency
func (config *TradingConfig) GetQuoteCurrency() string {
	return config.QuoteCurrency
}

// Validate validates the trading configuration
func (config *TradingConfig) Validate() error {
	if config.BaseCurrency == "" {
		return fmt.Errorf("base currency cannot be empty")
	}
	if config.QuoteCurrency == "" {
		return fmt.Errorf("quote currency cannot be empty")
	}
	if config.TradingPair == "" {
		return fmt.Errorf("trading pair cannot be empty")
	}
	if config.BaseCurrency == config.QuoteCurrency {
		return fmt.Errorf("base and quote currencies cannot be the same")
	}
	return nil
}
