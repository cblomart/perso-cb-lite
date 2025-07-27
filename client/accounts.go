package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CoinbaseAccount represents the raw account response from Coinbase Advanced Trade API
type CoinbaseAccount struct {
	UUID             string `json:"uuid"`
	Currency         string `json:"currency"`
	AvailableBalance struct {
		Value string `json:"value"`
	} `json:"available_balance"`
	Hold struct {
		Value string `json:"value"`
	} `json:"hold"`
	RetailPortfolioID string `json:"retail_portfolio_id"`
	Ready             bool   `json:"ready"`
}

// AccountsResponse represents the response from the accounts endpoint
type AccountsResponse struct {
	Accounts []CoinbaseAccount `json:"accounts"`
}

// GetAccounts retrieves accounts for the configured trading pair
func (c *CoinbaseClient) GetAccounts() ([]Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Extract base and quote currencies from trading pair
	parts := strings.Split(c.tradingPair, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid trading pair format: %s", c.tradingPair)
	}
	baseCurrency := parts[0]
	quoteCurrency := parts[1]

	c.logger.Printf("Fetching accounts for %s and %s...", baseCurrency, quoteCurrency)

	respBody, err := c.makeRequest(ctx, "GET", "/accounts", nil)
	if err != nil {
		c.logger.Printf("Error fetching accounts: %v", err)
		return nil, fmt.Errorf("failed to fetch accounts: %w", err)
	}

	var resp AccountsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal accounts response: %w", err)
	}

	// Filter and convert only accounts for the configured trading pair
	var accounts []Account
	for _, account := range resp.Accounts {
		currency := strings.ToUpper(account.Currency)
		if currency == baseCurrency || currency == quoteCurrency {
			accounts = append(accounts, Account{
				Currency:         currency,
				AvailableBalance: account.AvailableBalance.Value,
				Hold:             account.Hold.Value,
				TradingEnabled:   account.Ready,
			})
		}
	}

	c.logger.Printf("Successfully fetched %d trading accounts (%s/%s)", len(accounts), baseCurrency, quoteCurrency)
	return accounts, nil
}

// GetPositions retrieves current positions (for now, return empty as regular API might not have positions)
func (c *CoinbaseClient) GetPositions() ([]Account, error) {
	c.logger.Printf("Positions endpoint not available in regular Coinbase API, returning empty list")
	return []Account{}, nil
}
