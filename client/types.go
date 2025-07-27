package client

import "time"

// Account represents a Coinbase account with simplified structure for BTC/USDC trading
type Account struct {
	Currency         string `json:"currency"`
	AvailableBalance string `json:"available_balance"`
	Hold             string `json:"hold"`
	TradingEnabled   bool   `json:"trading_enabled"`
}

// Order represents a Coinbase order with simplified structure
type Order struct {
	ID           string    `json:"id"`
	ProductID    string    `json:"product_id"`
	Side         string    `json:"side"`
	Type         string    `json:"type"`
	Size         string    `json:"size,omitempty"`
	Price        string    `json:"price,omitempty"`
	StopPrice    string    `json:"stop_price,omitempty"`
	LimitPrice   string    `json:"limit_price,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	FilledSize   string    `json:"filled_size"`
	FilledValue  string    `json:"filled_value"`
	AveragePrice string    `json:"average_price"`
}

// TradingRequest represents a unified trading request with optional stop limit
type TradingRequest struct {
	Size       string `json:"size"`
	Price      string `json:"price"`
	StopPrice  string `json:"stop_price,omitempty"`
	LimitPrice string `json:"limit_price,omitempty"`
}

// CreateOrderRequest represents the request body for creating orders
type CreateOrderRequest struct {
	ProductID  string `json:"product_id"`
	Side       string `json:"side"`
	Type       string `json:"type"`
	Size       string `json:"size,omitempty"`
	Price      string `json:"price,omitempty"`
	StopPrice  string `json:"stop_price,omitempty"`
	LimitPrice string `json:"limit_price,omitempty"`
}
